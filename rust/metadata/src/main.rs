use std::collections::HashMap;
use std::fs::{File, Metadata as FileMetadata};
use std::io::{self, BufRead, BufReader, BufWriter, Write};
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result, bail};
use rayon::prelude::*;
use lofty::aac::AacFile;
use lofty::config::ParseOptions;
use lofty::file::{AudioFile, FileType, TaggedFile, TaggedFileExt};
use lofty::flac::FlacFile;
use lofty::mp4::{Mp4Codec, Mp4File};
use lofty::mpeg::MpegFile;
use lofty::ogg::OpusFile;
use lofty::ogg::tag::VorbisComments;
use lofty::picture::{Picture, PictureType};
use lofty::tag::{ItemKey, Tag};
use serde::{Deserialize, Serialize};

use navidrome_metadata::{compute_pid, map_media, tag_clean};

mod build_fts5_query_worker;
mod clean_tags_worker;
mod image_worker;
mod lyrics;
mod lyricsfile;
mod map_media_worker;
mod normalize_fts;
mod normalize_fts_worker;
mod parse_lyrics_worker;
mod ttml;

const PROTOCOL_VERSION: u32 = 1;
const MAX_BATCH_FILES: usize = 4096;

#[derive(Debug, Deserialize)]
struct Request {
    files: Vec<InputFile>,
    #[serde(default)]
    tag_mappings: HashMap<String, tag_clean::TagMappingConfig>,
    #[serde(default)]
    artist_split_exceptions: Vec<String>,
    #[serde(default)]
    artists_split: Vec<String>,
    #[serde(default)]
    roles_split: Vec<String>,
    #[serde(default)]
    artist_joiner: String,
    #[serde(default)]
    pid_config: Option<compute_pid::PidConfig>,
    #[serde(default)]
    library_id: i32,
}

#[derive(Debug, Deserialize)]
struct InputFile {
    key: String,
    path: PathBuf,
}

#[derive(Debug, Serialize)]
struct Response {
    protocol: u32,
    lofty: &'static str,
    results: HashMap<String, Metadata>,
    errors: HashMap<String, String>,
}

#[derive(Debug, Serialize)]
struct Metadata {
    #[serde(skip_serializing_if = "HashMap::is_empty")]
    tags: HashMap<String, Vec<String>>,
    file_info: FileInfo,
    duration_ns: u64,
    bit_rate: u32,
    bit_depth: u8,
    sample_rate: u32,
    channels: u8,
    codec: String,
    has_picture: bool,
    /// Pre-parsed OpenSubsonic lyrics JSON for the scan path. Omitted when parsing fails.
    #[serde(skip_serializing_if = "Option::is_none")]
    lyrics_json: Option<String>,
    /// Pre-mapped MediaFile scan fields (participants, titles, dates, etc.).
    #[serde(skip_serializing_if = "Option::is_none")]
    media_file_json: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    cleaned_tags: Option<HashMap<String, Vec<String>>>,
}

#[derive(Debug, Serialize)]
struct FileInfo {
    name: String,
    size: u64,
    modified_ns: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    created_ns: Option<i64>,
}

#[derive(Debug, Deserialize)]
struct PictureRequest {
    path: PathBuf,
    max_bytes: u64,
}

