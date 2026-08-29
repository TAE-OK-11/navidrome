use std::collections::{HashMap, HashSet};

use regex::Regex;
use serde::Deserialize;

#[derive(Debug, Deserialize, Clone)]
pub struct TagMappingConfig {
    #[serde(default)]
    pub aliases: Vec<String>,
    #[serde(rename = "type", default)]
    pub tag_type: String,
    #[serde(default)]
    pub max_length: usize,
    #[serde(default)]
    pub split: Vec<String>,
    #[serde(default)]
    pub album: bool,
}

const ZWSP: char = '\u{200b}';
const DEFAULT_MAX_TAG_LENGTH: usize = 1024;

static DATE_REGEX: std::sync::LazyLock<Regex> =
    std::sync::LazyLock::new(|| Regex::new(r"([12]\d\d\d)").expect("date regex"));
static VORBIS_PAIR_REGEX: std::sync::LazyLock<Regex> = std::sync::LazyLock::new(|| {
    Regex::new(r"\(([^()]+(?:\([^()]*\)[^()]*)*)\)").expect("vorbis pair regex")
});

pub fn clean(
    file_path: &str,
    raw_tags: &HashMap<String, Vec<String>>,
    mappings: &HashMap<String, TagMappingConfig>,
    artist_split_exceptions: &[String],
) -> HashMap<String, Vec<String>> {
    let lowered = lower_tags(raw_tags);
    let exceptions_rx = compile_exceptions_regex(artist_split_exceptions);
    let mut cleaned = HashMap::new();

    for (name, mapping) in mappings {
        let values = match mapping.tag_type.as_str() {
            "pair" => process_pair_mapping(name, mapping, &lowered),
            _ => process_regular_mapping(mapping, &lowered, participant_exceptions(name, &exceptions_rx)),
        };
        if !values.is_empty() {
            cleaned.insert(name.clone(), values);
        }
    }

    filter_empty_tags(&mut cleaned);
    sanitize_all(file_path, &mut cleaned, mappings)
}

fn participant_exceptions(name: &str, exceptions_rx: &Option<Regex>) -> Option<Regex> {
    if is_participant_tag(name) {
        exceptions_rx.clone()
    } else {
        None
    }
}

fn is_participant_tag(name: &str) -> bool {
    matches!(
        name,
        "artist"
            | "artists"
            | "artistsort"
            | "artistssort"
            | "albumartist"
            | "albumartists"
            | "albumartistsort"
            | "albumartistssort"
    ) || name.ends_with("sort")
}

fn lower_tags(raw_tags: &HashMap<String, Vec<String>>) -> HashMap<String, Vec<String>> {
    let mut lowered = HashMap::new();
    for (key, values) in raw_tags {
        lowered.insert(key.to_ascii_lowercase(), values.clone());
    }
    lowered
}

fn process_regular_mapping(
    mapping: &TagMappingConfig,
    lowered: &HashMap<String, Vec<String>>,
    exceptions_rx: Option<Regex>,
) -> Vec<String> {
    let mut values = Vec::new();
    for alias in &mapping.aliases {
        if let Some(vs) = lowered.get(alias) {
            values.extend(split_tag_values(mapping, vs, exceptions_rx.as_ref()));
        }
    }
    values
}

fn split_tag_values(
    mapping: &TagMappingConfig,
    values: &[String],
    exceptions_rx: Option<&Regex>,
) -> Vec<String> {
    if mapping.split.is_empty() {
        return values.to_vec();
    }
    let split_rx = compile_split_regex(&mapping.split);
    let mut result = Vec::new();
    for value in values {
        result.extend(split_value(value, split_rx.as_ref(), exceptions_rx));
    }
    result
}

fn split_value(value: &str, split_rx: Option<&Regex>, exceptions_rx: Option<&Regex>) -> Vec<String> {
    let Some(split_rx) = split_rx else {
        return vec![value.trim().to_owned()];
    };
    let protected = protected_spans(value, exceptions_rx);
    let mut parts = Vec::new();
    let mut start = 0usize;
    for mat in split_rx.find_iter(value) {
        let span = (mat.start(), mat.end());
        if overlaps_any(span, &protected) {
            continue;
        }
        parts.push(value[start..span.0].trim().to_owned());
        start = span.1;
    }
    parts.push(value[start..].trim().to_owned());
    parts.into_iter().filter(|part| !part.is_empty()).collect()
}

