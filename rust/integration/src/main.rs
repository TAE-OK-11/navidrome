use std::env;

use anyhow::{Result, bail};
use navidrome_grpc_listen::{
    arg_value, bind_tcp, default_listen, local_ipc_server, shutdown, LOCAL_MAX_MSG,
};
use navidrome_integration::OutboundService;
use navidrome_integration::proto::outbound_server::OutboundServer;

#[tokio::main]
async fn main() -> Result<()> {
    let args: Vec<String> = env::args().collect();
    if args.iter().any(|a| a == "--grpc-worker") {
        let listen =
            arg_value(&args, "--listen").unwrap_or_else(|| default_listen("navidrome-integration"));
        return serve(listen).await;
    }
    bail!(
        "usage: {} --grpc-worker [--listen unix:/path/to.sock|127.0.0.1:0]",
        args.first()
            .map(String::as_str)
            .unwrap_or("navidrome-integration")
    );
}

async fn serve(listen: String) -> Result<()> {
    let service = OutboundServer::new(OutboundService::new()?)
        .max_decoding_message_size(LOCAL_MAX_MSG)
        .max_encoding_message_size(LOCAL_MAX_MSG);
    if let Some(path) = listen.strip_prefix("unix:") {
        #[cfg(unix)]
        {
            let incoming = navidrome_grpc_listen::bind_unix(path).await?;
            return local_ipc_server()
                .add_service(service)
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
    local_ipc_server()
        .add_service(service)
        .serve_with_incoming_shutdown(incoming, shutdown())
        .await
        .map_err(|err| anyhow::anyhow!(err))
}
