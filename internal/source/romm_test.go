package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/meerhelm/jubilant-potato/internal/config"
)

// fakeRomm implements the slice of the RomM API the client uses.
type fakeRomm struct {
	approved bool
	revoked  bool
	polls    int
	roms     int // number of GBA roms to serve
}

func (f *fakeRomm) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	authed := r.Header.Get("Authorization") == "Bearer rmm_test" && !f.revoked
	fail := func(code int, detail string) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"detail": detail})
	}
	switch r.URL.Path {
	case "/api/auth/device/init":
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["client"] != "jubilant-potato" || body["client_device_identifier"] != "dev-1" {
			fail(422, "bad init payload")
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"device_code": "secret", "user_code": "ABCD-1234",
			"verification_path":          "/pair/device",
			"verification_path_complete": "/pair/device?user_code=ABCD-1234",
			"expires_in":                 600, "interval": 5,
		})
	case "/api/auth/device/token":
		f.polls++
		if !f.approved {
			fail(400, "authorization_pending")
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "rmm_test", "device_id": "d", "scopes": rommScopes})
	case "/api/platforms":
		if !authed {
			fail(401, "Not authenticated")
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": 3, "slug": "gba", "fs_slug": "gba", "name": "Game Boy Advance", "rom_count": f.roms},
			{"id": 4, "slug": "sfam", "fs_slug": "Super Famicom", "name": "Super Famicom", "rom_count": 1},
			{"id": 5, "slug": "ps2", "fs_slug": "ps2", "name": "PlayStation 2", "rom_count": 9}, // unknown to us
			{"id": 6, "slug": "nes", "fs_slug": "nes", "name": "NES", "rom_count": 0},           // empty
		})
	case "/api/roms":
		if !authed {
			fail(401, "Not authenticated")
			return
		}
		q := r.URL.Query()
		if q.Get("platform_ids") != "3" {
			fail(400, "unexpected platform")
			return
		}
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))
		var items []map[string]any
		for i := offset; i < f.roms && i < offset+limit; i++ {
			items = append(items, map[string]any{
				"id": i + 1, "name": fmt.Sprintf("Game %04d", i), "fs_name": fmt.Sprintf("game%04d.gba", i),
				"fs_name_no_ext": fmt.Sprintf("game%04d", i), "fs_size_bytes": 1024,
				"has_multiple_files": i == 0, "missing_from_fs": i == 1,
			})
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items, "total": f.roms, "limit": limit, "offset": offset})
	default:
		http.NotFound(w, r)
	}
}

func TestRommPairingAndCatalog(t *testing.T) {
	fake := &fakeRomm{roms: 2500} // more than one page
	srv := httptest.NewServer(fake)
	defer srv.Close()

	var saved []string
	src, err := newRomm(config.Source{Name: "RomM", URL: srv.URL + "/"}, srv.Client(),
		ClientInfo{DeviceID: "dev-1", Name: "RG34XX"}, func(tok string) error {
			saved = append(saved, tok)
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if !src.NeedsPairing() {
		t.Fatal("new source without token should need pairing")
	}
	if _, err := src.Systems(ctx); !errors.Is(err, ErrNeedsPairing) {
		t.Fatalf("Systems before pairing: %v", err)
	}

	p, err := src.StartPairing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.UserCode != "ABCD-1234" || p.URL != srv.URL+"/pair/device?user_code=ABCD-1234" {
		t.Errorf("pairing = %+v", p)
	}
	if err := src.PollPairing(ctx, p); !errors.Is(err, ErrPairingPending) {
		t.Fatalf("poll before approval: %v", err)
	}
	fake.approved = true
	if err := src.PollPairing(ctx, p); err != nil {
		t.Fatal(err)
	}
	if src.NeedsPairing() || len(saved) != 1 || saved[0] != "rmm_test" {
		t.Fatalf("token not stored: %v", saved)
	}

	systems, err := src.Systems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(systems) != 2 || systems[0].ID != "gba" || systems[0].Key != "3" || systems[1].ID != "snes" {
		t.Fatalf("systems = %+v", systems)
	}

	games, err := src.Games(ctx, systems[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != fake.roms-1 { // one is missing from the server's disk
		t.Fatalf("got %d games", len(games))
	}
	g := games[0]
	if g.File != "game0000.gba.zip" || !g.Extract || g.URL != srv.URL+"/api/roms/1/content/game0000.gba.zip" {
		t.Errorf("multi-file game = %+v", g)
	}
	if games[1].Name != "Game 0002" || games[1].Extract || games[1].Header.Get("Authorization") != "Bearer rmm_test" {
		t.Errorf("game = %+v", games[1])
	}

	// A revoked token sends the user back to pairing.
	fake.revoked = true
	if _, err := src.Systems(ctx); !errors.Is(err, ErrNeedsPairing) || !src.NeedsPairing() {
		t.Fatalf("revoked token: err=%v needsPairing=%v", err, src.NeedsPairing())
	}
	if saved[len(saved)-1] != "" {
		t.Error("revoked token should be cleared from config")
	}
}
