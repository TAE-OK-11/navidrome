//! Maps Lofty tag maps into scan-ready MediaFile JSON for the Go scanner hot path.

use std::collections::HashMap;
use std::path::Path;

use serde::Serialize;

#[derive(Debug, Serialize)]
struct ScanArtist {
    name: String,
    #[serde(rename = "sortArtistName", skip_serializing_if = "String::is_empty")]
    sort_artist_name: String,
    #[serde(rename = "orderArtistName", skip_serializing_if = "String::is_empty")]
    order_artist_name: String,
    #[serde(rename = "mbzArtistId", skip_serializing_if = "String::is_empty")]
    mbz_artist_id: String,
    #[serde(rename = "subRole", skip_serializing_if = "String::is_empty")]
    sub_role: String,
}

#[derive(Debug, Serialize)]
struct ScanMediaFile {
    title: String,
    album: String,
    artist: String,
    #[serde(rename = "albumArtist")]
    album_artist: String,
    #[serde(rename = "sortTitle", skip_serializing_if = "String::is_empty")]
    sort_title: String,
    #[serde(rename = "sortAlbumName", skip_serializing_if = "String::is_empty")]
    sort_album_name: String,
    #[serde(rename = "sortArtistName", skip_serializing_if = "String::is_empty")]
    sort_artist_name: String,
    #[serde(rename = "sortAlbumArtistName", skip_serializing_if = "String::is_empty")]
    sort_album_artist_name: String,
    #[serde(rename = "orderTitle", skip_serializing_if = "String::is_empty")]
    order_title: String,
    #[serde(rename = "orderAlbumName", skip_serializing_if = "String::is_empty")]
    order_album_name: String,
    compilation: bool,
    #[serde(rename = "trackNumber")]
    track_number: i32,
    #[serde(rename = "discNumber")]
    disc_number: i32,
    #[serde(rename = "discSubtitle", skip_serializing_if = "String::is_empty")]
    disc_subtitle: String,
    #[serde(rename = "catalogNum", skip_serializing_if = "String::is_empty")]
    catalog_num: String,
    comment: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    bpm: Option<i32>,
    lyrics: String,
    #[serde(rename = "explicitStatus", skip_serializing_if = "String::is_empty")]
    explicit_status: String,
    #[serde(rename = "originalYear")]
    original_year: i32,
    #[serde(rename = "originalDate", skip_serializing_if = "String::is_empty")]
    original_date: String,
    #[serde(rename = "releaseYear")]
    release_year: i32,
    #[serde(rename = "releaseDate", skip_serializing_if = "String::is_empty")]
    release_date: String,
    year: i32,
    date: String,
    #[serde(rename = "mbzRecordingID", skip_serializing_if = "String::is_empty")]
    mbz_recording_id: String,
    #[serde(rename = "mbzReleaseTrackID", skip_serializing_if = "String::is_empty")]
    mbz_release_track_id: String,
    #[serde(rename = "mbzAlbumID", skip_serializing_if = "String::is_empty")]
    mbz_album_id: String,
    #[serde(rename = "mbzReleaseGroupID", skip_serializing_if = "String::is_empty")]
    mbz_release_group_id: String,
    #[serde(rename = "mbzAlbumType", skip_serializing_if = "String::is_empty")]
    mbz_album_type: String,
    #[serde(rename = "rgAlbumPeak", skip_serializing_if = "Option::is_none")]
    rg_album_peak: Option<f64>,
    #[serde(rename = "rgAlbumGain", skip_serializing_if = "Option::is_none")]
    rg_album_gain: Option<f64>,
    #[serde(rename = "rgTrackPeak", skip_serializing_if = "Option::is_none")]
    rg_track_peak: Option<f64>,
    #[serde(rename = "rgTrackGain", skip_serializing_if = "Option::is_none")]
    rg_track_gain: Option<f64>,
    #[serde(skip_serializing_if = "HashMap::is_empty")]
    participants: HashMap<String, Vec<ScanArtist>>,
}

pub fn map_to_json(tags: &HashMap<String, Vec<String>>, path: &Path, lyrics_json: Option<&str>) -> Option<String> {
    let mapped = map_tags(tags, path, lyrics_json.unwrap_or("[]"))?;
    serde_json::to_string(&mapped).ok()
}