fn compile_split_regex(split: &[String]) -> Option<Regex> {
    let escaped: Vec<String> = split
        .iter()
        .filter(|s| !s.is_empty())
        .map(|s| regex::escape(s))
        .collect();
    if escaped.is_empty() {
        return None;
    }
    Regex::new(&format!("(?i)({})", escaped.join("|"))).ok()
}

fn compile_exceptions_regex(exceptions: &[String]) -> Option<Regex> {
    let mut names: Vec<String> = exceptions
        .iter()
        .map(|s| s.trim())
        .filter(|s| !s.is_empty())
        .map(ToOwned::to_owned)
        .collect();
    if names.is_empty() {
        return None;
    }
    names.sort_by(|a, b| b.len().cmp(&a.len()).then_with(|| a.cmp(b)));
    let escaped: Vec<String> = names.iter().map(|n| regex::escape(n)).collect();
    Regex::new(&format!("(?i)({})", escaped.join("|"))).ok()
}

fn protected_spans(tag: &str, rx: Option<&Regex>) -> Vec<(usize, usize)> {
    let Some(rx) = rx else {
        return Vec::new();
    };
    rx.find_iter(tag)
        .filter(|mat| is_word_bounded(tag, mat.start(), mat.end()))
        .map(|mat| (mat.start(), mat.end()))
        .collect()
}

fn is_word_bounded(text: &str, start: usize, end: usize) -> bool {
    let before = text[..start].chars().last();
    let after = text[end..].chars().next();
    !before.is_some_and(|c| c.is_alphanumeric())
        && !after.is_some_and(|c| c.is_alphanumeric())
}

fn overlaps_any(span: (usize, usize), spans: &[(usize, usize)]) -> bool {
    spans
        .iter()
        .any(|other| span.0 < other.1 && other.0 < span.1)
}

fn process_pair_mapping(
    name: &str,
    mapping: &TagMappingConfig,
    lowered: &HashMap<String, Vec<String>>,
) -> Vec<String> {
    let mut alias_values = Vec::new();
    for alias in &mapping.aliases {
        if let Some(vs) = lowered.get(alias) {
            alias_values.extend(vs.clone());
        }
    }

    let mut pairs = parse_id3_pairs(name, lowered);
    if !alias_values.is_empty() {
        if name == "lyrics" {
            for value in alias_values {
                pairs.push(new_pair("xxx", &value));
            }
        } else {
            pairs.extend(parse_vorbis_pairs(&alias_values));
        }
    }
    pairs
}

fn parse_id3_pairs(name: &str, lowered: &HashMap<String, Vec<String>>) -> Vec<String> {
    let prefix = format!("{name}:");
    let mut pairs = Vec::new();
    for (tag_key, tag_values) in lowered {
        if let Some(mut key_part) = tag_key.strip_prefix(&prefix) {
            if key_part == name {
                key_part = "";
            }
            for value in tag_values {
                pairs.push(new_pair(key_part, value));
            }
        }
    }
    pairs
}

fn parse_vorbis_pairs(values: &[String]) -> Vec<String> {
    let mut pairs = Vec::new();
    for value in values {
        let matches: Vec<_> = VORBIS_PAIR_REGEX.captures_iter(value).collect();
        if matches.is_empty() {
            pairs.push(new_pair("", value));
            continue;
        }
        let key = matches[0][1].trim().to_ascii_lowercase();
        let replaced = value.replacen(&format!("({})", &matches[0][1]), "", 1);
        let value_without_key = replaced.trim();
        pairs.push(new_pair(&key, value_without_key));
    }
    pairs
}

fn new_pair(key: &str, value: &str) -> String {
    format!("{key}{ZWSP}{value}")
}

fn filter_empty_tags(tags: &mut HashMap<String, Vec<String>>) {
    tags.retain(|_, values| {
        let cleaned = filter_duplicated_or_empty_values(values);
        if cleaned.is_empty() {
            false
        } else {
            *values = cleaned;
            true
        }
    });
}

