package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
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
	Files []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
		Size   string `json:"size"`
	} `json:"files"`
}

func (s *archiveOrg) Games(ctx context.Context, sys System) ([]Game, error) {
	psys, _ := platform.SystemByID(sys.ID)
	var out []Game
	for _, item := range strings.Split(sys.Key, "\n") {
		resp, err := get(ctx, s.client, "https://archive.org/metadata/"+url.PathEscape(item), nil)
		if err != nil {
			return nil, err
		}
		var md iaMetadata
		err = json.NewDecoder(resp.Body).Decode(&md)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("archive.org %s: %w", item, err)
		}
		if len(md.Files) == 0 {
			return nil, fmt.Errorf("archive.org %s: item not found or empty", item)
		}
		for _, f := range md.Files {
			file := path.Base(f.Name)
			if f.Source == "metadata" || !psys.Accepts(file) {
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
