use std::time::Duration;

use anyhow::{Context, Result};
use tokio::signal;
use tonic::transport::Server;

/// Match Go `core/rustworker.DialGRPC` local IPC windows (1 MiB stream / 2 MiB conn).
pub const LOCAL_STREAM_WINDOW: u32 = 1 << 20;
pub const LOCAL_CONN_WINDOW: u32 = 2 << 20;
pub const LOCAL_MAX_FRAME: u32 = 64 * 1024;
/// Soft HTTP/2 keepalive matching Go client Time/Timeout (unix peers usually
/// notice via socket close; pings still help TCP fallback / long-idle workers).
pub const LOCAL_KEEPALIVE_INTERVAL: Duration = Duration::from_secs(120);
pub const LOCAL_KEEPALIVE_TIMEOUT: Duration = Duration::from_secs(5);
/// Match Go `maxGRPCMsgSize` (64 MiB) for bulk metadata/search payloads.
pub const LOCAL_MAX_MSG: usize = 64 << 20;

/// Tonic server tuned for Navidrome Go↔Rust unix/TCP companion IPC.
pub fn local_ipc_server() -> Server {
    Server::builder()
        .initial_stream_window_size(LOCAL_STREAM_WINDOW)
        .initial_connection_window_size(LOCAL_CONN_WINDOW)
        .max_frame_size(LOCAL_MAX_FRAME)
        .http2_keepalive_interval(Some(LOCAL_KEEPALIVE_INTERVAL))
        .http2_keepalive_timeout(Some(LOCAL_KEEPALIVE_TIMEOUT))
}

pub fn arg_value(args: &[String], name: &str) -> Option<String> {
    args.windows(2).find_map(|pair| {
        if pair[0] == name {
            Some(pair[1].clone())
        } else {
            None
        }
    })
}

pub fn default_listen(prefix: &str) -> String {
    #[cfg(unix)]
    {
        let path = std::env::temp_dir().join(format!("{prefix}-{}.sock", std::process::id()));
        return format!("unix:{}", path.display());
    }
    #[cfg(not(unix))]
    {
        let _ = prefix;
        "127.0.0.1:0".to_string()
    }
}

pub async fn shutdown() {
    let _ = signal::ctrl_c().await;
}

#[cfg(unix)]
pub async fn bind_unix(path: &str) -> Result<tokio_stream::wrappers::UnixListenerStream> {
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
    Ok(UnixListenerStream::new(listener))
}

pub async fn bind_tcp(listen: &str) -> Result<tokio_stream::wrappers::TcpListenerStream> {
    use std::net::SocketAddr;

    use tokio_stream::wrappers::TcpListenerStream;

    let addr: SocketAddr = listen.parse().context("parse listen address")?;
    let listener = tokio::net::TcpListener::bind(addr)
        .await
        .with_context(|| format!("bind {addr}"))?;
    let bound = listener.local_addr()?;
    println!("READY {bound}");
    Ok(TcpListenerStream::new(listener))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn local_windows_match_go_dial() {
        assert_eq!(LOCAL_STREAM_WINDOW, 1 << 20);
        assert_eq!(LOCAL_CONN_WINDOW, 2 << 20);
        assert_eq!(LOCAL_MAX_MSG, 64 << 20);
        assert_eq!(LOCAL_KEEPALIVE_INTERVAL, Duration::from_secs(120));
        assert_eq!(LOCAL_KEEPALIVE_TIMEOUT, Duration::from_secs(5));
    }
}
