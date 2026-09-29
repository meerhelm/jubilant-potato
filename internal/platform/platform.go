package platform

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Firmware identifies the OS running on the handheld.
type Firmware string

const (
	Stock   Firmware = "stock"   // Anbernic stock Linux
	MuOS    Firmware = "muos"    // muOS (MustardOS)
	Rocknix Firmware = "rocknix" // ROCKNIX
	Desktop Firmware = "desktop" // development machine
)

// Platform is the runtime environment the app has detected.
type Platform struct {
	Firmware Firmware
	Device   string // human-readable device name when known
	RomRoot  string
	Language string // firmware UI language as an ISO 639-1 code, "" when unknown

	existing   map[string]string // system ID -> folder already present in RomRoot
	portMaster PortMaster
}

// PortMaster holds PortMaster's folders as its harbourmaster resolves them:
// Tools contains PortMaster itself (runtimes live in Tools/PortMaster/libs),
// Ports the port folders and Scripts their launch scripts. All are empty
// when PortMaster isn't installed.
type PortMaster struct {
	Tools, Ports, Scripts string
}

// Detect inspects the filesystem to find out which firmware we are on.
// romRootOverride, when non-empty, wins over the detected ROM root.
func Detect(appDir, romRootOverride string) Platform {
	p := Platform{Firmware: detectFirmware()}
	p.Device = detectDevice(p.Firmware)
	p.Language = detectLanguage(p.Firmware)
	p.RomRoot = romRootOverride
	if p.RomRoot == "" {
		p.RomRoot = defaultRomRoot(p.Firmware, appDir)
	}
	p.existing = scanExisting(p.Firmware, p.RomRoot)
	p.portMaster = detectPortMaster(p.Firmware, p.RomRoot)
	return p
}

// PortMaster returns PortMaster's folders, or zero values without it.
func (p Platform) PortMaster() PortMaster { return p.portMaster }

func detectPortMaster(fw Firmware, romRoot string) PortMaster {
	var pm PortMaster
	switch fw {
	case Rocknix:
		pm = PortMaster{"/storage/roms/ports", "/storage/roms/ports", "/storage/roms/ports"}
		if exists("/storage/roms/ports_scripts") {
			pm.Scripts = "/storage/roms/ports_scripts"
		}
	case MuOS:
		pm = PortMaster{"/mnt/mmc/MUOS", "/mnt/mmc/ports", "/mnt/mmc/ROMS/Ports"}
		// Ports go to SD2 when a card is there, unless PortMaster was
		// told to keep them on SD1.
		if !exists("/mnt/mmc/MUOS/PortMaster/config/muos_mmc_master_race.txt") && isMountPoint("/mnt/sdcard") {
			pm.Ports, pm.Scripts = "/mnt/sdcard/ports", "/mnt/sdcard/ROMS/Ports"
		}
	case Desktop:
		d := filepath.Join(romRoot, "ports")
		return PortMaster{d, d, d}
	default:
		return PortMaster{}
	}
	if !exists(filepath.Join(pm.Tools, "PortMaster")) {
		return PortMaster{}
	}
	return pm
}

func detectFirmware() Firmware {
	switch {
	case exists("/opt/muos"):
		return MuOS
	case osRelease("OS_NAME") == "ROCKNIX":
		return Rocknix
	case exists("/mnt/vendor/oem/board.ini"), exists("/mnt/mmc/Roms"):
		return Stock
	}
	return Desktop
}

// osRelease returns a key from /etc/os-release.
func osRelease(key string) string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), key+"="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

func detectDevice(fw Firmware) string {
	var paths []string
	switch fw {
	case MuOS:
		paths = append(paths, "/opt/muos/device/config/board/name")
	case Stock:
		paths = append(paths, "/mnt/vendor/oem/board.ini")
	}
	paths = append(paths, "/sys/firmware/devicetree/base/model", "/proc/device-tree/model")
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			line, _, _ := strings.Cut(string(b), "\n")
			if line = strings.TrimRight(line, "\x00\r "); line != "" {
				return line
			}
		}
	}
	return ""
}

// detectLanguage reads the firmware's own UI language setting. Stock
// Anbernic exports LANG=zh_CN whatever the menu language, so it's skipped.
func detectLanguage(fw Firmware) string {
	switch fw {
	case Rocknix:
		b, _ := os.ReadFile("/storage/.config/system/configs/system.cfg")
		return rocknixLanguage(string(b))
	case MuOS:
		b, _ := os.ReadFile("/opt/muos/config/settings/general/language")
		return muosLanguage(string(b))
	}
	return ""
}

