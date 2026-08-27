use std::collections::HashMap;
use std::io::{self, BufRead, BufReader, BufWriter, Write};
use std::path::PathBuf;

use anyhow::{Context, Result};
use navidrome_metadata::map_media;
use serde::{Deserialize, Serialize};

#[derive(Debug, Deserialize)]
struct MapMediaRequest {
    tags: HashMap<String, Vec<String>>,
    path: PathBuf,
    #[serde(default)]
    lyrics_json: String,
}

#[derive(Debug, Serialize)]
struct MapMediaResponse {
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    media_file_json: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

pub fn run() -> Result<()> {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut input = BufReader::with_capacity(16 * 1024, stdin.lock());
    let mut output = BufWriter::with_capacity(16 * 1024, stdout.lock());

    loop {
        let mut line = String::new();
        if input.read_line(&mut line)? == 0 {
            break;
        }
        let line = line.trim();
        if line.is_empty() {
            continue;
        }

        match serde_json::from_str::<MapMediaRequest>(line) {
            Ok(request) => {
                let lyrics = if request.lyrics_json.is_empty() {
                    "[]"
                } else {
                    &request.lyrics_json
                };
                match map_media::map_to_json(&request.tags, &request.path, Some(lyrics)) {
                    Some(json) => write_response(&mut output, true, Some(json), None)?,
                    None => write_response(
                        &mut output,
                        false,
                        None,
                        Some("map_media returned no result (missing title and album)".to_owned()),
                    )?,
                }
            }
            Err(error) => write_response(&mut output, false, None, Some(error.to_string()))?,
        }
        output.flush()?;
    }
    Ok(())
}

fn write_response(
    output: &mut impl Write,
    ok: bool,
    media_file_json: Option<String>,
    error: Option<String>,
) -> Result<()> {
    let response = MapMediaResponse {
        ok,
        media_file_json,
        error,
    };
    serde_json::to_writer(&mut *output, &response).context("encoding map media response")?;
    output.write_all(b"\n").context("framing map media response")?;
    Ok(())
}
