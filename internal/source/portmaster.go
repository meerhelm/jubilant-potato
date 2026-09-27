package source

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/platform"
)

// PortMaster ports from the catalog PortMaster itself installs from
// (github.com/PortsMaster/PortMaster-New). Ports are unpacked the way its
// harbourmaster does it, and the runtimes they need (Godot, Mono, ...) are
// downloaded into PortMaster's libs folder first.

// Catalog, featured lists and image locations; tests point them at a fake
// server.
var (
	portMasterIndex    = "https://github.com/PortsMaster/PortMaster-New/releases/latest/download/ports.json"
	portMasterFeatured = "https://github.com/PortsMaster/PortMaster-Info/raw/main/featured_ports.json"
	portMasterImages   = "https://raw.githubusercontent.com/PortsMaster/PortMaster-New/main/ports/"
)

// ErrNoPortMaster means PortMaster isn't installed, so ports can't run.
var ErrNoPortMaster = errors.New("PortMaster is not installed")

type portMaster struct {
	cfg    config.Source
	client *http.Client
	dirs   platform.PortMaster
	arch   string // PortMaster's name for the CPU: aarch64, armhf, x86_64
	fw     string // firmware as reqs name it: rocknix, muos
	cache  string // folder for the catalog's copy on disk; "" keeps none

	mu    sync.Mutex
	index *pmIndex
	stale bool // index is checked for updates on the next load
}

func newPortMaster(c config.Source, client *http.Client, dirs platform.PortMaster, fw, cacheDir string) *portMaster {
	arch := map[string]string{"arm64": "aarch64", "arm": "armhf", "amd64": "x86_64"}[runtime.GOARCH]
	s := &portMaster{cfg: c, client: client, dirs: dirs, arch: arch, fw: fw}
	if cacheDir != "" {
		s.cache = filepath.Join(cacheDir, "portmaster")
	}
	return s
}

func (s *portMaster) Name() string { return s.cfg.Name }
func (s *portMaster) Kind() string { return "PortMaster" }

// Systems groups the ports as PortMaster's own menu does: its featured
// picks, everything, and the ports that need no files from the user's own
// copy of the game.
func (s *portMaster) Systems(context.Context) ([]System, error) {
	if s.dirs.Ports == "" {
		return nil, ErrNoPortMaster
	}
	s.mu.Lock()
	s.stale = true // reopening the source checks for a newer catalog
	s.mu.Unlock()
	return []System{
		{ID: "ports", Key: "featured", Label: "Featured ports"},
		{ID: "ports", Key: "all", Label: "All ports"},
		{ID: "ports", Key: "rtr", Label: "Ready to run"},
	}, nil
}

// pmIndex is ports.json.
type pmIndex struct {
	Ports map[string]json.RawMessage `json:"ports"`
	Utils map[string]pmUtil          `json:"utils"`

	ports    map[string]*pmPort // decoded Ports
	featured map[string]bool    // port names in the featured lists
}

type pmPort struct {
	Name     string   `json:"name"`
	Items    []string `json:"items"`
	ItemsOpt []string `json:"items_opt"`
	Attr     struct {
		Title   string            `json:"title"`
		Porter  config.StringList `json:"porter"`
		Desc    string            `json:"desc"`
		Inst    string            `json:"inst"`
		Genres  []string          `json:"genres"`
		Rtr     bool              `json:"rtr"`
		Exp     bool              `json:"exp"`
		Runtime config.StringList `json:"runtime"`
		Reqs    []string          `json:"reqs"`
		Arch    []string          `json:"arch"`
		Image   struct {
			Screenshot string   `json:"screenshot"`
			Covers     []string `json:"covers"`
		} `json:"image"`
	} `json:"attr"`
	Source struct {
		MD5  string `json:"md5"`
		Size int64  `json:"size"`
		URL  string `json:"url"`
	} `json:"source"`

	raw json.RawMessage
}

