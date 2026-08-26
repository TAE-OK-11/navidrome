//! Embedded lyrics parsing for the Lofty metadata worker.
//!
//! Handles the common scan-path formats (LRC, plain text, SRT). TTML, Lyricsfile
//! YAML, and Enhanced LRC with word cues fall back to Go so OpenSubsonic karaoke
//! parity stays in one place.

use std::collections::HashMap;

use regex::Regex;
use serde::Serialize;
use serde_json::Value;

#[derive(Debug, Serialize)]
struct Cue {
    #[serde(skip_serializing_if = "Option::is_none")]
    start: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    end: Option<i64>,
    value: String,
    #[serde(rename = "byteStart")]
    byte_start: usize,
    #[serde(rename = "byteEnd")]
    byte_end: usize,
    #[serde(rename = "agentId", skip_serializing_if = "Option::is_none")]
    agent_id: Option<String>,
}

#[derive(Debug, Serialize)]
struct Line {
    #[serde(skip_serializing_if = "Option::is_none")]
    start: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    end: Option<i64>,
    value: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    cue: Vec<Cue>,
}

#[derive(Debug, Serialize)]
struct Lyrics {
    #[serde(rename = "displayArtist", skip_serializing_if = "String::is_empty")]
    display_artist: String,
    #[serde(rename = "displayTitle", skip_serializing_if = "String::is_empty")]
    display_title: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    kind: String,
    lang: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    agents: Vec<Value>,
    line: Vec<Line>,
    #[serde(skip_serializing_if = "Option::is_none")]
    offset: Option<i64>,
    synced: bool,
}

/// Returns JSON for `media_file.lyrics` when every lyric entry can be parsed in
/// Rust. Returns `None` when any entry needs the Go parsers (TTML / YAML / ELRC).
pub fn parse_tags_to_json(tags: &HashMap<String, Vec<String>>) -> Option<String> {
    let pairs = lyric_pairs(tags);
    if pairs.is_empty() {
        return Some("[]".to_owned());
    }

    let mut list = Vec::with_capacity(pairs.len());
    for (lang, text) in pairs {
        if needs_go_fallback(text) {
            return None;
        }
        let lyrics = parse_one(lang, text)?;
        if !lyrics.line.is_empty() {
            list.push(lyrics);
        }
    }
    serde_json::to_string(&list).ok()
}

fn lyric_pairs(tags: &HashMap<String, Vec<String>>) -> Vec<(String, &str)> {
    let mut pairs = Vec::new();
    for (key, values) in tags {
        let key = key.to_ascii_lowercase();
        if let Some(lang) = key.strip_prefix("lyrics:") {
            let lang = if lang.is_empty() || lang == "lyrics" {
                "xxx"
            } else {
                lang
            };
            for value in values {
                if !value.is_empty() {
                    pairs.push((normalize_lang(lang), value.as_str()));
                }
            }
        } else if key == "lyrics" || key == "unsyncedlyrics" {
            for value in values {
                if !value.is_empty() {
                    pairs.push(("xxx".to_owned(), value.as_str()));
                }
            }
        }
    }
    pairs
}

fn needs_go_fallback(text: &str) -> bool {
    let head = text.trim_start();
    if head.is_empty() {
        return false;
    }
    // TTML / XML
    if head.as_bytes().first().copied() == Some(b'<') {
        return true;
    }
    // Lyricsfile YAML
    if head.contains("version:")
        && (head.contains("\"1.0\"") || head.contains("'1.0'") || head.contains("version: 1.0"))
    {
        return true;
    }
    // Enhanced LRC word cues — keep Go for byte-offset karaoke parity.
    if ENHANCED_LRC.is_match(text) {
        return true;
    }
    false
}

fn parse_one(lang: String, text: &str) -> Option<Lyrics> {
    let text = sanitize_text(text);
    if text.trim().is_empty() {
        return None;
    }
    if text.contains("-->") {
        return parse_srt(lang, &text);
    }
    Some(parse_lrc(lang, &text))
}

