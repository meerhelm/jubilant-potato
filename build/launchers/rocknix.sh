#!/bin/bash
# Jubilant Potato for ROCKNIX: copy to /storage/roms/ports/.

# SDL_VIDEODRIVER=wayland and SDL_GAMECONTROLLERCONFIG_FILE come from here.
. /etc/profile

cd "$(dirname "$(readlink -f "$0")")/jubilantpotato" || exit 1
./potato >potato.out 2>&1
