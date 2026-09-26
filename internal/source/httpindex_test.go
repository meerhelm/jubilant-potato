package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/meerhelm/jubilant-potato/internal/config"
)

// Listings as produced by nginx autoindex and python -m http.server.
var listings = map[string]string{
	"/": `<html><body><h1>Index of /</h1><hr><pre><a href="../">../</a>
<a href="GBA/">GBA/</a>                 01-Jan-2026 00:00       -
<a href="Nintendo%20-%20Super%20Nintendo%20Entertainment%20System/">Nintendo - Super Nintendo Entertainment System/</a>
<a href="unknown-stuff/">unknown-stuff/</a>
<a href="readme.txt">readme.txt</a>
</pre></body></html>`,
	"/GBA/": `<ul>
<li><a href="Anguna.gba">Anguna.gba</a></li>
<li><a href="Some%20Game%20%28USA%29.zip">Some Game (USA).zip</a></li>
<li><a href="notes.txt">notes.txt</a></li>
<li><a href="?C=N;O=D">Name</a></li>
<li><a href="/GBA/">self</a></li>
<li><a href="http://evil.example/x.gba">external</a></li>
</ul>`,
	"/Nintendo - Super Nintendo Entertainment System/": `<a href="Nekotsume.sfc">Nekotsume.sfc</a>`,
}

func TestHTTPIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := listings[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()

	src, err := newHTTPIndex(config.Source{Name: "test", URL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	systems, err := src.Systems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(systems) != 2 || systems[0].ID != "gba" || systems[1].ID != "snes" {
		t.Fatalf("systems = %+v", systems)
	}

	games, err := src.Games(ctx, systems[0])
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Anguna.gba", "Some Game (USA).zip"}
	if len(games) != len(want) {
		t.Fatalf("games = %+v", games)
	}
	for i, g := range games {
		if g.File != want[i] {
			t.Errorf("game %d = %q, want %q", i, g.File, want[i])
		}
	}
	if games[1].Name != "Some Game (USA)" || games[1].URL != srv.URL+"/GBA/Some%20Game%20%28USA%29.zip" {
		t.Errorf("game = %+v", games[1])
	}

	snes, err := src.Games(ctx, systems[1])
	if err != nil || len(snes) != 1 || snes[0].File != "Nekotsume.sfc" {
		t.Fatalf("snes = %+v, %v", snes, err)
	}
}

func TestHTTPIndexExplicitSystems(t *testing.T) {
	src, err := newHTTPIndex(config.Source{
		URL:     "http://nas.local/roms",
		Systems: map[string]config.StringList{"gba": {"Game Boy Advance", "gba-hacks"}, "bogus": {"x"}},
	}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	systems, _ := src.Systems(context.Background())
	if len(systems) != 1 {
		t.Fatalf("systems = %+v", systems)
	}
	want := "http://nas.local/roms/Game%20Boy%20Advance/\nhttp://nas.local/roms/gba-hacks/"
	if systems[0].Key != want {
		t.Errorf("key = %q, want %q", systems[0].Key, want)
	}
}
