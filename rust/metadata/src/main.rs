use std::collections::HashMap;
use std::fs::File;
use std::io::{self, BufRead, BufReader, BufWriter, Write};
use std::path::{Path, PathBuf};

use anyhow::{Context, Result, bail};
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

const PROTOCOL_VERSION: u32 = 1;
const MAX_BATCH_FILES: usize = 4096;

#[derive(Debug, Deserialize)]
struct Request {
    files: Vec<InputFile>,
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
    tags: HashMap<String, Vec<String>>,
    duration_ns: u64,
    bit_rate: u32,
    bit_depth: u8,
    sample_rate: u32,
    channels: u8,
    codec: String,
    has_picture: bool,
}

fn main() -> Result<()> {
    let mut args = std::env::args_os().skip(1);
    if let Some(command) = args.next() {
        if command != "--extract-picture" {
            bail!("unsupported command {:?}", command);
        }
        let path = args
            .next()
            .map(PathBuf::from)
            .context("--extract-picture requires a file path")?;
        if args.next().is_some() {
            bail!("--extract-picture accepts exactly one file path");
        }
        let picture = extract_picture(&path)?;
        io::stdout().lock().write_all(&picture)?;
        return Ok(());
    }

    run_worker()
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

    for input in request.files {
        match parse_file(&input.path) {
            Ok(metadata) => {
                results.insert(input.key, metadata);
            }
            Err(error) => {
                errors.insert(input.key, format!("{error:#}"));
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

fn parse_file(path: &Path) -> Result<Metadata> {
    let (tagged, codec, raw_vorbis) = read_file(path)?;

    let mut tags = generic_tags(&tagged);
    if let Some(vorbis) = raw_vorbis.as_ref() {
        merge_vorbis_tags(&mut tags, vorbis);
    }

    let properties = tagged.properties();
    let has_picture = tagged.tags().iter().any(|tag| !tag.pictures().is_empty());

    Ok(Metadata {
        tags,
        duration_ns: properties.duration().as_nanos().min(u128::from(u64::MAX)) as u64,
        bit_rate: properties.audio_bitrate().unwrap_or(0),
        bit_depth: properties.bit_depth().unwrap_or(0),
        sample_rate: properties.sample_rate().unwrap_or(0),
        channels: properties.channels().unwrap_or(0),
        codec,
        has_picture,
    })
}

fn read_file(path: &Path) -> Result<(TaggedFile, String, Option<VorbisComments>)> {
    let mut raw_vorbis = None;
    let (tagged, codec) = match extension(path).as_str() {
        "flac" => {
            let mut file =
                File::open(path).with_context(|| format!("opening {}", path.display()))?;
            let parsed = FlacFile::read_from(&mut file, ParseOptions::new())
                .with_context(|| format!("decoding FLAC {}", path.display()))?;
            raw_vorbis = parsed.vorbis_comments().cloned();
            (TaggedFile::from(parsed), "flac".to_owned())
        }
        "opus" => {
            let mut file =
                File::open(path).with_context(|| format!("opening {}", path.display()))?;
            let parsed = OpusFile::read_from(&mut file, ParseOptions::new())
                .with_context(|| format!("decoding Opus {}", path.display()))?;
            raw_vorbis = Some(parsed.vorbis_comments().clone());
            (TaggedFile::from(parsed), "opus".to_owned())
        }
        "m4a" | "mp4" => {
            let mut file =
                File::open(path).with_context(|| format!("opening {}", path.display()))?;
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
            (TaggedFile::from(parsed), codec)
        }
        "aac" => {
            let mut file =
                File::open(path).with_context(|| format!("opening {}", path.display()))?;
            let parsed = AacFile::read_from(&mut file, ParseOptions::new())
                .with_context(|| format!("decoding AAC {}", path.display()))?;
            (TaggedFile::from(parsed), "aac".to_owned())
        }
        "mp3" => {
            let mut file =
                File::open(path).with_context(|| format!("opening {}", path.display()))?;
            let parsed = MpegFile::read_from(&mut file, ParseOptions::new())
                .with_context(|| format!("decoding MP3 {}", path.display()))?;
            (TaggedFile::from(parsed), "mp3".to_owned())
        }
        other => bail!("unsupported audio format {other:?}"),
    };

    match tagged.file_type() {
        FileType::Flac | FileType::Opus | FileType::Mp4 | FileType::Aac | FileType::Mpeg => {}
        other => bail!("Lofty detected unsupported file type {other:?}"),
    }

    Ok((tagged, codec, raw_vorbis))
}

fn extract_picture(path: &Path) -> Result<Vec<u8>> {
    let (tagged, _, _) = read_file(path)?;
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
    Ok(picture.data().to_vec())
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
            output.entry(key).or_default().push(value.to_owned());
        }
    }
    output
}

fn normalized_key(key: &ItemKey, _tag: &Tag) -> Option<String> {
    let value = match key {
        ItemKey::TrackTitle => "title",
        ItemKey::AlbumTitle => "album",
        ItemKey::TrackArtist | ItemKey::TrackArtists => "artist",
        ItemKey::AlbumArtist | ItemKey::AlbumArtists => "albumartist",
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
}
