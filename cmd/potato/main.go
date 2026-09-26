// Command potato is a gamepad-driven ROM downloader for Linux handhelds.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/download"
	"github.com/meerhelm/jubilant-potato/internal/platform"
	"github.com/meerhelm/jubilant-potato/internal/source"
	"github.com/meerhelm/jubilant-potato/internal/ui"
)

// Set at build time with -ldflags "-X main.version=... -X main.itchClientID=...".
var (
	version      = "dev"
	itchClientID = ""
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
	log.Printf("version=%s firmware=%s device=%q roms=%s", version, plat.Firmware, plat.Device, plat.RomRoot)

	client := source.NewHTTPClient()
	info := source.ClientInfo{
		DeviceID:     cfg.DeviceID,
		Name:         deviceName(plat),
		Platform:     string(plat.Firmware),
		Version:      version,
		ItchClientID: itchClientID,
	}
	if cfg.ItchClientID != "" {
		info.ItchClientID = cfg.ItchClientID
	}
	source.UserAgent = fmt.Sprintf("JubilantPotato/%s (+https://github.com/meerhelm/jubilant-potato; %s; %s; %s/%s)",
		version, plat.Firmware, info.Name, runtime.GOOS, runtime.GOARCH)
	cfgPath := filepath.Join(appDir, "config.json")
	var cfgMu sync.Mutex
	saveConfig := func() error {
		cfgMu.Lock()
		defer cfgMu.Unlock()
		return cfg.Save(cfgPath)
	}
	buildSources := func() (srcs []source.Source, notices []string) {
		for _, c := range cfg.Sources {
			if c.Disabled {
				continue
			}
			name := c.Name
			saveToken := func(tok string) error {
				cfgMu.Lock()
				defer cfgMu.Unlock()
				for i := range cfg.Sources {
					if cfg.Sources[i].Name == name {
						cfg.Sources[i].Token = tok
					}
				}
				return cfg.Save(cfgPath)
			}
			s, err := source.New(c, client, info, saveToken)
			if err != nil {
				log.Print(err)
				notices = append(notices, err.Error())
				continue
			}
			srcs = append(srcs, s)
		}
		return srcs, notices
	}
	srcs, notices := buildSources()

	restart, err := ui.Run(ui.Options{
		Config:       cfg,
		Platform:     plat,
		Sources:      srcs,
		Manager:      download.NewManager(client),
		HTTPClient:   client,
		Window:       *window,
		Version:      version,
		SaveConfig:   saveConfig,
		BuildSources: buildSources,
		Notices:      notices,
	})
	if err != nil {
		log.Fatal(err)
	}
	if restart {
		// Replace this process with the freshly installed binary, which
		// greets the user with the new version number.
		os.Setenv("POTATO_UPDATED_FROM", version)
		exe, err := os.Executable()
		if err == nil {
			err = syscall.Exec(exe, os.Args, os.Environ())
		}
		log.Fatalf("restart: %v", err)
	}
}

// deviceName is how the handheld shows up in a server's device list.
func deviceName(p platform.Platform) string {
	if p.Device != "" {
		return p.Device
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "Jubilant Potato"
}
