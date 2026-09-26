# Build environment for handheld binaries. Debian bullseye's glibc (2.31)
# is old enough to run on every supported firmware; the binary links only
# against glibc and the firmware's own libSDL2.
FROM debian:bullseye

# Bullseye has left the main mirrors; use the snapshot the image ships pinned to.
RUN sed -i -e 's|^deb http://deb.debian.org|# &|' -e 's|^# deb http://snapshot|deb http://snapshot|' /etc/apt/sources.list.d/* /etc/apt/sources.list 2>/dev/null; \
    apt-get -o Acquire::Check-Valid-Until=false update \
 && apt-get install -y --no-install-recommends ca-certificates curl gcc libc6-dev libsdl2-dev pkg-config \
 && rm -rf /var/lib/apt/lists/*

ARG GO_VERSION=1.27.1
ARG TARGETARCH
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" | tar -C /usr/local -xz
ENV PATH=/usr/local/go/bin:$PATH \
    GOCACHE=/cache/go-build \
    GOMODCACHE=/cache/gomod \
    CGO_ENABLED=1

WORKDIR /src
