// Command potato is a gamepad-driven ROM downloader for Linux handhelds.
package main

import (
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/download"
	"github.com/meerhelm/jubilant-potato/internal/platform"
	"github.com/meerhelm/jubilant-potato/internal/source"
	"github.com/meerhelm/jubilant-potato/internal/ui"
)

// SDL must run on the main OS thread.
func init() { runtime.LockOSThread() }

func main() {
	window := flag.String("window", "", "run in a WxH window instead of fullscreen, e.g. 720x480")
	home := flag.String("home", os.Getenv("POTATO_HOME"), "app directory holding config.json (default: next to the binary)")
	flag.Parse()

	appDir := *home
	if appDir == "" {
		exe, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		appDir = filepath.Dir(exe)
	}
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		log.Fatal(err)
	}

	if f, err := os.Create(filepath.Join(appDir, "potato.log")); err == nil {
		defer f.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}

	cfg, err := config.Load(filepath.Join(appDir, "config.json"))
	if err != nil {
		log.Fatal(err)
	}
	plat := platform.Detect(appDir, cfg.RomRoot)
	log.Printf("firmware=%s device=%q roms=%s", plat.Firmware, plat.Device, plat.RomRoot)
	if len(cfg.JoystickButtons) == 0 {
		cfg.JoystickButtons = plat.DefaultJoystickButtons()
	}

	client := source.NewHTTPClient()
	var srcs []source.Source
	var notices []string
	for _, c := range cfg.Sources {
		if c.Disabled {
			continue
		}
		s, err := source.New(c, client)
		if err != nil {
			log.Print(err)
			notices = append(notices, err.Error())
			continue
		}
		srcs = append(srcs, s)
	}

	err = ui.Run(ui.Options{
		Config:   cfg,
		Platform: plat,
		Sources:  srcs,
		Manager:  download.NewManager(client),
		Window:   *window,
		Notices:  notices,
	})
	if err != nil {
		log.Fatal(err)
	}
}