fn parse_lrc(mut lang: String, text: &str) -> Lyrics {
    let synced = SYNC_LRC.is_match(text);
    let mut lines = Vec::new();
    let mut artist = String::new();
    let mut title = String::new();
    let mut offset = None;
    let mut prior = String::new();
    let mut valid = false;
    let mut timestamps: Vec<i64> = Vec::new();
    let mut repeated = false;

    for raw_line in text.split('\n') {
        let line = raw_line.trim();
        if line.is_empty() {
            if valid {
                prior.push('\n');
            }
            continue;
        }
        if synced {
            if let Some(caps) = LRC_ID.captures(line) {
                let key = caps.get(1).map(|m| m.as_str()).unwrap_or_default();
                let value = caps.get(2).map(|m| m.as_str().trim()).unwrap_or_default();
                match key {
                    "ar" => artist = sanitize_text(value),
                    "ti" => title = sanitize_text(value),
                    "lang" => lang = normalize_lang(value),
                    "offset" => {
                        if let Ok(v) = value.parse::<i64>() {
                            offset = Some(v);
                        }
                    }
                    _ => {}
                }
                continue;
            }

            let matches: Vec<_> = TIME_LRC.find_iter(line).collect();
            if matches.len() > 1 {
                repeated = true;
            }
            if matches.is_empty() || matches[0].start() != 0 {
                if valid {
                    prior.push('\n');
                    prior.push_str(line);
                }
                continue;
            }

            if valid {
                for &ts in &timestamps {
                    lines.push(Line {
                        start: Some(ts),
                        end: None,
                        value: prior.trim().to_owned(),
                        cue: Vec::new(),
                    });
                }
                timestamps.clear();
            }

            let mut end = 0;
            for m in matches {
                if end != 0 {
                    let middle = line[end..m.start()].trim();
                    if !middle.is_empty() {
                        break;
                    }
                }
                end = m.end();
                if let Some(ms) = parse_lrc_time(m.as_str()) {
                    timestamps.push(ms);
                }
            }
            prior = if end >= line.len() {
                String::new()
            } else {
                line[end..].trim().to_owned()
            };
            valid = true;
        } else {
            lines.push(Line {
                start: None,
                end: None,
                value: line.to_owned(),
                cue: Vec::new(),
            });
        }
    }

    if valid {
        for &ts in &timestamps {
            lines.push(Line {
                start: Some(ts),
                end: None,
                value: prior.trim().to_owned(),
                cue: Vec::new(),
            });
        }
    }

    if repeated {
        lines.sort_by_key(|line| line.start.unwrap_or(0));
    }

    Lyrics {
        display_artist: artist,
        display_title: title,
        kind: String::new(),
        lang: normalize_lang(&lang),
        agents: Vec::new(),
        line: lines,
        offset,
        synced,
    }
}

fn parse_srt(lang: String, text: &str) -> Option<Lyrics> {
    let text = text.replace("\r\n", "\n").replace('\r', "\n");
    let mut lines = Vec::new();
    for block in text.split("\n\n") {
        let block = block.trim();
        if block.is_empty() {
            continue;
        }
        let mut parts = block.lines().map(str::trim).filter(|l| !l.is_empty());
        let first = parts.next()?;
        let timing = if first.chars().all(|c| c.is_ascii_digit()) {
            parts.next()?
        } else {
            first
        };
        let Some((start_raw, end_raw)) = timing.split_once("-->") else {
            continue;
        };
        let start = parse_srt_time(start_raw.trim())?;
        let end = parse_srt_time(end_raw.trim().split_whitespace().next().unwrap_or(""))?;
        let value = parts.collect::<Vec<_>>().join("\n");
        if value.is_empty() {
            continue;
        }
        lines.push(Line {
            start: Some(start),
            end: Some(end),
            value: sanitize_text(&value),
            cue: Vec::new(),
        });
    }
    if lines.is_empty() {
        return None;
    }
    Some(Lyrics {
        display_artist: String::new(),
        display_title: String::new(),
        kind: String::new(),
        lang: normalize_lang(&lang),
        agents: Vec::new(),
        line: lines,
        offset: None,
        synced: true,
    })
}

