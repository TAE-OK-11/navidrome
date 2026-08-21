use lofty::file::{AudioFile, FileType, TaggedFileExt};
use lofty::mp4::{Mp4Codec, Mp4File};
use lofty::ogg::{OggPictureStorage, OpusFile};
use lofty::prelude::ItemKey;
use lofty::properties::FileProperties;
use lofty::tag::{ItemValue, Tag};
use lofty::{config::ParseOptions, flac::FlacFile};
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashMap};
use std::env;
use std::fs::File;
use std::io::{self, BufRead, BufReader, BufWriter, Write};
use std::path::{Component, Path, PathBuf};

const LOFTY_VERSION: &str = "0.25.0";

#[derive(Debug, Deserialize)]
struct Request {
    files: Vec<String>,
}

#[derive(Debug, Default, Serialize)]
struct Response {
    files: BTreeMap<String, Metadata>,
    errors: BTreeMap<String, String>,
}

#[derive(Debug, Serialize)]
struct Metadata {
    tags: HashMap<String, Vec<String>>,
    audio_properties: AudioProperties,
    has_picture: bool,
}

#[derive(Debug, Serialize)]
struct AudioProperties {
    duration_ms: u64,
    bit_rate: u32,
    bit_depth: u8,
    sample_rate: u32,
    channels: u8,
    codec: String,
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut args = env::args().skip(1);
    let first = args.next();
    if matches!(first.as_deref(), Some("--version")) {
        println!("lofty/{LOFTY_VERSION}");
        return Ok(());
    }
    if first.as_deref() != Some("--root") {
        return Err("usage: navidrome-metadata --root <music-folder>".into());
    }
    let root = PathBuf::from(args.next().ok_or("missing --root value")?);
    let root = root.canonicalize()?;

    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut writer = BufWriter::new(stdout.lock());
    for line in stdin.lock().lines() {
        let line = line?;
        if line.trim().is_empty() {
            continue;
        }
        let request: Request = match serde_json::from_str(&line) {
            Ok(value) => value,
            Err(err) => {
                serde_json::to_writer(
                    &mut writer,
                    &Response {
                        errors: BTreeMap::from([("<request>".into(), err.to_string())]),
                        ..Response::default()
                    },
                )?;
                writer.write_all(b"\n")?;
                writer.flush()?;
                continue;
            }
        };

        let mut response = Response::default();
        for relative in request.files {
            match resolve_path(&root, &relative).and_then(|path| read_metadata(&path)) {
                Ok(metadata) => {
                    response.files.insert(relative, metadata);
                }
                Err(err) => {
                    response.errors.insert(relative, err);
                }
            }
        }
        serde_json::to_writer(&mut writer, &response)?;
        writer.write_all(b"\n")?;
        writer.flush()?;
    }
    Ok(())
}

fn resolve_path(root: &Path, relative: &str) -> Result<PathBuf, String> {
    let path = Path::new(relative);
    if path.is_absolute()
        || path.components().any(|part| matches!(part, Component::ParentDir | Component::RootDir | Component::Prefix(_)))
    {
        return Err("invalid relative media path".into());
    }
    Ok(root.join(path))
}

fn read_metadata(path: &Path) -> Result<Metadata, String> {
    let ext = path
        .extension()
        .and_then(|value| value.to_str())
        .unwrap_or_default()
        .to_ascii_lowercase();
    if !matches!(ext.as_str(), "aac" | "flac" | "mp3" | "opus" | "m4a") {
        return Err(format!("unsupported audio format: .{ext}"));
    }

    let tagged = lofty::read_from_path(path).map_err(|err| err.to_string())?;
    let expected_type = match ext.as_str() {
        "aac" => FileType::Aac,
        "flac" => FileType::Flac,
        "mp3" => FileType::Mpeg,
        "opus" => FileType::Opus,
        "m4a" => FileType::Mp4,
        _ => unreachable!(),
    };
    if tagged.file_type() != expected_type {
        return Err(format!(
            "file content does not match supported extension: expected {expected_type:?}, got {:?}",
            tagged.file_type()
        ));
    }

    let codec = if ext == "m4a" {
        mp4_codec(path)?
    } else {
        ext.clone()
    };
    let props = tagged.properties();
    let mut tags = HashMap::<String, Vec<String>>::new();
    for tag in tagged.tags() {
        add_generic_tag(tag, &mut tags);
    }

    let has_picture = match ext.as_str() {
        "flac" => add_flac_raw(path, &mut tags)?,
        "opus" => add_opus_raw(path, &mut tags)?,
        _ => tagged.tags().iter().any(|tag| !tag.pictures().is_empty()),
    };

    normalize_tuples(&mut tags);
    normalize_lyrics(&mut tags);

    Ok(Metadata {
        tags,
        audio_properties: wire_properties(props, codec),
        has_picture,
    })
}

