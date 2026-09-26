#!/bin/sh
# Builds install packages for each firmware from dist/arm64/potato.
set -eu

cd "$(dirname "$0")/.."
VERSION="${VERSION:-dev}"
BIN=dist/arm64/potato
L=build/launchers
OUT="$PWD/dist"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

[ -x "$BIN" ] || { echo "missing $BIN, run: make build-arm64" >&2; exit 1; }

# Licenses of everything linked into the binary ship with every package.
./tools/third-party-licenses.sh >"$STAGE/THIRD_PARTY_LICENSES.txt"
EXTRA="config.example.json LICENSE $STAGE/THIRD_PARTY_LICENSES.txt"
# Asset names carry no version so releases/latest/download links stay valid.
rm -f "$OUT"/JubilantPotato-*

# muOS: .muxapp is a plain zip extracted into MUOS/application.
app="$STAGE/muos/Jubilant Potato"
mkdir -p "$app"
cp "$BIN" $EXTRA "$app/"
cp "$L/mux_launch.sh" "$app/"
chmod +x "$app/potato" "$app/mux_launch.sh"
(cd "$STAGE/muos" && zip -qr0 "$OUT/JubilantPotato-muos.muxapp" "Jubilant Potato")

# ROCKNIX: extract into /storage/roms.
mkdir -p "$STAGE/rocknix/ports/jubilantpotato"
cp "$BIN" $EXTRA "$STAGE/rocknix/ports/jubilantpotato/"
cp "$L/rocknix.sh" "$STAGE/rocknix/ports/JubilantPotato.sh"
chmod +x "$STAGE/rocknix/ports/JubilantPotato.sh" "$STAGE/rocknix/ports/jubilantpotato/potato"
(cd "$STAGE/rocknix" && zip -qr "$OUT/JubilantPotato-rocknix.zip" ports)

# Stock Anbernic: extract into Roms/ on SD1 or SD2.
mkdir -p "$STAGE/stock/APPS/JubilantPotato"
cp "$BIN" $EXTRA "$STAGE/stock/APPS/JubilantPotato/"
cp "$L/stock.sh" "$STAGE/stock/APPS/JubilantPotato.sh"
chmod +x "$STAGE/stock/APPS/JubilantPotato.sh" "$STAGE/stock/APPS/JubilantPotato/potato"
(cd "$STAGE/stock" && zip -qr "$OUT/JubilantPotato-stock.zip" APPS)

echo "version $VERSION:"
ls -l "$OUT"/JubilantPotato-*
