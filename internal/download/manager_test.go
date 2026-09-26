package download

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meerhelm/jubilant-potato/internal/source"
)

func waitIdle(t *testing.T, m *Manager) []Info {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for m.Active() > 0 {
		if time.Now().After(deadline) {
			t.Fatal("downloads did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return m.Jobs()
}

func TestResumeAndExtract(t *testing.T) {
	payload := bytes.Repeat([]byte("potato"), 10000)
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, _ := zw.Create("Game.nds")
	w.Write(payload)
	zw.Close()

	var sawRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Game.zip":
			http.ServeContent(w, r, "Game.zip", time.Time{}, bytes.NewReader(zbuf.Bytes()))
		case "/Rom.gba":
			sawRange = r.Header.Get("Range")
			http.ServeContent(w, r, "Rom.gba", time.Time{}, bytes.NewReader(payload))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dest := t.TempDir()
	// Simulate an interrupted download of Rom.gba.
	os.WriteFile(filepath.Join(dest, "Rom.gba.part"), payload[:1000], 0o644)

	m := NewManager(srv.Client())
	m.Enqueue(source.Game{Name: "Rom", File: "Rom.gba", URL: srv.URL + "/Rom.gba"}, dest, false)
	m.Enqueue(source.Game{Name: "Game", File: "Game.zip", URL: srv.URL + "/Game.zip"}, dest, true)
	m.Enqueue(source.Game{Name: "Missing", File: "Missing.gba", URL: srv.URL + "/nope"}, dest, false)

	jobs := waitIdle(t, m)
	if jobs[0].State != Done || jobs[1].State != Done || jobs[2].State != Failed {
		t.Fatalf("states = %+v", jobs)
	}
	if sawRange != "bytes=1000-" {
		t.Errorf("Range = %q, want resume from 1000", sawRange)
	}
	for _, f := range []string{"Rom.gba", "Game.nds"} {
		b, err := os.ReadFile(filepath.Join(dest, f))
		if err != nil || !bytes.Equal(b, payload) {
			t.Errorf("%s: content mismatch (err %v)", f, err)
		}
	}
	entries, _ := os.ReadDir(dest)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") || e.Name() == "Game.zip" {
			t.Errorf("leftover file %s", e.Name())
		}
	}
}

func TestUnzipRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	zpath := filepath.Join(dir, "evil.zip")
	f, _ := os.Create(zpath)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("../escape.txt")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()

	if err := unzip(zpath, filepath.Join(dir, "out")); err == nil {
		t.Fatal("expected error for path traversal")
	}
}
