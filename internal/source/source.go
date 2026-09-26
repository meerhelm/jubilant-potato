// Package source lists games from remote catalogs.
package source

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	// Mozilla root CAs, used only when the firmware ships without a CA bundle.
	_ "golang.org/x/crypto/x509roots/fallback"

	"github.com/meerhelm/jubilant-potato/internal/config"
)

// System is a group of games offered by a source.
type System struct {
	ID    string // canonical platform system ID
	Key   string // source-specific key used to list games
	Label string
}

// Game is a single downloadable file.
type Game struct {
	Name    string // display name of this file, tags included
	File    string // file name to save as
	Title   string // clean game title shared by variants; empty = parse from File
	Group   string // variants with equal Group are the same game; empty = match by title
	Size    int64  // bytes, 0 when unknown
	URL     string
	Header  http.Header // extra request headers (auth)
	System  string      // canonical platform system ID
	Extract bool        // always unpack after download (multi-file bundles)
}

// Source is a remote catalog of games.
type Source interface {
	Name() string
	Systems(ctx context.Context) ([]System, error)
	Games(ctx context.Context, sys System) ([]Game, error)
}

const userAgent = "jubilant-potato/0.1"

// NewHTTPClient returns the client used for catalogs and downloads.
func NewHTTPClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Transport: tr}
}

// New builds a source from its configuration. saveToken persists
// credentials obtained by pairing.
func New(c config.Source, client *http.Client, info ClientInfo, saveToken func(string) error) (Source, error) {
	switch strings.ToLower(c.Type) {
	case "http", "http-index":
		return newHTTPIndex(c, client)
	case "archive.org", "archiveorg":
		return newArchiveOrg(c, client), nil
	case "romm":
		return newRomm(c, client, info, saveToken)
	}
	return nil, fmt.Errorf("source %q: unknown type %q", c.Name, c.Type)
}

func get(ctx context.Context, client *http.Client, url string, h http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range h {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

func sortGames(g []Game) {
	sort.Slice(g, func(i, j int) bool { return strings.ToLower(g[i].Name) < strings.ToLower(g[j].Name) })
}

func sortSystems(s []System) {
	sort.Slice(s, func(i, j int) bool { return strings.ToLower(s[i].Label) < strings.ToLower(s[j].Label) })
}

// displayName strips the extension from a file name.
func displayName(file string) string {
	if i := strings.LastIndexByte(file, '.'); i > 0 {
		return file[:i]
	}
	return file
}
