package source

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/platform"
)

// itch.io source: free homebrew listed from public tag feeds, files
// fetched through the v2 API with a token from QR (device) login. See
// https://itch.io/docs/api/oauth and itch.io's guidance for handheld
// clients in github.com/carroarmato0/NextUI-Itchio-Pak/issues/4.

// Tag feeds per system, most specific first.
var itchFeeds = []struct {
	system string
	tags   []string
}{
	{"gb", []string{"tag-gameboy-rom", "tag-gbstudio", "tag-game-boy"}},
	{"gbc", []string{"tag-gameboy-color", "tag-gbc"}},
	{"gba", []string{"tag-gameboy-advance"}},
	{"nes", []string{"tag-nes-rom", "tag-nes"}},
	{"snes", []string{"tag-snes-rom", "tag-snes"}},
	{"md", []string{"tag-sega-genesis", "tag-mega-drive"}},
	{"sms", []string{"tag-sega-master-system", "tag-master-system"}},
	{"gg", []string{"tag-game-gear"}},
	{"pce", []string{"tag-pc-engine"}},
	{"atari2600", []string{"tag-atari-2600"}},
}

const (
	itchFeedPages = 3 // 36 games per page, most popular first
	itchScopes    = "profile:me profile:owned game:view:uploads"
)

// Endpoints; tests point them at a fake server.
var (
	itchAPI = "https://api.itch.io"
	itchWeb = "https://itch.io"
)

var (
	// ErrItchNoClient means QR login isn't set up (no OAuth client ID).
	ErrItchNoClient = errors.New("itch.io QR login is not configured; put an API key in config.json")
	// ErrItchNotApproved means itch.io hasn't enabled QR login for the
	// client yet (it answers 404 until support approves it).
	ErrItchNotApproved = errors.New("itch.io hasn't approved QR login for this app yet; use an API key in config.json meanwhile")
	// ErrPaid means the game costs money and the user doesn't own it.
	ErrPaid = errors.New("paid game: buy it on itch.io first")
	// ErrNoFiles means the game has no downloadable ROM for this system.
	ErrNoFiles = errors.New("no ROM files for this system")
)

// Resolver is implemented by sources whose catalog lists game pages that
// turn into downloadable files only when opened (Game.Page is set).
type Resolver interface {
	Resolve(ctx context.Context, g Game) ([]Game, error)
}

// PairingOptional is implemented by sources that can be browsed before
// pairing; pairing is needed only to download.
type PairingOptional interface {
	PairingOptional() bool
}

type itchSource struct {
	cfg       config.Source
	client    *http.Client
	info      ClientInfo
	saveToken func(string) error

	mu    sync.Mutex
	token string

	limiter itchLimiter
}

func newItch(c config.Source, client *http.Client, info ClientInfo, saveToken func(string) error) *itchSource {
	return &itchSource{cfg: c, client: client, info: info, saveToken: saveToken, token: c.Token}
}

func (s *itchSource) Name() string          { return s.cfg.Name }
func (s *itchSource) Kind() string          { return "itch.io" }
func (s *itchSource) PairingOptional() bool { return true }

func (s *itchSource) NeedsPairing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token == ""
}

func (s *itchSource) setToken(tok string) error {
	s.mu.Lock()
	s.token = tok
	s.mu.Unlock()
	if s.saveToken != nil {
		return s.saveToken(tok)
	}
	return nil
}

func (s *itchSource) Systems(context.Context) ([]System, error) {
	var out []System
	for _, f := range itchFeeds {
		if sys, ok := platform.SystemByID(f.system); ok {
			out = append(out, System{ID: sys.ID, Key: sys.ID, Label: sys.Name})
		}
	}
	return out, nil
}

type itchRSS struct {
	Items []struct {
		Title string `xml:"plainTitle"`
		Link  string `xml:"link"`
		Price string `xml:"price"`
	} `xml:"channel>item"`
}

// Games lists free games from the system's tag feeds. Entries are game
// pages; Resolve turns one into files.
func (s *itchSource) Games(ctx context.Context, sys System) ([]Game, error) {
	var tags []string
	for _, f := range itchFeeds {
		if f.system == sys.ID {
			tags = f.tags
		}
	}
	seen := map[string]bool{}
	var out []Game
	for _, tag := range tags {
		for page := 1; page <= itchFeedPages; page++ {
			var feed itchRSS
			u := fmt.Sprintf("%s/games/free/%s.xml?page=%d", itchWeb, tag, page)
			if err := s.fetch(ctx, http.MethodGet, u, nil, false, func(r io.Reader) error {
				return xml.NewDecoder(r).Decode(&feed)
			}); err != nil {
				if len(out) > 0 {
					break // keep what we have
				}
				return nil, err
			}
			for _, it := range feed.Items {
				if it.Link == "" || seen[it.Link] {
					continue
				}
				seen[it.Link] = true
				out = append(out, Game{
					Name: it.Title, Title: it.Title, Group: "itch:" + it.Link,
					URL: it.Link, Page: it.Link, System: sys.ID,
				})
			}
			if len(feed.Items) < 30 {
				break // last page
			}
		}
	}
	sortGames(out)
	return out, nil
}

