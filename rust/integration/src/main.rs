use std::env;
use std::net::SocketAddr;

use anyhow::{Context, Result, bail};
use navidrome_integration::OutboundService;
use navidrome_integration::proto::outbound_server::OutboundServer;
use tokio::signal;
use tonic::transport::Server;

#[tokio::main]
async fn main() -> Result<()> {
    let args: Vec<String> = env::args().collect();
    if args.iter().any(|a| a == "--grpc-worker") {
        let listen = arg_value(&args, "--listen").unwrap_or_else(default_listen);
        return serve(listen).await;
    }
    bail!(
        "usage: {} --grpc-worker [--listen unix:/path/to.sock|127.0.0.1:0]",
        args.first()
            .map(String::as_str)
            .unwrap_or("navidrome-integration")
    );
}

fn arg_value(args: &[String], name: &str) -> Option<String> {
    args.windows(2).find_map(|pair| {
        if pair[0] == name {
            Some(pair[1].clone())
        } else {
            None
        }
    })
}

fn default_listen() -> String {
    #[cfg(unix)]
    {
        let path =
            std::env::temp_dir().join(format!("navidrome-integration-{}.sock", std::process::id()));
        return format!("unix:{}", path.display());
    }
    #[cfg(not(unix))]
    {
        "127.0.0.1:0".to_string()
    }
}

async fn serve(listen: String) -> Result<()> {
    let service = OutboundService::new().context("building outbound HTTP client")?;
    if let Some(path) = listen.strip_prefix("unix:") {
        return serve_unix(path, service).await;
    }
    serve_tcp(&listen, service).await
}

#[cfg(unix)]
async fn serve_unix(path: &str, service: OutboundService) -> Result<()> {
    use std::path::PathBuf;

    use tokio::net::UnixListener;
    use tokio_stream::wrappers::UnixListenerStream;

    let sock = PathBuf::from(path);
    if sock.exists() {
        std::fs::remove_file(&sock).ok();
    }
    if let Some(parent) = sock.parent() {
        std::fs::create_dir_all(parent).ok();
    }
    let listener = UnixListener::bind(&sock).with_context(|| format!("bind {}", sock.display()))?;
    println!("READY unix:{}", sock.display());
    let incoming = UnixListenerStream::new(listener);
    Server::builder()
        .add_service(OutboundServer::new(service))
        .serve_with_incoming_shutdown(incoming, shutdown())
        .await
        .context("gRPC unix server")?;
    let _ = std::fs::remove_file(&sock);
    Ok(())
}

#[cfg(not(unix))]
async fn serve_unix(_path: &str, _service: OutboundService) -> Result<()> {
    bail!("unix sockets are not supported on this platform")
}

async fn serve_tcp(listen: &str, service: OutboundService) -> Result<()> {
    use tokio_stream::wrappers::TcpListenerStream;

    let addr: SocketAddr = listen.parse().context("parse listen address")?;
    let listener = tokio::net::TcpListener::bind(addr)
        .await
        .with_context(|| format!("bind {addr}"))?;
    let bound = listener.local_addr()?;
    println!("READY {bound}");
    let incoming = TcpListenerStream::new(listener);
    Server::builder()
        .add_service(OutboundServer::new(service))
        .serve_with_incoming_shutdown(incoming, shutdown())
        .await
        .context("gRPC tcp server")?;
    Ok(())
}

async fn shutdown() {
    let _ = signal::ctrl_c().await;
}
