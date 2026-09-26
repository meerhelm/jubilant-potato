#!/bin/sh
# Verifies ROM dumps against No-Intro/Redump DATs with Igir (https://igir.io)
# and files them into the RomM library under canonical names:
#   <library>/roms/<romm platform>/<Game (Region) (Rev N)>.zip
#
# Usage: tools/sort-roms.sh INPUT_DIR
#
# Environment:
#   DATS     DAT files or glob (default: dats/*)   No-Intro: https://datomatic.no-intro.org
#                                                  Redump:   http://redump.org/downloads/
#   LIBRARY  RomM library root (default: server/romm/library)
#   MODE     copy | move (default: copy, the input is left untouched)
#   RETAIL=1 drop betas, demos, prototypes, BIOS and other non-retail dumps
#   SINGLE=1 keep one version per game ("1G1R"), chosen by REGIONS and LANGS
#   REGIONS  preferred regions (default: USA,WORLD,EUR,JPN)
#   LANGS    preferred languages (default: EN)
#
# Cartridge ROMs are zipped; disc images stay as they are and multi-disc
# games get an .m3u playlist. Files that match no DAT are left in the input
# and listed in the CSV report under reports/.
set -eu

[ $# -eq 1 ] || { sed -n '2,/^set -eu/p' "$0" | sed 's/^# \{0,1\}//' | sed '$d'; exit 2; }
INPUT=$1
DATS=${DATS:-dats/*}
LIBRARY=${LIBRARY:-server/romm/library}
MODE=${MODE:-copy}

case $MODE in copy | move) ;; *) echo "MODE must be copy or move" >&2; exit 2 ;; esac
[ -d "$INPUT" ] || { echo "no such directory: $INPUT" >&2; exit 1; }

mkdir -p reports "$LIBRARY/roms"
set -- "$MODE" zip test playlist report \
	--dat "$DATS" \
	--input "$INPUT" \
	--output "$LIBRARY/roms/{romm}" \
	--zip-exclude "**/*.{iso,bin,cue,chd,gdi,cdi,img,cso,pbp,m3u}" \
	--report-output "reports/igir-$(date +%Y%m%d-%H%M%S).csv"

if [ "${RETAIL:-}" = 1 ]; then
	set -- "$@" --only-retail --no-bios
fi
if [ "${SINGLE:-}" = 1 ]; then
	set -- "$@" --single --prefer-region "${REGIONS:-USA,WORLD,EUR,JPN}" --prefer-language "${LANGS:-EN}"
fi

exec npx --yes igir@latest "$@"
