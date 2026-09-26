# Jubilant Potato

A gamepad-driven ROM downloader for Linux handhelds. Browse a catalog on your
own server or on archive.org and download games straight into the right ROM
folder on the device.

Only download games you have the right to use: your own dumps, homebrew,
public-domain releases.

## Supported devices

| Device | Anbernic stock Linux | muOS | ROCKNIX |
|---|---|---|---|
| RG34XX / RG34XXSP (H700) | ✓ | ✓ | ✓ |
| RG DS (RK3568) | ✓ (untested) | — (not supported by muOS) | ✓ |

Any aarch64 device running one of these firmwares should work. The binary
needs only glibc ≥ 2.17 and the firmware's own `libSDL2`.

## Install

Grab the package for your firmware from `dist/` (see *Build*).

- **muOS**: copy `JubilantPotato-muos-*.muxapp` to `ARCHIVE/` on the SD card
  and install it with Archive Manager. It appears under Applications.
- **ROCKNIX**: unzip `JubilantPotato-rocknix-*.zip` into `/storage/roms/`
  (the `roms` share over SMB). It appears under Ports.
- **Stock**: unzip `JubilantPotato-stock-*.zip` into `Roms/` on either SD
  card. It appears in App Center → APPS.

On first launch the app creates an empty `config.json` next to the binary.

## Configuration

`config.json` lives in the app folder. See [config.example.json](config.example.json).

```json
{
  "language": "ru",
  "sources": [
    { "name": "Home server", "type": "http", "url": "http://192.168.1.10:8080/roms/" }
  ]
}
```

| Key | Meaning |
|---|---|
| `language` | `en` or `ru` (default: from `$LANG`, else English) |
| `rom_root` | override the detected ROM root |
| `swap_ab` | swap confirm/back if your pad mapping is positional |
| `joystick_buttons` | raw button index → action (`a`, `b`, `x`, `y`, `l1`, `r1`, `select`, `start`, `menu`) for pads SDL has no mapping for |
| `sources` | list of catalogs, see below |

### Source: `http`

Any web server with directory listings: nginx `autoindex on`, Apache,
Caddy `file_server browse`, or simply `python3 -m http.server` in your ROM
folder. Subfolders are matched to systems by name: `gba`, `GBA`,
`Nintendo - Game Boy Advance`, `SFC`, `megadrive`, and so on. To map folders
explicitly, use `systems`, where each system ID maps to one or more paths under
`url`. Basic auth is available through `username`/`password`.

### Source: `archive.org`

`systems` maps a system ID to one or more archive.org item identifiers. Every
original file in the item with a matching extension is listed.

System IDs: `nes snes gb gbc gba nds n64 md sms gg segacd 32x saturn dc psx
psp pce atari2600 neogeo fbneo mame`.

## Where games go

| Firmware | ROM root | Folders |
|---|---|---|
| muOS | `/mnt/sdcard/ROMS` if SD2 is mounted, else `/mnt/mmc/ROMS` | existing folders are reused by name (`Game Boy Advance`, `gba`, …); otherwise `GBA`, `SNES`, `PS`, `ARCADE`, … |
| ROCKNIX | `/storage/roms` | `gba`, `snes`, `genesis`, `psx`, … |
| Stock | `/mnt/sdcard/Roms` if SD2 is mounted, else `/mnt/mmc/Roms` | `GBA`, `SFC`, `FC`, `MD`, `PS`, `A2600`, … |

Disc-based systems and NDS `.zip` files are extracted after download; cartridge
ROMs are kept as downloaded. Interrupted downloads resume.

## Controls

| Button | Action |
|---|---|
| D-pad | move; left/right pages |
| A | open / download |
| B | back |
| Y | search (on-screen keyboard) |
| X | clear search |
| L1 / R1 | previous / next letter |
| Select | downloads |
| Menu | quit |

## Build

Requirements: Go, SDL2 (`brew install sdl2 pkg-config` on macOS), and Docker
for device builds.

```sh
make run          # desktop preview at 720x480, config and ROMs in ./dev
make test
make package      # arm64 binary via Docker + packages in dist/
```

Device binaries are built in a Debian bullseye container (glibc 2.31) and link
dynamically against the firmware's SDL2.

### Scripted runs

`POTATO_SCRIPT` replays button presses and saves screenshots, which is useful
for testing and bug reports:

```sh
SDL_VIDEODRIVER=dummy POTATO_SCRIPT="a,wait,shot:systems.png,quit" ./potato -window 640x480
```

Logs go to `potato.log` (app) and `potato.out` (launcher) in the app folder.
Set `POTATO_DEBUG=1` to also log every button press and the action it maps to,
which helps with `swap_ab` and `joystick_buttons` on unfamiliar pads.
