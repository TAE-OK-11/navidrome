use std::collections::HashMap;
use std::path::Path;

use serde::Deserialize;

use crate::id_hash::new_hash;

#[derive(Debug, Clone, Deserialize)]
pub struct PidConfig {
    pub track: String,
    pub album: String,
    #[serde(default)]
    pub group_album_releases: bool,
}

#[derive(Debug, Clone)]
pub struct PidInput<'a> {
    pub library_id: i32,
    pub path: &'a str,
    pub title: &'a str,
    pub album: &'a str,
    pub album_artist: &'a str,
    pub track_number: i32,
    pub disc_number: i32,
    pub tags: &'a HashMap<String, Vec<String>>,
    pub mbz_recording_id: &'a str,
    pub mbz_release_track_id: &'a str,
    pub mbz_album_id: &'a str,
    pub mbz_album_comment: &'a str,
    pub release_date: &'a str,
}

pub struct PidOutput {
    pub track_pid: String,
    pub album_id: String,
}

pub fn compute_pids(input: &PidInput<'_>, config: &PidConfig) -> PidOutput {
    PidOutput {
        track_pid: compute_pid(input, &config.track, true, config),
        album_id: compute_pid(input, &config.album, true, config),
    }
}

fn compute_pid(input: &PidInput<'_>, spec: &str, prepend_lib_id: bool, config: &PidConfig) -> String {
    match spec {
        "track_legacy" | "album_legacy" => String::new(),
        _ => {
            let mut pid_body = String::new();
            for field in spec.split('|') {
                let attributes: Vec<&str> = field.split(',').map(str::trim).collect();
                let mut values = Vec::with_capacity(attributes.len());
                let mut has_value = false;
                for attr in attributes {
                    let value = pid_attr(input, attr, spec, prepend_lib_id, config);
                    if !value.is_empty() {
                        has_value = true;
                    }
                    values.push(value);
                }
                if has_value {
                    pid_body = values.join("\\");
                    break;
                }
            }
            let hash_input = if prepend_lib_id {
                format!("{}\\{}", input.library_id, pid_body)
            } else {
                pid_body
            };
            new_hash(&[&hash_input])
        }
    }
}

fn pid_attr(input: &PidInput<'_>, attr: &str, spec: &str, prepend_lib_id: bool, config: &PidConfig) -> String {
    let attr = attr.trim().to_ascii_lowercase();
    match attr.as_str() {
        "albumid" => {
            if spec == config.album {
                return String::new();
            }
            compute_pid(input, &config.album, prepend_lib_id, config)
        }
        "folder" => parent_folder(input.path),
        "albumartistid" => new_hash(&[&clear_ascii(&input.album_artist.to_ascii_lowercase())]),
        "title" => input.title.to_owned(),
        "album" => clear_ascii(&tag_value(input, "album").to_ascii_lowercase()),
        _ => tag_value(input, &attr),
    }
}

fn parent_folder(path: &str) -> String {
    Path::new(path)
        .parent()
        .and_then(|p| p.to_str())
        .unwrap_or("")
        .to_owned()
}

fn tag_value(input: &PidInput<'_>, tag: &str) -> String {
    if let Some(values) = input.tags.get(tag) {
        if let Some(first) = values.first() {
            if !first.is_empty() {
                return first.clone();
            }
        }
    }
    match tag {
        "album" => input.album.to_owned(),
        "title" => input.title.to_owned(),
        "tracknumber" => {
            if input.track_number > 0 {
                return input.track_number.to_string();
            }
            String::new()
        }
        "discnumber" => {
            if input.disc_number > 0 {
                return input.disc_number.to_string();
            }
            String::new()
        }
        "musicbrainz_recordingid" => input.mbz_recording_id.to_owned(),
        "musicbrainz_trackid" => input.mbz_release_track_id.to_owned(),
        "musicbrainz_albumid" => input.mbz_album_id.to_owned(),
        "albumversion" => input.mbz_album_comment.to_owned(),
        "releasedate" => input.release_date.to_owned(),
        _ => String::new(),
    }
}

fn clear_ascii(value: &str) -> String {
    let mut out = String::with_capacity(value.len());
    for ch in value.chars() {
        out.push(match ch {
            '‘' | '’' | '‛' | '′' => '\'',
            '＂' | '〃' | 'ˮ' | 'ײ' | '᳓' | '″' | '‶' | '˶' | 'ʺ' | '“' | '”' | '˝' | '‟' => '"',
            '‐' | '–' | '—' | '−' | '―' => '-',
            other => other,
        });
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn sample_input<'a>(tags: &'a HashMap<String, Vec<String>>) -> PidInput<'a> {
        PidInput {
            library_id: 42,
            path: "/path/to/file.mp3",
            title: "Title",
            album: "Album Name",
            album_artist: "Album Artist",
            track_number: 1,
            disc_number: 1,
            tags,
            mbz_recording_id: "",
            mbz_release_track_id: "",
            mbz_album_id: "",
            mbz_album_comment: "",
            release_date: "",
        }
    }

    #[test]
    fn computes_track_pid_with_prepend() {
        let tags = HashMap::from([("album".to_owned(), vec!["Test Album".to_owned()])]);
        let input = sample_input(&tags);
        let config = PidConfig {
            track: "album".to_owned(),
            album: "album".to_owned(),
            group_album_releases: false,
        };
        let out = compute_pids(&input, &config);
        assert_eq!(out.track_pid, new_hash(&["42\\test album"]));
    }
}