fn filter_duplicated_or_empty_values(values: &[String]) -> Vec<String> {
    let mut seen = HashSet::new();
    let mut result = Vec::new();
    for value in values {
        if value.is_empty() || !seen.insert(value.clone()) {
            continue;
        }
        result.push(value.clone());
    }
    result
}

fn sanitize_all(
    file_path: &str,
    tags: &mut HashMap<String, Vec<String>>,
    mappings: &HashMap<String, TagMappingConfig>,
) -> HashMap<String, Vec<String>> {
    let mut cleaned = HashMap::new();
    for (name, values) in tags.drain() {
        let Some(mapping) = mappings.get(&name) else {
            continue;
        };
        let mut sanitized_values = Vec::new();
        for value in values {
            if let Some(cleaned_value) = sanitize(file_path, &name, mapping, &value) {
                sanitized_values.push(cleaned_value);
            }
        }
        if !sanitized_values.is_empty() {
            cleaned.insert(name, sanitized_values);
        }
    }
    cleaned
}

fn sanitize(file_path: &str, tag_name: &str, mapping: &TagMappingConfig, value: &str) -> Option<String> {
    let mut value = value.replace('\u{FFFD}', "");
    let max_length = if mapping.max_length > 0 {
        mapping.max_length
    } else {
        DEFAULT_MAX_TAG_LENGTH
    };
    if value.len() > max_length {
        value = truncate_utf8_bytes(&value, max_length);
    }

    match mapping.tag_type.as_str() {
        "date" => {
            let parsed = parse_date(file_path, tag_name, &value);
            if parsed.is_empty() {
                None
            } else {
                Some(parsed)
            }
        }
        "int" => value.parse::<i64>().ok().map(|_| value),
        "float" => value.parse::<f64>().ok().map(|_| value),
        "uuid" => uuid::Uuid::parse_str(&value).ok().map(|_| value),
        _ => Some(value),
    }
}

fn truncate_utf8_bytes(value: &str, max_bytes: usize) -> String {
    if value.len() <= max_bytes {
        return value.to_owned();
    }
    let mut end = max_bytes;
    while end > 0 && !value.is_char_boundary(end) {
        end -= 1;
    }
    value[..end].to_owned()
}

fn parse_date(file_path: &str, tag_name: &str, tag_value: &str) -> String {
    if tag_value.len() < 4 {
        return String::new();
    }
    let Some(caps) = DATE_REGEX.captures(tag_value) else {
        let _ = (file_path, tag_name);
        return String::new();
    };
    let year = caps[1].to_owned();
    if tag_value.len() < 5 {
        return year;
    }
    let truncated = &tag_value[..tag_value.len().min(10)];
    if Regex::new(r"^\d{4}-\d{2}-\d{2}$").is_ok_and(|rx| rx.is_match(truncated))
        && !truncated.ends_with("-00-00")
        && !truncated.contains("-00")
    {
        return truncated.to_owned();
    }
    let month = &tag_value[..tag_value.len().min(7)];
    if Regex::new(r"^\d{4}-\d{2}$").is_ok_and(|rx| rx.is_match(month)) {
        return month.to_owned();
    }
    year
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cleans_basic_tags() {
        let mut mappings = HashMap::new();
        mappings.insert(
            "artist".to_owned(),
            TagMappingConfig {
                aliases: vec!["tpe1".to_owned(), "artist".to_owned()],
                tag_type: String::new(),
                max_length: 0,
                split: Vec::new(),
                album: false,
            },
        );
        mappings.insert(
            "album".to_owned(),
            TagMappingConfig {
                aliases: vec!["album".to_owned()],
                tag_type: String::new(),
                max_length: 0,
                split: Vec::new(),
                album: false,
            },
        );
        let raw = HashMap::from([
            ("TPE1".to_owned(), vec!["Artist Name".to_owned(), "".to_owned()]),
            ("Album".to_owned(), vec!["Album Name".to_owned(), "".to_owned()]),
            ("UNKNOWN".to_owned(), vec!["x".to_owned()]),
        ]);
        let cleaned = clean("file.mp3", &raw, &mappings, &[]);
        assert_eq!(cleaned.get("artist").unwrap(), &vec!["Artist Name".to_owned()]);
        assert_eq!(cleaned.get("album").unwrap(), &vec!["Album Name".to_owned()]);
    }
}
