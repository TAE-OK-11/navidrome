use std::collections::HashSet;
use std::io::{self, BufRead, BufReader, BufWriter, Write};

use anyhow::{Context, Result, bail};
use serde::{Deserialize, Serialize};
use tantivy::collector::TopDocs;
use tantivy::query::{BooleanQuery, BoostQuery, Occur, Query, TermQuery};
use tantivy::schema::{
    Field, INDEXED, IndexRecordOption, STORED, STRING, Schema, TantivyDocument, TextFieldIndexing,
    TextOptions, Value,
};
use tantivy::tokenizer::{LowerCaser, NgramTokenizer, TextAnalyzer, Tokenizer};
use tantivy::{Index, IndexReader, IndexWriter, ReloadPolicy, Term, doc};

const PROTOCOL_VERSION: u32 = 1;
const MAX_DOCUMENTS_PER_REQUEST: usize = 250_000;
const MAX_RESULTS: usize = 500;
const WRITER_MEMORY_BYTES: usize = 32 * 1024 * 1024;
const NGRAM_TOKENIZER: &str = "navidrome_ngram";

#[derive(Debug, Clone, Deserialize)]
struct SearchDocument {
    key: String,
    id: String,
    kind: String,
    #[serde(default)]
    library_ids: Vec<u64>,
    primary: String,
    #[serde(default)]
    secondary: String,
}

#[derive(Debug, Deserialize)]
#[serde(tag = "op", rename_all = "snake_case")]
enum Request {
    Replace {
        documents: Vec<SearchDocument>,
    },
    Upsert {
        documents: Vec<SearchDocument>,
    },
    Delete {
        keys: Vec<String>,
    },
    Search {
        query: String,
        kind: String,
        #[serde(default)]
        library_ids: Vec<u64>,
        #[serde(default)]
        offset: usize,
        limit: usize,
    },
    Stats,
}

#[derive(Debug, Serialize)]
struct Hit {
    id: String,
    score: f32,
}

#[derive(Debug, Serialize)]
struct Response {
    protocol: u32,
    ok: bool,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    hits: Vec<Hit>,
    indexed: u64,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

#[derive(Clone, Copy)]
struct Fields {
    key: Field,
    id: Field,
    kind: Field,
    library_id: Field,
    exact: Field,
    primary: Field,
    secondary: Field,
}

struct Engine {
    index: Index,
    reader: IndexReader,
    writer: IndexWriter,
    fields: Fields,
    indexed: u64,
}

impl Engine {
    fn new() -> Result<Self> {
        let mut schema = Schema::builder();
        let key = schema.add_text_field("key", STRING | STORED);
        let id = schema.add_text_field("id", STORED);
        let kind = schema.add_text_field("kind", STRING);
        let library_id = schema.add_u64_field("library_id", INDEXED);
        let exact = schema.add_text_field("exact", STRING);
        let ngram_options = TextOptions::default().set_indexing_options(
            TextFieldIndexing::default()
                .set_tokenizer(NGRAM_TOKENIZER)
                .set_index_option(IndexRecordOption::WithFreqs),
        );
        let primary = schema.add_text_field("primary", ngram_options.clone());
        let secondary = schema.add_text_field("secondary", ngram_options);
        let schema = schema.build();

        let index = Index::create_in_ram(schema);
        let analyzer = TextAnalyzer::builder(NgramTokenizer::new(2, 3, false)?)
            .filter(LowerCaser)
            .build();
        index.tokenizers().register(NGRAM_TOKENIZER, analyzer);
        let writer = index.writer(WRITER_MEMORY_BYTES)?;
        let reader = index
            .reader_builder()
            .reload_policy(ReloadPolicy::Manual)
            .try_into()?;

        Ok(Self {
            index,
            reader,
            writer,
            fields: Fields {
                key,
                id,
                kind,
                library_id,
                exact,
                primary,
                secondary,
            },
            indexed: 0,
        })
    }

    fn replace(&mut self, documents: Vec<SearchDocument>) -> Result<()> {
        validate_document_batch(&documents)?;
        self.writer.delete_all_documents()?;
        self.indexed = 0;
        self.add_documents(documents)?;
        self.commit()
    }

    fn upsert(&mut self, documents: Vec<SearchDocument>) -> Result<()> {
        validate_document_batch(&documents)?;
        for document in &documents {
            self.writer
                .delete_term(Term::from_field_text(self.fields.key, &document.key));
        }
        self.add_documents(documents)?;
        self.commit()
    }

    fn delete(&mut self, keys: Vec<String>) -> Result<()> {
        if keys.len() > MAX_DOCUMENTS_PER_REQUEST {
            bail!("delete contains too many keys");
        }
        for key in keys {
            self.writer
                .delete_term(Term::from_field_text(self.fields.key, &key));
        }
        self.commit()
    }

