//! LRCLIB Lyricsfile YAML parser for the scan hot path.
//!
//! See https://github.com/tranxuanthang/lrcget/blob/main/LYRICSFILE_CONCEPT.md

use serde::Deserialize;

use crate::lyrics::{Agent, Cue, Line, Lyrics};

const LYRICSFILE_VERSION: &str = "1.0";

fn utf8_prefix(text: &str, max_bytes: usize) -> &str {
    if text.len() <= max_bytes {
        return text;
    }
    let mut end = max_bytes;
    while end > 0 && !text.is_char_boundary(end) {
        end -= 1;
    }
    &text[..end]
}

pub fn looks_like_lyricsfile(text: &str) -> bool {
    let head = utf8_prefix(text, 4096);
    if !head.contains("version:") {
        return false;
    }
    head.contains(&format!("\"{LYRICSFILE_VERSION}\""))
        || head.contains(&format!("'{LYRICSFILE_VERSION}'"))
        || head.contains("version: 1.0")
        || head.contains("version:1.0")
}

pub fn parse_lyricsfile(default_lang: &str, text: &str) -> Option<Vec<Lyrics>> {
    let doc: LyricsfileDocument = serde_yaml::from_str(text).ok()?;
    if doc.version.trim() != LYRICSFILE_VERSION {
        return None;
    }

    let mut lang = doc.metadata.language.trim().to_string();
    if lang.is_empty() {
        lang = default_lang.to_owned();
    }
    let mut lyrics = Lyrics {
        display_artist: sanitize_text(&doc.metadata.artist),
        display_title: sanitize_text(&doc.metadata.title),
        kind: "main".to_owned(),
        lang: normalize_lang(&lang),
        agents: Vec::new(),
        line: Vec::new(),
        offset: if doc.metadata.offset_ms != 0 {
            Some(doc.metadata.offset_ms)
        } else {
            None
        },
        synced: false,
    };

    if doc.metadata.instrumental {
        return Some(vec![lyrics]);
    }

    if doc.lines.is_empty() {
        let lines = build_plain_lines(&doc.plain);
        if lines.is_empty() {
            return None;
        }
        lyrics.line = lines;
        return Some(vec![lyrics]);
    }

    let (lines, agents) = build_lines(&doc.lines);
    lyrics.line = lines;
    lyrics.agents = agents;
    lyrics.synced = true;
    Some(vec![lyrics])
}

#[derive(Debug, Deserialize)]
struct LyricsfileDocument {
    version: String,
    metadata: LyricsfileMetadata,
    #[serde(default)]
    lines: Vec<LyricsfileLineEntry>,
    #[serde(default)]
    plain: String,
}

#[derive(Debug, Default, Deserialize)]
struct LyricsfileMetadata {
    #[serde(default)]
    title: String,
    #[serde(default)]
    artist: String,
    #[serde(default)]
    language: String,
    #[serde(default, rename = "offset_ms")]
    offset_ms: i64,
    #[serde(default)]
    instrumental: bool,
}

#[derive(Debug, Deserialize)]
struct LyricsfileLineEntry {
    #[serde(default)]
    text: String,
    #[serde(default, rename = "start_ms")]
    start_ms: i64,
    #[serde(default, rename = "end_ms")]
    end_ms: Option<i64>,
    #[serde(default)]
    words: Vec<LyricsfileWordEntry>,
}

#[derive(Debug, Deserialize)]
struct LyricsfileWordEntry {
    text: String,
    #[serde(rename = "start_ms")]
    start_ms: i64,
    #[serde(default, rename = "end_ms")]
    end_ms: Option<i64>,
}

fn build_plain_lines(plain: &str) -> Vec<Line> {
    sanitize_text(plain)
        .split('\n')
        .filter_map(|raw| {
            let value = raw.trim().to_owned();
            if value.is_empty() {
                None
            } else {
                Some(Line {
                    start: None,
                    end: None,
                    value,
                    cue: Vec::new(),
                })
            }
        })
        .collect()
}

