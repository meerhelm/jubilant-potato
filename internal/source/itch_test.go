package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/meerhelm/jubilant-potato/internal/config"
)

func TestItch(t *testing.T) {
	rom := strings.Repeat("GB", 500)
	var approved bool
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authed := r.Header.Get("Authorization") == "Bearer tok"
		switch p := r.URL.Path; {
		case p == "/games/free/tag-gameboy-rom.xml":
			if r.URL.Query().Get("page") != "1" {
				fmt.Fprint(w, `<rss><channel></channel></rss>`)
				return
			}
			fmt.Fprintf(w, `<rss><channel>
<item><plainTitle>Polar Peril</plainTitle><link>%[1]s/polar</link><price>$0.00</price></item>
<item><plainTitle>Paid One</plainTitle><link>%[1]s/paid</link><price>$5.00</price></item>
<item><plainTitle>Web Only</plainTitle><link>%[1]s/web</link><price>$0.00</price></item>
</channel></rss>`, srv.URL)
		case p == "/games/free/tag-gbstudio.xml", p == "/games/free/tag-game-boy.xml":
			fmt.Fprintf(w, `<rss><channel><item><plainTitle>Polar Peril</plainTitle><link>%s/polar</link></item></channel></rss>`, srv.URL)
		case p == "/polar/data.json":
			fmt.Fprint(w, `{"id":11,"price":"$0.00"}`)
		case p == "/paid/data.json":
			fmt.Fprint(w, `{"id":12,"price":"$5.00"}`)
		case p == "/web/data.json":
			fmt.Fprint(w, `{"id":13}`)
		case !authed && !strings.HasPrefix(p, "/oauth/"):
			w.WriteHeader(401)
			fmt.Fprint(w, `{"errors":["authentication required"]}`)
		case p == "/games/11/uploads":
			fmt.Fprint(w, `{"uploads":[
{"id":101,"filename":"polar.gb","display_name":"Game Boy ROM","size":1000,"type":"default","storage":"hosted"},
{"id":102,"filename":"polar-win.zip","size":9,"type":"default","storage":"external"},
{"id":103,"filename":"manual.pdf","size":9,"type":"documentation","storage":"hosted"}]}`)
		case p == "/games/13/uploads":
			fmt.Fprint(w, `{"uploads":{}}`) // itch.io's empty list
		case p == "/profile/owned-keys":
			fmt.Fprint(w, `{"owned_keys":{}}`)
		case p == "/games/11/download-sessions":
			fmt.Fprint(w, `{"uuid":"u-1"}`)
		case p == "/uploads/101/download":
			if r.URL.Query().Get("uuid") != "u-1" {
				w.WriteHeader(400)
				return
			}
			http.Redirect(w, r, srv.URL+"/cdn/polar.gb", http.StatusFound)
		case p == "/cdn/polar.gb":
			http.ServeContent(w, r, "polar.gb", testTime, strings.NewReader(rom))
		case p == "/oauth/device":
			if r.FormValue("client_id") != "cid" || r.FormValue("code_challenge_method") != "S256" || r.FormValue("scope") != itchScopes {
				w.WriteHeader(400)
				return
			}
			fmt.Fprint(w, `{"device_code":"dc","user_code":"KX7T-4MPB","verification_uri_complete":"https://itch.io/device?code=KX7T","expires_in":600,"interval":5}`)
		case p == "/oauth/device/poll":
			if !approved {
				fmt.Fprint(w, `{"status":"pending"}`)
				return
			}
			fmt.Fprint(w, `{"status":"approved","code":"c1"}`)
		case p == "/oauth/token":
			if r.FormValue("code") != "c1" || r.FormValue("code_verifier") == "" || r.FormValue("redirect_uri") != "urn:itchio:poll" {
				w.WriteHeader(400)
				return
			}
			fmt.Fprint(w, `{"access_token":"tok","token_type":"bearer"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	itchAPI, itchWeb = srv.URL, srv.URL
	defer func() { itchAPI, itchWeb = "https://api.itch.io", "https://itch.io" }()

	var saved string
	src := newItch(config.Source{Name: "itch.io"}, srv.Client(), ClientInfo{ItchClientID: "cid", Name: "RG DS"},
		func(tok string) error { saved = tok; return nil })
	ctx := context.Background()

	games, err := src.Games(ctx, System{ID: "gb"})
	if err != nil || len(games) != 3 || games[1].Title != "Polar Peril" || games[1].Page == "" {
		t.Fatalf("games = %+v, %v", games, err)
	}
	polar := games[1]

	if _, err := src.Resolve(ctx, polar); !errors.Is(err, ErrNeedsPairing) {
		t.Fatalf("resolve before login: %v", err)
	}
	p, err := src.StartPairing(ctx)
	if err != nil || p.UserCode != "KX7T-4MPB" {
		t.Fatalf("pairing = %+v, %v", p, err)
	}
	if err := src.PollPairing(ctx, p); !errors.Is(err, ErrPairingPending) {
		t.Fatalf("poll: %v", err)
	}
	approved = true
	if err := src.PollPairing(ctx, p); err != nil || saved != "tok" {
		t.Fatalf("poll approved: %v, saved %q", err, saved)
	}

	files, err := src.Resolve(ctx, polar)
	if err != nil || len(files) != 1 || files[0].File != "polar.gb" || files[0].Size != 1000 {
		t.Fatalf("files = %+v, %v", files, err)
	}
	if _, err := src.Resolve(ctx, games[0]); !errors.Is(err, ErrPaid) {
		t.Errorf("paid game: %v", err)
	}
	if _, err := src.Resolve(ctx, games[2]); !errors.Is(err, ErrNoFiles) {
		t.Errorf("web-only game: %v", err)
	}

	// Download with resume from byte 100.
	body, total, err := files[0].Open(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(body)
	body.Close()
	if total != int64(len(rom)) || string(b) != rom[100:] {
		t.Errorf("download: total %d, got %d bytes", total, len(b))
	}
}

var testTime = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
