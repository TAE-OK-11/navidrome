//! Simplified TTML lyrics parser for the scan hot path.
//!
//! Handles common documents with clock-time `begin`/`end` on `<p>` / `<span>`.
//! Documents that need frame/tick rates, agents, or iTunesMetadata tracks
//! return `None` so Go keeps full parity.

use std::io::Cursor;

use quick_xml::events::Event;
use quick_xml::reader::Reader;

use crate::lyrics::{Cue, Line, Lyrics};

pub fn looks_like_ttml(text: &str) -> bool {
    let head = text.trim_start();
    head.as_bytes().first().copied() == Some(b'<')
        && (head.contains("<tt") || head.contains("<TT") || head.contains(":tt"))
}

pub fn needs_full_go_ttml(text: &str) -> bool {
    let lower = text.to_ascii_lowercase();
    lower.contains("itunesmetadata")
        || lower.contains("<translation")
        || lower.contains("<transliteration")
        || lower.contains("<agent")
        || lower.contains("framerate")
        || lower.contains("tickrate")
        || lower.contains("subframerate")
}

pub fn parse_ttml(default_lang: &str, text: &str) -> Option<Lyrics> {
    if needs_full_go_ttml(text) {
        return None;
    }

    let mut reader = Reader::from_reader(Cursor::new(text.as_bytes()));
    reader.config_mut().trim_text(false);

    let mut lines: Vec<Line> = Vec::new();
    let mut lang = normalize_lang(default_lang);
    let mut buf = Vec::new();

    let mut timing_stack: Vec<(Option<i64>, Option<i64>, String)> =
        vec![(None, None, lang.clone())];

    let mut in_p = false;
    let mut p_begin: Option<i64> = None;
    let mut p_end: Option<i64> = None;
    let mut p_raw = String::new();
    let mut p_cues: Vec<Cue> = Vec::new();
    let mut span_begin: Option<i64> = None;
    let mut span_end: Option<i64> = None;
    let mut span_text = String::new();
    let mut in_span = false;
    let mut saw_tt = false;

    loop {
        match reader.read_event_into(&mut buf) {
            Ok(Event::Start(e)) => {
                let local = local_name(e.name().as_ref());
                match local.as_str() {
                    "tt" => {
                        saw_tt = true;
                        if let Some(l) = attr_lang(&e) {
                            lang = normalize_lang(&l);
                            if let Some(top) = timing_stack.last_mut() {
                                top.2 = lang.clone();
                            }
                        }
                    }
                    "body" | "div" => {
                        let parent = timing_stack
                            .last()
                            .cloned()
                            .unwrap_or((None, None, lang.clone()));
                        let begin = attr_time(&e, b"begin").or(parent.0);
                        let end = attr_time(&e, b"end").or(parent.1);
                        let child_lang = attr_lang(&e)
                            .map(|l| normalize_lang(&l))
                            .unwrap_or(parent.2);
                        timing_stack.push((begin, end, child_lang.clone()));
                        lang = child_lang;
                    }
                    "p" => {
                        in_p = true;
                        let parent = timing_stack
                            .last()
                            .cloned()
                            .unwrap_or((None, None, lang.clone()));
                        p_begin = attr_time(&e, b"begin").or(parent.0);
                        p_end = attr_time(&e, b"end").or(parent.1);
                        if let Some(l) = attr_lang(&e) {
                            lang = normalize_lang(&l);
                        }
                        p_raw.clear();
                        p_cues.clear();
                    }
                    "span" if in_p => {
                        in_span = true;
                        span_begin = attr_time(&e, b"begin");
                        span_end = attr_time(&e, b"end");
                        span_text.clear();
                    }
                    _ => {}
                }
            }
            Ok(Event::Empty(e)) => {
                if local_name(e.name().as_ref()) == "br" && in_p {
                    if in_span {
                        span_text.push('\n');
                    } else {
                        p_raw.push('\n');
                    }
                }
            }
            Ok(Event::Text(t)) => {
                let raw = t.unescape().ok()?.into_owned();
                let collapsed = collapse_xml_space(&raw);
                if collapsed.is_empty() {
                    continue;
                }
                if in_span {
                    span_text.push_str(&collapsed);
                } else if in_p {
                    p_raw.push_str(&collapsed);
                }
            }
            Ok(Event::CData(t)) => {
                let raw = t.decode().ok()?.into_owned();
                let collapsed = collapse_xml_space(&raw);
                if collapsed.is_empty() {
                    continue;
                }
                if in_span {
                    span_text.push_str(&collapsed);
                } else if in_p {
                    p_raw.push_str(&collapsed);
                }
            }
            Ok(Event::End(e)) => {
                let local = local_name(e.name().as_ref());
                match local.as_str() {
                    "span" if in_span => {
                        in_span = false;
                        let value = span_text.clone();
                        if !value.is_empty() {
                            let byte_start = p_raw.len();
                            p_raw.push_str(&value);
                            let byte_end = p_raw.len();
                            if span_begin.is_some() || span_end.is_some() {
                                p_cues.push(Cue {
                                    start: span_begin,
                                    end: span_end,
                                    value: value.trim_end().to_owned(),
                                    byte_start,
                                    byte_end: byte_end.saturating_sub(1),
                                    agent_id: None,
                                });
                            }
                        }
                        span_begin = None;
                        span_end = None;
                        span_text.clear();
                    }
                    "p" if in_p => {
                        in_p = false;
                        let value = p_raw.trim().to_owned();
                        if !value.is_empty() {
                            let left_trim = p_raw.len() - p_raw.trim_start().len();
                            let mut cues = p_cues.clone();
                            for cue in &mut cues {
                                if cue.byte_start >= left_trim {
                                    cue.byte_start -= left_trim;
                                    cue.byte_end = cue.byte_end.saturating_sub(left_trim);
                                }
                            }
                            lines.push(Line {
                                start: p_begin,
                                end: p_end,
                                value,
                                cue: cues,
                            });
                        }
                        p_raw.clear();
                        p_cues.clear();
                        p_begin = None;
                        p_end = None;
                    }
                    "body" | "div" => {
                        timing_stack.pop();
                        if let Some((_, _, l)) = timing_stack.last() {
                            lang = l.clone();
                        }
                    }
                    _ => {}
                }
            }
            Ok(Event::Eof) => break,
            Err(_) => return None,
            _ => {}
        }
        buf.clear();
    }

    if !saw_tt || lines.is_empty() {
        return None;
    }

    let synced = lines
        .iter()
        .any(|line| line.start.is_some() || !line.cue.is_empty());
    Some(Lyrics {
        display_artist: String::new(),
        display_title: String::new(),
        kind: String::new(),
        lang,
        agents: Vec::new(),
        line: lines,
        offset: None,
        synced,
    })
}

