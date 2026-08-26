//! Embedded lyrics parsing for the Lofty metadata worker.
//!
//! Handles the common scan-path formats (LRC, plain text, SRT, Enhanced LRC,
//! and full TTML). All common embedded lyric formats parse in Rust.

use std::collections::HashMap;
use std::sync::LazyLock;

use regex::Regex;
use serde::Serialize;

static SYNC_LRC: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"(?m)(^|\n)\s*\[(?:[0-9]{1,2}:)?[0-9]{1,2}:[0-9]{1,2}(?:\.[0-9]{1,3})?\]").unwrap());
static TIME_LRC: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"\[(?:[0-9]{1,2}:)?[0-9]{1,2}:[0-9]{1,2}(?:\.[0-9]{1,3})?\]").unwrap());
static LRC_ID: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"^\[(ar|ti|offset|lang):([^\]]+)\]").unwrap());
static ENHANCED_LRC: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"<(?:[0-9]{1,2}:)?[0-9]{1,2}:[0-9]{1,2}(?:\.[0-9]{1,3})?>").unwrap());
static HTML_TAG_RE: LazyLock<Regex> = LazyLock::new(|| Regex::new(r"</?[A-Za-z][^>]*>").unwrap());

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub(crate) struct Agent {
    pub(crate) id: String,
    pub(crate) role: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub(crate) name: String,
}

#[derive(Debug, Clone, Serialize)]
pub(crate) struct Cue {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) start: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) end: Option<i64>,
    pub(crate) value: String,
    #[serde(rename = "byteStart")]
    pub(crate) byte_start: usize,
    #[serde(rename = "byteEnd")]
    pub(crate) byte_end: usize,
    #[serde(rename = "agentId", skip_serializing_if = "Option::is_none")]
    pub(crate) agent_id: Option<String>,
}

#[derive(Debug, Clone, Serialize)]
pub(crate) struct Line {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) start: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) end: Option<i64>,
    pub(crate) value: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub(crate) cue: Vec<Cue>,
}

#[derive(Debug, Serialize)]
pub(crate) struct Lyrics {
    #[serde(rename = "displayArtist", skip_serializing_if = "String::is_empty")]
    pub(crate) display_artist: String,
    #[serde(rename = "displayTitle", skip_serializing_if = "String::is_empty")]
    pub(crate) display_title: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub(crate) kind: String,
    pub(crate) lang: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub(crate) agents: Vec<Agent>,
    pub(crate) line: Vec<Line>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) offset: Option<i64>,
    pub(crate) synced: bool,
}

