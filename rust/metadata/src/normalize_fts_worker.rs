use std::io::{self, BufRead, BufReader, BufWriter, Write};

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

#[derive(Debug, Deserialize)]
struct NormalizeRequest {
    values: Vec<String>,
}

#[derive(Debug, Serialize)]
struct NormalizeResponse {
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    normalized: Option<String>,
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

        match serde_json::from_str::<NormalizeRequest>(line) {
            Ok(request) => {
                let normalized = crate::normalize_fts::normalize_for_fts(&request.values);
                write_response(&mut output, true, Some(normalized), None)?;
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
    normalized: Option<String>,
    error: Option<String>,
) -> Result<()> {
    let response = NormalizeResponse {
        ok,
        normalized,
        error,
    };
    serde_json::to_writer(&mut *output, &response).context("encoding normalize response")?;
    output.write_all(b"\n").context("framing normalize response")?;
    Ok(())
}