fn map_tags(tags: &HashMap<String, Vec<String>>, path: &Path, lyrics_json: &str) -> Option<ScanMediaFile> {
    let title = first(tags, "title");
    let album = first(tags, "album");
    if title.is_empty() && album.is_empty() {
        return None;
    }

    let artist = display_artist(tags);
    let album_artist = display_album_artist(tags);
    let (original_date, release_date, date) = map_dates(tags);
    let (track_number, _) = tuple(tags, "tracknumber", "tracktotal");
    let (disc_number, _) = tuple(tags, "discnumber", "disctotal");

    Some(ScanMediaFile {
        title: if title.is_empty() {
            path.file_stem()?.to_str()?.to_owned()
        } else {
            title
        },
        album,
        artist: artist.clone(),
        album_artist: album_artist.clone(),
        sort_title: first(tags, "titlesort"),
        sort_album_name: first(tags, "albumsort"),
        sort_artist_name: non_empty_or(first(tags, "artistsort"), first(tags, "albumartistsort")),
        sort_album_artist_name: first(tags, "albumartistsort"),
        order_title: sanitize_sort(&first(tags, "title")),
        order_album_name: sanitize_sort_no_article(&first(tags, "album")),
        compilation: parse_bool(first(tags, "compilation")),
        track_number,
        disc_number,
        disc_subtitle: first(tags, "discsubtitle"),
        catalog_num: first(tags, "catalognumber"),
        comment: first(tags, "comment"),
        bpm: parse_bpm(first(tags, "bpm")),
        lyrics: lyrics_json.to_owned(),
        explicit_status: map_explicit(non_empty_or(first(tags, "itunesadvisory"), first(tags, "explicit"))),
        original_year: year_from_date(&original_date),
        original_date,
        release_year: year_from_date(&release_date),
        release_date,
        year: year_from_date(&date),
        date,
        mbz_recording_id: first(tags, "musicbrainz_recordingid"),
        mbz_release_track_id: first(tags, "musicbrainz_trackid"),
        mbz_album_id: first(tags, "musicbrainz_albumid"),
        mbz_release_group_id: first(tags, "musicbrainz_releasegroupid"),
        mbz_album_type: first(tags, "releasetype"),
        rg_album_peak: parse_float(strip_db(first(tags, "replaygain_album_peak"))),
        rg_album_gain: map_gain(
            first(tags, "replaygain_album_gain"),
            first(tags, "r128_album_gain"),
        ),
        rg_track_peak: parse_float(strip_db(first(tags, "replaygain_track_peak"))),
        rg_track_gain: map_gain(
            first(tags, "replaygain_track_gain"),
            first(tags, "r128_track_gain"),
        ),
        participants: map_participants(tags, &artist, &album_artist),
    })
}

fn map_participants(
    tags: &HashMap<String, Vec<String>>,
    display_artist: &str,
    display_album_artist: &str,
) -> HashMap<String, Vec<ScanArtist>> {
    let mut out = HashMap::new();
    out.insert(
        "artist".to_owned(),
        artists_from_tags(tags, "artist", "artists", "artistsort", "musicbrainz_artistid"),
    );
    let mut album_artists = artists_from_tags(
        tags,
        "albumartist",
        "albumartists",
        "albumartistsort",
        "musicbrainz_albumartistid",
    );
    if album_artists.is_empty() {
        album_artists.push(scan_artist(display_album_artist, "", ""));
    }
    out.insert("albumartist".to_owned(), album_artists);

    for (role, key) in [
        ("composer", "composer"),
        ("conductor", "conductor"),
        ("lyricist", "lyricist"),
        ("arranger", "arranger"),
        ("producer", "producer"),
        ("director", "director"),
        ("engineer", "engineer"),
        ("mixer", "mixer"),
        ("remixer", "remixer"),
        ("djmixer", "djmixer"),
    ] {
        let artists = artists_from_tags(tags, key, key, &format!("{key}sort"), &format!("musicbrainz_{key}id"));
        if !artists.is_empty() {
            out.insert(role.to_owned(), artists);
        }
    }

    if out["artist"].is_empty() && !display_artist.is_empty() {
        out.insert("artist".to_owned(), vec![scan_artist(display_artist, "", "")]);
    }
    out
}

fn artists_from_tags(
    tags: &HashMap<String, Vec<String>>,
    single: &str,
    plural: &str,
    sort_key: &str,
    mbid_key: &str,
) -> Vec<ScanArtist> {
    let names = all(tags, plural);
    let names = if names.is_empty() { all(tags, single) } else { names };
    let sorts = all(tags, sort_key);
    let mbids = all(tags, mbid_key);
    names
        .into_iter()
        .enumerate()
        .map(|(idx, name)| {
            scan_artist(
                &name,
                sorts.get(idx).map(String::as_str).unwrap_or_default(),
                mbids.get(idx).map(String::as_str).unwrap_or_default(),
            )
        })
        .collect()
}

fn scan_artist(name: &str, sort: &str, mbid: &str) -> ScanArtist {
    ScanArtist {
        name: name.to_owned(),
        sort_artist_name: sort.to_owned(),
        order_artist_name: sanitize_sort_no_article(name),
        mbz_artist_id: mbid.to_owned(),
        sub_role: String::new(),
    }
}

fn display_artist(tags: &HashMap<String, Vec<String>>) -> String {
    let values = all(tags, "artists");
    let values = if values.is_empty() { all(tags, "artist") } else { values };
    join(values)
}

fn display_album_artist(tags: &HashMap<String, Vec<String>>) -> String {
    let values = all(tags, "albumartists");
    let values = if values.is_empty() {
        all(tags, "albumartist")
    } else {
        values
    };
    join(values)
}

fn join(values: Vec<String>) -> String {
    values.join("; ")
}

fn all(tags: &HashMap<String, Vec<String>>, key: &str) -> Vec<String> {
    tags.get(key)
        .cloned()
        .unwrap_or_default()
        .into_iter()
        .filter(|value| !value.is_empty())
        .collect()
}

