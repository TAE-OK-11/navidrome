# syntax=docker/dockerfile:1.25

########################################################################################################################
### Build Navidrome UI on Debian 13 slim (Trixie)
FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/node:24-trixie-slim AS ui

ARG BUILDARCH

ENV npm_config_audit=false \
    npm_config_fund=false \
    npm_config_update_notifier=false

WORKDIR /workspace/ui

RUN --mount=type=cache,id=jbs-apt-ui-cache-${BUILDARCH},target=/var/cache/apt,sharing=locked \
    --mount=type=cache,id=jbs-apt-ui-lists-${BUILDARCH},target=/var/lib/apt/lists,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
      ca-certificates \
      git

COPY --link ui/package.json ui/package-lock.json ./
COPY --link ui/bin/ ./bin/

RUN --mount=type=cache,id=jbs-npm-${BUILDARCH},target=/root/.npm,sharing=locked \
    npm ci --prefer-offline --no-audit --no-fund

COPY --link ui/ ./
RUN --mount=type=cache,id=jbs-ui-build-${BUILDARCH},target=/workspace/ui/node_modules/.cache,sharing=locked \
    npm run build -- --outDir=/out/ui

########################################################################################################################
### Build Navidrome Linux binary on Debian 13 slim (Trixie / glibc)
FROM public.ecr.aws/docker/library/debian:13-slim AS build

ARG GO_VERSION=1.26.5
ARG GO_SHA256_AMD64=5c2c3b16caefa1d968a94c1daca04a7ca301a496d9b086e17ad77bb81393f053
ARG GO_SHA256_ARM64=fe4789e92b1f33358680864bbe8704289e7bb5fc207d80623c308935bd696d49
ARG TARGETARCH
ARG GIT_SHA=unknown
ARG GIT_TAG=dev

ENV GOPATH=/go \
    GOMODCACHE=/go/pkg/mod \
    GOCACHE=/root/.cache/go-build \
    GOTOOLCHAIN=local \
    GOFLAGS=-mod=readonly \
    CGO_CFLAGS="-O3 -pipe -fno-plt -ffunction-sections -fdata-sections" \
    CGO_CXXFLAGS="-O3 -pipe -fno-plt -ffunction-sections -fdata-sections" \
    PATH="/usr/local/go/bin:/go/bin:${PATH}"

WORKDIR /workspace

RUN --mount=type=cache,id=jbs-apt-go-cache-${TARGETARCH},target=/var/cache/apt,sharing=locked \
    --mount=type=cache,id=jbs-apt-go-lists-${TARGETARCH},target=/var/lib/apt/lists,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
      build-essential \
      ca-certificates \
      curl \
      file \
      git \
      libwebp-dev \
      pkg-config \
      tar \
      zlib1g-dev \
    && case "${TARGETARCH}" in \
         amd64) go_sha256="${GO_SHA256_AMD64}" ;; \
         arm64) go_sha256="${GO_SHA256_ARM64}" ;; \
         *) echo "Unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
       esac \
    && curl -fsSLo /tmp/go.tar.gz "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" \
    && echo "${go_sha256}  /tmp/go.tar.gz" | sha256sum -c - \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm -f /tmp/go.tar.gz \
    && go version

COPY --link go.mod go.sum ./
RUN --mount=type=cache,id=jbs-go-mod-${TARGETARCH},target=/go/pkg/mod,sharing=locked \
    go mod download

# Keep frontend-only edits from invalidating the Go source layer. The compiled UI is added separately below.
COPY --link --exclude=ui --exclude=.git . ./
COPY --link --from=ui /out/ui ./ui/build

RUN --mount=type=cache,id=jbs-go-build-${TARGETARCH},target=/root/.cache/go-build,sharing=locked \
    --mount=type=cache,id=jbs-go-mod-${TARGETARCH},target=/go/pkg/mod,sharing=locked \
    --mount=type=tmpfs,target=/tmp <<'EOF'
set -eux
mkdir -p /out
BUILD_TAGS="$(./release/build-tags.sh)"
CGO_ENABLED=1 go build \
  -p "$(nproc)" \
  -trimpath \
  -buildvcs=false \
  -tags="${BUILD_TAGS}" \
  -ldflags="-w -s -linkmode=external -extldflags '-Wl,-O2,--as-needed,--gc-sections,-z,relro,-z,now' \
    -X github.com/navidrome/navidrome/consts.gitSha=${GIT_SHA} \
    -X github.com/navidrome/navidrome/consts.gitTag=${GIT_TAG}" \
  -o /out/navidrome .

./release/verify-binary.sh /out/navidrome
file /out/navidrome
test "$(ldd /out/navidrome | grep -c 'not found' || true)" -eq 0
EOF

########################################################################################################################
### Standalone Debian 13 binary output
FROM scratch AS binary
COPY --link --from=build /out/navidrome /navidrome

########################################################################################################################
### Debian 13 slim runtime image
FROM public.ecr.aws/docker/library/debian:13-slim AS final

ARG TARGETARCH
ARG GIT_SHA=unknown
ARG GIT_TAG=dev

LABEL org.opencontainers.image.title="JBS Navidrome" \
      org.opencontainers.image.description="JBS Networks custom Navidrome on Debian 13 slim" \
      org.opencontainers.image.vendor="JBS Networks" \
      org.opencontainers.image.source="https://github.com/TAE-OK-11/navidrome" \
      org.opencontainers.image.revision="${GIT_SHA}" \
      org.opencontainers.image.version="${GIT_TAG}"

RUN --mount=type=cache,id=jbs-apt-runtime-cache-${TARGETARCH},target=/var/cache/apt,sharing=locked \
    --mount=type=cache,id=jbs-apt-runtime-lists-${TARGETARCH},target=/var/lib/apt/lists,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
      ca-certificates \
      ffmpeg \
      libwebp7 \
      libwebpdemux2 \
      libwebpmux3 \
      mpv \
      sqlite3 \
      tzdata \
    && for lib in libwebp libwebpdemux libwebpmux; do \
         target="$(find /usr/lib -name "${lib}.so.*" -print -quit)"; \
         if [ -n "${target}" ]; then \
           ln -sf "$(basename "${target}")" "$(dirname "${target}")/${lib}.so"; \
         fi; \
       done \
    && rm -rf /tmp/* /var/tmp/*

COPY --link --from=build /out/navidrome /app/navidrome

VOLUME ["/data", "/music"]

ENV ND_MUSICFOLDER=/music \
    ND_DATAFOLDER=/data \
    ND_CONFIGFILE=/data/navidrome.toml \
    ND_PORT=4533 \
    PATH="/app:${PATH}"

RUN touch /.nddockerenv

EXPOSE 4533
WORKDIR /app
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/navidrome"]
