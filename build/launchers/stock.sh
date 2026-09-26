#!/bin/sh
# Jubilant Potato for Anbernic stock Linux: copy to Roms/APPS/ on either card.

cd "$(dirname "$(readlink -f "$0")")/JubilantPotato" || exit 1

export SDL_NOMOUSE=1
# Prefer the vendor SDL (2.0.12, mali video driver) over the generic
# Ubuntu one in /usr/lib/aarch64-linux-gnu.
export LD_LIBRARY_PATH="/usr/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"

./potato >potato.out 2>&1
