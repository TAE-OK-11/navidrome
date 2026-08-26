//! Full TTML lyrics parser matching Go `model/lyrics_ttml.go`.

use std::collections::HashMap;
use std::io::Cursor;
use std::sync::LazyLock;

use quick_xml::events::{BytesStart, Event};
use quick_xml::reader::Reader;
use regex::Regex;

use crate::lyrics::{Agent, Cue, Line, Lyrics};

const DEFAULT_FRAME_RATE: f64 = 30.0;
const DEFAULT_SUB_FRAME_RATE: f64 = 1.0;
const DEFAULT_TICK_RATE: f64 = 1.0;
const BACKGROUND_AGENT_PREFIX: &str = "__nd_bg__|";

const LYRIC_KIND_MAIN: &str = "main";
const LYRIC_KIND_TRANSLATION: &str = "translation";
const LYRIC_KIND_PRONUNCIATION: &str = "pronunciation";

static OFFSET_TIME_RE: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"^([0-9]+(?:\.[0-9]+)?)(h|m|s|ms|f|t)$").unwrap());
static XML_ENCODING_RE: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r#"(?i)<\?xml([^>]*?)encoding\s*=\s*["'][^"']+["']([^>]*)\?>"#).unwrap()
});

#[derive(Clone, Copy, PartialEq, Eq)]
enum TimeKind {
    Absolute,
    Offset,
    Ambiguous,
}

#[derive(Clone, Copy)]
struct TimingParams {
    frame_rate: f64,
    sub_frame_rate: f64,
    tick_rate: f64,
}

impl TimingParams {
    fn default_params() -> Self {
        Self {
            frame_rate: DEFAULT_FRAME_RATE,
            sub_frame_rate: DEFAULT_SUB_FRAME_RATE,
            tick_rate: DEFAULT_TICK_RATE,
        }
    }
}

#[derive(Clone)]
struct TimingContext {
    lang: String,
    role: String,
    agent_id: String,
    begin: i64,
    has_begin: bool,
    end: i64,
    has_end: bool,
    invalid: bool,
}

impl TimingContext {
    fn root(default_lang: &str) -> Self {
        Self {
            lang: normalize_lyric_lang(default_lang),
            role: String::new(),
            agent_id: String::new(),
            begin: 0,
            has_begin: false,
            end: 0,
            has_end: false,
            invalid: false,
        }
    }
}

struct LineRef {
    order: i32,
    line: Line,
}

struct MetadataEntry {
    key: String,
    line: Line,
    seq: i32,
}

struct ResolvedMetadataLine {
    order: i32,
    seq: i32,
    line: Line,
}

#[derive(Clone)]
struct DefinedAgent {
    agent_type: String,
    name: String,
}

struct Piece {
    raw: String,
    cue: Option<Cue>,
    is_break: bool,
}

struct FinalLine {
    text: String,
    cues: Vec<Cue>,
}

struct TtmlParser<'a> {
    reader: Reader<Cursor<&'a [u8]>>,
    params: TimingParams,
    main_lang_order: Vec<String>,
    main_lines_by_lang: HashMap<String, Vec<Line>>,
    main_line_refs_by_key: HashMap<String, LineRef>,
    main_line_order: i32,
    translation_lang_order: Vec<String>,
    translation_entries_by_lang: HashMap<String, Vec<MetadataEntry>>,
    pronunciation_lang_order: Vec<String>,
    pronunciation_entries_by_lang: HashMap<String, Vec<MetadataEntry>>,
    defined_agents: HashMap<String, DefinedAgent>,
    metadata_seq: i32,
}

pub fn looks_like_ttml(text: &str) -> bool {
    let head = text.trim_start();
    head.as_bytes().first().copied() == Some(b'<')
        && (head.contains("<tt") || head.contains("<TT") || head.contains(":tt"))
}

pub fn parse_ttml_list(default_lang: &str, text: &str) -> Option<Vec<Lyrics>> {
    let contents = fix_xml_encoding(text);
    if !is_ttml_document(&contents) {
        return Some(Vec::new());
    }

    let mut parser = TtmlParser::new(&contents);
    let root = TimingContext::root(default_lang);
    let mut buf = Vec::new();
    loop {
        let event = parser.next_event(&mut buf)?;
        match event {
            Event::Start(start) => {
                parser.parse_element(start, root.clone())?;
            }
            Event::Eof => break,
            _ => {}
        }
    }
    Some(parser.to_lyric_list())
}

