package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/platform"
)

// ClientInfo describes this app and device to servers that pair devices.
type ClientInfo struct {
	DeviceID string
	Name     string // shown in the server's device list
	Platform string
	Version  string
}

// Pairer is implemented by sources that need an interactive device pairing
// before they can be used.
type Pairer interface {
	NeedsPairing() bool
	StartPairing(ctx context.Context) (*Pairing, error)
	// PollPairing returns ErrPairingPending until the user approves.
	PollPairing(ctx context.Context, p *Pairing) error
}

// Pairing is an in-progress device authorization.
type Pairing struct {
	UserCode   string
	URL        string // page where the user approves; encode as QR
	Interval   time.Duration
	Expires    time.Time
	deviceCode string
}

var (
	ErrPairingPending = errors.New("waiting for approval")
	ErrPairingDenied  = errors.New("pairing denied")
	ErrPairingExpired = errors.New("pairing code expired")
	ErrNeedsPairing   = errors.New("device is not paired")
)

const (
	rommClientName = "jubilant-potato"
	rommPageSize   = 1000
)

var rommScopes = []string{"platforms.read", "roms.read"}

// romm talks to a RomM server (https://romm.app) through its REST API.
type romm struct {
	cfg       config.Source
	base      *url.URL
	client    *http.Client
	info      ClientInfo
	saveToken func(string) error

	mu    sync.Mutex
	token string
}

func newRomm(c config.Source, client *http.Client, info ClientInfo, saveToken func(string) error) (*romm, error) {
	u, err := url.Parse(strings.TrimRight(c.URL, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("source %q: invalid url %q", c.Name, c.URL)
	}
	return &romm{cfg: c, base: u, client: client, info: info, saveToken: saveToken, token: c.Token}, nil
}

func (s *romm) Name() string { return s.cfg.Name }

func (s *romm) NeedsPairing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token == ""
}

func (s *romm) auth() http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := http.Header{}
	if s.token != "" {
		h.Set("Authorization", "Bearer "+s.token)
	}
	return h
}

func (s *romm) setToken(tok string) error {
	s.mu.Lock()
	s.token = tok
	s.mu.Unlock()
	if s.saveToken != nil {
		return s.saveToken(tok)
	}
	return nil
}

func (s *romm) api(path string, q url.Values) string {
	u := *s.base
	u.Path += "/api" + path
	u.RawQuery = q.Encode()
	return u.String()
}