fn parse_lrc_time(token: &str) -> Option<i64> {
    let inner = token.trim().trim_start_matches('[').trim_end_matches(']');
    let mut parts = inner.split(':');
    let first = parts.next()?;
    let second = parts.next()?;
    let (hours, minutes, seconds_raw) = if let Some(third) = parts.next() {
        (
            first.parse::<i64>().ok()?,
            second.parse::<i64>().ok()?,
            third,
        )
    } else {
        (0, first.parse::<i64>().ok()?, second)
    };
    let (secs, frac) = match seconds_raw.split_once('.') {
        Some((s, f)) => (s.parse::<i64>().ok()?, f),
        None => (seconds_raw.parse::<i64>().ok()?, ""),
    };
    let mut ms = hours * 3_600_000 + minutes * 60_000 + secs * 1000;
    if !frac.is_empty() {
        let padded = format!("{frac:0<3}");
        ms += padded[..3].parse::<i64>().ok()?;
    }
    Some(ms)
}

fn parse_srt_time(value: &str) -> Option<i64> {
    // 00:00:01,000 or 00:00:01.000
    let value = value.trim();
    let (hms, frac) = value
        .split_once(',')
        .or_else(|| value.split_once('.'))?;
    let mut parts = hms.split(':');
    let h: i64 = parts.next()?.parse().ok()?;
    let m: i64 = parts.next()?.parse().ok()?;
    let s: i64 = parts.next()?.parse().ok()?;
    let padded = format!("{frac:0<3}");
    let ms: i64 = padded[..3.min(padded.len())].parse().ok()?;
    Some(h * 3_600_000 + m * 60_000 + s * 1000 + ms)
}

fn normalize_lang(lang: &str) -> String {
    let lang = lang.trim().to_ascii_lowercase();
    if lang.is_empty() {
        "xxx".to_owned()
    } else {
        lang
    }
}

/// Lightweight stand-in for Go's bluemonday UGC sanitize on lyric payloads:
/// strip tags and unescape a few common entities. Typical LRC has no HTML.
fn sanitize_text(text: &str) -> String {
    let no_tags = TAG_RE.replace_all(text, "");
    no_tags
        .replace("&amp;", "&")
        .replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&quot;", "\"")
        .replace("&#39;", "'")
        .replace("&apos;", "'")
}

use std::sync::LazyLock;

static SYNC_LRC: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"(?m)(^|\n)\s*\[(?:[0-9]{1,2}:)?[0-9]{1,2}:[0-9]{1,2}(?:\.[0-9]{1,3})?\]").unwrap());
static TIME_LRC: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"\[(?:[0-9]{1,2}:)?[0-9]{1,2}:[0-9]{1,2}(?:\.[0-9]{1,3})?\]").unwrap());
static LRC_ID: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"^\[(ar|ti|offset|lang):([^\]]+)\]").unwrap());
static ENHANCED_LRC: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"<(?:[0-9]{1,2}:)?[0-9]{1,2}:[0-9]{1,2}(?:\.[0-9]{1,3})?>").unwrap());
static TAG_RE: LazyLock<Regex> = LazyLock::new(|| Regex::new(r"<[^>]*>").unwrap());

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_synced_lrc() {
        let mut tags = HashMap::new();
        tags.insert(
            "lyrics".to_owned(),
            vec!["[00:01.00]hello\n[00:02.50]world".to_owned()],
        );
        let json = parse_tags_to_json(&tags).unwrap();
        assert!(json.contains("\"synced\":true"));
        assert!(json.contains("hello"));
        assert!(json.contains("1000"));
    }

    #[test]
    fn falls_back_for_ttml() {
        let mut tags = HashMap::new();
        tags.insert(
            "lyrics".to_owned(),
            vec![r#"<tt xmlns="http://www.w3.org/ns/ttml"><body/></tt>"#.to_owned()],
        );
        assert!(parse_tags_to_json(&tags).is_none());
    }

    #[test]
    fn falls_back_for_enhanced_lrc() {
        let mut tags = HashMap::new();
        tags.insert(
            "lyrics".to_owned(),
            vec!["[00:01.00]<00:01.00>Lead <00:01.50>words".to_owned()],
        );
        assert!(parse_tags_to_json(&tags).is_none());
    }

    #[test]
    fn parses_srt() {
        let mut tags = HashMap::new();
        tags.insert(
            "lyrics:eng".to_owned(),
            vec!["1\n00:00:01,000 --> 00:00:02,000\nFirst\n".to_owned()],
        );
        let json = parse_tags_to_json(&tags).unwrap();
        assert!(json.contains("\"lang\":\"eng\""));
        assert!(json.contains("First"));
    }
}
