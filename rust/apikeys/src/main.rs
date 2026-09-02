use std::env;

use anyhow::{Result, bail};
use navidrome_apikeys::run_worker;

fn main() -> Result<()> {
    let args: Vec<String> = env::args().collect();
    if args.len() == 2 && args[1] == "--apikeys-worker" {
        return run_worker();
    }
    bail!(
        "usage: {} --apikeys-worker",
        args.first().map(String::as_str).unwrap_or("navidrome-apikeys")
    );
}