fn wire_properties(props: &FileProperties, codec: String) -> AudioProperties {
    let millis = props.duration().as_millis() as u64;
    AudioProperties {
        duration_ms: ((millis + 5) / 10) * 10,
        bit_rate: props.audio_bitrate().or_else(|| props.overall_bitrate()).unwrap_or_default(),
        bit_depth: props.bit_depth().unwrap_or_default(),
        sample_rate: props.sample_rate().unwrap_or_default(),
        channels: props.channels().unwrap_or_default(),
        codec,
    }
}

fn mp4_codec(path: &Path) -> Result<String, String> {
    let mut file = File::open(path).map_err(|err| err.to_string())?;
    let parsed = Mp4File::read_from(&mut file, ParseOptions::new()).map_err(|err| err.to_string())?;
    match parsed.properties().codec() {
        Mp4Codec::AAC => Ok("aac".into()),
        Mp4Codec::ALAC => Ok("alac".into()),
        other => Err(format!("unsupported M4A codec: {other:?}; only AAC and ALAC are enabled")),
    }
}

fn add_flac_raw(path: &Path, tags: &mut HashMap<String, Vec<String>>) -> Result<bool, String> {
    let mut file = File::open(path).map_err(|err| err.to_string())?;
    let parsed = FlacFile::read_from(&mut file, ParseOptions::new()).map_err(|err| err.to_string())?;
    if let Some(comments) = parsed.vorbis_comments() {
        add_vorbis_comments(comments.items(), tags);
    }
    Ok(!parsed.pictures().is_empty())
}

fn add_opus_raw(path: &Path, tags: &mut HashMap<String, Vec<String>>) -> Result<bool, String> {
    let mut file = File::open(path).map_err(|err| err.to_string())?;
    let parsed = OpusFile::read_from(&mut file, ParseOptions::new()).map_err(|err| err.to_string())?;
    let comments = parsed.vorbis_comments();
    add_vorbis_comments(comments.items(), tags);
    Ok(!comments.pictures().is_empty())
}

fn add_vorbis_comments<'a>(
    items: impl Iterator<Item = (&'a str, &'a str)>,
    tags: &mut HashMap<String, Vec<String>>,
) {
    for (key, value) in items {
        push_tag(tags, key.to_ascii_lowercase(), value.to_string());
    }
}

fn add_generic_tag(tag: &Tag, tags: &mut HashMap<String, Vec<String>>) {
    for item in tag.items() {
        let Some(key) = canonical_key(item.key()) else {
            continue;
        };
        let value = match item.value() {
            ItemValue::Text(value) | ItemValue::Locator(value) => value,
            ItemValue::Binary(_) => continue,
        };
        push_tag(tags, key.to_string(), value.to_string());
    }
}