fn build_lines(entries: &[LyricsfileLineEntry]) -> (Vec<Line>, Vec<Agent>) {
    if entries.is_empty() {
        return (Vec::new(), Vec::new());
    }

    let ends: Vec<Option<i64>> = entries
        .iter()
        .enumerate()
        .map(|(i, entry)| {
            let next_start = entries.get(i + 1).map(|e| e.start_ms);
            line_end(entry, next_start)
        })
        .collect();

    let mut active: std::collections::HashMap<i32, i64> = std::collections::HashMap::new();
    let mut max_voice = -1;
    let mut any_cues = false;
    let mut lines = Vec::with_capacity(entries.len());

    for (i, entry) in entries.iter().enumerate() {
        active.retain(|_, v_end| *v_end > entry.start_ms);

        let mut voice_id = 0;
        while active.contains_key(&voice_id) {
            voice_id += 1;
        }
        max_voice = max_voice.max(voice_id);

        let agent_id = format!("voice-{voice_id}");
        let (cues, value) = words_to_line_cues(entry, &agent_id);
        if !cues.is_empty() {
            any_cues = true;
        }

        lines.push(Line {
            start: Some(entry.start_ms),
            end: ends[i],
            value,
            cue: cues,
        });

        let end_ms = ends[i].unwrap_or(entry.start_ms);
        active.insert(voice_id, end_ms);
    }

    if max_voice <= 0 || !any_cues {
        for line in &mut lines {
            for cue in &mut line.cue {
                cue.agent_id = None;
            }
        }
        return (lines, Vec::new());
    }

    let agents: Vec<Agent> = (0..=max_voice)
        .map(|v| Agent {
            id: format!("voice-{v}"),
            role: if v == 0 { "main".to_owned() } else { "voice".to_owned() },
            name: String::new(),
        })
        .collect();
    (lines, agents)
}

fn line_end(entry: &LyricsfileLineEntry, next_start: Option<i64>) -> Option<i64> {
    if let Some(end) = entry.end_ms {
        return Some(end);
    }
    if let Some(last) = entry.words.last()
        && let Some(end) = last.end_ms
    {
        return Some(end);
    }
    next_start
}

fn words_to_line_cues(entry: &LyricsfileLineEntry, agent_id: &str) -> (Vec<Cue>, String) {
    if entry.words.is_empty() {
        return (Vec::new(), sanitize_text(&entry.text));
    }

    let line_value: String = entry.words.iter().map(|w| w.text.as_str()).collect();
    let mut cues = Vec::with_capacity(entry.words.len());
    let mut cursor = 0usize;

    for (i, word) in entry.words.iter().enumerate() {
        let value_bytes = word.text.len();
        let byte_start = cursor;
        let byte_end = if value_bytes > 0 {
            cursor += value_bytes;
            byte_start + value_bytes - 1
        } else {
            byte_start
        };

        let mut cue = Cue {
            start: Some(word.start_ms),
            end: word.end_ms,
            value: word.text.clone(),
            byte_start,
            byte_end,
            agent_id: Some(agent_id.to_owned()),
        };
        if cue.end.is_none()
            && let Some(next) = entry.words.get(i + 1)
        {
            cue.end = Some(next.start_ms);
        }
        cues.push(cue);
    }

    (cues, line_value)
}

fn sanitize_text(raw: &str) -> String {
    raw.replace("&amp;", "&")
        .replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&quot;", "\"")
        .replace("&#39;", "'")
        .replace("&apos;", "'")
}

fn normalize_lang(lang: &str) -> String {
    let lang = lang.trim().to_ascii_lowercase();
    if lang.is_empty() {
        "xxx".to_owned()
    } else {
        lang
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_word_level_yaml_with_voice_agents() {
        let yaml = r#"version: "1.0"
metadata:
  title: Song
  artist: Artist
  language: eng
lines:
  - start_ms: 1000
    words:
      - text: "Hello "
        start_ms: 1000
        end_ms: 1500
      - text: "world"
        start_ms: 1500
        end_ms: 2000
  - start_ms: 3000
    text: "Next line"
"#;
        let list = parse_lyricsfile("xxx", yaml).expect("yaml lyrics");
        assert_eq!(list.len(), 1);
        assert!(list[0].synced);
        assert_eq!(list[0].lang, "eng");
        assert_eq!(list[0].line.len(), 2);
        assert!(list[0].line[0].cue.len() >= 2);
    }

    #[test]
    fn skips_non_lyricsfile_yaml() {
        let yaml = "title: not lyricsfile\nversion: 2.0\n";
        assert!(parse_lyricsfile("eng", yaml).is_none());
    }

    #[test]
    fn looks_like_lyricsfile_handles_utf8_boundary_at_prefix_limit() {
        let mut text = String::from("version: \"1.0\"\n");
        while text.len() < 4096 {
            text.push('가');
        }
        assert!(looks_like_lyricsfile(&text));
    }
}