// rocknixLanguage returns the locale from a system.cfg line such as
// "system.language=ru_RU".
func rocknixLanguage(cfg string) string {
	for _, line := range strings.Split(cfg, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "system.language="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// muosLanguages maps muOS language names to locale codes.
var muosLanguages = map[string]string{
	"English": "en", "English (American)": "en", "Russian": "ru", "Ukrainian": "uk",
	"Polish": "pl", "Spanish": "es", "Portuguese (BR)": "pt_BR", "Portuguese (PT)": "pt_PT",
	"Chinese (Simplified)": "zh_CN", "Chinese (Traditional)": "zh_TW",
}

// muosLanguage maps a muOS language name ("Russian", "Chinese (Traditional)")
// to a locale code, or "" for languages without a mapping.
func muosLanguage(name string) string {
	return muosLanguages[strings.TrimSpace(name)]
}

func defaultRomRoot(fw Firmware, appDir string) string {
	// SD2 is preferred when a card is mounted there: that's where people
	// keep their library on two-slot devices.
	var sd2, sd1 string
	switch fw {
	case MuOS:
		sd2, sd1 = "/mnt/sdcard/ROMS", "/mnt/mmc/ROMS"
	case Stock:
		sd2, sd1 = "/mnt/sdcard/Roms", "/mnt/mmc/Roms"
	case Rocknix:
		// Merged view of internal and SD2 storage.
		return "/storage/roms"
	default:
		return filepath.Join(appDir, "roms")
	}
	if isMountPoint("/mnt/sdcard") && exists(sd2) {
		return sd2
	}
	return sd1
}

// folders maps system IDs to ROM folder names for each firmware.
var folders = map[Firmware]map[string]string{
	Stock: {
		"nes": "FC", "snes": "SFC", "gb": "GB", "gbc": "GBC", "gba": "GBA", "nds": "NDS",
		"n64": "N64", "md": "MD", "sms": "SMS", "gg": "GG", "segacd": "MDCD", "32x": "SEGA32X",
		"saturn": "SATURN", "dc": "DREAMCAST", "psx": "PS", "psp": "PSP", "pce": "PCE",
		"atari2600": "A2600", "neogeo": "NEOGEO", "fbneo": "FBNEO", "mame": "MAME",
		"pico8": "PICO",
	},
	// muOS folder names are free-form; these are recognised by its
	// auto-assign. MAME has no auto-assign entry, so arcade goes to ARCADE.
	MuOS: {
		"nes": "NES", "snes": "SNES", "gb": "GB", "gbc": "GBC", "gba": "GBA", "nds": "NDS",
		"n64": "N64", "md": "MD", "sms": "SMS", "gg": "GG", "segacd": "SEGACD", "32x": "32X",
		"saturn": "SATURN", "dc": "DC", "psx": "PS", "psp": "PSP", "pce": "PCE",
		"atari2600": "ATARI2600", "neogeo": "NEOGEO", "fbneo": "ARCADE", "mame": "ARCADE",
		"pico8": "PICO8",
	},
	Rocknix: {
		"nes": "nes", "snes": "snes", "gb": "gb", "gbc": "gbc", "gba": "gba", "nds": "nds",
		"n64": "n64", "md": "genesis", "sms": "mastersystem", "gg": "gamegear", "segacd": "segacd",
		"32x": "sega32x", "saturn": "saturn", "dc": "dreamcast", "psx": "psx", "psp": "psp",
		"pce": "pcengine", "atari2600": "atari2600", "neogeo": "neogeo", "fbneo": "fbneo", "mame": "mame",
		"pico8": "pico-8",
	},
}

// scanExisting finds ROM folders the user already has, so downloads land
// next to their games even when the folder isn't named our default way.
// Only muOS allows free-form names; elsewhere the firmware name must match
// exactly apart from case.
func scanExisting(fw Firmware, root string) map[string]string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		for id, name := range folders[fw] {
			if strings.EqualFold(e.Name(), name) {
				out[id] = e.Name()
			}
		}
		if fw == MuOS {
			if sys, ok := MatchSystem(e.Name()); ok && out[sys.ID] == "" {
				out[sys.ID] = e.Name()
			}
		}
	}
	return out
}

// SystemDir returns the directory where ROMs for a system are stored.
func (p Platform) SystemDir(systemID string) string {
	if systemID == "ports" && p.portMaster.Ports != "" {
		return p.portMaster.Ports
	}
	if dir, ok := p.existing[systemID]; ok {
		return filepath.Join(p.RomRoot, dir)
	}
	if dir, ok := folders[p.Firmware][systemID]; ok {
		return filepath.Join(p.RomRoot, dir)
	}
	return filepath.Join(p.RomRoot, systemID)
}

// DefaultJoystickButtons maps raw joystick buttons to action names for pads
// SDL has no controller mapping for. Stock firmware ships no mapping DB;
// these are the ANBERNIC-keys indices under its SDL 2.0.12.
func (p Platform) DefaultJoystickButtons() map[string]string {
	if p.Firmware != Stock {
		return nil
	}
	return map[string]string{
		"0": "a", "1": "b", "2": "y", "3": "x", "4": "l1", "5": "r1",
		"6": "select", "7": "start", "13": "menu",
	}
}

// FrontendInputConfig is the frontend's controller config, which names
// buttons after their printed labels, or "" when there is none to follow.
func (p Platform) FrontendInputConfig() string {
	if p.Firmware == Rocknix {
		return "/storage/.config/emulationstation/es_input.cfg"
	}
	return ""
}

// FreeBytes returns free space on the filesystem holding dir.
func FreeBytes(dir string) (uint64, error) {
	for {
		var st syscall.Statfs_t
		err := syscall.Statfs(dir, &st)
		if err == nil {
			return st.Bavail * uint64(st.Bsize), nil
		}
		parent := filepath.Dir(dir)
		if !os.IsNotExist(err) || parent == dir {
			return 0, err
		}
		dir = parent
	}
}

func isMountPoint(dir string) bool {
	b, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if f := strings.Fields(line); len(f) > 1 && f[1] == dir {
			return true
		}
	}
	return false
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