fn local_name(qname: &[u8]) -> String {
    let name = std::str::from_utf8(qname).unwrap_or_default();
    name.rsplit(':').next().unwrap_or(name).to_ascii_lowercase()
}

fn attr_lang(e: &quick_xml::events::BytesStart<'_>) -> Option<String> {
    for attr in e.attributes().flatten() {
        let key = std::str::from_utf8(attr.key.as_ref()).ok()?;
        let local = key.rsplit(':').next().unwrap_or(key);
        if local.eq_ignore_ascii_case("lang") {
            return Some(attr.unescape_value().ok()?.into_owned());
        }
    }
    None
}

fn attr_time(e: &quick_xml::events::BytesStart<'_>, name: &[u8]) -> Option<i64> {
    for attr in e.attributes().flatten() {
        if attr.key.local_name().as_ref() == name {
            let value = attr.unescape_value().ok()?;
            return parse_clock_time(value.as_ref());
        }
    }
    None
}

fn parse_clock_time(value: &str) -> Option<i64> {
    let value = value.trim();
    if let Some(num) = value.strip_suffix("ms") {
        return num.parse::<f64>().ok().map(|v| v.round() as i64);
    }
    if let Some(num) = value.strip_suffix('s')
        && !num.contains(':')
    {
        return num
            .parse::<f64>()
            .ok()
            .map(|v| (v * 1000.0).round() as i64);
    }
    if let Some(num) = value.strip_suffix('m')
        && !num.contains(':')
    {
        return num
            .parse::<f64>()
            .ok()
            .map(|v| (v * 60_000.0).round() as i64);
    }
    if let Some(num) = value.strip_suffix('h')
        && !num.contains(':')
    {
        return num
            .parse::<f64>()
            .ok()
            .map(|v| (v * 3_600_000.0).round() as i64);
    }

    // Reject frame/tick forms so Go keeps parity.
    let parts: Vec<&str> = value.split(':').collect();
    if parts.len() == 4 || value.ends_with('t') || value.ends_with('f') {
        return None;
    }
    if parts.len() != 2 && parts.len() != 3 {
        return value
            .parse::<f64>()
            .ok()
            .map(|v| (v * 1000.0).round() as i64);
    }
    let (hours, minutes_idx) = if parts.len() == 3 {
        (parts[0].parse::<i64>().ok()?, 1)
    } else {
        (0, 0)
    };
    let minutes = parts[minutes_idx].parse::<i64>().ok()?;
    let seconds = parts[minutes_idx + 1].parse::<f64>().ok()?;
    let total = (hours * 3600 + minutes * 60) as f64 + seconds;
    Some((total * 1000.0).round() as i64)
}

fn collapse_xml_space(raw: &str) -> String {
    let mut out = String::with_capacity(raw.len());
    let mut prev_space = true;
    for ch in raw.chars() {
        if ch == '\n' || ch == '\r' || ch == '\t' || ch == ' ' {
            if !prev_space && !out.is_empty() {
                out.push(' ');
                prev_space = true;
            }
        } else {
            out.push(ch);
            prev_space = false;
        }
    }
    out
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
    fn parses_simple_clock_ttml_with_word_spans() {
        let xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xml:lang="eng">
  <body>
    <div>
      <p begin="00:00:00.000" end="00:00:04.500"><span begin="00:00:00.000" end="00:00:00.900">Should </span><span begin="00:00:00.900" end="00:00:01.800">auld </span></p>
      <p begin="00:00:04.500" end="00:00:09.000">And never brought to mind?</p>
    </div>
  </body>
</tt>"#;
        let lyrics = parse_ttml("eng", xml).expect("simple TTML");
        assert!(lyrics.synced);
        assert_eq!(lyrics.lang, "eng");
        assert_eq!(lyrics.line.len(), 2);
        assert!(lyrics.line[0].value.contains("Should"));
        assert!(lyrics.line[0].value.contains("auld"));
        assert!(lyrics.line[0].cue.len() >= 2);
        assert_eq!(lyrics.line[1].value, "And never brought to mind?");
        assert_eq!(lyrics.line[1].start, Some(4500));
    }

    #[test]
    fn falls_back_for_frame_rate_documents() {
        let xml = r#"<tt ttp:frameRate="30" xmlns:ttp="http://www.w3.org/ns/ttml#parameter"><body><div><p begin="00:00:01:00">Hi</p></div></body></tt>"#;
        assert!(parse_ttml("eng", xml).is_none());
    }
}
