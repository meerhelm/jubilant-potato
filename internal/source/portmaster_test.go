package source

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/platform"
)

const pmTestIndex = `{
 "ports": {
  "2048.zip": {"version": 2, "name": "2048.zip", "items": ["2048.sh", "2048/"], "items_opt": null,
   "attr": {"title": "2048", "porter": "Christian", "desc": "The 2048 puzzle game", "inst": "Ready to run.",
    "genres": ["puzzle"], "image": {"screenshot": "screenshot.jpg", "covers": []},
    "rtr": true, "runtime": [], "reqs": [], "arch": ["aarch64", "armhf"], "exp": false},
   "source": {"md5": "abc", "size": 100, "url": "%[1]s/2048.zip"}},
  "axiom.verge.zip": {"version": 2, "name": "axiom.verge.zip", "items": ["axiom-verge/", "Axiom Verge.sh"],
   "attr": {"title": "Axiom Verge", "porter": ["a", "b"], "desc": "Metroidvania.", "inst": "Copy the game files.",
    "genres": ["action"], "image": {"screenshot": "screenshot.jpg", "covers": ["cover.png"]},
    "rtr": false, "runtime": ["frt_3.5.2"], "reqs": [], "arch": [], "exp": false},
   "source": {"md5": "def", "size": 200, "url": "%[1]s/axiom.verge.zip"}},
  "x86only.zip": {"name": "x86only.zip", "items": ["x/"], "attr": {"title": "X", "rtr": true, "arch": ["x86_64"]},
   "source": {"url": "%[1]s/x86only.zip"}},
  "norocknix.zip": {"name": "norocknix.zip", "items": ["n/"], "attr": {"title": "N", "rtr": true, "reqs": ["!rocknix"]},
   "source": {"url": "%[1]s/norocknix.zip"}},
  "beta.zip": {"name": "beta.zip", "items": ["b/"], "attr": {"title": "B", "rtr": true, "exp": true},
   "source": {"url": "%[1]s/beta.zip"}}
 },
 "utils": {
  "frt_3.5.2.squashfs": {"name": "Godot/FRT 3.5.2", "runtime_name": "frt_3.5.2.squashfs", "runtime_arch": "aarch64",
   "md5": "r1", "size": 7, "url": "%[1]s/frt.aarch64"},
  "frt_3.5.2.armhf.squashfs": {"name": "Godot/FRT 3.5.2", "runtime_name": "frt_3.5.2.squashfs", "runtime_arch": "armhf",
   "md5": "r2", "size": 8, "url": "%[1]s/frt.armhf"}
 }
}`