// itchList decodes a JSON array; itch.io sends empty arrays as {}.
type itchList[T any] []T

func (l *itchList[T]) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '{' {
		*l = nil
		return nil
	}
	return json.Unmarshal(b, (*[]T)(l))
}

type itchUpload struct {
	ID          int64  `json:"id"`
	Filename    string `json:"filename"`
	DisplayName string `json:"display_name"`
	Size        int64  `json:"size"`
	Type        string `json:"type"`
	Storage     string `json:"storage"`
}

// Resolve lists the ROM files of a game page for its system.
func (s *itchSource) Resolve(ctx context.Context, g Game) ([]Game, error) {
	if s.NeedsPairing() {
		return nil, ErrNeedsPairing
	}
	var data struct {
		ID    int64  `json:"id"`
		Price string `json:"price"`
	}
	if err := s.fetch(ctx, http.MethodGet, strings.TrimRight(g.Page, "/")+"/data.json", nil, false, func(r io.Reader) error {
		return json.NewDecoder(r).Decode(&data)
	}); err != nil {
		return nil, err
	}
	if data.ID == 0 {
		return nil, fmt.Errorf("itch.io: no game id for %s", g.Page)
	}

	// Paid games need one of the user's download keys.
	var keyID int64
	if data.Price != "" && data.Price != "$0.00" {
		var keys struct {
			OwnedKeys itchList[struct {
				ID int64 `json:"id"`
			}] `json:"owned_keys"`
		}
		q := "/profile/owned-keys?game_ids=" + strconv.FormatInt(data.ID, 10)
		if err := s.api(ctx, http.MethodGet, q, nil, &keys); err != nil {
			return nil, err
		}
		if len(keys.OwnedKeys) == 0 {
			return nil, ErrPaid
		}
		keyID = keys.OwnedKeys[0].ID
	}

	path := fmt.Sprintf("/games/%d/uploads", data.ID)
	if keyID != 0 {
		path += "?download_key_id=" + strconv.FormatInt(keyID, 10)
	}
	var resp struct {
		Uploads itchList[itchUpload] `json:"uploads"`
	}
	if err := s.api(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}

	psys, _ := platform.SystemByID(g.System)
	var out []Game
	for _, up := range resp.Uploads {
		if up.Storage == "external" || (up.Type != "" && up.Type != "default") || !psys.Accepts(up.Filename) {
			continue
		}
		name := up.DisplayName
		if name == "" {
			name = displayName(up.Filename)
		}
		gameID, uploadID := data.ID, up.ID
		out = append(out, Game{
			Name: name, Title: g.Title, File: up.Filename, Size: up.Size,
			URL: fmt.Sprintf("https://itch.io/upload/%d", up.ID), System: g.System, Group: g.Group,
			Open: func(ctx context.Context, offset int64) (io.ReadCloser, int64, error) {
				return s.download(ctx, gameID, uploadID, keyID, offset)
			},
		})
	}
	if len(out) == 0 {
		return nil, ErrNoFiles
	}
	return out, nil
}

// download starts a download session and streams the upload from offset.
func (s *itchSource) download(ctx context.Context, gameID, uploadID, keyID int64, offset int64) (io.ReadCloser, int64, error) {
	key := ""
	if keyID != 0 {
		key = "download_key_id=" + strconv.FormatInt(keyID, 10)
	}
	var sess struct {
		UUID string `json:"uuid"`
	}
	path := fmt.Sprintf("/games/%d/download-sessions", gameID)
	if key != "" {
		path += "?" + key
	}
	if err := s.api(ctx, http.MethodPost, path, nil, &sess); err != nil {
		return nil, 0, err
	}
	q := url.Values{"uuid": {sess.UUID}}
	if keyID != 0 {
		q.Set("download_key_id", strconv.FormatInt(keyID, 10))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/uploads/%d/download?%s", itchAPI, uploadID, q.Encode()), nil)
	if err != nil {
		return nil, 0, err
	}
	s.authorize(req)
	if offset > 0 {
		// Go drops Authorization on the redirect to the CDN but keeps Range.
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		return resp.Body, offset + resp.ContentLength, nil
	case http.StatusOK:
		// Range ignored: skip what we already have.
		if _, err := io.CopyN(io.Discard, resp.Body, offset); err != nil {
			resp.Body.Close()
			return nil, 0, err
		}
		return resp.Body, resp.ContentLength, nil
	}
	resp.Body.Close()
	return nil, 0, fmt.Errorf("itch.io download: %s", resp.Status)
}

func (s *itchSource) authorize(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent)
	s.mu.Lock()
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	s.mu.Unlock()
}

