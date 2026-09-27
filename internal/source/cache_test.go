package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestFetchCached(t *testing.T) {
	body, etag := "v1", `"1"`
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Write([]byte(body))
	}))
	path := filepath.Join(t.TempDir(), "c", "ports.json")
	fetch := func() (string, bool) {
		t.Helper()
		b, changed, err := fetchCached(context.Background(), srv.Client(), srv.URL, path, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		return string(b), changed
	}

	if b, changed := fetch(); b != "v1" || !changed {
		t.Errorf("no copy yet: %q %v", b, changed)
	}
	if b, changed := fetch(); b != "v1" || changed {
		t.Errorf("unchanged: %q %v", b, changed)
	}
	body, etag = "v2", `"2"`
	if b, changed := fetch(); b != "v2" || !changed {
		t.Errorf("updated: %q %v", b, changed)
	}
	srv.Close()
	if b, changed := fetch(); b != "v2" || changed {
		t.Errorf("offline: %q %v", b, changed)
	}
	if hits != 3 {
		t.Errorf("%d requests, want 3", hits)
	}
	if _, _, err := fetchCached(context.Background(), srv.Client(), srv.URL, filepath.Join(t.TempDir(), "none"), 1<<20); err == nil {
		t.Error("offline without a copy: no error")
	}
}