func TestPortMasterGames(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ports.json":
			fmt.Fprintf(w, pmTestIndex, srv.URL)
		case "/featured_ports.json":
			fmt.Fprint(w, `[{"name": "Old", "deprecated": true, "ports": ["2048.zip"]},
				{"name": "Premium", "type": "category", "children": [{"name": "Metroidvania", "ports": ["axiom.verge.zip", "gone.zip"]}]}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	oldIndex, oldFeatured := portMasterIndex, portMasterFeatured
	portMasterIndex, portMasterFeatured = srv.URL+"/ports.json", srv.URL+"/featured_ports.json"
	defer func() { portMasterIndex, portMasterFeatured = oldIndex, oldFeatured }()

	dirs := platform.PortMaster{Tools: "/t", Ports: "/p", Scripts: "/s"}
	src := newPortMaster(config.Source{Name: "PortMaster"}, srv.Client(), dirs, "rocknix", "")
	src.arch = "aarch64"
	systems, err := src.Systems(context.Background())
	if err != nil || len(systems) != 3 {
		t.Fatalf("systems: %v %v", systems, err)
	}
	titles := func(sys System) []string {
		games, err := src.Games(context.Background(), sys)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, g := range games {
			out = append(out, g.Title)
		}
		return out
	}
	for i, want := range [][]string{{"Axiom Verge"}, {"2048", "Axiom Verge"}, {"2048"}} {
		if got := titles(systems[i]); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, want %v", systems[i].Label, got, want)
		}
	}

	all, _ := src.Games(context.Background(), systems[1])
	if g := all[0]; g.File != "2048.zip" || g.MD5 != "abc" || g.Install == nil || g.Needs != nil {
		t.Errorf("2048: %+v", g)
	}
	g := all[1]
	if g.File != "axiom-verge.zip" {
		t.Errorf("file = %q, want the port's folder", g.File)
	}
	want := []Game{{Name: "Godot/FRT 3.5.2", File: "frt_3.5.2.squashfs", Size: 7, URL: srv.URL + "/frt.aarch64", MD5: "r1", Dir: "/t/PortMaster/libs"}}
	if !reflect.DeepEqual(g.Needs, want) {
		t.Errorf("needs = %+v, want %+v", g.Needs, want)
	}

	d, err := src.Details(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	img := portMasterImages + "axiom.verge/"
	if d.Author != "a, b" || d.Description != "Metroidvania.\n\nCopy the game files." ||
		!reflect.DeepEqual(d.Images, []string{img + "screenshot.jpg", img + "cover.png"}) {
		t.Errorf("details = %+v", d)
	}

	if _, err := newPortMaster(config.Source{}, nil, platform.PortMaster{}, "stock", "").Systems(context.Background()); err != ErrNoPortMaster {
		t.Errorf("without PortMaster: %v", err)
	}
}

func TestInstallPort(t *testing.T) {
	root := t.TempDir()
	dirs := platform.PortMaster{Tools: root, Ports: filepath.Join(root, "ports"), Scripts: filepath.Join(root, "scripts")}
	archive := filepath.Join(root, "port.zip")
	f, _ := os.Create(archive)
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{
		"Balatro.sh":         "#!/bin/bash\n# PORTMASTER: old.zip, Balatro.sh\necho hi\n",
		"balatro/":           "",
		"balatro/bin/game":   "ELF",
		"balatro/port.json":  "{}",
		"screenshot.jpg":     "JPG",
		"balatro/README.txt": "read me",
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	f.Close()

	idx, err := parsePortMasterIndex([]byte(`{"ports": {"balatro.zip": {"name": "balatro.zip", "items": ["Balatro.sh", "balatro/"],
		"attr": {"title": "Balatro", "runtime": []}, "source": {"md5": "m", "url": "u"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := installPort(archive, dirs, idx.ports["balatro.zip"]); err != nil {
		t.Fatal(err)
	}

	script, _ := os.ReadFile(filepath.Join(dirs.Scripts, "Balatro.sh"))
	if string(script) != "#!/bin/bash\n# PORTMASTER: balatro.zip, Balatro.sh\necho hi\n" {
		t.Errorf("script = %q", script)
	}
	if st, err := os.Stat(filepath.Join(dirs.Ports, "balatro/bin/game")); err != nil || st.Mode().Perm()&0o111 == 0 {
		t.Errorf("binary not executable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dirs.Ports, "balatro/screenshot.jpg")); err != nil {
		t.Errorf("top-level image not moved into the port folder: %v", err)
	}
	var info struct {
		Name   string            `json:"name"`
		Status map[string]string `json:"status"`
		Files  map[string]string `json:"files"`
		Source any               `json:"source"`
	}
	b, _ := os.ReadFile(filepath.Join(dirs.Ports, "balatro/port.json"))
	if err := json.Unmarshal(b, &info); err != nil {
		t.Fatal(err)
	}
	wantFiles := map[string]string{"port.json": "balatro/port.json", "Balatro.sh": "Balatro.sh", "balatro/": "balatro/"}
	if info.Name != "balatro.zip" || info.Status["status"] != "Installed" || info.Source != nil || !reflect.DeepEqual(info.Files, wantFiles) {
		t.Errorf("port.json = %s", b)
	}

	// A zip entry must not escape the ports folder.
	f, _ = os.Create(archive)
	zw = zip.NewWriter(f)
	w, _ := zw.Create("../evil/x")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	if err := installPort(archive, dirs, idx.ports["balatro.zip"]); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("escaping entry: %v", err)
	}
}
