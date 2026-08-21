#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BUN_VERSION = os.environ["BUN_VERSION"]
BUN_ALPINE_DIGEST = os.environ["BUN_ALPINE_DIGEST"]
BUN_DEBIAN_DIGEST = os.environ["BUN_DEBIAN_DIGEST"]


def read(path: str) -> str:
    return (ROOT / path).read_text()


def write(path: str, value: str) -> None:
    (ROOT / path).write_text(value)


def replace_required(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"expected pattern not found for {label}: {old!r}")
    return text.replace(old, new)


# ui/package.json: Bun becomes the runtime for project scripts. The exact canary
# revision is validated by CI while .bun-version deliberately tracks canary,
# because Bun's canary release tag is mutable and does not publish a semver tag
# for every revision.
pkg_path = ROOT / "ui/package.json"
pkg = json.loads(pkg_path.read_text())
pkg["scripts"]["build"] = replace_required(
    pkg["scripts"]["build"],
    "vite build && node bin/precompress-build-assets.mjs",
    "vite build && bun bin/precompress-build-assets.mjs",
    "ui build runtime",
)
pkg.pop("packageManager", None)
pkg_path.write_text(json.dumps(pkg, indent=2, ensure_ascii=False) + "\n")

# Root version channel used by Makefile and GitHub Actions. CI additionally
# asserts that the resolved binary is a 1.4.x canary before any build runs.
write(".bun-version", "canary\n")

# Makefile: remove npm/npx/node environment requirements.
make = read("Makefile")
make = replace_required(make, "NODE_VERSION=$(shell cat .nvmrc)", "BUN_CHANNEL=$(shell cat .bun-version)", "Makefile version pin")
make = make.replace("check_node_env", "check_bun_env")
make = make.replace("Downloading Node dependencies", "Downloading Bun dependencies")
make = make.replace("npm ci", "bun install --frozen-lockfile")
make = make.replace("npx foreman", "bunx foreman")
make = make.replace("npm run", "bun run")
check_pattern = re.compile(r"check_bun_env:\n.*?\.PHONY: check_bun_env", re.S)
check_replacement = r'''check_bun_env:
	@(hash bun) || (echo "\nERROR: Bun environment not setup properly!\n"; exit 1)
	@current_bun_version=`bun --version` && \
		case "$$current_bun_version" in 1.4.*) ;; *) \
			echo "\nERROR: Bun 1.4 is required (channel: $(BUN_CHANNEL)); got $$current_bun_version\n"; exit 1 ;; esac
.PHONY: check_bun_env'''
make, count = check_pattern.subn(check_replacement, make, count=1)
if count != 1:
    raise SystemExit("failed to rewrite Makefile check_bun_env target")
write("Makefile", make)

# Local development process runner.
proc = read("Procfile.dev")
proc = replace_required(proc, 'JS: sh -c "cd ./ui && npm start"', 'JS: sh -c "cd ./ui && bun run start"', "Procfile UI command")
write("Procfile.dev", proc)

# Worktree bootstrap.
worktree = read("scripts/setup-worktree.sh")
worktree = worktree.replace("frontend (npm) setup", "frontend (Bun) setup")
worktree = replace_required(
    worktree,
    '(cd ui && npm ci --prefer-offline --no-audit 2>/dev/null || npm ci)',
    '(cd ui && bun install --frozen-lockfile --prefer-offline)',
    "worktree dependency install",
)
write("scripts/setup-worktree.sh", worktree)

# Standard Docker build: port upstream mirror.gcr.io mitigation and use Bun for UI.
docker = read("Dockerfile")
docker = docker.replace("public.ecr.aws/docker/library/alpine:", "mirror.gcr.io/library/alpine:")
docker = docker.replace("public.ecr.aws/docker/library/golang:", "mirror.gcr.io/library/golang:")
docker = replace_required(
    docker,
    "FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/node:lts-alpine AS ui",
    f"FROM --platform=$BUILDPLATFORM oven/bun:canary-alpine@{BUN_ALPINE_DIGEST} AS ui",
    "Dockerfile UI base",
)
docker = docker.replace("# Install node dependencies", "# Install Bun dependencies")
docker = replace_required(docker, "COPY ui/package.json ui/package-lock.json ./", "COPY ui/package.json ui/bun.lock ./", "Dockerfile lockfile")
docker = replace_required(docker, "RUN npm ci", "RUN --mount=type=cache,target=/root/.bun/install/cache bun install --frozen-lockfile", "Dockerfile install")
docker = replace_required(docker, "RUN npm run build -- --outDir=/build", "RUN bun run build -- --outDir=/build", "Dockerfile build")
write("Dockerfile", docker)

# JBS production image: preserve the custom native/PGO stack and only swap the UI toolchain.
jbs = read("Dockerfile.jbs")
jbs = re.sub(r"^#   - Node\.js 26\.5\.0$", f"#   - Bun {BUN_VERSION} (1.4 canary)", jbs, count=1, flags=re.M)
jbs, count = re.subn(
    r"^ARG NODE_IMAGE=.*$",
    f"ARG BUN_IMAGE=oven/bun:canary-debian@{BUN_DEBIAN_DIGEST}",
    jbs,
    count=1,
    flags=re.M,
)
if count != 1:
    raise SystemExit("failed to replace NODE_IMAGE in Dockerfile.jbs")
