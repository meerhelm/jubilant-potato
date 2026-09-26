BUILDER := jubilant-potato-builder:arm64
DOCKER_RUN := docker run --rm --platform linux/arm64 -v "$(CURDIR)":/src -v potato-gocache:/cache $(BUILDER)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: run test build-arm64 builder package sort clean

# Desktop preview at RG34XX resolution; config and ROMs live in ./dev.
run:
	POTATO_HOME=$(CURDIR)/dev go run ./cmd/potato -window 720x480

test:
	go vet ./...
	go test -race ./...

builder:
	docker build --platform linux/arm64 -t $(BUILDER) build

build-arm64: builder
	$(DOCKER_RUN) go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o dist/arm64/potato ./cmd/potato

package: build-arm64
	VERSION=$(VERSION) ./build/package.sh

# Verify and file dumps into the RomM library: make sort INPUT=~/Downloads/ROMs
# (see tools/sort-roms.sh for DATS, SINGLE, RETAIL, REGIONS, LANGS).
sort:
	@test -n "$(INPUT)" || { echo "usage: make sort INPUT=<dir>"; exit 2; }
	./tools/sort-roms.sh "$(INPUT)"

clean:
	rm -rf bin dist
