package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/platform"
)

// archiveOrg lists files of user-configured archive.org items.
type archiveOrg struct {
	cfg    config.Source
	client *http.Client
}

func newArchiveOrg(c config.Source, client *http.Client) *archiveOrg {
	return &archiveOrg{cfg: c, client: client}
}

func (s *archiveOrg) Name() string { return s.cfg.Name }
func (s *archiveOrg) Kind() string { return "archive.org" }

func (s *archiveOrg) Systems(ctx context.Context) ([]System, error) {
	var out []System
	for id, items := range s.cfg.Systems {
		sys, ok := platform.SystemByID(id)
		if !ok || len(items) == 0 {
			continue
		}
		out = append(out, System{ID: id, Key: strings.Join(items, "\n"), Label: sys.Name})
	}
	sortSystems(out)
	return out, nil
}

type iaMetadata struct {
	Metadata struct {
		Title any `json:"title"` // string, or a list for some items
	} `json:"metadata"`
	Files []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
		Size   string `json:"size"`
	} `json:"files"`
}

func fetchMetadata(ctx context.Context, client *http.Client, item string) (*iaMetadata, error) {
	resp, err := get(ctx, client, "https://archive.org/metadata/"+url.PathEscape(item), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var md iaMetadata
	if err := json.NewDecoder(resp.Body).Decode(&md); err != nil {
		return nil, fmt.Errorf("archive.org %s: %w", item, err)
	}
	if len(md.Files) == 0 {
		return nil, fmt.Errorf("archive.org %s: item not found or empty", item)
	}
	return &md, nil
}

func (md *iaMetadata) title() string {
	switch t := md.Metadata.Title.(type) {
	case string:
		return t
	case []any:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

// ArchiveItem is an archive.org search result.
type ArchiveItem struct {
	ID    string
	Title string
	Size  int64
}

// ArchiveSearch finds software and data items whose title matches query,
// most downloaded first. (Full-text search drowns results in unrelated
// items, so only titles are searched.)
func ArchiveSearch(ctx context.Context, client *http.Client, query string) ([]ArchiveItem, error) {
	q := url.Values{
		"q":      {"title:(" + query + ") AND mediatype:(software OR data)"},
		"fl[]":   {"identifier", "title", "item_size"},
		"sort[]": {"downloads desc"},
		"rows":   {"100"},
		"output": {"json"},
	}
	resp, err := get(ctx, client, "https://archive.org/advancedsearch.php?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		Response struct {
			Docs []struct {
				Identifier string `json:"identifier"`
				Title      any    `json:"title"`
				ItemSize   int64  `json:"item_size"`
			} `json:"docs"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("archive.org search: %w", err)
	}
	out := make([]ArchiveItem, 0, len(r.Response.Docs))
	for _, d := range r.Response.Docs {
		title, _ := d.Title.(string)
		if title == "" {
			title = d.Identifier
		}
		out = append(out, ArchiveItem{ID: d.Identifier, Title: title, Size: d.ItemSize})
	}
	return out, nil
}

// SystemCount is how many files of an item belong to a system.
type SystemCount struct {
	ID    string
	Files int
}

// ArchiveDetect lists the systems found in an item, most files first.
// Archives that name no system by folder are attributed to the system in
// the item's title.
func ArchiveDetect(ctx context.Context, client *http.Client, item string) (string, []SystemCount, error) {
	md, err := fetchMetadata(ctx, client, item)
	if err != nil {
		return "", nil, err
	}
	fallback, hasFallback := platform.GuessSystem(md.title() + " " + item)
	counts := map[string]int{}
	for _, f := range md.Files {
		if f.Source == "metadata" {
			continue
		}
		if s, ok := platform.SystemForFile(f.Name); ok {
			counts[s.ID]++
		} else if hasFallback && fallback.Accepts(f.Name) {
			counts[fallback.ID]++
		}
	}
	out := make([]SystemCount, 0, len(counts))
	for id, n := range counts {
		out = append(out, SystemCount{ID: id, Files: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Files > out[j].Files })
	return md.title(), out, nil
}

func (s *archiveOrg) Games(ctx context.Context, sys System) ([]Game, error) {
	psys, _ := platform.SystemByID(sys.ID)
	var out []Game
	for _, item := range strings.Split(sys.Key, "\n") {
		md, err := fetchMetadata(ctx, s.client, item)
		if err != nil {
			return nil, err
		}
		for _, f := range md.Files {
			file := path.Base(f.Name)
			if f.Source == "metadata" || !psys.Accepts(file) {
				continue
			}
			// In multi-system collections keep only this system's folder.
			if other, ok := platform.SystemForFile(f.Name); ok && other.ID != sys.ID && strings.Contains(f.Name, "/") {
				continue
			}
			size, _ := strconv.ParseInt(f.Size, 10, 64)
			out = append(out, Game{
				Name:   displayName(file),
				File:   file,
				Size:   size,
				URL:    "https://archive.org/download/" + url.PathEscape(item) + "/" + escapePath(f.Name),
				System: sys.ID,
			})
		}
	}
	sortGames(out)
	return out, nil
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