fn fix_xml_encoding(text: &str) -> Vec<u8> {
    XML_ENCODING_RE
        .replace(text, r#"<?xml$1encoding="UTF-8"$2?>"#)
        .into_owned()
        .into_bytes()
}

fn is_ttml_document(contents: &[u8]) -> bool {
    let mut reader = Reader::from_reader(Cursor::new(contents));
    reader.config_mut().trim_text(false);
    let mut buf = Vec::new();
    loop {
        match reader.read_event_into(&mut buf) {
            Ok(Event::Start(e)) => {
                let is_tt = local_name(e.name().as_ref()) == "tt";
                buf.clear();
                return is_tt;
            }
            Ok(Event::Eof) => return false,
            Ok(_) => {
                buf.clear();
            }
            Err(_) => return false,
        }
    }
}

impl<'a> TtmlParser<'a> {
    fn new(contents: &'a [u8]) -> Self {
        let mut reader = Reader::from_reader(Cursor::new(contents));
        reader.config_mut().trim_text(false);
        Self {
            reader,
            params: TimingParams::default_params(),
            main_lang_order: Vec::new(),
            main_lines_by_lang: HashMap::new(),
            main_line_refs_by_key: HashMap::new(),
            main_line_order: 0,
            translation_lang_order: Vec::new(),
            translation_entries_by_lang: HashMap::new(),
            pronunciation_lang_order: Vec::new(),
            pronunciation_entries_by_lang: HashMap::new(),
            defined_agents: HashMap::new(),
            metadata_seq: 0,
        }
    }

    fn next_event(&mut self, buf: &mut Vec<u8>) -> Option<Event<'static>> {
        match self.reader.read_event_into(buf) {
            Ok(event) => {
                let owned = event.into_owned();
                buf.clear();
                Some(owned)
            }
            Err(_) => None,
        }
    }

    fn parse_element(&mut self, start: BytesStart<'_>, parent: TimingContext) -> Option<()> {
        let local = local_name(start.name().as_ref());
        if local == "tt" {
            self.update_timing_params(&start);
        }

        match local.as_str() {
            "translation" => return self.parse_metadata_track(start, parent, LYRIC_KIND_TRANSLATION),
            "transliteration" => {
                return self.parse_metadata_track(start, parent, LYRIC_KIND_PRONUNCIATION);
            }
            "agent" => return self.parse_agent_definition(start),
            _ => {}
        }

        let ctx = self.child_context(&start, parent.clone());
        if local == "p" {
            let (line_text, tokens) = self.parse_paragraph(ctx.clone())?;
            if ctx.invalid || line_text.is_empty() {
                return Some(());
            }

            let mut parsed_line = Line {
                start: ctx.has_begin.then_some(ctx.begin),
                end: ctx.has_end.then_some(ctx.end),
                value: line_text,
                cue: tokens,
            };
            parsed_line = normalize_line_timing(parsed_line);

            let line_key = attr_value(&start, "key").unwrap_or_default();
            self.add_main_line(&ctx.lang, &line_key, parsed_line);
            return Some(());
        }

        let mut buf = Vec::new();
        loop {
            let event = self.next_event(&mut buf)?;
            match event {
                Event::Start(child) => {
                    let next_parent = if ctx.invalid {
                        parent.clone()
                    } else {
                        ctx.clone()
                    };
                    self.parse_element(child, next_parent)?;
                }
                Event::End(end) => {
                    if names_match(end.name().as_ref(), start.name().as_ref()) {
                        return Some(());
                    }
                }
                Event::Eof => return None,
                _ => {}
            }
        }
    }

    fn parse_metadata_track(
        &mut self,
        start: BytesStart<'_>,
        parent: TimingContext,
        kind: &str,
    ) -> Option<()> {
        let mut buf = Vec::new();
        let ctx = self.child_context(&start, parent.clone());
        let lang = normalize_lyric_lang(&ctx.lang);

        loop {
            let event = self.next_event(&mut buf)?;
            match event {
                Event::Start(child) => {
                    if local_name(child.name().as_ref()) == "text" {
                        if let Some(entry) = self.parse_metadata_text(child, ctx.clone())? {
                            self.add_metadata_entry(kind, &lang, entry);
                        }
                        continue;
                    }

                    let next_parent = if ctx.invalid {
                        parent.clone()
                    } else {
                        ctx.clone()
                    };
                    self.parse_element(child, next_parent)?;
                }
                Event::End(end) => {
                    if names_match(end.name().as_ref(), start.name().as_ref()) {
                        return Some(());
                    }
                }
                Event::Eof => return None,
                _ => {}
            }
        }
    }

    fn parse_agent_definition(&mut self, start: BytesStart<'_>) -> Option<()> {
        let mut buf = Vec::new();
        let id = attr_value(&start, "id")?;
        let id = id.trim().to_owned();
        if id.is_empty() {
            return self.skip_element(start);
        }

        let mut agent = DefinedAgent {
            agent_type: attr_or_empty(&start, "type").to_ascii_lowercase(),
            name: String::new(),
        };

        loop {
            let event = self.next_event(&mut buf)?;
            match event {
                Event::Start(child) => {
                    if local_name(child.name().as_ref()) == "name" {
                        let name = self.collect_element_text(child)?;
                        let name = sanitize_ttml_text(&name);
                        if !name.is_empty() && agent.name.is_empty() {
                            agent.name = name;
                        }
                        continue;
                    }
                    self.skip_element(child)?;
                }
                Event::End(end) => {
                    if names_match(end.name().as_ref(), start.name().as_ref()) {
                        self.defined_agents.insert(id, agent);
                        return Some(());
                    }
                }
                Event::Eof => return None,
                _ => {}
            }
        }
    }

    fn parse_metadata_text(
        &mut self,
        start: BytesStart<'_>,
        parent: TimingContext,
    ) -> Option<Option<MetadataEntry>> {
        let for_key = attr_value(&start, "for");
        let for_key = for_key.map(|k| k.trim().to_owned());
        let ctx = self.child_context(&start, parent.clone());

        let pieces = self.parse_inline_element(start, parent)?;
        let Some(for_key) = for_key.filter(|k| !k.is_empty()) else {
            return Some(None);
        };

        if ctx.invalid {
            return Some(None);
        }

        let (value, tokens) = build_line_from_pieces(&pieces);
        let mut line = Line {
            start: ctx.has_begin.then_some(ctx.begin),
            end: ctx.has_end.then_some(ctx.end),
            value,
            cue: tokens,
        };
        line = normalize_line_timing(line);

        if line.value.is_empty() && line.cue.is_empty() {
            return Some(None);
        }

        Some(Some(MetadataEntry {
            key: for_key,
            line,
            seq: 0,
        }))
    }

    fn parse_paragraph(&mut self, parent: TimingContext) -> Option<(String, Vec<Cue>)> {
        let mut pieces = Vec::new();
        let mut buf = Vec::new();

        loop {
            let event = self.next_event(&mut buf)?;
            match event {
                Event::Start(start) => {
                    let inline = self.parse_inline_element(start, parent.clone())?;
                    pieces.extend(inline);
                }
                Event::Empty(empty) => {
                    if local_name(empty.name().as_ref()) == "br" {
                        pieces.push(Piece {
                            raw: String::new(),
                            cue: None,
                            is_break: true,
                        });
                    }
                }
                Event::End(end) => {
                    if local_name(end.name().as_ref()) == "p" {
                        return Some(build_line_from_pieces(&pieces));
                    }
                }
                Event::Text(text) => {
                    let raw = text.unescape().ok()?.into_owned();
                    pieces.push(Piece {
                        raw,
                        cue: None,
                        is_break: false,
                    });
                }
                Event::CData(text) => {
                    let raw = text.decode().ok()?.into_owned();
                    pieces.push(Piece {
                        raw,
                        cue: None,
                        is_break: false,
                    });
                }
                Event::Eof => return None,
                _ => {}
            }
        }
    }

    fn parse_inline_element(
        &mut self,
        start: BytesStart<'_>,
        parent: TimingContext,
    ) -> Option<Vec<Piece>> {
        let local = local_name(start.name().as_ref());
        if local == "br" {
            return Some(vec![Piece {
                raw: String::new(),
                cue: None,
                is_break: true,
            }]);
        }

        let ctx = self.child_context(&start, parent.clone());
        let has_begin = attr_value(&start, "begin").is_some();
        let has_end = attr_value(&start, "end").is_some();
        let has_dur = attr_value(&start, "dur").is_some();
        let has_own_timing = has_begin || has_end || has_dur;

        let mut pieces = Vec::new();
        let mut buf = Vec::new();

        loop {
            let event = self.next_event(&mut buf)?;
            match event {
                Event::Start(child) => {
                    let inline = self.parse_inline_element(child, ctx.clone())?;
                    pieces.extend(inline);
                }
                Event::Empty(empty) => {
                    if local_name(empty.name().as_ref()) == "br" {
                        pieces.push(Piece {
                            raw: String::new(),
                            cue: None,
                            is_break: true,
                        });
                    }
                }
                Event::End(end) => {
                    if !names_match(end.name().as_ref(), start.name().as_ref()) {
                        continue;
                    }

                    if local == "span"
                        && has_own_timing
                        && !ctx.invalid
                        && !pieces_contain_cue(&pieces)
                    {
                        let raw_value = concat_piece_raw(&pieces);
                        let token_text = sanitize_ttml_text(&raw_value);
                        if !token_text.is_empty() {
                            let parsed_token = Cue {
                                start: ctx.has_begin.then_some(ctx.begin),
                                end: ctx.has_end.then_some(ctx.end),
                                value: String::new(),
                                byte_start: 0,
                                byte_end: 0,
                                agent_id: Some(self.resolve_cue_agent_id(&ctx)),
                            };
                            return Some(vec![Piece {
                                raw: raw_value,
                                cue: Some(parsed_token),
                                is_break: false,
                            }]);
                        }
                    }

                    return Some(pieces);
                }
                Event::Text(text) => {
                    let raw = text.unescape().ok()?.into_owned();
                    pieces.push(Piece {
                        raw,
                        cue: None,
                        is_break: false,
                    });
                }
                Event::CData(text) => {
                    let raw = text.decode().ok()?.into_owned();
                    pieces.push(Piece {
                        raw,
                        cue: None,
                        is_break: false,
                    });
                }
                Event::Eof => return None,
                _ => {}
            }
        }
    }

    fn collect_element_text(&mut self, start: BytesStart<'_>) -> Option<String> {
        let mut text = String::new();
        let mut buf = Vec::new();
        loop {
            let event = self.next_event(&mut buf)?;
            match event {
                Event::Start(child) => {
                    text.push_str(&self.collect_element_text(child)?);
                }
                Event::End(end) => {
                    if names_match(end.name().as_ref(), start.name().as_ref()) {
                        return Some(text);
                    }
                }
                Event::Text(t) => {
                    text.push_str(&t.unescape().ok()?.into_owned());
                }
                Event::CData(t) => {
                    text.push_str(&t.decode().ok()?.into_owned());
                }
                Event::Eof => return None,
                _ => {}
            }
        }
    }

    fn skip_element(&mut self, _start: BytesStart<'_>) -> Option<()> {
        let mut depth = 1;
        let mut buf = Vec::new();
        while depth > 0 {
            let event = self.next_event(&mut buf)?;
            match event {
                Event::Start(_) => depth += 1,
                Event::End(_) => depth -= 1,
                Event::Eof => return None,
                _ => {}
            }
        }
        Some(())
    }

    fn to_lyric_list(&self) -> Vec<Lyrics> {
        let mut res = Vec::new();

        for lang in &self.main_lang_order {
            let lines = self.main_lines_by_lang.get(lang).cloned().unwrap_or_default();
            if lines.is_empty() {
                continue;
            }
            let synced = lines_are_synced(&lines);
            res.push(self.finalize_lyrics(Lyrics {
                display_artist: String::new(),
                display_title: String::new(),
                kind: LYRIC_KIND_MAIN.to_owned(),
                lang: lang.clone(),
                agents: Vec::new(),
                line: lines,
                offset: None,
                synced,
            }));
        }

        res.extend(self.build_metadata_lyrics(
            LYRIC_KIND_TRANSLATION,
            &self.translation_lang_order,
            &self.translation_entries_by_lang,
        ));
        res.extend(self.build_metadata_lyrics(
            LYRIC_KIND_PRONUNCIATION,
            &self.pronunciation_lang_order,
            &self.pronunciation_entries_by_lang,
        ));
        res
    }

    fn build_metadata_lyrics(
        &self,
        kind: &str,
        lang_order: &[String],
        entries_by_lang: &HashMap<String, Vec<MetadataEntry>>,
    ) -> Vec<Lyrics> {
        let mut res = Vec::new();

        for lang in lang_order {
            let entries = match entries_by_lang.get(lang) {
                Some(e) if !e.is_empty() => e,
                _ => continue,
            };

            let mut seen_keys = HashMap::new();
            let mut resolved = Vec::new();
            for entry in entries {
                if seen_keys.contains_key(&entry.key) {
                    continue;
                }
                seen_keys.insert(entry.key.clone(), ());

                let Some(line_ref) = self.main_line_refs_by_key.get(&entry.key) else {
                    continue;
                };

                let mut line = entry.line.clone();
                if line.start.is_none() {
                    line.start = line_ref.line.start;
                }
                if line.end.is_none() {
                    line.end = line_ref.line.end;
                }
                line = normalize_line_timing(line);

                if line.value.is_empty() && line.cue.is_empty() {
                    continue;
                }

                resolved.push(ResolvedMetadataLine {
                    order: line_ref.order,
                    seq: entry.seq,
                    line,
                });
            }

            if resolved.is_empty() {
                continue;
            }

            resolved.sort_by(|a, b| a.order.cmp(&b.order).then(a.seq.cmp(&b.seq)));
            let lines: Vec<Line> = resolved.into_iter().map(|r| r.line).collect();
            res.push(self.finalize_lyrics(Lyrics {
                display_artist: String::new(),
                display_title: String::new(),
                kind: kind.to_owned(),
                lang: lang.clone(),
                agents: Vec::new(),
                line: lines.clone(),
                offset: None,
                synced: lines_are_synced(&lines),
            }));
        }

        res
    }

    fn finalize_lyrics(&self, mut lyrics: Lyrics) -> Lyrics {
        let (lines, agents) = self.resolve_agents(lyrics.line);
        lyrics.line = normalize_cue_lines(lines);
        lyrics.agents = agents;
        lyrics
    }

    fn resolve_agents(&self, mut lines: Vec<Line>) -> (Vec<Line>, Vec<Agent>) {
        if lines.is_empty() {
            return (lines, Vec::new());
        }

        let mut used_order = Vec::new();
        let mut used_set = HashMap::new();
        let mut saw_empty_cue = false;

        for line in &lines {
            for cue in &line.cue {
                let agent_id = cue.agent_id.as_deref().unwrap_or("").trim();
                if agent_id.is_empty() {
                    saw_empty_cue = true;
                    continue;
                }
                if !used_set.contains_key(agent_id) {
                    used_set.insert(agent_id.to_owned(), ());
                    used_order.push(agent_id.to_owned());
                }
            }
        }

        if used_order.is_empty() {
            return (lines, Vec::new());
        }

        let mut main_id = String::new();
        for agent_id in &used_order {
            let role = self.base_role_for_agent(agent_id);
            if role != "bg" && role != "group" {
                main_id = agent_id.clone();
                break;
            }
        }
        if main_id.is_empty() && saw_empty_cue {
            main_id = "main".to_owned();
        }
        if main_id.is_empty() {
            for agent_id in &used_order {
                if self.base_role_for_agent(agent_id) != "bg" {
                    main_id = agent_id.clone();
                    break;
                }
            }
        }
        if main_id.is_empty() {
            main_id = used_order[0].clone();
        }

        if !used_set.contains_key(&main_id) {
            used_set.insert(main_id.clone(), ());
            used_order.insert(0, main_id.clone());
        }

        for line in &mut lines {
            for cue in &mut line.cue {
                if cue.agent_id.as_deref().unwrap_or("").trim().is_empty() {
                    cue.agent_id = Some(main_id.clone());
                }
            }
        }

        let agents = used_order
            .iter()
            .map(|agent_id| {
                let mut role = self.base_role_for_agent(agent_id);
                if agent_id == &main_id {
                    role = "main".to_owned();
                }
                Agent {
                    id: agent_id.clone(),
                    role,
                    name: self.agent_name_for_id(agent_id),
                }
            })
            .collect();

        (lines, agents)
    }

    fn resolve_cue_agent_id(&self, ctx: &TimingContext) -> String {
        let mut agent_id = ctx.agent_id.trim().to_owned();
        if context_has_role(&ctx.role, "x-bg") {
            if agent_id.is_empty() {
                agent_id = "main".to_owned();
            }
            return background_agent_id(&agent_id);
        }
        agent_id
    }

    fn base_role_for_agent(&self, agent_id: &str) -> String {
        if is_background_agent_id(agent_id) {
            return "bg".to_owned();
        }
        if let Some(agent) = self.defined_agents.get(agent_id) {
            return match agent.agent_type.as_str() {
                "group" => "group".to_owned(),
                _ => "voice".to_owned(),
            };
        }
        "voice".to_owned()
    }

    fn agent_name_for_id(&self, agent_id: &str) -> String {
        if is_background_agent_id(agent_id) {
            let base_id = agent_id.trim_start_matches(BACKGROUND_AGENT_PREFIX);
            if base_id == "main" {
                return String::new();
            }
            if let Some(agent) = self.defined_agents.get(base_id) {
                return agent.name.clone();
            }
            return String::new();
        }
        self.defined_agents
            .get(agent_id)
            .map(|a| a.name.clone())
            .unwrap_or_default()
    }

    fn add_main_line(&mut self, lang: &str, line_key: &str, line: Line) {
        let lang = normalize_lyric_lang(lang);
        if !self.main_lines_by_lang.contains_key(&lang) {
            self.main_lang_order.push(lang.clone());
        }
        self.main_lines_by_lang
            .entry(lang)
            .or_default()
            .push(line.clone());

        let line_key = line_key.trim();
        if !line_key.is_empty()
            && !self.main_line_refs_by_key.contains_key(line_key)
        {
            self.main_line_refs_by_key.insert(
                line_key.to_owned(),
                LineRef {
                    order: self.main_line_order,
                    line,
                },
            );
        }
        self.main_line_order += 1;
    }

    fn add_metadata_entry(&mut self, kind: &str, lang: &str, mut entry: MetadataEntry) {
        let lang = normalize_lyric_lang(lang);
        entry.seq = self.metadata_seq;
        self.metadata_seq += 1;

        match kind {
            LYRIC_KIND_TRANSLATION => {
                if !self.translation_entries_by_lang.contains_key(&lang) {
                    self.translation_lang_order.push(lang.clone());
                }
                self.translation_entries_by_lang
                    .entry(lang)
                    .or_default()
                    .push(entry);
            }
            LYRIC_KIND_PRONUNCIATION => {
                if !self.pronunciation_entries_by_lang.contains_key(&lang) {
                    self.pronunciation_lang_order.push(lang.clone());
                }
                self.pronunciation_entries_by_lang
                    .entry(lang)
                    .or_default()
                    .push(entry);
            }
            _ => {}
        }
    }

    fn child_context(&self, start: &BytesStart<'_>, parent: TimingContext) -> TimingContext {
        let mut ctx = parent.clone();

        if let Some(lang) = attr_value(start, "lang") {
            ctx.lang = normalize_lyric_lang(&lang);
        }
        if let Some(agent_id) = attr_value(start, "agent") {
            ctx.agent_id = agent_id.trim().to_owned();
        }
        if let Some(role) = attr_value(start, "role") {
            let role = role.trim();
            if !role.is_empty() {
                ctx.role = append_roles(&ctx.role, role);
            }
        }

        let has_begin = attr_value(start, "begin");
        let has_end = attr_value(start, "end");
        let has_dur = attr_value(start, "dur");

        if let Some(begin_expr) = has_begin {
            let Some((begin, kind)) = parse_time_expression(&begin_expr, self.params) else {
                ctx.invalid = true;
                return ctx;
            };

            let base = if parent.has_begin { parent.begin } else { 0 };
            ctx.begin = resolve_time(begin, kind, base, &parent);
            ctx.has_begin = true;
        } else {
            ctx.begin = parent.begin;
            ctx.has_begin = parent.has_begin;
        }

        let mut calculated_end = 0_i64;
        let mut calculated_has_end = false;

        if let Some(end_expr) = has_end {
            let Some((end, kind)) = parse_time_expression(&end_expr, self.params) else {
                ctx.invalid = true;
                return ctx;
            };

            let base = if ctx.has_begin {
                ctx.begin
            } else if parent.has_begin {
                parent.begin
            } else {
                0
            };
            calculated_end = resolve_time(end, kind, base, &parent);
            calculated_has_end = true;
        }

        if let Some(dur_expr) = has_dur {
            let Some(dur) = parse_duration_expression(&dur_expr, self.params) else {
                ctx.invalid = true;
                return ctx;
            };
            if ctx.has_begin {
                let dur_end = ctx.begin + dur;
                if !calculated_has_end || dur_end < calculated_end {
                    calculated_end = dur_end;
                    calculated_has_end = true;
                }
            }
        }

        if !calculated_has_end && parent.has_end {
            calculated_end = parent.end;
            calculated_has_end = true;
        }

        ctx.end = calculated_end;
        ctx.has_end = calculated_has_end;
        ctx
    }

    fn update_timing_params(&mut self, start: &BytesStart<'_>) {
        let mut frame_rate = self.params.frame_rate;
        if let Some(value) = attr_value(start, "frameRate")
            && let Ok(parsed) = value.parse::<f64>()
            && parsed > 0.0
        {
            frame_rate = parsed;
        }

        if let Some(value) = attr_value(start, "frameRateMultiplier") {
            let parts: Vec<&str> = value.split_whitespace().collect();
            if parts.len() == 2
                && let (Ok(numerator), Ok(denominator)) =
                    (parts[0].parse::<f64>(), parts[1].parse::<f64>())
                && denominator > 0.0
            {
                frame_rate *= numerator / denominator;
            }
        }

        let mut sub_frame_rate = self.params.sub_frame_rate;
        if let Some(value) = attr_value(start, "subFrameRate")
            && let Ok(parsed) = value.parse::<f64>()
            && parsed > 0.0
        {
            sub_frame_rate = parsed;
        }

        let mut tick_rate = self.params.tick_rate;
        if let Some(value) = attr_value(start, "tickRate")
            && let Ok(parsed) = value.parse::<f64>()
            && parsed > 0.0
        {
            tick_rate = parsed;
        }

        self.params.frame_rate = if frame_rate > 0.0 {
            frame_rate
        } else {
            DEFAULT_FRAME_RATE
        };
        self.params.sub_frame_rate = if sub_frame_rate > 0.0 {
            sub_frame_rate
        } else {
            DEFAULT_SUB_FRAME_RATE
        };
        self.params.tick_rate = if tick_rate > 0.0 {
            tick_rate
        } else {
            DEFAULT_TICK_RATE
        };
    }
}

