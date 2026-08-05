# syntax=docker/dockerfile:1.25

########################################################################################################################
### Build Navidrome UI on Debian 13 (Trixie)
FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/node:24-trixie-slim AS ui

WORKDIR /workspace/ui

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
      git \
    && rm -rf /var/lib/apt/lists/*

COPY ui/package.json ui/package-lock.json ./
COPY ui/bin/ ./bin/

RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund

COPY ui/ ./
RUN npm run build -- --outDir=/out/ui

########################################################################################################################
### Build Navidrome Linux binary on Debian 13 (Trixie / glibc)
FROM public.ecr.aws/docker/library/golang:1.26.5-trixie AS build

ARG GIT_SHA=unknown
ARG GIT_TAG=dev

WORKDIR /workspace

RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential \
      ca-certificates \
      file \
      git \
      libwebp-dev \
      pkg-config \
      zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . ./
COPY --from=ui /out/ui ./ui/build

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod <<'EOF'
set -eux
BUILD_TAGS="$(./release/build-tags.sh)"
CGO_ENABLED=1 go build \
  -trimpath \
  -buildvcs=false \
  -tags="${BUILD_TAGS}" \
  -ldflags="-w -s -linkmode=external -extldflags '-Wl,-z,relro,-z,now' \
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
COPY --from=build /out/navidrome /navidrome

########################################################################################################################
### Debian 13 slim runtime image
FROM public.ecr.aws/docker/library/debian:13-slim AS final

ARG GIT_SHA=unknown
ARG GIT_TAG=dev

LABEL org.opencontainers.image.title="JBS Navidrome" \
      org.opencontainers.image.description="JBS Networks custom Navidrome on Debian 13 slim" \
      org.opencontainers.image.vendor="JBS Networks" \
      org.opencontainers.image.source="https://github.com/TAE-OK-11/navidrome" \
      org.opencontainers.image.revision="${GIT_SHA}" \
      org.opencontainers.image.version="${GIT_TAG}"

RUN apt-get update && apt-get install -y --no-install-recommends \
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
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY --from=build /out/navidrome /app/navidrome

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
