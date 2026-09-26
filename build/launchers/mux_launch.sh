#!/bin/sh
# HELP: Download ROMs from your own server or archive.org
# ICON: app
# GRID: Potato

. /opt/muos/script/var/func.sh

SETUP_STAGE_OVERLAY

APP_BIN="potato"
# Exports the SDL video and SDL_GAMECONTROLLERCONFIG for this device.
SETUP_APP "$APP_BIN" ""

cd "$1" || exit
./"$APP_BIN" >potato.out 2>&1