fn build_line_from_pieces(pieces: &[Piece]) -> (String, Vec<Cue>) {
    let mut finalized = finalize_lines(split_pieces_by_break(pieces));
    while finalized.first().is_some_and(|l| l.text.is_empty() && l.cues.is_empty()) {
        finalized.remove(0);
    }
    while finalized.last().is_some_and(|l| l.text.is_empty() && l.cues.is_empty()) {
        finalized.pop();
    }

    let mut value = String::new();
    let mut cues = Vec::with_capacity(8);
    let mut byte_offset = 0_usize;

    for (i, line) in finalized.iter().enumerate() {
        if i > 0 {
            value.push('\n');
            byte_offset += 1;
        }
        value.push_str(&line.text);
        for cue in &line.cues {
            let mut cue = cue.clone();
            cue.byte_start += byte_offset;
            cue.byte_end += byte_offset;
            cues.push(cue);
        }
        byte_offset += line.text.len();
    }

    (value, cues)
}

fn finalize_lines(lines: Vec<Vec<Piece>>) -> Vec<FinalLine> {
    lines
        .into_iter()
        .map(|line| {
            let (text, cues) = finalize_logical_line(&line);
            FinalLine { text, cues }
        })
        .collect()
}

fn split_pieces_by_break(pieces: &[Piece]) -> Vec<Vec<Piece>> {
    let mut lines: Vec<Vec<Piece>> = vec![Vec::new()];
    let mut prev_ended_with_space = true;

    for piece in pieces {
        if piece.is_break {
            lines.push(Vec::new());
            prev_ended_with_space = true;
            continue;
        }

        let mut raw = normalize_piece_raw(&piece.raw);
        if prev_ended_with_space {
            raw = raw.trim_start_matches(' ').to_owned();
        }
        if raw.is_empty() {
            continue;
        }
        prev_ended_with_space = raw.ends_with(' ');
        lines.last_mut().unwrap().push(Piece {
            raw,
            cue: piece.cue.clone(),
            is_break: false,
        });
    }

    lines
}

