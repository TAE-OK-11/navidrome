use std::io::{self, BufRead, BufReader, BufWriter, Read, Write};

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

const MAX_INPUT_BYTES: usize = 16 * 1024 * 1024;

#[derive(Debug, Deserialize)]
struct ParseLyricsRequest {
    suffix: String,
    lang: String,
    input_size: usize,
}

#[derive(Debug, Serialize)]
struct ParseLyricsResponse {
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    lyrics_json: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

pub fn run() -> Result<()> {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut input = BufReader::with_capacity(64 * 1024, stdin.lock());
    let mut output = BufWriter::with_capacity(64 * 1024, stdout.lock());

    loop {
        let mut header = String::new();
        if input.read_line(&mut header)? == 0 {
            break;
        }
        let header = header.trim();
        if header.is_empty() {
            continue;
        }

        match serde_json::from_str::<ParseLyricsRequest>(header) {
            Ok(request) => {
                if request.input_size == 0 {
                    write_response(&mut output, false, None, Some("empty lyrics payload".to_owned()))?;
                    continue;
                }
                if request.input_size > MAX_INPUT_BYTES {
                    write_response(
                        &mut output,
                        false,
                        None,
                        Some(format!(
                            "lyrics payload exceeds maximum size of {MAX_INPUT_BYTES} bytes"
                        )),
                    )?;
                    continue;
                }

                let mut payload = vec![0_u8; request.input_size];
                if let Err(error) = read_exact_payload(&mut input, &mut payload) {
                    write_response(&mut output, false, None, Some(error))?;
                    continue;
                }

                match crate::lyrics::parse_lyrics_external(
                    &request.suffix,
                    &request.lang,
                    &payload,
                ) {
                    Ok(json) => write_response(&mut output, true, Some(json), None)?,
                    Err(error) => write_response(&mut output, false, None, Some(error))?,
                }
            }
            Err(error) => write_response(&mut output, false, None, Some(error.to_string()))?,
        }
        output.flush()?;
    }
    Ok(())
}

fn read_exact_payload(input: &mut impl Read, payload: &mut [u8]) -> Result<(), String> {
    input
        .read_exact(payload)
        .map_err(|error| format!("reading lyrics payload: {error}"))
}

fn write_response(
    output: &mut impl Write,
    ok: bool,
    lyrics_json: Option<String>,
    error: Option<String>,
) -> Result<()> {
    let response = ParseLyricsResponse {
        ok,
        lyrics_json,
        error,
    };
    serde_json::to_writer(&mut *output, &response).context("encoding lyrics response")?;
    output.write_all(b"\n").context("framing lyrics response")?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_oversized_payload() {
        let request = ParseLyricsRequest {
            suffix: ".lrc".to_owned(),
            lang: "eng".to_owned(),
            input_size: MAX_INPUT_BYTES + 1,
        };
        let mut output = Vec::new();
        write_response(&mut output, false, None, Some("too big".to_owned())).unwrap();
        assert!(output.starts_with(b"{"));
        let _ = request;
    }
}