#[derive(Debug, Serialize)]
struct PictureResponse {
    ok: bool,
    size: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

fn main() -> Result<()> {
    let mut args = std::env::args_os().skip(1);
    if let Some(command) = args.next() {
        if command == "--image-worker" {
            if args.next().is_some() {
                bail!("--image-worker accepts no arguments");
            }
            return image_worker::run();
        }
        if command == "--parse-lyrics-worker" {
            if args.next().is_some() {
                bail!("--parse-lyrics-worker accepts no arguments");
            }
            return parse_lyrics_worker::run();
        }
        if command == "--normalize-fts-worker" {
            if args.next().is_some() {
                bail!("--normalize-fts-worker accepts no arguments");
            }
            return normalize_fts_worker::run();
        }
        if command == "--build-fts5-query-worker" {
            if args.next().is_some() {
                bail!("--build-fts5-query-worker accepts no arguments");
            }
            return build_fts5_query_worker::run();
        }
        if command == "--clean-tags-worker" {
            if args.next().is_some() {
                bail!("--clean-tags-worker accepts no arguments");
            }
            return clean_tags_worker::run();
        }
        if command == "--map-media-worker" {
            if args.next().is_some() {
                bail!("--map-media-worker accepts no arguments");
            }
            return map_media_worker::run();
        }
        if command == "--picture-worker" {
            if args.next().is_some() {
                bail!("--picture-worker accepts no arguments");
            }
            return run_picture_worker();
        }
        if command == "--extract-picture" {
            let path = args
                .next()
                .map(PathBuf::from)
                .context("--extract-picture requires a file path")?;
            if args.next().is_some() {
                bail!("--extract-picture accepts exactly one file path");
            }
            let (tagged, _, _, _) = read_file(&path)?;
            let picture = picture_data(&tagged, &path)?;
            io::stdout().lock().write_all(picture)?;
            return Ok(());
        }
        bail!("unsupported command {:?}", command);
    }

    run_worker()
}

fn run_picture_worker() -> Result<()> {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut output = BufWriter::with_capacity(256 * 1024, stdout.lock());

    for line in BufReader::with_capacity(16 * 1024, stdin.lock()).lines() {
        let line = line.context("reading picture request")?;
        if line.trim().is_empty() {
            continue;
        }

        match serde_json::from_str::<PictureRequest>(&line) {
            Ok(request) => match read_file(&request.path).and_then(|(tagged, _, _, _)| {
                let picture = picture_data(&tagged, &request.path)?;
                if picture.len() as u64 > request.max_bytes {
                    bail!(
                        "embedded artwork exceeds maximum size of {} bytes",
                        request.max_bytes
                    );
                }
                write_picture_response(&mut output, picture)
            }) {
                Ok(()) => {}
                Err(error) => write_picture_error(&mut output, format!("{error:#}"))?,
            },
            Err(error) => write_picture_error(&mut output, error.to_string())?,
        }
        output.flush()?;
    }
    Ok(())
}

fn write_picture_response(output: &mut impl Write, picture: &[u8]) -> Result<()> {
    serde_json::to_writer(
        &mut *output,
        &PictureResponse {
            ok: true,
            size: picture.len(),
            error: None,
        },
    )?;
    output.write_all(b"\n")?;
    output.write_all(picture)?;
    Ok(())
}

fn write_picture_error(output: &mut impl Write, error: String) -> Result<()> {
    serde_json::to_writer(
        &mut *output,
        &PictureResponse {
            ok: false,
            size: 0,
            error: Some(error),
        },
    )?;
    output.write_all(b"\n")?;
    Ok(())
}

fn run_worker() -> Result<()> {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut output = BufWriter::with_capacity(256 * 1024, stdout.lock());

    for line in BufReader::with_capacity(256 * 1024, stdin.lock()).lines() {
        let line = line.context("reading metadata request")?;
        if line.trim().is_empty() {
            continue;
        }

        let response = match serde_json::from_str::<Request>(&line) {
            Ok(request) => handle_request(request),
            Err(error) => Response {
                protocol: PROTOCOL_VERSION,
                lofty: env!("CARGO_PKG_VERSION"),
                results: HashMap::new(),
                errors: HashMap::from([("$request".to_owned(), error.to_string())]),
            },
        };
        serde_json::to_writer(&mut output, &response).context("encoding metadata response")?;
        output.write_all(b"\n")?;
        output.flush()?;
    }
    Ok(())
}

fn handle_request(request: Request) -> Response {
    let file_count = request.files.len();
    let mut results = HashMap::with_capacity(file_count);
    let mut errors = HashMap::new();

    if request.files.len() > MAX_BATCH_FILES {
        errors.insert(
            "$request".to_owned(),
            format!(
                "batch contains {} files; maximum is {MAX_BATCH_FILES}",
                request.files.len()
            ),
        );
        return Response {
            protocol: PROTOCOL_VERSION,
            lofty: env!("CARGO_PKG_VERSION"),
            results,
            errors,
        };
    }

    let parsed: Vec<(String, Result<Metadata>)> = request
        .files
        .par_iter()
        .map(|input| {
            (
                input.key.clone(),
                parse_file(&input.path, &request),
            )
        })
        .collect();
    for (key, outcome) in parsed {
        match outcome {
            Ok(metadata) => {
                results.insert(key, metadata);
            }
            Err(error) => {
                errors.insert(key, format!("{error:#}"));
            }
        }
    }

    Response {
        protocol: PROTOCOL_VERSION,
        lofty: env!("CARGO_PKG_VERSION"),
        results,
        errors,
    }
}

fn parse_file(path: &Path, request: &Request) -> Result<Metadata> {
    let (tagged, codec, raw_vorbis, file_metadata) = read_file(path)?;

    let mut tags = generic_tags(&tagged);
    if let Some(vorbis) = raw_vorbis.as_ref() {
        merge_vorbis_tags(&mut tags, vorbis);
    }

    let cleaned_tags = if request.tag_mappings.is_empty() {
        None
    } else {
        Some(tag_clean::clean(
            &path.to_string_lossy(),
            &tags,
            &request.tag_mappings,
            &request.artist_split_exceptions,
        ))
    };

    let properties = tagged.properties();
    let has_picture = tagged.tags().iter().any(|tag| !tag.pictures().is_empty());
    let lyrics_json = lyrics::parse_tags_to_json(&tags);
    let map_config = map_media::MapMediaConfig {
        artists_split: if request.artists_split.is_empty() {
            map_media::MapMediaConfig::with_defaults().artists_split
        } else {
            request.artists_split.clone()
        },
        roles_split: if request.roles_split.is_empty() {
            map_media::MapMediaConfig::with_defaults().roles_split
        } else {
            request.roles_split.clone()
        },
        artist_split_exceptions: request.artist_split_exceptions.clone(),
        artist_joiner: if request.artist_joiner.is_empty() {
            map_media::MapMediaConfig::with_defaults().artist_joiner
        } else {
            request.artist_joiner.clone()
        },
    };
    let media_file_json = map_media::map_to_json_with_pid(
        &tags,
        path,
        lyrics_json.as_deref(),
        request.pid_config.as_ref(),
        request.library_id,
        &path.to_string_lossy(),
        Some(&map_config),
    );

    Ok(Metadata {
        tags,
        file_info: build_file_info(path, &file_metadata)?,
        duration_ns: properties.duration().as_nanos().min(u128::from(u64::MAX)) as u64,
        bit_rate: properties.audio_bitrate().unwrap_or(0),
        bit_depth: properties.bit_depth().unwrap_or(0),
        sample_rate: properties.sample_rate().unwrap_or(0),
        channels: properties.channels().unwrap_or(0),
        codec,
        has_picture,
        lyrics_json,
        media_file_json,
        cleaned_tags,
    })
}

fn read_file(path: &Path) -> Result<(TaggedFile, String, Option<VorbisComments>, FileMetadata)> {
    let mut raw_vorbis = None;
    let (tagged, codec, file_metadata) = match extension(path).as_str() {
        "flac" => {
            let (mut file, file_metadata) = open_file(path)?;
            let parsed = FlacFile::read_from(&mut file, ParseOptions::new())
                .with_context(|| format!("decoding FLAC {}", path.display()))?;
            raw_vorbis = parsed.vorbis_comments().cloned();
            (TaggedFile::from(parsed), "flac".to_owned(), file_metadata)
        }
        "opus" => {
            let (mut file, file_metadata) = open_file(path)?;
            let parsed = OpusFile::read_from(&mut file, ParseOptions::new())
                .with_context(|| format!("decoding Opus {}", path.display()))?;
            raw_vorbis = Some(parsed.vorbis_comments().clone());
            (TaggedFile::from(parsed), "opus".to_owned(), file_metadata)
        }
        "m4a" | "mp4" => {
            let (mut file, file_metadata) = open_file(path)?;
            let parsed = Mp4File::read_from(&mut file, ParseOptions::new())
                .with_context(|| format!("decoding M4A/MP4 {}", path.display()))?;
            let codec = match parsed.properties().codec() {
                Some(Mp4Codec::AAC) => "aac",
                Some(Mp4Codec::ALAC) => "alac",
                Some(Mp4Codec::MP3) => "mp3",
                Some(Mp4Codec::FLAC) => "flac",
                Some(_) | None => "m4a",
            }
            .to_owned();
            (TaggedFile::from(parsed), codec, file_metadata)
        }
        "aac" => {
            let (mut file, file_metadata) = open_file(path)?;
            let parsed = AacFile::read_from(&mut file, ParseOptions::new())
                .with_context(|| format!("decoding AAC {}", path.display()))?;
            (TaggedFile::from(parsed), "aac".to_owned(), file_metadata)
        }
        "mp3" => {
            let (mut file, file_metadata) = open_file(path)?;
            let parsed = MpegFile::read_from(&mut file, ParseOptions::new())
                .with_context(|| format!("decoding MP3 {}", path.display()))?;
            (TaggedFile::from(parsed), "mp3".to_owned(), file_metadata)
        }
        other => {
            let (mut file, file_metadata) = open_file(path)?;
            let parsed = TaggedFile::read_from(&mut file, ParseOptions::new())
                .with_context(|| format!("decoding {other} {}", path.display()))?;
            let codec = if other.is_empty() {
                "unknown".to_owned()
            } else {
                other.to_owned()
            };
            (parsed, codec, file_metadata)
        }
    };

    match tagged.file_type() {
        FileType::Flac | FileType::Opus | FileType::Mp4 | FileType::Aac | FileType::Mpeg => {}
        other => bail!("Lofty detected unsupported file type {other:?}"),
    }

    Ok((tagged, codec, raw_vorbis, file_metadata))
}

fn open_file(path: &Path) -> Result<(File, FileMetadata)> {
    let file = File::open(path).with_context(|| format!("opening {}", path.display()))?;
    let metadata = file
        .metadata()
        .with_context(|| format!("reading file information for {}", path.display()))?;
    Ok((file, metadata))
}

fn build_file_info(path: &Path, metadata: &FileMetadata) -> Result<FileInfo> {
    let modified = metadata
        .modified()
        .with_context(|| format!("reading modification time for {}", path.display()))?;
    Ok(FileInfo {
        name: path
            .file_name()
            .unwrap_or_default()
            .to_string_lossy()
            .into_owned(),
        size: metadata.len(),
        modified_ns: unix_nanos(modified),
        created_ns: metadata.created().ok().map(unix_nanos),
    })
}

fn unix_nanos(time: SystemTime) -> i64 {
    let nanos = match time.duration_since(UNIX_EPOCH) {
        Ok(duration) => duration.as_nanos() as i128,
        Err(error) => -(error.duration().as_nanos() as i128),
    };
    nanos.clamp(i64::MIN as i128, i64::MAX as i128) as i64
}

fn picture_data<'a>(tagged: &'a TaggedFile, path: &Path) -> Result<&'a [u8]> {
    let pictures = tagged
        .tags()
        .iter()
        .flat_map(|tag| tag.pictures().iter())
        .collect::<Vec<_>>();
    let picture = preferred_picture(&pictures)
        .with_context(|| format!("no embedded picture found in {}", path.display()))?;
    if picture.data().is_empty() {
        bail!("embedded picture is empty in {}", path.display());
    }
    Ok(picture.data())
}

