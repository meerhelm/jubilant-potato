package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/meerhelm/jubilant-potato/internal/platform"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		cur, latest string
		want        bool
	}{
		{"v0.1.0", "v0.1.1", true},
		{"v0.1.9", "v0.2.0", true},
		{"v1.0.0", "v0.9.9", false},
		{"v0.2.0", "v0.2.0", false},
		{"dev", "v9.0.0", false},
		{"7aa76f0-dirty", "v0.2.0", false},
		{"v0.1.0", "nightly", false},
	} {
		if got := Newer(c.cur, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.cur, c.latest, got)
		}
	}
}

func rocknixPackage(t *testing.T) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"ports/JubilantPotato.sh":                  "#!/bin/bash\n",
		"ports/jubilantpotato/potato":              "NEW BINARY",
		"ports/jubilantpotato/LICENSE":             "GPL",
		"ports/jubilantpotato/config.example.json": "{}",
		"ports/jubilantpotato/config.json":         "must not be installed",
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestInstall(t *testing.T) {
	pkg := rocknixPackage(t)
	sum := sha256.Sum256(pkg)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprintf(w, `{"tag_name":"v0.2.0","body":"notes","assets":[
{"name":"JubilantPotato-muos.muxapp","browser_download_url":"%[1]s/muos","size":1},
{"name":"JubilantPotato-rocknix.zip","browser_download_url":"%[1]s/rocknix","size":%[2]d,"digest":"sha256:%[3]s"}]}`,
				srv.URL, len(pkg), hex.EncodeToString(sum[:]))
		case "/rocknix":
			w.Write(pkg)
		}
	}))
	defer srv.Close()
	API = srv.URL + "/latest"
	defer func() { API = "https://api.github.com/repos/meerhelm/jubilant-potato/releases/latest" }()

	ctx := context.Background()
	rel, err := Latest(ctx, srv.Client(), "test")
	if err != nil || rel.Version != "v0.2.0" {
		t.Fatalf("latest = %+v, %v", rel, err)
	}
	asset, ok := rel.AssetFor(platform.Rocknix)
	if !ok || asset.SHA256 == "" {
		t.Fatalf("asset = %+v", asset)
	}
	if _, ok := rel.AssetFor(platform.Desktop); ok {
		t.Error("desktop should have no package")
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "potato")
	os.WriteFile(exe, []byte("OLD BINARY"), 0o755)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"mine":true}`), 0o644)

	var last int64
	if err := Install(ctx, srv.Client(), asset, exe, func(done, total int64) { last = done }); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW BINARY" {
		t.Errorf("binary = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "config.json")); string(b) != `{"mine":true}` {
		t.Errorf("config.json was overwritten: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "LICENSE")); string(b) != "GPL" {
		t.Errorf("LICENSE = %q", b)
	}
	if last != int64(len(pkg)) {
		t.Errorf("progress ended at %d of %d", last, len(pkg))
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 4 { // potato, config.json, LICENSE, config.example.json
		t.Errorf("leftover files: %v", entries)
	}

	// A corrupted download must leave the app untouched.
	os.WriteFile(exe, []byte("OLD BINARY"), 0o755)
	asset.SHA256 = "00"
	if err := Install(ctx, srv.Client(), asset, exe, nil); err == nil {
		t.Fatal("expected checksum error")
	}
	if b, _ := os.ReadFile(exe); string(b) != "OLD BINARY" {
		t.Errorf("binary changed after failed update: %q", b)
	}
}