// call performs a JSON request against the API and decodes the response.
func (s *romm) call(ctx context.Context, method, path string, q url.Values, body, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.api(path, q), rd)
	if err != nil {
		return err
	}
	for k, v := range s.auth() {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if s.auth().Get("Authorization") != "" {
			// Token revoked or expired: forget it so the UI pairs again.
			s.setToken("")
		}
		return ErrNeedsPairing
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Detail any `json:"detail"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		if d, ok := e.Detail.(string); ok {
			return &rommError{status: resp.StatusCode, detail: d}
		}
		return &rommError{status: resp.StatusCode, detail: resp.Status}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type rommError struct {
	status int
	detail string
}

func (e *rommError) Error() string { return fmt.Sprintf("RomM: %s (HTTP %d)", e.detail, e.status) }

func (s *romm) StartPairing(ctx context.Context) (*Pairing, error) {
	req := map[string]any{
		"client_device_identifier": s.info.DeviceID,
		"name":                     s.info.Name,
		"client":                   rommClientName,
		"platform":                 s.info.Platform,
		"client_version":           s.info.Version,
		"requested_scopes":         rommScopes,
	}
	var resp struct {
		DeviceCode               string `json:"device_code"`
		UserCode                 string `json:"user_code"`
		VerificationPathComplete string `json:"verification_path_complete"`
		ExpiresIn                int    `json:"expires_in"`
		Interval                 int    `json:"interval"`
	}
	if err := s.call(ctx, http.MethodPost, "/auth/device/init", nil, req, &resp); err != nil {
		return nil, err
	}
	// The server returns a path; the approval page lives on the same origin.
	page := *s.base
	ref, err := url.Parse(resp.VerificationPathComplete)
	if err != nil {
		return nil, err
	}
	return &Pairing{
		UserCode:   resp.UserCode,
		URL:        page.ResolveReference(ref).String(),
		Interval:   time.Duration(max(resp.Interval, 1)) * time.Second,
		Expires:    time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		deviceCode: resp.DeviceCode,
	}, nil
}

func (s *romm) PollPairing(ctx context.Context, p *Pairing) error {
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	err := s.call(ctx, http.MethodPost, "/auth/device/token", nil, map[string]string{"device_code": p.deviceCode}, &resp)
	var re *rommError
	if errors.As(err, &re) {
		switch re.detail {
		case "authorization_pending":
			return ErrPairingPending
		case "slow_down":
			p.Interval += 5 * time.Second
			return ErrPairingPending
		case "access_denied":
			return ErrPairingDenied
		case "expired_token":
			return ErrPairingExpired
		}
	}
	if err != nil {
		return err
	}
	if resp.AccessToken == "" {
		return errors.New("RomM: empty token in response")
	}
	return s.setToken(resp.AccessToken)
}

type rommPlatform struct {
	ID         int    `json:"id"`
	Slug       string `json:"slug"`
	FSSlug     string `json:"fs_slug"`
	Name       string `json:"name"`
	CustomName string `json:"custom_name"`
	RomCount   int    `json:"rom_count"`
}

func (s *romm) Systems(ctx context.Context) ([]System, error) {
	if s.NeedsPairing() {
		return nil, ErrNeedsPairing
	}
	var platforms []rommPlatform
	if err := s.call(ctx, http.MethodGet, "/platforms", nil, nil, &platforms); err != nil {
		return nil, err
	}
	var out []System
	for _, p := range platforms {
		if p.RomCount == 0 {
			continue
		}
		sys, ok := platform.MatchSystem(p.FSSlug)
		if !ok {
			if sys, ok = platform.MatchSystem(p.Slug); !ok {
				continue // no known ROM folder for this platform on the device
			}
		}
		label := p.CustomName
		if label == "" {
			label = p.Name
		}
		out = append(out, System{ID: sys.ID, Key: strconv.Itoa(p.ID), Label: label})
	}
	sortSystems(out)
	return out, nil
}

type rommRom struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	FSName           string `json:"fs_name"`
	FSNameNoExt      string `json:"fs_name_no_ext"`
	FSSizeBytes      int64  `json:"fs_size_bytes"`
	HasMultipleFiles bool   `json:"has_multiple_files"`
	MissingFromFS    bool   `json:"missing_from_fs"`
	SiblingRoms      []struct {
		ID int `json:"id"`
	} `json:"sibling_roms"`
}

func (s *romm) Games(ctx context.Context, sys System) ([]Game, error) {
	if s.NeedsPairing() {
		return nil, ErrNeedsPairing
	}
	var out []Game
	for offset := 0; ; offset += rommPageSize {
		q := url.Values{
			"platform_ids":       {sys.Key},
			"limit":              {strconv.Itoa(rommPageSize)},
			"offset":             {strconv.Itoa(offset)},
			"order_by":           {"name"},
			"order_dir":          {"asc"},
			"with_char_index":    {"false"},
			"with_filter_values": {"false"},
			"with_rom_id_index":  {"false"},
		}
		var page struct {
			Items []rommRom `json:"items"`
			Total int       `json:"total"`
		}
		if err := s.call(ctx, http.MethodGet, "/roms", q, nil, &page); err != nil {
			return nil, err
		}
		for _, r := range page.Items {
			if r.MissingFromFS {
				continue
			}
			out = append(out, s.game(r, sys.ID))
		}
		if len(page.Items) < rommPageSize || offset+len(page.Items) >= page.Total {
			break
		}
	}
	sortGames(out)
	return out, nil
}

func (s *romm) game(r rommRom, systemID string) Game {
	// RomM links regional versions of a game as siblings, even when their
	// titles differ; the lowest ID names the group for all of them.
	group := r.ID
	for _, s := range r.SiblingRoms {
		group = min(group, s.ID)
	}
	file := r.FSName
	if r.HasMultipleFiles {
		// Multi-file games (folders, multi-disc) are served as one zip.
		file += ".zip"
	}
	u := *s.base
	u.Path += fmt.Sprintf("/api/roms/%d/content/%s", r.ID, file)
	return Game{
		Name:    r.FSNameNoExt,
		Title:   r.Name, // metadata title, empty when unmatched
		Group:   fmt.Sprintf("romm:%d", group),
		File:    file,
		Size:    r.FSSizeBytes,
		URL:     u.String(),
		Header:  s.auth(),
		System:  systemID,
		Extract: r.HasMultipleFiles,
	}
}
