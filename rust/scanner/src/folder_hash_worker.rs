use std::io::{self, BufRead, BufReader, BufWriter, Write};

use anyhow::{Context, Result};
use serde::Serialize;

use crate::scan::{folder_hash_from_input, FolderHashInput};

#[derive(Debug, Serialize)]
struct HashResponse {
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    hash: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

pub fn run() -> Result<()> {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut input = BufReader::with_capacity(64 * 1024, stdin.lock());
    let mut output = BufWriter::with_capacity(16 * 1024, stdout.lock());
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

        match serde_json::from_str::<FolderHashInput>(trimmed) {
            Ok(request) => {
                let hash = folder_hash_from_input(&request);
                write_response(&mut output, true, Some(hash), None)?;
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
    hash: Option<String>,
    error: Option<String>,
) -> Result<()> {
    let response = HashResponse { ok, hash, error };
    serde_json::to_writer(&mut *output, &response).context("encoding folder hash response")?;
    output.write_all(b"\n").context("framing folder hash response")?;
    Ok(())
}
