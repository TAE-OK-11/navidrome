use anyhow::{Context, Result};
use tokio::signal;

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
