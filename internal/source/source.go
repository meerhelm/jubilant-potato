// Package source lists games from remote catalogs.
package source

import (
	"context"
	"fmt"
	"io"
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

	// Page, when set, marks a catalog entry that is a game page rather than
	// a file; the source's Resolver turns it into files (itch.io).
	Page string

	// Info, when set, is a web page describing the game; the source's
	// Describer reads it (PICO-8 BBS threads).
	Info string

	// Open, when set, reads the file from offset instead of fetching URL
	// over HTTP, and reports the file's total size (SMB shares).
	Open func(ctx context.Context, offset int64) (io.ReadCloser, int64, error)

	// MD5, when set, is the hex digest the downloaded file must have.
	MD5 string

	// Install, when set, installs the downloaded archive instead of it
	// being kept or extracted (PortMaster ports).
	Install func(archive string) error

	// Needs lists files the game can't run without (PortMaster runtimes).
	// They are downloaded into their Dir first unless already there.
	Needs []Game
	Dir   string // destination of a needed file
}

// Source is a remote catalog of games.
type Source interface {
	Name() string
	Kind() string // short type label, e.g. "RomM", "SMB"
	Systems(ctx context.Context) ([]System, error)
	Games(ctx context.Context, sys System) ([]Game, error)
}

// Details describes a game beyond its files.
type Details struct {
	Author      string
	Description string // plain text, paragraphs separated by blank lines
	Tags        []string
	Images      []string // image URLs, cover first
}

// Describer is implemented by sources that can describe a game (Game.Info)
// before it is downloaded.
type Describer interface {
	Details(ctx context.Context, g Game) (*Details, error)
}

// UserAgent identifies the app to servers; main sets the full value with
// version, firmware and device, as itch.io asks clients to.
var UserAgent = "JubilantPotato (+https://github.com/meerhelm/jubilant-potato)"

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
	case "smb":
		return newSMB(c)
	case "itch", "itch.io":
		return newItch(c, client, info, saveToken), nil
	case "pico8", "pico-8", "lexaloffle":
		return newLexaloffle(c, client), nil
	case "portmaster":
		return newPortMaster(c, client, info.PortMaster, info.Platform, info.CacheDir), nil
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
	req.Header.Set("User-Agent", UserAgent)
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
	sort.SliceStable(g, func(i, j int) bool { return strings.ToLower(g[i].Name) < strings.ToLower(g[j].Name) })
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