    fn add_documents(&mut self, documents: Vec<SearchDocument>) -> Result<()> {
        for document in documents {
            let mut indexed = doc!(
                self.fields.key => document.key,
                self.fields.id => document.id,
                self.fields.kind => document.kind,
                self.fields.exact => normalize(&document.primary),
                self.fields.primary => normalize(&document.primary),
                self.fields.secondary => normalize(&document.secondary),
            );
            for library_id in document.library_ids {
                indexed.add_u64(self.fields.library_id, library_id);
            }
            self.writer.add_document(indexed)?;
            self.indexed = self.indexed.saturating_add(1);
        }
        Ok(())
    }

    fn commit(&mut self) -> Result<()> {
        self.writer.commit()?;
        self.reader.reload()?;
        self.indexed = self.reader.searcher().num_docs();
        Ok(())
    }

    fn search(
        &self,
        query: &str,
        kind: &str,
        library_ids: &[u64],
        offset: usize,
        limit: usize,
    ) -> Result<Vec<Hit>> {
        let normalized = normalize(query);
        if normalized.chars().filter(|c| c.is_alphanumeric()).count() < 2 || limit == 0 {
            return Ok(Vec::new());
        }
        let limit = limit.min(MAX_RESULTS);
        let requested = offset.saturating_add(limit).min(MAX_RESULTS);

        let text_query = self.ngram_query(&normalized)?;
        let exact_query: Box<dyn Query> = Box::new(BoostQuery::new(
            Box::new(TermQuery::new(
                Term::from_field_text(self.fields.exact, &normalized),
                IndexRecordOption::Basic,
            )),
            12.0,
        ));
        let relevance: Box<dyn Query> = Box::new(BooleanQuery::new(vec![
            (Occur::Should, exact_query),
            (Occur::Should, text_query),
        ]));

        let mut clauses: Vec<(Occur, Box<dyn Query>)> = vec![
            (
                Occur::Must,
                Box::new(TermQuery::new(
                    Term::from_field_text(self.fields.kind, kind),
                    IndexRecordOption::Basic,
                )),
            ),
            (Occur::Must, relevance),
        ];
        if !library_ids.is_empty() {
            let scopes = library_ids
                .iter()
                .map(|library_id| {
                    (
                        Occur::Should,
                        Box::new(TermQuery::new(
                            Term::from_field_u64(self.fields.library_id, *library_id),
                            IndexRecordOption::Basic,
                        )) as Box<dyn Query>,
                    )
                })
                .collect();
            clauses.push((Occur::Must, Box::new(BooleanQuery::new(scopes))));
        }

        let searcher = self.reader.searcher();
        let top_docs = searcher.search(
            &BooleanQuery::new(clauses),
            &TopDocs::with_limit(requested).order_by_score(),
        )?;
        let mut hits = Vec::with_capacity(limit);
        for (score, address) in top_docs.into_iter().skip(offset) {
            let document: TantivyDocument = searcher.doc(address)?;
            let id = document
                .get_first(self.fields.id)
                .and_then(Value::as_str)
                .context("search result is missing id")?;
            hits.push(Hit {
                id: id.to_owned(),
                score,
            });
        }
        Ok(hits)
    }

