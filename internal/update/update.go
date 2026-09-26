// Package update checks GitHub releases for a newer version and installs
// it over the running app.
package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/meerhelm/jubilant-potato/internal/platform"
)

// API is the latest-release endpoint; tests point it elsewhere.
var API = "https://api.github.com/repos/meerhelm/jubilant-potato/releases/latest"

// Release is a published version.
type Release struct {
	Version string // "v0.2.0"
	Notes   string
	Assets  []Asset
}

// Asset is a downloadable package of a release.
type Asset struct {
	Name   string
	URL    string
	Size   int64
	SHA256 string // hex, empty when GitHub didn't provide one
}

// Latest fetches the newest published (non-draft, non-prerelease) release.
func Latest(ctx context.Context, client *http.Client, userAgent string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, API, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub: %s", resp.Status)
	}
	var r struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		Assets  []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Size   int64  `json:"size"`
			Digest string `json:"digest"` // "sha256:<hex>"
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	rel := &Release{Version: r.TagName, Notes: r.Body}
	for _, a := range r.Assets {
		sum, _ := strings.CutPrefix(a.Digest, "sha256:")
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size, SHA256: sum})
	}
	return rel, nil
}

// packageName is the release asset for each firmware.
var packageName = map[platform.Firmware]string{
	platform.MuOS:    "JubilantPotato-muos.muxapp",
	platform.Rocknix: "JubilantPotato-rocknix.zip",
	platform.Stock:   "JubilantPotato-stock.zip",
}

// AssetFor picks the package matching the firmware.
func (r *Release) AssetFor(fw platform.Firmware) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == packageName[fw] {
			return a, true
		}
	}
	return Asset{}, false
}

// Newer reports whether latest is a higher version than current. Builds
// that aren't plain releases (dev, commit hashes) never report updates.
func Newer(current, latest string) bool {
	c, ok1 := parse(current)
	l, ok2 := parse(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// IsRelease reports whether v looks like a release version ("v1.2.3").
func IsRelease(v string) bool {
	_, ok := parse(v)
	return ok
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// appFiles are the package files that live in the app folder and are
// replaced by an update; the user's config.json is never touched.
var appFiles = map[string]bool{
	"potato": true, "LICENSE": true, "THIRD_PARTY_LICENSES.txt": true, "config.example.json": true,
}

// Install downloads the package, verifies its checksum and replaces the
// app files next to exe. progress receives bytes done and total.
func Install(ctx context.Context, client *http.Client, a Asset, exe string, progress func(done, total int64)) error {
	dir := filepath.Dir(exe)
	pkg := filepath.Join(dir, "update.part")
	defer os.Remove(pkg)

	if err := download(ctx, client, a, pkg, progress); err != nil {
		return err
	}
	zr, err := zip.OpenReader(pkg)
	if err != nil {
		return fmt.Errorf("update package: %w", err)
	}
	defer zr.Close()

	// Stage every file first so a broken package leaves the app intact.
	staged := map[string]string{}
	defer func() {
		for _, tmp := range staged {
			os.Remove(tmp)
		}
	}()
	for _, f := range zr.File {
		name := path.Base(f.Name)
		if f.FileInfo().IsDir() || !appFiles[name] {
			continue
		}
		target := filepath.Join(dir, name)
		if name == "potato" {
			target = exe
		}
		tmp := target + ".new"
		if err := extract(f, tmp); err != nil {
			return err
		}
		staged[target] = tmp
	}
	if _, ok := staged[exe]; !ok {
		return errors.New("update package has no app binary")
	}
	for target, tmp := range staged {
		// Renaming over the running binary is fine on Linux: the process
		// keeps the old inode until it restarts.
		if err := os.Rename(tmp, target); err != nil {
			return err
		}
		delete(staged, target)
	}
	return nil
}

func download(ctx context.Context, client *http.Client, a Asset, dst string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	h := sha256.New()
	total := a.Size
	if total <= 0 {
		total = resp.ContentLength
	}
	var done int64
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if a.SHA256 != "" && hex.EncodeToString(h.Sum(nil)) != strings.ToLower(a.SHA256) {
		return errors.New("update package checksum mismatch")
	}
	return nil
}

func extract(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, rc)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
