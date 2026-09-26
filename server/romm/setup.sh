#!/bin/sh
# Generates secrets on first run and starts RomM.
set -eu
cd "$(dirname "$0")"

if [ ! -f .env ]; then
	cat >.env <<ENV
DB_PASSWD=$(openssl rand -hex 16)
DB_ROOT_PASSWD=$(openssl rand -hex 16)
ROMM_AUTH_SECRET_KEY=$(openssl rand -hex 32)
# Optional metadata providers, see https://docs.romm.app/latest/Getting-Started/Metadata-Providers/
SCREENSCRAPER_USER=
SCREENSCRAPER_PASSWORD=
STEAMGRIDDB_API_KEY=
RETROACHIEVEMENTS_API_KEY=
ENV
	echo "created .env with fresh secrets"
fi
mkdir -p library/roms assets config
docker compose up -d
echo "RomM: http://localhost:8080"