// pmUtil is a runtime, or another helper file PortMaster downloads.
type pmUtil struct {
	Name        string `json:"name"`
	RuntimeName string `json:"runtime_name"`
	RuntimeArch string `json:"runtime_arch"`
	MD5         string `json:"md5"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
}

// load returns the catalog. It is downloaded only when GitHub has a newer
// one than the copy on disk, parsed only when that copy changed, and read
// from the copy when GitHub can't be reached.
func (s *portMaster) load(ctx context.Context) (*pmIndex, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index != nil && !s.stale {
		return s.index, nil
	}
	b, changed, err := fetchCached(ctx, s.client, portMasterIndex, s.cachePath("ports.json"), 64<<20)
	if err != nil {
		return nil, err
	}
	// Without the featured lists the other groups still work.
	fb, fchanged, ferr := fetchCached(ctx, s.client, portMasterFeatured, s.cachePath("featured_ports.json"), 4<<20)
	if s.index != nil && !changed && !fchanged {
		s.stale = false
		return s.index, nil
	}
	idx, err := parsePortMasterIndex(b)
	if err != nil {
		return nil, err
	}
	if ferr == nil {
		idx.featured = parseFeatured(fb)
	}
	s.index, s.stale = idx, false
	return idx, nil
}

func (s *portMaster) cachePath(name string) string {
	if s.cache == "" {
		return ""
	}
	return filepath.Join(s.cache, name)
}

// pmFeatured is a featured list, or a category of them.
type pmFeatured struct {
	Deprecated bool         `json:"deprecated"`
	Ports      []string     `json:"ports"`
	Children   []pmFeatured `json:"children"`
}

// parseFeatured collects the ports of the current featured lists.
func parseFeatured(b []byte) map[string]bool {
	var lists []pmFeatured
	if json.Unmarshal(b, &lists) != nil {
		return nil
	}
	out := map[string]bool{}
	var add func([]pmFeatured)
	add = func(ls []pmFeatured) {
		for _, l := range ls {
			if l.Deprecated {
				continue
			}
			for _, p := range l.Ports {
				out[p] = true
			}
			add(l.Children)
		}
	}
	add(lists)
	return out
}

func parsePortMasterIndex(b []byte) (*pmIndex, error) {
	var idx pmIndex
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, err
	}
	idx.ports = make(map[string]*pmPort, len(idx.Ports))
	for name, raw := range idx.Ports {
		p := &pmPort{raw: raw}
		if json.Unmarshal(raw, p) != nil || p.Source.URL == "" {
			continue
		}
		p.Name = name
		idx.ports[name] = p
	}
	return &idx, nil
}

func (s *portMaster) Games(ctx context.Context, sys System) ([]Game, error) {
	idx, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	var out []Game
	for _, p := range idx.ports {
		switch {
		case !s.runs(p),
			sys.Key == "rtr" && !p.Attr.Rtr,
			sys.Key == "featured" && !idx.featured[p.Name]:
			continue
		}
		title := strings.TrimSpace(p.Attr.Title)
		if title == "" {
			title = strings.TrimSuffix(p.Name, ".zip")
		}
		out = append(out, Game{
			Name: title, Title: title,
			// Named after the port's folder, so an installed port shows
			// as installed.
			File:    p.folder() + ".zip",
			Group:   "pm:" + p.Name,
			Size:    p.Source.Size,
			URL:     p.Source.URL,
			MD5:     p.Source.MD5,
			System:  sys.ID,
			Extract: true, // space for the archive and its contents
			Info:    p.Name,
			Install: func(archive string) error { return installPort(archive, s.dirs, p) },
			Needs:   s.runtimes(idx, p),
		})
	}
	sortGames(out)
	return out, nil
}

// runs reports whether a port is meant for this device: built for its CPU,
// not experimental and not excluded on this firmware.
func (s *portMaster) runs(p *pmPort) bool {
	if p.Attr.Exp || len(p.Attr.Arch) > 0 && !slices.Contains(p.Attr.Arch, s.arch) {
		return false
	}
	for _, r := range p.Attr.Reqs {
		if strings.EqualFold(r, "!"+s.fw) {
			return false
		}
	}
	return true
}

// folder is the port's own directory in the ports folder.
func (p *pmPort) folder() string {
	for _, it := range p.Items {
		if !strings.HasSuffix(strings.ToLower(it), ".sh") {
			return strings.Trim(it, "/")
		}
	}
	return strings.TrimSuffix(p.Name, ".zip")
}

// runtimes lists the runtime images a port mounts, for this CPU.
func (s *portMaster) runtimes(idx *pmIndex, p *pmPort) []Game {
	libs := filepath.Join(s.dirs.Tools, "PortMaster", "libs")
	var out []Game
	for _, r := range p.Attr.Runtime {
		if !strings.HasSuffix(r, ".squashfs") {
			r += ".squashfs"
		}
		for _, u := range idx.Utils {
			if u.RuntimeName == r && u.RuntimeArch == s.arch && u.URL != "" {
				out = append(out, Game{Name: u.Name, File: r, Size: u.Size, URL: u.URL, MD5: u.MD5, Dir: libs})
				break
			}
		}
	}
	return out
}

// Details comes from the catalog; the pictures are the ones PortMaster
// shows, from the port's folder in the repository.
func (s *portMaster) Details(ctx context.Context, g Game) (*Details, error) {
	idx, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	p := idx.ports[g.Info]
	if p == nil {
		return &Details{}, nil
	}
	d := &Details{Author: strings.Join(p.Attr.Porter, ", "), Tags: p.Attr.Genres}
	var text []string
	for _, t := range []string{p.Attr.Desc, p.Attr.Inst} {
		if t = strings.TrimSpace(t); t != "" {
			text = append(text, t)
		}
	}
	d.Description = strings.Join(text, "\n\n")
	base := portMasterImages + strings.TrimSuffix(p.Name, ".zip") + "/"
	for _, img := range append([]string{p.Attr.Image.Screenshot}, p.Attr.Image.Covers...) {
		if img != "" && !strings.Contains(img, "/") {
			d.Images = append(d.Images, base+img)
		}
	}
	return d, nil
}
