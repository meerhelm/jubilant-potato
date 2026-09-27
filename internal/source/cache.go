package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// cacheMeta holds the validators a cached file was served with.
type cacheMeta struct {
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

// fetchCached downloads url, keeping a copy at path: once there is one, the
// server is asked for the file only if it changed (ETag, Last-Modified),
// and the copy is used when the server can't be reached. changed is false
// when the content is the copy already on disk. An empty path disables the
// copy.
func fetchCached(ctx context.Context, client *http.Client, url, path string, limit int64) (body []byte, changed bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", UserAgent)
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			var m cacheMeta
			if b, err := os.ReadFile(path + ".meta"); err == nil && json.Unmarshal(b, &m) == nil {
				if m.ETag != "" {
					req.Header.Set("If-None-Match", m.ETag)
				}
				if m.LastModified != "" {
					req.Header.Set("If-Modified-Since", m.LastModified)
				}
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fromCache(path, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		return fromCache(path, fmt.Errorf("GET %s: %s", url, resp.Status))
	case http.StatusOK:
	default:
		return fromCache(path, fmt.Errorf("GET %s: %s", url, resp.Status))
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return fromCache(path, err)
	}
	if path != "" {
		m := cacheMeta{ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}
		if err := saveCached(path, body, m); err != nil {
			log.Printf("cache %s: %v", path, err)
		}
	}
	return body, true, nil
}

// fromCache returns the copy on disk, or err when there is none.
func fromCache(path string, err error) ([]byte, bool, error) {
	if path == "" {
		return nil, false, err
	}
	b, rerr := os.ReadFile(path)
	if rerr != nil {
		return nil, false, err
	}
	return b, false, nil
}

// saveCached writes the file, then its validators, each atomically.
func saveCached(path string, body []byte, m cacheMeta) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	meta, _ := json.Marshal(m)
	for _, f := range []struct {
		path string
		data []byte
	}{{path, body}, {path + ".meta", meta}} {
		tmp := f.path + ".tmp"
		if err := os.WriteFile(tmp, f.data, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, f.path); err != nil {
			return err
		}
	}
	return nil
}
