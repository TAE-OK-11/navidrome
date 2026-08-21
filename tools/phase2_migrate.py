#!/usr/bin/env python3
from pathlib import Path
import re
import shutil


def require_replace(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing expected {label}")
    return text.replace(old, new)


def migrate_go() -> None:
    shutil.rmtree("adapters/gotaglib", ignore_errors=True)

    root = Path("cmd/root.go")
    text = root.read_text()
    text = require_replace(
        text,
        '_ "github.com/navidrome/navidrome/adapters/gotaglib"',
        '_ "github.com/navidrome/navidrome/adapters/lofty"',
        "gotaglib adapter import",
    )
    root.write_text(text)

    consts = Path("consts/consts.go")
    text = consts.read_text()
    text = require_replace(
        text,
        'DefaultScannerExtractor = "taglib"',
        'DefaultScannerExtractor = "lofty"',
        "default scanner extractor",
    )
    consts.write_text(text)


def metadata_builder_stage() -> str:
    return r'''# =========================================================
# Lofty metadata companion (persistent Go <-> Rust worker)
# =========================================================

FROM ${RUST_IMAGE} AS metadata-builder

ARG NAVIDROME_BUILD_CORES
ARG RUST_TARGET_CPU

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      musl-tools && \
    rustup target add x86_64-unknown-linux-musl

WORKDIR /build

COPY --link --from=source /src/rust/metadata/Cargo.toml /src/rust/metadata/Cargo.lock ./

RUN --mount=type=cache,target=/usr/local/cargo/registry,sharing=locked \
    --mount=type=cache,target=/usr/local/cargo/git,sharing=locked \
    mkdir src && \
    printf 'fn main() {}\n' > src/main.rs && \
    CARGO_BUILD_JOBS="${NAVIDROME_BUILD_CORES}" \
    CARGO_TARGET_X86_64_UNKNOWN_LINUX_MUSL_LINKER=musl-gcc \
    RUSTFLAGS="-C target-cpu=${RUST_TARGET_CPU} -C target-feature=+crt-static" \
    cargo build --locked --release --target x86_64-unknown-linux-musl && \
    rm -rf src target/x86_64-unknown-linux-musl/release/deps/navidrome_metadata-* \
      target/x86_64-unknown-linux-musl/release/navidrome-metadata

COPY --link --from=source /src/rust/metadata/src ./src

RUN --mount=type=cache,target=/usr/local/cargo/registry,sharing=locked \
    --mount=type=cache,target=/usr/local/cargo/git,sharing=locked \
    CARGO_BUILD_JOBS="${NAVIDROME_BUILD_CORES}" \
    cargo test --locked --release && \
    CARGO_BUILD_JOBS="${NAVIDROME_BUILD_CORES}" \
    CARGO_TARGET_X86_64_UNKNOWN_LINUX_MUSL_LINKER=musl-gcc \
    RUSTFLAGS="-C target-cpu=${RUST_TARGET_CPU} -C target-feature=+crt-static" \
    cargo build --locked --release --target x86_64-unknown-linux-musl && \
    install -D -m 755 target/x86_64-unknown-linux-musl/release/navidrome-metadata \
      /out/navidrome-metadata && \
    test -x /out/navidrome-metadata

'''


def migrate_jbs() -> None:
    path = Path("Dockerfile.jbs")
    text = path.read_text()

    text = text.replace("#   - TagLib 2.3\n", "#   - Lofty 0.25 metadata worker\n")
    text = re.sub(r"^ARG TAGLIB_VERSION=.*\n", "", text, flags=re.MULTILINE)
    text = re.sub(r"^ARG TAGLIB_SHA256=.*\n", "", text, flags=re.MULTILINE)

    old_ui = r'''RUN --mount=type=cache,target=/root/.bun/install/cache,sharing=locked \
    test -s ./bin/update-workbox.sh && \
    sed -i 's/\r$//' ./bin/update-workbox.sh && \
    chmod 755 ./bin/update-workbox.sh && \
    bun install --frozen-lockfile --prefer-offline'''
    new_ui = r'''RUN --mount=type=cache,target=/root/.bun/install/cache,sharing=locked \
    test -s ./bin/update-workbox.ts && \
    bun install --frozen-lockfile --prefer-offline'''
    text = require_replace(text, old_ui, new_ui, "Bun TypeScript Workbox bootstrap")

    text = require_replace(
        text,
        "# Tokio-quiche HTTP/3 companion\n# =========================================================\n\nFROM ${RUST_IMAGE} AS h3-builder",
        metadata_builder_stage()
        + "# =========================================================\n# Tokio-quiche HTTP/3 companion\n# =========================================================\n\nFROM ${RUST_IMAGE} AS h3-builder",
        "HTTP/3 builder insertion point",
    )

    text = text.replace("ARG TAGLIB_VERSION\n", "")
    text = text.replace("ARG TAGLIB_SHA256\n", "")

    taglib_block = re.compile(
        r"\nWORKDIR /taglib-build\n\nRUN wget --https-only.*?test -f /usr/local/lib/libtag_c\.a\n\nWORKDIR /src",
        re.DOTALL,
    )
    text, count = taglib_block.subn("\nWORKDIR /src", text, count=1)
    if count != 1:
        raise SystemExit("failed to remove TagLib C++ build block")

    for package_line in [
        "      cmake \\\n",
        "      libutfcpp-dev \\\n",
        "      ninja-build \\\n",
        "      pkg-config \\\n",
        "      wget \\\n",
        "      zlib1g-dev\n",
    ]:
        text = text.replace(package_line, "")

    text = text.replace("    PKG_CONFIG_PATH=/usr/local/lib/pkgconfig \\\n", "")
    text = text.replace(
        '    CGO_LDFLAGS="${LLVM_FAT_LTO_FLAGS} ${LLD_FLAGS} -L/usr/local/lib -lstdc++ -lz -lm"',
        '    CGO_LDFLAGS="${LLVM_FAT_LTO_FLAGS} ${LLD_FLAGS}"',
    )

    text = text.replace(
        "--enable-demuxer=mov,mp3,aac,flac,wav,aiff,ogg,ape,matroska,wv,pcm_s16le",
        "--enable-demuxer=mov,mp3,aac,flac,ogg,pcm_s16le",
    )
    text = text.replace(
        "--enable-decoder=alac,aac,mp3,mp3float,flac,opus,vorbis,ape,wavpack,pcm_s16le,pcm_s16be,pcm_s24le,pcm_s24be,pcm_s32le,pcm_f32le",
        "--enable-decoder=alac,aac,mp3,mp3float,flac,opus,pcm_s16le,pcm_s24le,pcm_s32le,pcm_f32le",
    )
    text = text.replace(
        "for name in alac aac mp3 flac opus vorbis ape wavpack pcm_s16le pcm_s24le pcm_s32le pcm_f32le; do",
        "for name in alac aac mp3 flac opus pcm_s16le pcm_s24le pcm_s32le pcm_f32le; do",
    )
    text = text.replace(
        "for name in mov mp3 aac flac wav aiff ogg ape matroska wv s16le; do",
        "for name in mov mp3 aac flac ogg s16le; do",
    )
    text = text.replace(
        '&["alac", "aac", "mp3", "flac", "opus", "vorbis"],',
        '&["alac", "aac", "mp3", "flac", "opus"],',
    )
    text = text.replace(
        '&["mov", "mp3", "aac", "flac", "wav", "aiff", "ogg"],',
        '&["mov", "mp3", "aac", "flac", "ogg"],',
    )

    text = require_replace(
        text,
        "COPY --link --chown=1000:1000 --chmod=750 --from=h3-builder /out/navidrome-h3 /app/navidrome-h3\n",
        "COPY --link --chown=1000:1000 --chmod=750 --from=h3-builder /out/navidrome-h3 /app/navidrome-h3\n"
        "COPY --link --chown=1000:1000 --chmod=750 --from=metadata-builder /out/navidrome-metadata /app/navidrome-metadata\n",
        "runtime companion copy",
    )
    text = require_replace(
        text,
        "    test -x /app/navidrome-h3 && \\\n",
        "    test -x /app/navidrome-h3 && \\\n    test -x /app/navidrome-metadata && \\\n",
        "runtime companion validation",
    )
    text = text.replace("/Node26.5/Rust1.97/", "/Bun1.4/Rust1.97/")

    if re.search(r"TAGLIB|TagLib|taglib-build|libtag(?:_c)?\.a", text):
        raise SystemExit("TagLib reference remains in Dockerfile.jbs")

    path.write_text(text)


if __name__ == "__main__":
    migrate_go()
    migrate_jbs()