// api calls an api.itch.io endpoint and decodes JSON into out.
func (s *itchSource) api(ctx context.Context, method, path string, form url.Values, out any) error {
	return s.fetch(ctx, method, itchAPI+path, form, true, func(r io.Reader) error {
		return json.NewDecoder(r).Decode(out)
	})
}

// fetch performs a rate-limited request, retrying when itch.io asks to
// slow down, and hands the body to decode.
func (s *itchSource) fetch(ctx context.Context, method, u string, form url.Values, auth bool, decode func(io.Reader) error) error {
	for attempt := 0; ; attempt++ {
		if err := s.limiter.wait(ctx); err != nil {
			return err
		}
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, u, body)
		if err != nil {
			return err
		}
		if auth {
			s.authorize(req)
		} else {
			req.Header.Set("User-Agent", UserAgent)
		}
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return err
		}
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) && attempt < 4 {
			resp.Body.Close()
			s.limiter.backoff(time.Second << attempt)
			continue
		}
		defer resp.Body.Close()
		if auth && resp.StatusCode == http.StatusUnauthorized {
			s.setToken("")
			return ErrNeedsPairing
		}
		if resp.StatusCode >= 300 {
			var e struct {
				Errors []string `json:"errors"`
			}
			json.NewDecoder(resp.Body).Decode(&e)
			msg := resp.Status
			if len(e.Errors) > 0 {
				msg = strings.Join(e.Errors, "; ")
			}
			return &itchError{status: resp.StatusCode, msg: msg}
		}
		return decode(resp.Body)
	}
}

type itchError struct {
	status int
	msg    string
}

func (e *itchError) Error() string { return "itch.io: " + e.msg }

// itchLimiter spaces requests to itch.io and pauses them all after a 429.
type itchLimiter struct {
	mu   sync.Mutex
	next time.Time
}

const itchInterval = 250 * time.Millisecond

func (l *itchLimiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	at := l.next
	if at.Before(now) {
		at = now
	}
	l.next = at.Add(itchInterval)
	l.mu.Unlock()
	select {
	case <-time.After(time.Until(at)):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *itchLimiter) backoff(d time.Duration) {
	l.mu.Lock()
	if t := time.Now().Add(d); t.After(l.next) {
		l.next = t
	}
	l.mu.Unlock()
}

// ---- QR login (OAuth device authorization grant with PKCE) -----------------

func (s *itchSource) StartPairing(ctx context.Context) (*Pairing, error) {
	if s.info.ItchClientID == "" {
		return nil, ErrItchNoClient
	}
	verifier := make([]byte, 32)
	rand.Read(verifier)
	v := base64.RawURLEncoding.EncodeToString(verifier)
	sum := sha256.Sum256([]byte(v))

	var resp struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	form := url.Values{
		"client_id":             {s.info.ItchClientID},
		"scope":                 {itchScopes},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}
	if err := s.api(ctx, http.MethodPost, "/oauth/device", form, &resp); err != nil {
		var ie *itchError
		if errors.As(err, &ie) && ie.status == http.StatusNotFound {
			return nil, ErrItchNotApproved
		}
		return nil, err
	}
	return &Pairing{
		UserCode:   resp.UserCode,
		URL:        resp.VerificationURIComplete,
		Interval:   time.Duration(max(resp.Interval, 1)) * time.Second,
		Expires:    time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		deviceCode: resp.DeviceCode,
		verifier:   v,
	}, nil
}

func (s *itchSource) PollPairing(ctx context.Context, p *Pairing) error {
	var poll struct {
		Status string `json:"status"`
		Code   string `json:"code"`
	}
	form := url.Values{"client_id": {s.info.ItchClientID}, "device_code": {p.deviceCode}}
	if err := s.api(ctx, http.MethodPost, "/oauth/device/poll", form, &poll); err != nil {
		return err
	}
	switch poll.Status {
	case "pending":
		return ErrPairingPending
	case "denied":
		return ErrPairingDenied
	case "expired":
		return ErrPairingExpired
	case "approved":
	default:
		return fmt.Errorf("itch.io: unexpected login status %q", poll.Status)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	form = url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {poll.Code},
		"code_verifier": {p.verifier},
		"redirect_uri":  {"urn:itchio:poll"},
		"client_id":     {s.info.ItchClientID},
		"device_info":   {s.info.Name + ", Jubilant Potato " + s.info.Version},
	}
	if err := s.api(ctx, http.MethodPost, "/oauth/token", form, &tok); err != nil {
		return err
	}
	if tok.AccessToken == "" {
		return errors.New("itch.io: empty token in response")
	}
	return s.setToken(tok.AccessToken)
}
