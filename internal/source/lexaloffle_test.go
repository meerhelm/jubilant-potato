package source

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/platform"
)

func TestLexaloffle(t *testing.T) {
	featured := "<script>\n\t\tp_sub = 2;\n\t\tpdat=[\n" +
		"\t['11722', 2145, `Celeste`,\"/bbs/thumbs/pico15133.png\",256,170.6,\"2015-07-21 06:06:51\",10070,\"noel\"],\n" +
		"\t\t['196890', 159507, `COPS &amp; ZOMBIES: 2?`,\"/bbs/thumbs/pico8_cyz2-0.png\",256,170.6,\"x\",1,\"a\"],\n" +
		"\t\t['196000', 159000, `No cart`,\"\",256,170.6,\"x\",1,\"a\"],\n" +
		"\t\t];\n\t\tvar updat=[\n\t];</script>"
	newest := "pdat=[\n" +
		"\t['196890', 159507, `COPS &amp; ZOMBIES: 2?`,\"/bbs/thumbs/pico8_cyz2-1.png\",256,170.6,\"x\",1,\"a\"],\n" +
		"\t\t['196995', 159548, `Celeste`,\"/bbs/thumbs/pico8_giraffing_around-0.png\",256,170.6,\"x\",1,\"a\"],\n" +
		"\t\t];"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/lister.php" || q.Get("cat") != "7" || q.Get("sub") != "2" || q.Get("mode") != "carts" {
			http.NotFound(w, r)
			return
		}
		switch {
		case q.Get("page") != "1":
			fmt.Fprint(w, "pdat=[\n\t\t];")
		case q.Get("orderby") == "featured":
			fmt.Fprint(w, featured)
		case q.Get("orderby") == "ts":
			fmt.Fprint(w, newest)
		}
	}))
	defer srv.Close()
	old := lexaloffleBBS
	lexaloffleBBS = srv.URL
	defer func() { lexaloffleBBS = old }()

	src, err := New(config.Source{Name: "PICO-8 BBS", Type: "pico8"}, srv.Client(), ClientInfo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	systems, _ := src.Systems(context.Background())
	if len(systems) != 1 || systems[0].ID != "pico8" {
		t.Fatalf("systems = %+v", systems)
	}
	games, err := src.Games(context.Background(), systems[0])
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ name, file, url string }{
		{"Celeste", "Celeste.p8.png", srv.URL + "/cposts/1/15133.p8.png"},
		{"Celeste", "Celeste giraffing_around-0.p8.png", srv.URL + "/cposts/gi/giraffing_around-0.p8.png"},
		{"COPS & ZOMBIES: 2?", "COPS & ZOMBIES 2.p8.png", srv.URL + "/cposts/cy/cyz2-0.p8.png"},
	}
	if len(games) != len(want) {
		t.Fatalf("got %d games: %+v", len(games), games)
	}
	pico8, _ := platform.SystemByID("pico8")
	for i, w := range want {
		g := games[i]
		if g.Name != w.name || g.File != w.file || g.URL != w.url || g.System != "pico8" {
			t.Errorf("game %d = %q %q %q, want %q %q %q", i, g.Name, g.File, g.URL, w.name, w.file, w.url)
		}
		if !pico8.Accepts(g.File) {
			t.Errorf("PICO-8 doesn't accept %q", g.File)
		}
	}
}

func TestPico8Thread(t *testing.T) {
	page := `<html><body>
<div><a href="/bbs/?tid=1">Giraffing Around</a><a href="/bbs/?uid=7"><b style="color:#fff">ginozump</b></a></div>
<div style="max-width:880px"><div style="min-height:44px;"><p>can you find the
   win condition?<br />
<div style="display:table"><div id="pgiraffing_around-0" class="playarea_0"><script>var x = 1;</script>
  <img src="/bbs/thumbs/pico8_giraffing_around-0.png"><a href="/bbs/cposts/gi/giraffing_around-0.p8.png">Cart</a> #giraffing_around-0</div>
  <div style="display:none" id="cartembed">Copy and paste the snippet</div></div>
<br><br />inspired by &quot;the&quot; demo<br />
<img src="/media/1/shot.gif"><img src="/gfx/set_like0.png"><img src="https://example.com/a.png"></p>
<h2>Controls</h2><ul><li>arrows: move</li></ul></div>
<div><a href="/bbs/?cat=7#tag=giraffe"><span class="tag">giraffe</span></a> <span class="tag">short</span></div></div>
<div id=p197007><a href="/bbs/?uid=9"><b>commenter</b></a><div style="min-height:44px;">nice</div></div>
</body></html>`
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	base, _ := url.Parse("https://www.lexaloffle.com/bbs/?tid=1")
	d := parsePico8Thread(doc, base)
	want := &Details{
		Author:      "ginozump",
		Description: "can you find the win condition?\n\ninspired by \"the\" demo\n\nControls\n\narrows: move",
		Tags:        []string{"giraffe", "short"},
		Images: []string{
			"https://www.lexaloffle.com/bbs/thumbs/pico8_giraffing_around-0.png",
			"https://www.lexaloffle.com/media/1/shot.gif",
			"https://example.com/a.png",
		},
	}
	if !reflect.DeepEqual(d, want) {
		t.Errorf("got  %#v\nwant %#v", d, want)
	}
}
