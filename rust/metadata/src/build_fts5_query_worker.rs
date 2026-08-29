use std::io::{self, BufRead, BufReader, BufWriter, Write};

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

#[derive(Debug, Deserialize)]
struct BuildRequest {
    query: String,
}

#[derive(Debug, Serialize)]
struct BuildResponse {
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    query: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    degraded: Option<bool>,
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

        match serde_json::from_str::<BuildRequest>(line) {
            Ok(request) => {
                let built = fts_normalize::build_fts5_query(&request.query);
                write_response(
                    &mut output,
                    true,
                    Some(built.query),
                    Some(built.degraded),
                    None,
                )?;
            }
            Err(error) => write_response(&mut output, false, None, None, Some(error.to_string()))?,
        }
        output.flush()?;
    }
    Ok(())
}

fn write_response(
    output: &mut impl Write,
    ok: bool,
    query: Option<String>,
    degraded: Option<bool>,
    error: Option<String>,
) -> Result<()> {
    let response = BuildResponse {
        ok,
        query,
        degraded,
        error,
    };
    serde_json::to_writer(&mut *output, &response).context("encoding build fts5 query response")?;
    output.write_all(b"\n").context("framing build fts5 query response")?;
    Ok(())
}