fn non_empty_or(a: String, b: String) -> String {
    if a.is_empty() { b } else { a }
}

fn first(tags: &HashMap<String, Vec<String>>, key: &str) -> String {
    tags.get(key)
        .and_then(|values| values.first())
        .cloned()
        .unwrap_or_default()
}

fn parse_bool(value: String) -> bool {
    matches!(value.trim().to_ascii_lowercase().as_str(), "1" | "true" | "yes")
}

fn parse_bpm(value: String) -> Option<i32> {
    value.trim().parse::<f64>().ok().map(|v| v.round() as i32).filter(|&v| v != 0)
}

fn parse_float(value: String) -> Option<f64> {
    if value.is_empty() {
        return None;
    }
    value.parse().ok()
}

fn strip_db(value: String) -> String {
    value.replace("dB", "").replace("db", "").trim().to_owned()
}

fn map_gain(rg: String, r128: String) -> Option<f64> {
    if let Some(v) = parse_float(strip_db(rg)) {
        return Some(v);
    }
    if let Ok(v) = r128.trim().parse::<i64>() {
        return Some(v as f64 / 256.0 + 5.0);
    }
    None
}

fn map_explicit(value: String) -> String {
    match value.trim() {
        "1" | "4" => "e".to_owned(),
        "2" => "c".to_owned(),
        _ => String::new(),
    }
}

fn tuple(tags: &HashMap<String, Vec<String>>, key: &str, total_key: &str) -> (i32, i32) {
    let raw = first(tags, key);
    if raw.is_empty() {
        return (0, 0);
    }
    let mut parts = raw.split('/');
    let first_num = parts.next().unwrap_or_default().parse().unwrap_or(0);
    let second = parts
        .next()
        .and_then(|v| v.parse().ok())
        .or_else(|| first(tags, total_key).parse().ok())
        .unwrap_or(0);
    (first_num, second)
}

fn map_dates(tags: &HashMap<String, Vec<String>>) -> (String, String, String) {
    let mut original = first(tags, "originaldate");
    let mut release = first(tags, "releasedate");
    let mut date = first(tags, "date");
    if original.is_empty() {
        original = first(tags, "originalyear");
    }
    if release.is_empty() {
        release = first(tags, "releaseyear");
    }
    if date.is_empty() {
        date = first(tags, "year");
    }
    if !original.is_empty() && release.is_empty() && !date.is_empty() && date >= original {
        return (original, date.clone(), date);
    }
    let resolved = if !date.is_empty() {
        date.clone()
    } else if !original.is_empty() {
        original.clone()
    } else {
        release.clone()
    };
    (original, release, resolved)
}

fn year_from_date(value: &str) -> i32 {
    value.get(0..4).and_then(|y| y.parse().ok()).unwrap_or(0)
}

fn sanitize_sort(value: &str) -> String {
    value.trim().to_owned()
}

fn sanitize_sort_no_article(value: &str) -> String {
    let lower = value.trim().to_ascii_lowercase();
    for prefix in ["the ", "a ", "an "] {
        if lower.starts_with(prefix) {
            return value[prefix.len()..].trim().to_owned();
        }
    }
    value.trim().to_owned()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn maps_basic_tags() {
        let mut tags = HashMap::new();
        tags.insert("title".to_owned(), vec!["Song".to_owned()]);
        tags.insert("album".to_owned(), vec!["Album".to_owned()]);
        tags.insert("artist".to_owned(), vec!["Artist".to_owned()]);
        let json = map_to_json(&tags, Path::new("music/song.mp3"), Some("[]")).expect("json");
        assert!(json.contains("Song"));
        assert!(json.contains("Album"));
    }

    #[test]
    fn maps_legacy_release_date_mapping() {
        let mut tags = HashMap::new();
        tags.insert("title".to_owned(), vec!["Song".to_owned()]);
        tags.insert("album".to_owned(), vec!["Album".to_owned()]);
        tags.insert("date".to_owned(), vec!["2020-05-15".to_owned()]);
        tags.insert("originaldate".to_owned(), vec!["2019-02-10".to_owned()]);
        let json = map_to_json(&tags, Path::new("music/song.mp3"), Some("[]")).expect("json");
        assert!(json.contains(r#""releaseDate":"2020-05-15""#));
    }

    #[test]
    fn maps_properly_tagged_dates() {
        let mut tags = HashMap::new();
        tags.insert("title".to_owned(), vec!["Song".to_owned()]);
        tags.insert("album".to_owned(), vec!["Album".to_owned()]);
        tags.insert("originaldate".to_owned(), vec!["1978-09-10".to_owned()]);
        tags.insert("date".to_owned(), vec!["1977-03-04".to_owned()]);
        tags.insert("releasedate".to_owned(), vec!["2002-01-02".to_owned()]);
        let json = map_to_json(&tags, Path::new("music/song.mp3"), Some("[]")).expect("json");
        assert!(json.contains(r#""date":"1977-03-04""#));
        assert!(json.contains(r#""originalDate":"1978-09-10""#));
        assert!(json.contains(r#""releaseDate":"2002-01-02""#));
    }
}
