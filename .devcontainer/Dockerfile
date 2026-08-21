# Development container for Navidrome

ARG VARIANT="1"
FROM mcr.microsoft.com/vscode/devcontainers/go:${VARIANT}

ARG BUN_VERSION="canary"

RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends ca-certificates curl ffmpeg unzip \
    && rm -rf /var/lib/apt/lists/*

RUN su vscode -c 'curl -fsSL https://bun.com/install | bash -s "${BUN_VERSION}"'
ENV PATH="/home/vscode/.bun/bin:${PATH}"
