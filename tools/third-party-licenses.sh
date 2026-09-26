#!/bin/sh
# Collects the licenses of every module linked into the device binary.
set -eu
cd "$(dirname "$0")/.."
MODCACHE=$(go env GOMODCACHE)
echo "Jubilant Potato includes the following third-party software."
printf '\n================================================================\nThe Go programming language (runtime and standard library)\n================================================================\n\n'
GOROOT=$(go env GOROOT)
# Homebrew keeps Go's LICENSE one level above GOROOT.
cat "$GOROOT/LICENSE" 2>/dev/null || cat "$GOROOT/../LICENSE"
GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}}{{end}}{{end}}' ./cmd/potato |
	sort -u | while read -r path version; do
		dir="$MODCACHE/$(echo "$path" | sed 's/[A-Z]/!&/g' | tr 'A-Z' 'a-z')@$version"
		lic=$(ls "$dir" 2>/dev/null | grep -iE '^(licen[cs]e|copying)' | head -1)
		printf '\n================================================================\n%s %s\n================================================================\n\n' "$path" "$version"
		if [ -n "$lic" ]; then cat "$dir/$lic"; else echo "(license file not found)"; fi
		if [ -f "$dir/NOTICE" ]; then
			echo
			cat "$dir/NOTICE"
		fi
	done
