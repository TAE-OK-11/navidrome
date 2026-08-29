fn main() -> anyhow::Result<()> {
    let mut args = std::env::args_os().skip(1);
    if let Some(command) = args.next() {
        if command == "--folder-hash-worker" {
            if args.next().is_some() {
                anyhow::bail!("--folder-hash-worker accepts no arguments");
            }
            return navidrome_scanner::folder_hash_worker::run();
        }
    }
    navidrome_scanner::run()
}