fn finalize_logical_line(line: &[Piece]) -> (String, Vec<Cue>) {
    let raw_line = concat_piece_raw(line);
    if raw_line.is_empty() {
        return (String::new(), Vec::new());
    }

    let left_trim_bytes = raw_line.len() - raw_line.trim_start().len();
    let right_trim_bytes = raw_line.len() - raw_line.trim_end().len();
    let trimmed_end = raw_line.len().saturating_sub(right_trim_bytes).max(left_trim_bytes);
    let trimmed = raw_line.trim().to_owned();

    let mut cues = Vec::with_capacity(line.len());
    let mut cursor = 0_usize;
    for piece in line {
        let piece_end = cursor + piece.raw.len();
        if let Some(mut cue) = piece.cue.clone() {
            let byte_start = cursor.max(left_trim_bytes);
            let byte_end = piece_end.min(trimmed_end);
            if byte_start < byte_end {
                cue.value = raw_line[byte_start..byte_end].to_owned();
                cue.byte_start = byte_start - left_trim_bytes;
                cue.byte_end = byte_end - left_trim_bytes - 1;
                cues.push(cue);
            }
        }
        cursor = piece_end;
    }

    (trimmed, cues)
}

fn normalize_piece_raw(raw: &str) -> String {
    collapse_ttml_whitespace(&sanitize_ttml_piece_raw(raw))
}