/// Returns JSON for `media_file.lyrics` when every lyric entry can be parsed in
/// Rust. Returns `None` only when parsing fails entirely.
pub fn parse_tags_to_json(tags: &HashMap<String, Vec<String>>) -> Option<String> {
    let pairs = lyric_pairs(tags);
    if pairs.is_empty() {
        return Some("[]".to_owned());
    }

    let mut list = Vec::new();
    for (lang, text) in pairs {
        let tracks = parse_one(lang, text)?;
        for lyrics in tracks {
            if !lyrics.line.is_empty() {
                list.push(lyrics);
            }
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

fn parse_one(lang: String, text: &str) -> Option<Vec<Lyrics>> {
    // TTML must keep markup; sanitize_text strips tags for LRC/SRT/plain.
    if crate::ttml::looks_like_ttml(text) {
        return crate::ttml::parse_ttml_list(&lang, text);
    }
    if crate::lyricsfile::looks_like_lyricsfile(text) {
        return crate::lyricsfile::parse_lyricsfile(&lang, text);
    }
    let text = sanitize_text(text);
    if text.trim().is_empty() {
        return None;
    }
    if text.contains("-->") {
        return parse_srt(lang, &text).map(|lyrics| vec![lyrics]);
    }
    Some(vec![parse_lrc(lang, &text)])
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
                let (value, base_cues) = parse_enhanced_line(&prior);
                for (idx, &ts) in timestamps.iter().enumerate() {
                    let offset = if idx == 0 {
                        0
                    } else {
                        ts - timestamps[0]
                    };
                    lines.push(Line {
                        start: Some(ts),
                        end: None,
                        value: value.clone(),
                        cue: shift_elrc_cues(&base_cues, offset),
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
        let (value, base_cues) = parse_enhanced_line(&prior);
        for (idx, &ts) in timestamps.iter().enumerate() {
            let offset = if idx == 0 { 0 } else { ts - timestamps[0] };
            lines.push(Line {
                start: Some(ts),
                end: None,
                value: value.clone(),
                cue: shift_elrc_cues(&base_cues, offset),
            });
        }
    }

    if repeated {
        lines.sort_by_key(|line| line.start.unwrap_or(0));
    }

    normalize_cue_lines(&mut lines);

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

fn parse_enhanced_line(text: &str) -> (String, Vec<Cue>) {
    let matches: Vec<_> = ENHANCED_LRC.find_iter(text).collect();
    if matches.is_empty() {
        return (text.trim().to_owned(), Vec::new());
    }

    struct Segment {
        start: i64,
        raw_start: usize,
        raw_end: usize,
    }

    let mut segments = Vec::new();
    let mut raw_value = String::new();
    let mut trailing_end: Option<i64> = None;

    for (i, m) in matches.iter().enumerate() {
        let inner = &text[m.start() + 1..m.end() - 1];
        let Some(time_ms) = parse_lrc_time(&format!("[{inner}]")) else {
            continue;
        };
        let text_start = m.end();
        let text_end = if i + 1 < matches.len() {
            matches[i + 1].start()
        } else {
            text.len()
        };
        let word = &text[text_start..text_end];
        if word.is_empty() {
            if i + 1 == matches.len() {
                trailing_end = Some(time_ms);
            }
            continue;
        }
        let raw_start = raw_value.len();
        raw_value.push_str(word);
        segments.push(Segment {
            start: time_ms,
            raw_start,
            raw_end: raw_value.len(),
        });
    }

    if segments.is_empty() {
        let stripped = ENHANCED_LRC.replace_all(text, "");
        return (stripped.trim().to_owned(), Vec::new());
    }

    let left_trim = raw_value.len() - raw_value.trim_start().len();
    let right_trim = raw_value.len() - raw_value.trim_end().len();
    let trimmed_end = raw_value.len().saturating_sub(right_trim).max(left_trim);

    let mut cues = Vec::with_capacity(segments.len());
    for seg in segments {
        let byte_start = seg.raw_start.max(left_trim);
        let byte_end = seg.raw_end.min(trimmed_end);
        if byte_start >= byte_end {
            continue;
        }
        cues.push(Cue {
            start: Some(seg.start),
            end: None,
            value: raw_value[byte_start..byte_end].to_owned(),
            byte_start: byte_start - left_trim,
            byte_end: byte_end - left_trim - 1,
            agent_id: None,
        });
    }
    if let (Some(end), true) = (trailing_end, !cues.is_empty()) {
        cues.last_mut().unwrap().end = Some(end);
    }
    (raw_value.trim().to_owned(), cues)
}

fn shift_elrc_cues(base: &[Cue], offset_ms: i64) -> Vec<Cue> {
    if base.is_empty() {
        return Vec::new();
    }
    base.iter()
        .map(|cue| Cue {
            start: cue.start.map(|s| s + offset_ms),
            end: cue.end.map(|e| e + offset_ms),
            value: cue.value.clone(),
            byte_start: cue.byte_start,
            byte_end: cue.byte_end,
            agent_id: cue.agent_id.clone(),
        })
        .collect()
}

fn normalize_cue_lines(lines: &mut [Line]) {
    for i in 0..lines.len() {
        if lines[i].cue.is_empty() {
            continue;
        }
        let fallback_end = lines[i].end.or_else(|| {
            lines
                .get(i + 1)
                .and_then(|next| next.start)
        });
        let cues = &mut lines[i].cue;
        for j in 0..cues.len() {
            if cues[j].end.is_none() {
                cues[j].end = cues
                    .get(j + 1)
                    .and_then(|next| next.start)
                    .or(fallback_end);
            }
        }
        if lines[i].start.is_none() {
            lines[i].start = cues.iter().filter_map(|c| c.start).min();
        }
        if lines[i].end.is_none() {
            lines[i].end = cues
                .iter()
                .filter_map(|c| c.end.or(c.start))
                .max();
        }
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
/// strip HTML tags (not Enhanced LRC `<mm:ss>` markers) and unescape entities.
fn sanitize_text(text: &str) -> String {
    let no_tags = HTML_TAG_RE.replace_all(text, "");
    no_tags
        .replace("&amp;", "&")
        .replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&quot;", "\"")
        .replace("&#39;", "'")
        .replace("&apos;", "'")
}

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
    fn parses_enhanced_lrc_word_cues() {
        let mut tags = HashMap::new();
        tags.insert(
            "lyrics".to_owned(),
            vec!["[00:01.00]<00:01.00>Lead <00:01.50>words".to_owned()],
        );
        let json = parse_tags_to_json(&tags).expect("ELRC should parse in Rust");
        assert!(json.contains("\"synced\":true"));
        assert!(json.contains("Lead words"));
        assert!(json.contains("byteStart"));
        assert!(json.contains("1000"));
        assert!(json.contains("1500"));
    }

    #[test]
    fn parses_frame_rate_ttml_via_tags() {
        let mut tags = HashMap::new();
        tags.insert(
            "lyrics:eng".to_owned(),
            vec![r#"<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttp="http://www.w3.org/ns/ttml#parameter" ttp:frameRate="30"><body><div><p begin="00:00:01:00">Hi</p></div></body></tt>"#.to_owned()],
        );
        let json = parse_tags_to_json(&tags).expect("frame-rate TTML");
        assert!(json.contains("Hi"));
        assert!(json.contains("1000"));
    }

    #[test]
    fn parses_simple_ttml_via_tags() {
        let mut tags = HashMap::new();
        tags.insert(
            "lyrics:eng".to_owned(),
            vec![r#"<tt xmlns="http://www.w3.org/ns/ttml" xml:lang="eng"><body><div><p begin="00:00:01.000" end="00:00:02.000">Hello</p></div></body></tt>"#.to_owned()],
        );
        let json = parse_tags_to_json(&tags).expect("simple TTML");
        assert!(json.contains("Hello"));
        assert!(json.contains("1000"));
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
