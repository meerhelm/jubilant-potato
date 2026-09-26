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

# muOS: .muxapp is a plain zip extracted into MUOS/application.
app="$STAGE/muos/Jubilant Potato"
mkdir -p "$app"
cp "$BIN" config.example.json "$app/"
cp "$L/mux_launch.sh" "$app/"
chmod +x "$app/potato" "$app/mux_launch.sh"
(cd "$STAGE/muos" && zip -qr0 "$OUT/JubilantPotato-muos-$VERSION.muxapp" "Jubilant Potato")

# ROCKNIX: extract into /storage/roms.
mkdir -p "$STAGE/rocknix/ports/jubilantpotato"
cp "$BIN" config.example.json "$STAGE/rocknix/ports/jubilantpotato/"
cp "$L/rocknix.sh" "$STAGE/rocknix/ports/JubilantPotato.sh"
chmod +x "$STAGE/rocknix/ports/JubilantPotato.sh" "$STAGE/rocknix/ports/jubilantpotato/potato"
(cd "$STAGE/rocknix" && zip -qr "$OUT/JubilantPotato-rocknix-$VERSION.zip" ports)

# Stock Anbernic: extract into Roms/ on SD1 or SD2.
mkdir -p "$STAGE/stock/APPS/JubilantPotato"
cp "$BIN" config.example.json "$STAGE/stock/APPS/JubilantPotato/"
cp "$L/stock.sh" "$STAGE/stock/APPS/JubilantPotato.sh"
chmod +x "$STAGE/stock/APPS/JubilantPotato.sh" "$STAGE/stock/APPS/JubilantPotato/potato"
(cd "$STAGE/stock" && zip -qr "$OUT/JubilantPotato-stock-$VERSION.zip" APPS)

ls -l "$OUT"/JubilantPotato-*"$VERSION"*