fn preferred_picture<'a>(pictures: &[&'a Picture]) -> Option<&'a Picture> {
    pictures
        .iter()
        .copied()
        .find(|picture| picture.pic_type() == PictureType::CoverFront)
        .or_else(|| pictures.first().copied())
}

fn extension(path: &Path) -> String {
    path.extension()
        .and_then(|value| value.to_str())
        .unwrap_or_default()
        .to_ascii_lowercase()
}

fn generic_tags(file: &TaggedFile) -> HashMap<String, Vec<String>> {
    let mut output = HashMap::<String, Vec<String>>::new();
    for tag in file.tags() {
        for item in tag.items() {
            let Some(value) = item.value().text().or_else(|| item.value().locator()) else {
                continue;
            };
            if value.is_empty() {
                continue;
            }
            let item_key = item.key();
            let key =
                normalized_key(&item_key, tag).unwrap_or_else(|| fallback_key(&item_key, tag));
            if key.is_empty() {
                continue;
            }
            let values = output.entry(key).or_default();
            if !values.iter().any(|existing| existing == value) {
                values.push(value.to_owned());
            }
        }
    }
    output
}

fn normalized_key(key: &ItemKey, _tag: &Tag) -> Option<String> {
    let value = match key {
        ItemKey::TrackTitle => "title",
        ItemKey::AlbumTitle => "album",
        ItemKey::TrackArtist => "artist",
        ItemKey::TrackArtists => "artists",
        ItemKey::AlbumArtist => "albumartist",
        ItemKey::AlbumArtists => "albumartists",
        ItemKey::TrackNumber => "tracknumber",
        ItemKey::TrackTotal => "tracktotal",
        ItemKey::DiscNumber => "discnumber",
        ItemKey::DiscTotal => "disctotal",
        ItemKey::RecordingDate | ItemKey::Year => "date",
        ItemKey::ReleaseDate => "releasedate",
        ItemKey::OriginalReleaseDate => "originaldate",
        ItemKey::Genre => "genre",
        ItemKey::Comment => "comment",
        ItemKey::Bpm | ItemKey::IntegerBpm => "bpm",
        ItemKey::FlagCompilation => "compilation",
        ItemKey::ReplayGainAlbumGain => "replaygain_album_gain",
        ItemKey::ReplayGainAlbumPeak => "replaygain_album_peak",
        ItemKey::ReplayGainTrackGain => "replaygain_track_gain",
        ItemKey::ReplayGainTrackPeak => "replaygain_track_peak",
        ItemKey::MusicBrainzRecordingId => "musicbrainz_recordingid",
        ItemKey::MusicBrainzTrackId => "musicbrainz_trackid",
        ItemKey::MusicBrainzReleaseId => "musicbrainz_albumid",
        ItemKey::MusicBrainzReleaseGroupId => "musicbrainz_releasegroupid",
        ItemKey::MusicBrainzArtistId => "musicbrainz_artistid",
        ItemKey::MusicBrainzReleaseArtistId => "musicbrainz_albumartistid",
        ItemKey::MusicBrainzWorkId => "musicbrainz_workid",
        ItemKey::Isrc => "isrc",
        ItemKey::Barcode => "barcode",
        ItemKey::CatalogNumber => "catalognumber",
        ItemKey::Composer => "composer",
        ItemKey::Conductor => "conductor",
        ItemKey::Arranger => "arranger",
        ItemKey::Engineer => "engineer",
        ItemKey::Producer => "producer",
        ItemKey::Publisher => "publisher",
        ItemKey::Label => "label",
        ItemKey::Lyricist => "lyricist",
        ItemKey::MixDj => "djmixer",
        ItemKey::MixEngineer => "mixer",
        ItemKey::Remixer => "remixer",
        ItemKey::Mood => "mood",
        ItemKey::InitialKey => "initialkey",
        ItemKey::Language => "language",
        ItemKey::Lyrics | ItemKey::UnsyncLyrics => "lyrics",
        _ => return None,
    };
    Some(value.to_owned())
}

