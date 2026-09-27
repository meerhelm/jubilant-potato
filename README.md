# Jubilant Potato

[![Buy me a coffee](https://img.shields.io/badge/Buy%20me%20a%20coffee-f2b13b?logo=buymeacoffee&logoColor=1b1406)](https://buymeacoffee.com/f2mzlbyhvc)
[![Website](https://img.shields.io/badge/website-meerhelm.github.io-16171d)](https://meerhelm.github.io/jubilant-potato/)

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

Download the package for your firmware from the
[latest release](https://github.com/meerhelm/jubilant-potato/releases/latest),
or build it yourself (see *Build*).

- **muOS**: copy `JubilantPotato-muos.muxapp` to `ARCHIVE/` on the SD card
  and install it with Archive Manager. It appears under Applications.
- **ROCKNIX**: unzip `JubilantPotato-rocknix.zip` into `/storage/roms/`
  (the `roms` share over SMB). It appears under Ports.
- **Stock**: unzip `JubilantPotato-stock.zip` into `Roms/` on either SD
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
| `language` | `en`, `ru`, `uk`, `be`, `pl`, `es`, `pt`, `zh-Hans` or `zh-Hant`; also changeable in the app (default: the firmware's UI language on ROCKNIX and muOS, else `$LANG`, else English) |
| `rom_root` | override the detected ROM root |
| `swap_ab` | swap confirm/back if your pad mapping is positional |
| `disable_update_check` | don't check GitHub for new versions on startup |
| `prefer_regions` | variant preference, e.g. `["USA", "Europe", "Japan"]` |
| `prefer_languages` | variant preference, e.g. `["ru", "en"]` (default: UI language, then English) |
| `joystick_buttons` | raw button index → action (`a`, `b`, `x`, `y`, `l1`, `r1`, `select`, `start`, `menu`) for pads SDL has no mapping for |
| `sources` | list of catalogs, see below |

### Source: `http`

Any web server with directory listings: nginx `autoindex on`, Apache,
Caddy `file_server browse`, or simply `python3 -m http.server` in your ROM
folder. Subfolders are matched to systems by name: `gba`, `GBA`,
`Nintendo - Game Boy Advance`, `SFC`, `megadrive`, and so on. To map folders
explicitly, use `systems`, where each system ID maps to one or more paths under
`url`. Basic auth is available through `username`/`password`.

### Source: `romm`

A [RomM](https://romm.app) server (5.3+). Set only the URL:

```json
{ "name": "RomM", "type": "romm", "url": "http://192.168.1.10:8080" }
```

The first time you open it, the handheld shows a QR code and a short code.
Scan the code with your phone, or open the RomM page shown on screen, and
approve the device. The app stores the token in `config.json`. Revoking the
device in RomM makes it ask to pair again. Platforms are matched to ROM
folders by their RomM slug, and multi-file games arrive as a zip that gets
extracted.

To run RomM locally, use `./server/romm/setup.sh`: it generates secrets,
starts RomM 5.3.1 with MariaDB on port 8080, and serves the library from
`server/romm/library/roms/<platform>/`. Set `ROMM_LIBRARY` to point it at your
own library.

### Source: `archive.org`

`systems` maps a system ID to one or more archive.org item identifiers. Every
original file in the item with a matching extension is listed.

System IDs: `nes snes gb gbc gba nds n64 md sms gg segacd 32x saturn dc psx
psp pce atari2600 neogeo fbneo mame`.

On the device, **Add source → archive.org** searches item titles, detects
which systems an item contains (from folders, extensions and the title),
and adds the item under the system you pick.

### Source: `itch.io`

Free homebrew from itch.io tag feeds (`gameboy-rom`, `gbstudio`,
`gameboy-advance`, `nes-rom`, and others), listed without signing in.
Downloading goes through the official API and needs a sign-in:

- **QR login** (OAuth device flow). This needs an itch.io OAuth app
  approved for QR login. Register one at itch.io/user/settings/oauth-apps
  and ask itch.io support to enable "QR code login (device authorization
  grant)". Then build with `make package ITCH_CLIENT_ID=...` or set
  `"itch_client_id"` in `config.json`.
- **API key** as a fallback: create one at itch.io/user/settings/api-keys
  and put it into the source's `"token"`.

Paid games download only if you own them. The app identifies itself with
its own User-Agent and spaces out its requests, as itch.io asks.

### Source: `pico8`

PICO-8 carts from the Lexaloffle BBS: every featured cart plus the newest
releases of the Cartridges forum, no account needed. Carts download as
`.p8.png` files named after their title, the same files SPLORE fetches, into
`PICO8` (muOS), `pico-8` (ROCKNIX) or `PICO` (stock). **A** on a cart opens
its page: the label, pictures and description from its BBS thread.

ROCKNIX runs PICO-8 carts with the official PICO-8 by default, which you
have to buy and install yourself: copy `pico8_64` and `pico8.dat` from the
*Raspberry Pi* download into `roms/pico-8/aarch64/`. Without it, pick the
free fake-08 core for the system: *Game settings → Per system advanced
configuration → PICO-8 → Emulator → retroarch: fake08*.

```json
{"name": "PICO-8 BBS", "type": "pico8"}
```

### Source: `portmaster`

Ports of PC games from the catalog PortMaster itself installs from,
grouped as PortMaster's own menu does: featured ports, all ports, and
ports that are ready to run. Only ports built for the device's CPU and
allowed on its firmware are listed. Needs PortMaster (ROCKNIX, muOS).
The catalog is kept in `cache/portmaster` next to the app and downloaded
again only when it changes on GitHub (ETag); without a connection the
kept copy is used.

Ports are installed the way PortMaster does it, so it lists them as its
own: the launch script goes to `ports` (ROCKNIX) or `ROMS/Ports` (muOS),
the game folder next to PortMaster's ports, and the runtimes a port mounts
(Godot, Mono, Java, ...) into `PortMaster/libs` before the port itself.
Downloads are checked against the catalog's MD5. **A** on a port opens its
page: screenshot, description and what to copy from your own copy of the
game, for ports that aren't ready to run.

```json
{"name": "PortMaster", "type": "portmaster"}
```

## Game versions

Regional releases, revisions, betas, hacks and translations of a game appear
as a single row. File names are parsed in the No-Intro, Redump and GoodTools
conventions: `(USA)`, `(En,Fr)`, `(Rev 1)`, `(Beta)`, `[T+Rus]`, `[h1]`, `[b]`.
RomM's sibling links also merge regional titles such as *Mother 2* and
*EarthBound*.

**A** downloads the preferred version: a retail release in your preferred
language and region, at its latest revision. **X** lists every version.
**Start** toggles betas, demos, hacks and translations, which are hidden by
default.

## Organising a library

Big dump collections come with many variants and inconsistent names.
`make sort` verifies files against No-Intro (cartridges) and Redump (discs)
DATs with [Igir](https://igir.io), renames them canonically, and files them
into the RomM library by platform:

```sh
# put DAT files into ./dats first
make sort INPUT=~/Downloads/ROMs
SINGLE=1 RETAIL=1 REGIONS=USA,EUR,JPN make sort INPUT=~/Downloads/ROMs  # one version per game
```

Cartridge ROMs are zipped. Disc images are copied as they are and multi-disc
games get `.m3u` playlists; converting discs to CHD with `chdman` first saves
2–3× space. Files that match no DAT (hacks, translations, junk) stay in the
input folder and are listed in `reports/`. Then run a scan in RomM. Keeping
the full verified set on the server and choosing versions on the device
(see above) loses nothing.

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
| X | all versions of a game |
| Start | show/hide betas, demos and hacks |
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

## Support

Jubilant Potato is free. If it saves you time, you can
[buy me a coffee](https://buymeacoffee.com/f2mzlbyhvc): it pays for test
devices and support for more handhelds. The app has the same link as a QR
code under **Support the project**.

## License

Copyright © 2026 the Jubilant Potato authors.

Jubilant Potato is free software: you can redistribute it and/or modify it
under the terms of the GNU General Public License as published by the Free
Software Foundation, either version 3 of the License, or (at your option)
any later version. See [LICENSE](LICENSE).

Release packages also include `THIRD_PARTY_LICENSES.txt` for the libraries
compiled into the binary.
