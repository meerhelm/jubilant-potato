package source

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	"golang.org/x/net/html"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/platform"
)

// httpIndex browses a plain web-server directory listing (nginx/Apache/Caddy
// autoindex, python -m http.server, ...) with one folder per system.
type httpIndex struct {
	cfg    config.Source
	root   *url.URL
	header http.Header
	client *http.Client
}

func newHTTPIndex(c config.Source, client *http.Client) (*httpIndex, error) {
	u, err := url.Parse(c.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("source %q: invalid url %q", c.Name, c.URL)
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	h := http.Header{}
	if c.Username != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password))
		h.Set("Authorization", "Basic "+cred)
	}
	return &httpIndex{cfg: c, root: u, header: h, client: client}, nil
}

func (s *httpIndex) Name() string { return s.cfg.Name }

func (s *httpIndex) Systems(ctx context.Context) ([]System, error) {
	var out []System
	if len(s.cfg.Systems) > 0 {
		for id, paths := range s.cfg.Systems {
			sys, ok := platform.SystemByID(id)
			if !ok {
				continue
			}
			var keys []string
			for _, p := range paths {
				keys = append(keys, s.root.ResolveReference(&url.URL{Path: dirPath(p)}).String())
			}
			out = append(out, System{ID: id, Key: strings.Join(keys, "\n"), Label: sys.Name})
		}
		sortSystems(out)
		return out, nil
	}

	links, err := s.list(ctx, s.root)
	if err != nil {
		return nil, err
	}
	byID := map[string]int{}
	for _, l := range links {
		if !l.dir {
			continue
		}
		sys, ok := platform.MatchSystem(l.name)
		if !ok {
			continue
		}
		// Several folders may map to one system (e.g. "gb" and "Game Boy").
		if i, ok := byID[sys.ID]; ok {
			out[i].Key += "\n" + l.url.String()
			continue
		}
		byID[sys.ID] = len(out)
		out = append(out, System{ID: sys.ID, Key: l.url.String(), Label: sys.Name})
	}
	sortSystems(out)
	return out, nil
}

func (s *httpIndex) Games(ctx context.Context, sys System) ([]Game, error) {
	psys, _ := platform.SystemByID(sys.ID)
	var out []Game
	for _, key := range strings.Split(sys.Key, "\n") {
		u, err := url.Parse(key)
		if err != nil {
			return nil, err
		}
		links, err := s.list(ctx, u)
		if err != nil {
			return nil, err
		}
		for _, l := range links {
			if l.dir || !psys.Accepts(l.name) {
				continue
			}
			out = append(out, Game{
				Name:   displayName(l.name),
				File:   l.name,
				URL:    l.url.String(),
				Header: s.header,
				System: sys.ID,
			})
		}
	}
	sortGames(out)
	return out, nil
}

type link struct {
	name string
	url  *url.URL
	dir  bool
}

// list fetches a directory listing and returns its direct children.
func (s *httpIndex) list(ctx context.Context, dir *url.URL) ([]link, error) {
	resp, err := get(ctx, s.client, dir.String(), s.header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	hrefs, err := parseHrefs(resp)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []link
	for _, href := range hrefs {
		if href == "" || strings.HasPrefix(href, "?") || strings.HasPrefix(href, "#") {
			continue
		}
		ref, err := url.Parse(href)
		if err != nil {
			continue
		}
		u := dir.ResolveReference(ref)
		u.RawQuery, u.Fragment = "", ""
		// Only direct children of dir on the same host: skips parent links,
		// sort links and anything pointing elsewhere.
		if u.Host != dir.Host || !strings.HasPrefix(u.Path, dir.Path) || u.Path == dir.Path {
			continue
		}
		rest := strings.TrimPrefix(u.Path, dir.Path)
		isDir := strings.HasSuffix(rest, "/")
		rest = strings.TrimSuffix(rest, "/")
		if rest == "" || strings.Contains(rest, "/") || seen[rest] {
			continue
		}
		seen[rest] = true
		out = append(out, link{name: rest, url: u, dir: isDir})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func parseHrefs(resp *http.Response) ([]string, error) {
	var hrefs []string
	z := html.NewTokenizer(resp.Body)
	for {
		switch z.Next() {
		case html.ErrorToken:
			if err := z.Err(); err != io.EOF {
				return nil, err
			}
			return hrefs, nil
		case html.StartTagToken:
			name, hasAttr := z.TagName()
			if string(name) != "a" || !hasAttr {
				continue
			}
			for {
				k, v, more := z.TagAttr()
				if string(k) == "href" {
					hrefs = append(hrefs, string(v))
				}
				if !more {
					break
				}
			}
		}
	}
}

func dirPath(p string) string {
	p = path.Clean("/" + p)[1:]
	if p != "" {
		p += "/"
	}
	return p
}
