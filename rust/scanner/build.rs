use std::env;
use std::fs;
use std::path::PathBuf;

const MIME_TYPES_PATH: &str = "../../resources/mime_types.yaml";
const EXCLUDED_AUDIO_TYPES: &[&str] = &["audio/mpegurl", "audio/x-mpegurl", "audio/x-scpls"];

fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo:rerun-if-changed={MIME_TYPES_PATH}");
    let source = fs::read_to_string(MIME_TYPES_PATH)
        .unwrap_or_else(|error| panic!("reading {MIME_TYPES_PATH}: {error}"));
    let (audio, images) = configured_extensions(&source);
    assert!(!audio.is_empty(), "no audio MIME extensions configured");
    assert!(!images.is_empty(), "no image MIME extensions configured");

    let generated = format!(
        "const AUDIO_EXTENSIONS: &[&str] = &{audio:?};\nconst IMAGE_EXTENSIONS: &[&str] = &{images:?};\n"
    );
    let output = PathBuf::from(env::var_os("OUT_DIR").expect("OUT_DIR is set by Cargo"))
        .join("media_extensions.rs");
    fs::write(&output, generated)
        .unwrap_or_else(|error| panic!("writing {}: {error}", output.display()));

    let protoc = protoc_bin_vendored::protoc_bin_path()?;
    unsafe {
        std::env::set_var("PROTOC", protoc);
    }
    let proto = "../../proto/navidrome/scanner/v1/scanner.proto";
    println!("cargo:rerun-if-changed={proto}");
    tonic_prost_build::configure()
        .build_server(true)
        .build_client(false)
        .compile_protos(&[proto], &["../../proto"])?;
    Ok(())
}

fn configured_extensions(source: &str) -> (Vec<String>, Vec<String>) {
    let mut in_types = false;
    let mut audio = Vec::new();
    let mut images = Vec::new();
    for raw_line in source.lines() {
        let line = raw_line.trim();
        match line {
            "types:" => {
                in_types = true;
                continue;
            }
            "lossless:" => break,
            _ if !in_types || line.is_empty() || line.starts_with('#') => continue,
            _ => {}
        }
        let Some((raw_extension, raw_mime)) = line.split_once(':') else {
            continue;
        };
        let extension = raw_extension.trim().trim_start_matches('.');
        let mime_type = raw_mime.split('#').next().unwrap_or_default().trim();
        assert!(
            !extension.is_empty() && extension.bytes().all(|byte| byte.is_ascii_alphanumeric()),
            "invalid MIME extension {raw_extension:?}"
        );
        if mime_type.starts_with("audio/") && !EXCLUDED_AUDIO_TYPES.contains(&mime_type) {
            audio.push(extension.to_ascii_lowercase());
        } else if mime_type.starts_with("image/") {
            images.push(extension.to_ascii_lowercase());
        }
    }
    audio.sort_unstable();
    audio.dedup();
    images.sort_unstable();
    images.dedup();
    (audio, images)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn separates_media_and_excludes_playlist_mime_types() {
        let (audio, images) = configured_extensions(
            "types:\n  .flac: audio/flac\n  .m3u: audio/x-mpegurl\n  .jxl: image/jxl\nlossless:\n  - .flac\n",
        );
        assert_eq!(audio, ["flac"]);
        assert_eq!(images, ["jxl"]);
    }
}
