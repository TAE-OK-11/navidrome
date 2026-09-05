use std::env;

use anyhow::Result;
use navidrome_grpc_listen::{arg_value, bind_tcp, default_listen, local_ipc_server, shutdown, LOCAL_MAX_MSG};
use navidrome_search::grpc::SearchService;
use navidrome_search::proto::search_server::SearchServer;

#[tokio::main]
async fn main() -> Result<()> {
    let args: Vec<String> = env::args().collect();
    if args.iter().any(|a| a == "--grpc-worker") {
        let listen = arg_value(&args, "--listen").unwrap_or_else(|| default_listen("navidrome-search"));
        return serve(listen).await;
    }
    navidrome_search::run()
}

async fn serve(listen: String) -> Result<()> {
    let service = SearchServer::new(SearchService::new())
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
            anyhow::bail!("unix sockets are not supported on this platform");
        }
    }
    let incoming = bind_tcp(&listen).await?;
    local_ipc_server()
        .add_service(service)
        .serve_with_incoming_shutdown(incoming, shutdown())
        .await
        .map_err(|err| anyhow::anyhow!(err))
}
