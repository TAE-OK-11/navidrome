use std::env;

use anyhow::{Result, bail};
use navidrome_grpc_listen::{arg_value, bind_tcp, default_listen, shutdown};
use navidrome_scanner::grpc::{FolderHashService, ScannerService};
use navidrome_scanner::proto::folder_hash_server::FolderHashServer;
use navidrome_scanner::proto::scanner_server::ScannerServer;
use tonic::transport::Server;

#[tokio::main]
async fn main() -> Result<()> {
    let args: Vec<String> = env::args().collect();
    if args.iter().any(|a| a == "--grpc-worker") {
        let listen =
            arg_value(&args, "--listen").unwrap_or_else(|| default_listen("navidrome-scanner"));
        return serve(listen).await;
    }
    let mut rest = args.iter().skip(1);
    if let Some(command) = rest.next() {
        if command == "--folder-hash-worker" {
            if rest.next().is_some() {
                bail!("--folder-hash-worker accepts no arguments");
            }
            return navidrome_scanner::folder_hash_worker::run();
        }
    }
    navidrome_scanner::run()
}

async fn serve(listen: String) -> Result<()> {
    let folder_hash = FolderHashServer::new(FolderHashService);
    let scanner = ScannerServer::new(ScannerService);
    if let Some(path) = listen.strip_prefix("unix:") {
        #[cfg(unix)]
        {
            let incoming = navidrome_grpc_listen::bind_unix(path).await?;
            return Server::builder()
                .add_service(folder_hash)
                .add_service(scanner)
                .serve_with_incoming_shutdown(incoming, shutdown())
                .await
                .map_err(|err| anyhow::anyhow!(err));
        }
        #[cfg(not(unix))]
        {
            let _ = path;
            bail!("unix sockets are not supported on this platform");
        }
    }
    let incoming = bind_tcp(&listen).await?;
    Server::builder()
        .add_service(folder_hash)
        .add_service(scanner)
        .serve_with_incoming_shutdown(incoming, shutdown())
        .await
        .map_err(|err| anyhow::anyhow!(err))
}