fn canonical_key(key: &ItemKey) -> Option<&'static str> {
    Some(match key {
        ItemKey::TrackTitle => "title",
        ItemKey::TrackTitleSortOrder => "titlesort",
        ItemKey::TrackSubtitle => "subtitle",
        ItemKey::TrackArtist => "artist",
        ItemKey::TrackArtists => "artists",
        ItemKey::TrackArtistSortOrder => "artistsort",
        ItemKey::AlbumTitle => "album",
        ItemKey::AlbumTitleSortOrder => "albumsort",
        ItemKey::AlbumArtist => "albumartist",
        ItemKey::AlbumArtists => "albumartists",
        ItemKey::AlbumArtistSortOrder => "albumartistsort",
        ItemKey::Arranger => "arranger",
        ItemKey::Writer => "writer",
        ItemKey::Composer => "composer",
        ItemKey::ComposerSortOrder => "composersort",
        ItemKey::Conductor => "conductor",
        ItemKey::Director => "director",
        ItemKey::Engineer => "engineer",
        ItemKey::Lyricist => "lyricist",
        ItemKey::MixDj => "djmixer",
        ItemKey::MixEngineer => "mixer",
        ItemKey::Performer => "performer",
        ItemKey::Producer => "producer",
        ItemKey::Publisher => "publisher",
        ItemKey::Label => "label",
        ItemKey::Remixer => "remixer",
        ItemKey::DiscNumber => "discnumber",
        ItemKey::DiscTotal => "disctotal",
        ItemKey::TrackNumber => "tracknumber",
        ItemKey::TrackTotal => "tracktotal",
        ItemKey::ParentalAdvisory => "explicitstatus",
        ItemKey::RecordingDate | ItemKey::Year => "date",
        ItemKey::ReleaseDate => "releasedate",
        ItemKey::OriginalReleaseDate => "originaldate",
        ItemKey::Isrc => "isrc",
        ItemKey::Barcode => "barcode",
        ItemKey::CatalogNumber => "catalognumber",
        ItemKey::Work => "work",
        ItemKey::Movement => "movementname",
        ItemKey::MovementNumber => "movement",
        ItemKey::MovementTotal => "movementtotal",
        ItemKey::ReleaseCountry => "releasecountry",
        ItemKey::MusicBrainzRecordingId => "musicbrainz_recordingid",
        ItemKey::MusicBrainzTrackId => "musicbrainz_trackid",
        ItemKey::MusicBrainzReleaseId => "musicbrainz_albumid",
        ItemKey::MusicBrainzReleaseGroupId => "musicbrainz_releasegroupid",
        ItemKey::MusicBrainzArtistId => "musicbrainz_artistid",
        ItemKey::MusicBrainzReleaseArtistId => "musicbrainz_albumartistid",
        ItemKey::MusicBrainzWorkId => "musicbrainz_workid",
        ItemKey::MusicBrainzReleaseType => "releasetype",
        ItemKey::FlagCompilation => "compilation",
        ItemKey::EncodedBy => "encodedby",
        ItemKey::EncoderSettings => "encodersettings",
        ItemKey::ReplayGainAlbumGain => "replaygain_album_gain",
        ItemKey::ReplayGainAlbumPeak => "replaygain_album_peak",
        ItemKey::ReplayGainTrackGain => "replaygain_track_gain",
        ItemKey::ReplayGainTrackPeak => "replaygain_track_peak",
        ItemKey::Genre => "genre",
        ItemKey::InitialKey => "key",
        ItemKey::Mood => "mood",
        ItemKey::Bpm | ItemKey::IntegerBpm => "bpm",
        ItemKey::CopyrightMessage => "copyright",
        ItemKey::License => "license",
        ItemKey::Comment => "comment",
        ItemKey::Description => "description",
        ItemKey::Language => "language",
        ItemKey::Script => "script",
        ItemKey::Lyrics | ItemKey::UnsyncLyrics => "lyrics",
        _ => return None,
    })
}

fn push_tag(tags: &mut HashMap<String, Vec<String>>, key: String, value: String) {
    if value.is_empty() {
        return;
    }
    let values = tags.entry(key).or_default();
    if !values.contains(&value) {
        values.push(value);
    }
}

fn normalize_tuples(tags: &mut HashMap<String, Vec<String>>) {
    for name in ["track", "disc"] {
        let number_key = format!("{name}number");
        let total_key = format!("{name}total");
        let Some(value) = tags.get(&number_key).and_then(|values| values.first()).cloned() else {
            continue;
        };
        let Some((number, total)) = value.split_once('/') else {
            continue;
        };
        tags.insert(number_key, vec![number.to_string()]);
        if !total.is_empty() {
            tags.insert(total_key, vec![total.to_string()]);
        }
    }
}

fn normalize_lyrics(tags: &mut HashMap<String, Vec<String>>) {
    if let Some(values) = tags.remove("lyrics") {
        let target = tags.entry("lyrics:xxx".into()).or_default();
        for value in values {
            if !target.contains(&value) {
                target.push(value);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_escape_paths() {
        let root = Path::new("/music");
        assert!(resolve_path(root, "../secret.mp3").is_err());
        assert!(resolve_path(root, "/etc/passwd").is_err());
        assert_eq!(resolve_path(root, "Taylor/song.m4a").unwrap(), root.join("Taylor/song.m4a"));
    }

    #[test]
    fn supported_extensions_are_explicit() {
        for ext in ["aac", "flac", "mp3", "opus", "m4a"] {
            assert!(matches!(ext, "aac" | "flac" | "mp3" | "opus" | "m4a"));
        }
        assert!(!matches!("wma", "aac" | "flac" | "mp3" | "opus" | "m4a"));
    }
}