    fn ngram_query(&self, query: &str) -> Result<Box<dyn Query>> {
        let mut analyzer = self
            .index
            .tokenizers()
            .get(NGRAM_TOKENIZER)
            .context("n-gram tokenizer is not registered")?;
        let mut stream = analyzer.token_stream(query);
        let mut seen = HashSet::new();
        let mut terms = Vec::new();
        while stream.advance() {
            let token = stream.token();
            if seen.insert(token.text.clone()) {
                let primary: Box<dyn Query> = Box::new(BoostQuery::new(
                    Box::new(TermQuery::new(
                        Term::from_field_text(self.fields.primary, &token.text),
                        IndexRecordOption::WithFreqs,
                    )),
                    4.0,
                ));
                let secondary: Box<dyn Query> = Box::new(TermQuery::new(
                    Term::from_field_text(self.fields.secondary, &token.text),
                    IndexRecordOption::WithFreqs,
                ));
                terms.push((
                    Occur::Must,
                    Box::new(BooleanQuery::new(vec![
                        (Occur::Should, primary),
                        (Occur::Should, secondary),
                    ])) as Box<dyn Query>,
                ));
            }
        }
        if terms.is_empty() {
            bail!("query produced no searchable n-grams");
        }
        Ok(Box::new(BooleanQuery::new(terms)))
    }
}

fn validate_document_batch(documents: &[SearchDocument]) -> Result<()> {
    if documents.len() > MAX_DOCUMENTS_PER_REQUEST {
        bail!(
            "request contains {} documents; maximum is {MAX_DOCUMENTS_PER_REQUEST}",
            documents.len()
        );
    }
    let mut keys = HashSet::with_capacity(documents.len());
    for document in documents {
        if document.key.is_empty()
            || document.id.is_empty()
            || document.kind.is_empty()
            || document.primary.is_empty()
        {
            bail!("search documents require key, id, kind, and primary");
        }
        if !keys.insert(document.key.as_str()) {
            bail!("search document keys must be unique within a request");
        }
    }
    Ok(())
}

fn normalize(value: &str) -> String {
    let mut normalized = String::with_capacity(value.len());
    let mut needs_space = false;
    for ch in value.chars().flat_map(char::to_lowercase) {
        if ch.is_alphanumeric() {
            if needs_space && !normalized.is_empty() {
                normalized.push(' ');
            }
            normalized.push(ch);
            needs_space = false;
        } else {
            needs_space = true;
        }
    }
    normalized
}

fn main() -> Result<()> {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut output = BufWriter::with_capacity(64 * 1024, stdout.lock());
    let mut engine = Engine::new()?;

    for line in BufReader::with_capacity(256 * 1024, stdin.lock()).lines() {
        let line = line.context("reading search request")?;
        if line.trim().is_empty() {
            continue;
        }
        let response = match serde_json::from_str::<Request>(&line) {
            Ok(request) => handle_request(&mut engine, request).unwrap_or_else(|error| Response {
                protocol: PROTOCOL_VERSION,
                ok: false,
                hits: Vec::new(),
                indexed: engine.indexed,
                error: Some(format!("{error:#}")),
            }),
            Err(error) => Response {
                protocol: PROTOCOL_VERSION,
                ok: false,
                hits: Vec::new(),
                indexed: engine.indexed,
                error: Some(error.to_string()),
            },
        };
        serde_json::to_writer(&mut output, &response)?;
        output.write_all(b"\n")?;
        output.flush()?;
    }
    Ok(())
}

fn handle_request(engine: &mut Engine, request: Request) -> Result<Response> {
    let hits = match request {
        Request::Replace { documents } => {
            engine.replace(documents)?;
            Vec::new()
        }
        Request::Upsert { documents } => {
            engine.upsert(documents)?;
            Vec::new()
        }
        Request::Delete { keys } => {
            engine.delete(keys)?;
            Vec::new()
        }
        Request::Search {
            query,
            kind,
            library_ids,
            offset,
            limit,
        } => engine.search(&query, &kind, &library_ids, offset, limit)?,
        Request::Stats => Vec::new(),
    };
    Ok(Response {
        protocol: PROTOCOL_VERSION,
        ok: true,
        hits,
        indexed: engine.indexed,
        error: None,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn document(key: &str, id: &str, kind: &str, library_id: u64, primary: &str) -> SearchDocument {
        SearchDocument {
            key: key.to_owned(),
            id: id.to_owned(),
            kind: kind.to_owned(),
            library_ids: vec![library_id],
            primary: primary.to_owned(),
            secondary: String::new(),
        }
    }

    #[test]
    fn exact_name_outranks_partial_match() -> Result<()> {
        let mut engine = Engine::new()?;
        engine.replace(vec![
            document("artist:1", "1", "artist", 1, "Muse Tribute"),
            document("artist:2", "2", "artist", 1, "Muse"),
        ])?;
        let hits = engine.search("Muse", "artist", &[1], 0, 10)?;
        assert_eq!(hits.first().map(|hit| hit.id.as_str()), Some("2"));
        Ok(())
    }

    #[test]
    fn finds_korean_substrings_and_applies_library_scope() -> Result<()> {
        let mut engine = Engine::new()?;
        engine.replace(vec![
            document("artist:1", "1", "artist", 1, "방탄소년단"),
            document("artist:2", "2", "artist", 2, "소년공화국"),
        ])?;
        let hits = engine.search("소년", "artist", &[1], 0, 10)?;
        assert_eq!(hits.len(), 1);
        assert_eq!(hits[0].id, "1");
        Ok(())
    }

    #[test]
    fn upsert_and_delete_are_visible_immediately() -> Result<()> {
        let mut engine = Engine::new()?;
        engine.replace(vec![document("song:1", "1", "song", 1, "Old Name")])?;
        engine.upsert(vec![document("song:1", "1", "song", 1, "New Name")])?;
        assert!(engine.search("Old", "song", &[1], 0, 10)?.is_empty());
        assert_eq!(engine.search("New", "song", &[1], 0, 10)?.len(), 1);
        engine.delete(vec!["song:1".to_owned()])?;
        assert!(engine.search("New", "song", &[1], 0, 10)?.is_empty());
        Ok(())
    }

    #[test]
    fn normalization_preserves_unicode_words() {
        assert_eq!(normalize("  AC/DC — 소년! "), "ac dc 소년");
    }
}
