use std::io::{self, BufRead, BufReader, BufWriter, Write};

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

use navidrome_metadata::tag_clean::TagMappingConfig;

#[derive(Debug, Deserialize)]
struct CleanRequest {
    path: String,
    tags: HashMap<String, Vec<String>>,
    #[serde(default)]
    mappings: HashMap<String, TagMappingConfig>,
    #[serde(default)]
    artist_split_exceptions: Vec<String>,
}

#[derive(Debug, Serialize)]
struct CleanResponse {
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    tags: Option<HashMap<String, Vec<String>>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

pub fn run() -> Result<()> {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut input = BufReader::with_capacity(64 * 1024, stdin.lock());
    let mut output = BufWriter::with_capacity(64 * 1024, stdout.lock());
    let mut line = String::with_capacity(4096);

    loop {
        line.clear();
        if input.read_line(&mut line)? == 0 {
            break;
        }
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }

        match serde_json::from_str::<CleanRequest>(trimmed) {
            Ok(request) => {
                let cleaned = navidrome_metadata::tag_clean::clean(
                    &request.path,
                    &request.tags,
                    &request.mappings,
                    &request.artist_split_exceptions,
                );
                write_response(&mut output, true, Some(cleaned), None)?;
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
    tags: Option<HashMap<String, Vec<String>>>,
    error: Option<String>,
) -> Result<()> {
    let response = CleanResponse { ok, tags, error };
    serde_json::to_writer(&mut *output, &response).context("encoding clean tags response")?;
    output.write_all(b"\n").context("framing clean tags response")?;
    Ok(())
}