fn sanitize_ttml_piece_raw(raw: &str) -> String {
    let no_tags = HTML_TAG_RE.replace_all(raw, "");
    no_tags
        .replace("&amp;", "&")
        .replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&quot;", "\"")
        .replace("&#39;", "'")
        .replace("&apos;", "'")
}

fn collapse_ttml_whitespace(raw: &str) -> String {
    let mut out = String::with_capacity(raw.len());
    let mut prev_space = false;
    for ch in raw.chars() {
        if matches!(ch, ' ' | '\t' | '\n' | '\r') {
            if !prev_space {
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

fn concat_piece_raw(pieces: &[Piece]) -> String {
    pieces
        .iter()
        .map(|piece| normalize_piece_raw(&piece.raw))
        .collect()
}

fn pieces_contain_cue(pieces: &[Piece]) -> bool {
    pieces.iter().any(|piece| piece.cue.is_some())
}

fn normalize_cue_lines(lines: Vec<Line>) -> Vec<Line> {
    if lines.is_empty() {
        return lines;
    }

    let mut normalized = lines;
    for i in 0..normalized.len() {
        if normalized[i].cue.is_empty() {
            continue;
        }

        let fallback_end = normalized[i]
            .end
            .or_else(|| normalized.get(i + 1).and_then(|next| next.start));

        let line = std::mem::replace(
            &mut normalized[i],
            Line {
                start: None,
                end: None,
                value: String::new(),
                cue: Vec::new(),
            },
        );
        normalized[i] = normalize_cue_line(line, fallback_end);
    }
    normalized
}

fn normalize_cue_line(mut line: Line, fallback_end: Option<i64>) -> Line {
    if line.cue.is_empty() {
        return line;
    }
    line.cue = normalize_cue_ends_by_agent(&line.cue, fallback_end);
    normalize_line_timing(line)
}

fn normalize_line_timing(mut line: Line) -> Line {
    if line.cue.is_empty() {
        return line;
    }

    let mut earliest_start = None;
    let mut latest_end = None;
    for token in &line.cue {
        if let Some(start) = token.start {
            earliest_start = Some(earliest_start.map_or(start, |e: i64| e.min(start)));
        }
        let candidate_end = token.end.or(token.start);
        if let Some(end) = candidate_end {
            latest_end = Some(latest_end.map_or(end, |e: i64| e.max(end)));
        }
    }

    if line.start.is_none() {
        line.start = earliest_start;
    }
    if line.end.is_none() {
        line.end = latest_end;
    }
    line
}

fn normalize_cue_ends_by_agent(cues: &[Cue], fallback_end: Option<i64>) -> Vec<Cue> {
    let mut groups: HashMap<String, Vec<usize>> = HashMap::new();
    let mut order = Vec::new();
    for (i, cue) in cues.iter().enumerate() {
        let id = cue.agent_id.clone().unwrap_or_default();
        if !groups.contains_key(&id) {
            order.push(id.clone());
        }
        groups.entry(id).or_default().push(i);
    }

    if order.len() <= 1 {
        return normalize_cue_ends(cues, fallback_end);
    }

    let mut out = cues.to_vec();
    for id in order {
        let idxs = &groups[&id];
        let group: Vec<Cue> = idxs.iter().map(|&pos| cues[pos].clone()).collect();
        let group = normalize_cue_ends(&group, fallback_end);
        for (gi, &pos) in idxs.iter().enumerate() {
            out[pos] = group[gi].clone();
        }
    }
    out
}

fn normalize_cue_ends(cues: &[Cue], fallback_end: Option<i64>) -> Vec<Cue> {
    if cues.is_empty() {
        return Vec::new();
    }

    let mut out = cues.to_vec();
    for i in 0..out.len() {
        let mut end = out[i].end;
        if end.is_none() {
            end = out
                .get(i + 1)
                .and_then(|next| next.start)
                .or(fallback_end);
        }
        if let Some(end_val) = end {
            if let Some(next_start) = out.get(i + 1).and_then(|next| next.start)
                && end_val > next_start
            {
                end = Some(next_start);
            }
            if let Some(start) = out[i].start
                && end_val < start
            {
                end = Some(start);
            }
        }
        out[i].end = end;
    }

    if out.iter().any(|cue| cue.end.is_none()) {
        for cue in &mut out {
            cue.end = None;
        }
    }
    out
}

fn parse_duration_expression(expr: &str, params: TimingParams) -> Option<i64> {
    let (value, _, ok) = parse_time_expression_inner(expr, params)?;
    ok.then_some(value)
}

fn parse_time_expression(expr: &str, params: TimingParams) -> Option<(i64, TimeKind)> {
    let (value, kind, ok) = parse_time_expression_inner(expr, params)?;
    ok.then_some((value, kind))
}

fn parse_time_expression_inner(
    expr: &str,
    params: TimingParams,
) -> Option<(i64, TimeKind, bool)> {
    let expr = expr.trim();
    if expr.is_empty() {
        return Some((0, TimeKind::Offset, false));
    }

    let lower = expr.to_ascii_lowercase();
    if lower.contains("wallclock(") || lower.contains(".begin") || lower.contains(".end") {
        return Some((0, TimeKind::Offset, false));
    }

    if let Ok(value) = lower.parse::<f64>()
        && value >= 0.0
    {
        return Some((
            (value * 1000.0).round() as i64,
            TimeKind::Ambiguous,
            true,
        ));
    }

    if let Some(caps) = OFFSET_TIME_RE.captures(&lower)
        && caps.len() == 3
    {
        let value: f64 = caps[1].parse().ok()?;
        let seconds = match &caps[2] {
            "h" => value * 60.0 * 60.0,
            "m" => value * 60.0,
            "s" => value,
            "ms" => value / 1000.0,
            "f" => value / params.frame_rate,
            "t" => value / params.tick_rate,
            _ => return Some((0, TimeKind::Offset, false)),
        };
        return Some((
            (seconds * 1000.0).round() as i64,
            TimeKind::Offset,
            true,
        ));
    }

    let colon_count = expr.matches(':').count();
    match colon_count {
        1 | 2 => {
            let clock_ms = parse_clock_time(expr)?;
            Some((clock_ms, TimeKind::Absolute, true))
        }
        3 => {
            let frames_ms = parse_frame_time(expr, params)?;
            Some((frames_ms, TimeKind::Absolute, true))
        }
        _ => Some((0, TimeKind::Offset, false)),
    }
}

fn resolve_time(value: i64, kind: TimeKind, base: i64, parent: &TimingContext) -> i64 {
    match kind {
        TimeKind::Absolute => value,
        TimeKind::Offset => base + value,
        TimeKind::Ambiguous => {
            let absolute = value;
            let offset = base + value;

            if !parent.has_begin && !parent.has_end && base != 0 {
                return absolute;
            }

            if parent.has_begin && parent.has_end {
                let absolute_in_parent =
                    absolute >= parent.begin && absolute <= parent.end;
                let offset_in_parent = offset >= parent.begin && offset <= parent.end;
                if absolute_in_parent && !offset_in_parent {
                    return absolute;
                }
                if offset_in_parent && !absolute_in_parent {
                    return offset;
                }
            }

            if parent.has_begin {
                if absolute < parent.begin && offset >= parent.begin {
                    return offset;
                }
                if absolute >= parent.begin && offset > absolute {
                    return absolute;
                }
            }
            offset
        }
    }
}

fn parse_clock_time(value: &str) -> Option<i64> {
    let parts: Vec<&str> = value.split(':').collect();
    if parts.len() != 2 && parts.len() != 3 {
        return None;
    }

    let (hours, minutes_idx) = if parts.len() == 3 {
        (parts[0].parse::<i64>().ok()?, 1)
    } else {
        (0, 0)
    };
    let minutes = parts[minutes_idx].parse::<i64>().ok()?;
    let seconds = parts[minutes_idx + 1].parse::<f64>().ok()?;
    let total_seconds = (hours * 60 * 60 + minutes * 60) as f64 + seconds;
    Some((total_seconds * 1000.0).round() as i64)
}

fn parse_frame_time(value: &str, params: TimingParams) -> Option<i64> {
    let parts: Vec<&str> = value.split(':').collect();
    if parts.len() != 4 {
        return None;
    }

    let hours = parts[0].parse::<i64>().ok()?;
    let minutes = parts[1].parse::<i64>().ok()?;
    let seconds = parts[2].parse::<i64>().ok()?;

    let frame_parts: Vec<&str> = parts[3].splitn(2, '.').collect();
    let frames = frame_parts[0].parse::<f64>().ok()?;
    let sub_frames = if frame_parts.len() == 2 {
        frame_parts[1].parse::<f64>().ok()?
    } else {
        0.0
    };

    let mut total_seconds = (hours * 60 * 60 + minutes * 60 + seconds) as f64;
    total_seconds += frames / params.frame_rate;
    total_seconds += sub_frames / (params.sub_frame_rate * params.frame_rate);
    Some((total_seconds * 1000.0).round() as i64)
}

fn attr_value(start: &BytesStart<'_>, key: &str) -> Option<String> {
    for attr in start.attributes().flatten() {
        let local = local_name(attr.key.as_ref());
        if local.eq_ignore_ascii_case(key) {
            return attr
                .unescape_value()
                .ok()
                .map(|v| v.trim().to_owned());
        }
    }
    None
}

fn attr_or_empty(start: &BytesStart<'_>, key: &str) -> String {
    attr_value(start, key).unwrap_or_default()
}

fn local_name(qname: &[u8]) -> String {
    let name = std::str::from_utf8(qname).unwrap_or_default();
    name.rsplit(':').next().unwrap_or(name).to_ascii_lowercase()
}

fn names_match(end: &[u8], start: &[u8]) -> bool {
    local_name(end) == local_name(start)
}

fn normalize_lyric_lang(lang: &str) -> String {
    let lang = lang.trim().to_ascii_lowercase();
    if lang.is_empty() {
        "xxx".to_owned()
    } else {
        lang
    }
}

fn sanitize_ttml_text(raw: &str) -> String {
    let no_tags = HTML_TAG_RE.replace_all(raw, "");
    let unescaped = no_tags
        .replace("&amp;", "&")
        .replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&quot;", "\"")
        .replace("&#39;", "'")
        .replace("&apos;", "'");
    let normalized = unescaped.replace("\r\n", "\n").replace('\r', "\n");
    let lines: Vec<&str> = normalized
        .split('\n')
        .map(str::trim)
        .collect();
    lines.join("\n").trim().to_owned()
}

static HTML_TAG_RE: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"</?[A-Za-z][^>]*>").unwrap());

fn lines_are_synced(lines: &[Line]) -> bool {
    lines.iter().any(|line| {
        line.start.is_some() || line.cue.iter().any(|cue| cue.start.is_some())
    })
}

fn background_agent_id(agent_id: &str) -> String {
    format!("{BACKGROUND_AGENT_PREFIX}{agent_id}")
}

fn is_background_agent_id(agent_id: &str) -> bool {
    agent_id.starts_with(BACKGROUND_AGENT_PREFIX)
}

fn context_has_role(roles: &str, role: &str) -> bool {
    let lower_role = role.to_ascii_lowercase();
    roles
        .to_ascii_lowercase()
        .split_whitespace()
        .any(|candidate| candidate == lower_role)
}

fn append_roles(existing: &str, roles: &str) -> String {
    let mut existing = existing.to_owned();
    for role in roles.split_whitespace() {
        if context_has_role(&existing, role) {
            continue;
        }
        if existing.is_empty() {
            existing = role.to_owned();
        } else {
            existing.push(' ');
            existing.push_str(role);
        }
    }
    existing
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse(default_lang: &str, xml: &str) -> Vec<Lyrics> {
        parse_ttml_list(default_lang, xml).expect("parse TTML")
    }

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
        let list = parse("eng", xml);
        assert_eq!(list.len(), 1);
        let lyrics = &list[0];
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
    fn parses_frame_rate_multi_lang() {
        let xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttp="http://www.w3.org/ns/ttml#parameter" ttp:frameRate="30" ttp:subFrameRate="2" ttp:tickRate="10">
  <body>
    <div xml:lang="eng" begin="1s">
      <p begin="2s">Line one</p>
      <p begin="00:00:04:15.1"><span>Line two</span><br/>with break</p>
    </div>
    <div xml:lang="por">
      <p begin="45t">Linha</p>
    </div>
  </body>
</tt>"#;
        let list = parse("xxx", xml);
        assert_eq!(list.len(), 2);

        let eng = &list[0];
        assert_eq!(eng.lang, "eng");
        assert!(eng.synced);
        assert_eq!(eng.line[0].start, Some(3000));
        assert_eq!(eng.line[0].value, "Line one");
        assert_eq!(eng.line[1].start, Some(4517));
        assert_eq!(eng.line[1].value, "Line two\nwith break");

        let por = &list[1];
        assert_eq!(por.lang, "por");
        assert_eq!(por.line[0].start, Some(4500));
        assert_eq!(por.line[0].value, "Linha");
    }

    #[test]
    fn skips_wallclock_cues() {
        let xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml">
  <body xml:lang="eng">
    <div>
      <p begin="wallclock(2026-01-01T00:00:00Z)">Skip me</p>
      <p begin="1s">Keep me</p>
    </div>
  </body>
</tt>"#;
        let list = parse("xxx", xml);
        assert_eq!(list.len(), 1);
        assert_eq!(list[0].line.len(), 1);
        assert_eq!(list[0].line[0].start, Some(1000));
        assert_eq!(list[0].line[0].value, "Keep me");
    }

    #[test]
    fn parses_bare_decimal_seconds() {
        let xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml">
  <body xml:lang="eng" begin="10">
    <div>
      <p begin="0.170">First line</p>
      <p begin="3.710">Second line</p>
    </div>
  </body>
</tt>"#;
        let list = parse("xxx", xml);
        assert_eq!(list.len(), 1);
        assert_eq!(list[0].line.len(), 2);
        assert_eq!(list[0].line[0].start, Some(10170));
        assert_eq!(list[0].line[0].value, "First line");
        assert_eq!(list[0].line[1].start, Some(13710));
        assert_eq!(list[0].line[1].value, "Second line");
    }

    #[test]
    fn extracts_word_timing_with_background_role() {
        let xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata">
  <body xml:lang="eng">
    <div>
      <p begin="00:01.000" end="00:03.000">
        <span begin="00:01.000" end="00:01.400">He</span><span begin="00:01.400" end="00:01.800">llo</span>
        <span ttm:role="x-bg"><span begin="00:02.000" end="00:02.500">echo</span></span>
      </p>
    </div>
  </body>
</tt>"#;
        let list = parse("xxx", xml);
        assert_eq!(list.len(), 1);
        assert_eq!(
            list[0].agents,
            vec![
                Agent {
                    id: "main".to_owned(),
                    role: "main".to_owned(),
                    name: String::new(),
                },
                Agent {
                    id: "__nd_bg__|main".to_owned(),
                    role: "bg".to_owned(),
                    name: String::new(),
                },
            ]
        );

        let line = &list[0].line[0];
        assert_eq!(line.start, Some(1000));
        assert_eq!(line.value, "Hello echo");
        assert_eq!(line.end, Some(3000));
        assert_eq!(line.cue.len(), 3);
        assert_eq!(line.cue[0].start, Some(1000));
        assert_eq!(line.cue[0].end, Some(1400));
        assert_eq!(line.cue[0].value, "He");
        assert_eq!(line.cue[0].byte_start, 0);
        assert_eq!(line.cue[0].byte_end, 1);
        assert_eq!(line.cue[0].agent_id.as_deref(), Some("main"));
        assert_eq!(line.cue[2].agent_id.as_deref(), Some("__nd_bg__|main"));
    }

    #[test]
    fn parses_itunes_metadata_tracks() {
        let xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:itunes="http://music.apple.com/lyric-ttml-internal">
  <head>
    <metadata>
      <iTunesMetadata xmlns="http://music.apple.com/lyric-ttml-internal">
        <translations>
          <translation xml:lang="es">
            <text for="L1">Hola</text>
            <text for="MISSING">Skip me</text>
          </translation>
        </translations>
        <transliterations>
          <transliteration xml:lang="ja-Latn">
            <text for="L2"><span begin="00:02.000" end="00:02.300" xmlns="http://www.w3.org/ns/ttml">ko</span><span begin="00:02.300" end="00:02.600" xmlns="http://www.w3.org/ns/ttml">nni</span></text>
          </transliteration>
        </transliterations>
      </iTunesMetadata>
    </metadata>
  </head>
  <body xml:lang="ja">
    <div>
      <p begin="00:01.000" end="00:01.500" itunes:key="L1">こんにちは</p>
      <p begin="00:02.000" end="00:02.700" itunes:key="L2">こんばんは</p>
    </div>
  </body>
</tt>"#;
        let list = parse("xxx", xml);
        assert_eq!(list.len(), 3);

        assert_eq!(list[0].kind, "main");
        assert_eq!(list[0].lang, "ja");
        assert_eq!(list[0].line.len(), 2);

        assert_eq!(list[1].kind, "translation");
        assert_eq!(list[1].lang, "es");
        assert_eq!(list[1].line.len(), 1);
        assert_eq!(list[1].line[0].start, Some(1000));
        assert_eq!(list[1].line[0].value, "Hola");
        assert_eq!(list[1].line[0].end, Some(1500));

        assert_eq!(list[2].kind, "pronunciation");
        assert_eq!(list[2].lang, "ja-latn");
        assert_eq!(list[2].line.len(), 1);
        assert_eq!(list[2].line[0].start, Some(2000));
        assert_eq!(list[2].line[0].value, "konni");
        assert_eq!(list[2].line[0].end, Some(2600));
        assert_eq!(list[2].line[0].cue.len(), 2);
    }
}