jbs = replace_required(jbs, "FROM ${NODE_IMAGE} AS jsbuilder", "FROM ${BUN_IMAGE} AS jsbuilder", "JBS UI base")
jbs = replace_required(
    jbs,
    "ENV NODE_OPTIONS=--max-old-space-size=2048 \\\n    ND_UI_SOURCEMAP=false \\\n    STATIC_COMPRESSION_JOBS=${STATIC_COMPRESSION_JOBS} \\\n    UV_THREADPOOL_SIZE=${STATIC_COMPRESSION_JOBS}",
    "ENV ND_UI_SOURCEMAP=false \\\n    STATIC_COMPRESSION_JOBS=${STATIC_COMPRESSION_JOBS}",
    "JBS Node-only env",
)
jbs = replace_required(
    jbs,
    "COPY --link --from=source /src/ui/package.json /src/ui/package-lock.json ./",
    "COPY --link --from=source /src/ui/package.json /src/ui/bun.lock ./",
    "JBS lockfile",
)
jbs = jbs.replace("--mount=type=cache,target=/root/.npm,sharing=locked", "--mount=type=cache,target=/root/.bun/install/cache,sharing=locked")
jbs = replace_required(jbs, "npm ci --prefer-offline --no-audit --fund=false", "bun install --frozen-lockfile --prefer-offline", "JBS install")
jbs = replace_required(jbs, "npm run build", "bun run build", "JBS build")
write("Dockerfile.jbs", jbs)

# GitHub CI: pin Bun to the canary channel, assert 1.4 before use, and pin
# golangci-lint to the Makefile version so local and CI lint do not drift.
pipeline = read(".github/workflows/pipeline.yml")
pipeline = pipeline.replace("    env:\n      NODE_OPTIONS: --max_old_space_size=4096\n", "")
pipeline = replace_required(
    pipeline,
    "      - uses: actions/setup-node@v6\n        with:\n          node-version: 24\n          cache: npm\n          cache-dependency-path: ui/package-lock.json",
    "      - uses: oven-sh/setup-bun@v2\n        with:\n          bun-version-file: .bun-version\n      - name: Verify Bun 1.4 runtime\n        run: |\n          case \"$(bun --version)\" in 1.4.*) ;; *) echo \"Bun 1.4 required\" >&2; exit 1 ;; esac\n          bun -e 'const m = new Map([[2**32, \"a\"], [2**33, \"b\"]]); if (m.size !== 2) throw new Error(\"Bun 1.4 Map/Set large-integer regression detected\")'",
    "CI Bun setup",
)
pipeline = pipeline.replace("run: npm ci", "run: bun install --frozen-lockfile")
pipeline = pipeline.replace("run: npm run check-formatting && npm run lint && npm test", "run: bun run check-formatting && bun run lint && bun run test")
pipeline = pipeline.replace("run: npm run build", "run: bun run build")
pipeline = replace_required(pipeline, "          version: latest", "          version: v2.12.0", "golangci-lint pin")
write(".github/workflows/pipeline.yml", pipeline)

# Devcontainer: remove nvm/Node install and install the current Bun canary.
dev_docker = '''# Development container for Navidrome\n\nARG VARIANT="1"\nFROM mcr.microsoft.com/vscode/devcontainers/go:${VARIANT}\n\nARG BUN_VERSION="canary"\n\nRUN apt-get update && export DEBIAN_FRONTEND=noninteractive \\\n    && apt-get -y install --no-install-recommends ca-certificates curl ffmpeg unzip \\\n    && rm -rf /var/lib/apt/lists/*\n\nRUN su vscode -c 'curl -fsSL https://bun.com/install | bash -s "${BUN_VERSION}"'\nENV PATH="/home/vscode/.bun/bin:${PATH}"\n'''
write(".devcontainer/Dockerfile", dev_docker)

dev_json = read(".devcontainer/devcontainer.json")
dev_json = replace_required(
    dev_json,
    '\t\t\t"INSTALL_NODE": "true",\n\t\t\t"NODE_VERSION": "v24"',
    '\t\t\t"BUN_VERSION": "canary"',
    "devcontainer Bun args",
)
write(".devcontainer/devcontainer.json", dev_json)

# Functional migration guard. @types/node and node:* imports are intentionally retained:
# they describe compatibility APIs used by Vite and the precompression script.
for path in [
    "Makefile",
    "Procfile.dev",
    "Dockerfile",
    "Dockerfile.jbs",
    ".github/workflows/pipeline.yml",
    "scripts/setup-worktree.sh",
    ".devcontainer/Dockerfile",
    ".devcontainer/devcontainer.json",
    "ui/package.json",
]:
    text = read(path)
    forbidden = ["npm ci", "npm run", "npm start", "npx ", "setup-node@", "package-lock.json", "NODE_IMAGE", "NODE_OPTIONS", "INSTALL_NODE", "NODE_VERSION"]
    hits = [token for token in forbidden if token in text]
    if hits:
        raise SystemExit(f"remaining Node/npm build-tool references in {path}: {hits}")

if not (ROOT / "ui/bun.lock").is_file():
    raise SystemExit("ui/bun.lock was not generated")

print(f"Bun migration prepared with {BUN_VERSION}")