fn fallback_key(key: &ItemKey, tag: &Tag) -> String {
    key.map_key(tag.tag_type())
        .unwrap_or_default()
        .trim()
        .to_ascii_lowercase()
}

fn merge_vorbis_tags(tags: &mut HashMap<String, Vec<String>>, vorbis: &VorbisComments) {
    // Vorbis comments can contain arbitrary application-defined fields. Keep the
    // original key semantics instead of converting through generic Tag, where
    // Lofty intentionally drops fields without an ItemKey mapping.
    for (key, value) in vorbis.items() {
        let key = key.trim().to_ascii_lowercase();
        if key.is_empty() || value.is_empty() {
            continue;
        }
        let values = tags.entry(key).or_default();
        if !values.iter().any(|existing| existing == value) {
            values.push(value.to_owned());
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_supported_extensions_are_accepted_by_dispatch() {
        for ext in ["mp3", "flac", "opus", "aac", "m4a"] {
            assert_eq!(extension(Path::new(&format!("song.{ext}"))), ext);
        }
    }

    #[test]
    fn protocol_is_stable() {
        assert_eq!(PROTOCOL_VERSION, 1);
    }

    #[test]
    fn prefers_front_cover_picture_data() {
        let back = Picture::unchecked(vec![1, 2, 3])
            .pic_type(PictureType::CoverBack)
            .build();
        let front = Picture::unchecked(vec![4, 5, 6])
            .pic_type(PictureType::CoverFront)
            .build();
        let pictures = [&back, &front];
        assert_eq!(preferred_picture(&pictures).unwrap().data(), &[4, 5, 6]);
    }

    #[test]
    fn reuses_open_file_information_in_metadata_response() {
        let file_name = format!(
            "navidrome-metadata-file-info-{}-{}",
            std::process::id(),
            unix_nanos(SystemTime::now())
        );
        let path = std::env::temp_dir().join(&file_name);
        std::fs::write(&path, b"navidrome").unwrap();
        let metadata = std::fs::metadata(&path).unwrap();

        let info = build_file_info(&path, &metadata).unwrap();

        assert_eq!(info.name, file_name);
        assert_eq!(info.size, 9);
        assert!(info.modified_ns > 0);
        std::fs::remove_file(path).unwrap();
    }
}
