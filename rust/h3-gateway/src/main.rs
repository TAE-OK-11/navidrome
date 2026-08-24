use std::collections::HashMap;
use std::convert::Infallible;
use std::error::Error as StdError;
use std::io::{BufRead, BufReader, Write};
use std::net::{IpAddr, SocketAddr};
use std::os::fd::{FromRawFd, RawFd};
use std::os::unix::net::UnixStream as StdUnixStream;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use anyhow::{Context, Result, anyhow, bail};
use bytes::Bytes;
use futures::{SinkExt, StreamExt, stream};
use http::header::{CONNECTION, HOST, TE, TRAILER, TRANSFER_ENCODING, UPGRADE};
use http::{HeaderMap, HeaderName, HeaderValue, Method, Request, StatusCode, Uri, Version};
use http_body_util::combinators::UnsyncBoxBody;
use http_body_util::{BodyExt, Empty, StreamBody};
use hyper::body::Frame;
use hyper::client::conn::http2::{self, SendRequest};
use hyper_util::rt::{TokioExecutor, TokioIo};
use log::{info, warn};
use serde::Deserialize;
use socket2::{Domain, Protocol, Socket, Type};
use tokio::net::UdpSocket;
use tokio::sync::{Semaphore, oneshot};
use tokio_quiche::http3::driver::{
    H3Event, InboundFrame, IncomingH3Headers, OutboundFrame, OutboundFrameSender,
    ServerH3Controller, ServerH3Event,
};
use tokio_quiche::http3::settings::Http3Settings;
use tokio_quiche::metrics::DefaultMetrics;
use tokio_quiche::quiche::h3::{self, NameValue};
use tokio_quiche::settings::{CertificateKind, Hooks, QuicSettings, TlsCertificatePaths};
use tokio_quiche::socket::QuicListener;
use tokio_quiche::{ConnectionParams, ServerH3Driver, listen_with_capabilities};

const TOKEN_HEADER: &str = "x-navidrome-h3-token";
const AUTHORITY_HEADER: &str = "x-navidrome-h3-authority";
const REMOTE_ADDR_HEADER: &str = "x-navidrome-h3-remote-addr";
const MAX_UDP_PAYLOAD: usize = 1452;
const CONTROL_STREAM_LIMIT: u64 = 8;
const SEND_CAPACITY_FACTOR: f64 = 2.0;
const MAX_AMPLIFICATION_FACTOR: usize = 3;
const SOCKET_BUFFER_SIZE: usize = 7 * 1024 * 1024;
const MAX_ADMISSION_PEERS: usize = 2_048;
const ADMISSION_IDLE: Duration = Duration::from_secs(600);
const BRIDGE_READY_TIMEOUT: Duration = Duration::from_secs(5);
const BRIDGE_RESPONSE_HEADER_TIMEOUT: Duration = Duration::from_secs(30);

static CONNECTION_REJECTIONS: AtomicU64 = AtomicU64::new(0);
static REQUEST_REJECTIONS: AtomicU64 = AtomicU64::new(0);

const CONTROL_FD_ENV: &str = "NAVIDROME_H3_CONTROL_FD";
const BRIDGE_FD_ENV: &str = "NAVIDROME_H3_BRIDGE_FD";

type BoxError = Box<dyn StdError + Send + Sync>;
type ProxyBody = UnsyncBoxBody<Bytes, BoxError>;
type ProxyClient = SendRequest<ProxyBody>;

#[derive(Debug, Deserialize)]
struct Config {
    udp_address: String,
    certificate: String,
    private_key: String,
    internal_token: String,
    alt_svc_max_age_seconds: i64,
    qlog_dir: Option<String>,
    handshake_timeout_seconds: u64,
    idle_timeout_seconds: u64,
    max_concurrent_streams: u64,
    max_connections: usize,
    max_connections_per_ip: usize,
    max_in_flight_requests: usize,
    connection_rate_per_second: f64,
    connection_burst: u32,
    congestion_control: String,
}

fn normalize_congestion_control(value: &str) -> Result<String> {
    let value = value.trim().to_ascii_lowercase();
    match value.as_str() {
        "bbr2" | "cubic" | "reno" => Ok(value),
        _ => bail!("unsupported congestion control {value:?}"),
    }
}

fn inherited_fd(name: &str) -> Result<RawFd> {
    std::env::var(name)
        .with_context(|| format!("{name} is required"))?
        .parse()
        .with_context(|| format!("{name} is invalid"))
}

fn main() -> Result<()> {
    env_logger::Builder::from_env(env_logger::Env::default().default_filter_or("info")).init();
    let control_fd = inherited_fd(CONTROL_FD_ENV)?;
    let bridge_fd = inherited_fd(BRIDGE_FD_ENV)?;

    // SAFETY: both descriptors are passed exclusively to this process by the
    // Go supervisor and ownership is transferred exactly once here.
    let control = unsafe { StdUnixStream::from_raw_fd(control_fd) };
    let bridge = unsafe { StdUnixStream::from_raw_fd(bridge_fd) };
    bridge
        .set_nonblocking(true)
        .context("failed to set inherited bridge nonblocking")?;

    let mut config_line = String::new();
    BufReader::new(control.try_clone()?)
        .read_line(&mut config_line)
        .context("failed to read supervisor configuration")?;
    let config: Config =
        serde_json::from_str(&config_line).context("failed to decode supervisor configuration")?;

    let worker_threads = std::env::var("NAVIDROME_H3_THREADS")
        .ok()
        .and_then(|value| value.parse().ok())
        .unwrap_or_else(|| {
            std::thread::available_parallelism()
                .map(usize::from)
                .unwrap_or(1)
                .clamp(1, 4)
        })
        .clamp(1, 4);
    tokio::runtime::Builder::new_multi_thread()
        .worker_threads(worker_threads)
        .thread_name("navidrome-h3")
        .enable_all()
        .build()
        .context("failed to build tokio runtime")?
        .block_on(run(config, control, bridge))
}

async fn run(config: Config, mut control: StdUnixStream, bridge: StdUnixStream) -> Result<()> {
    let public: SocketAddr = config.udp_address.parse().context("invalid UDP address")?;
    let congestion_control = normalize_congestion_control(&config.congestion_control)?;

    let bridge = tokio::net::UnixStream::from_std(bridge)
        .context("failed to adopt inherited HTTP/2 bridge")?;
    let mut h2 = http2::Builder::new(TokioExecutor::new());
    h2.adaptive_window(true);
    h2.max_header_list_size(64 * 1024);
    let (client, connection) = h2
        .handshake::<_, ProxyBody>(TokioIo::new(bridge))
        .await
        .context("failed to establish inherited HTTP/2 bridge")?;
    let (bridge_stopped_tx, mut bridge_stopped_rx) = oneshot::channel();
    tokio::spawn(async move {
        let result = connection.await.map_err(|error| error.to_string());
        let _ = bridge_stopped_tx.send(result);
    });

    let socket = tuned_udp_socket(public)?;
    let socket = UdpSocket::from_std(socket).context("failed to create Tokio UDP socket")?;
    let mut listener = QuicListener::try_from(socket).context("failed to prepare QUIC listener")?;
    listener.apply_max_capabilities();
    info!("UDP capabilities: {:?}", listener.capabilities);

    let mut quic = QuicSettings::default();
    quic.enable_dgram = false;
    quic.dgram_recv_max_queue_len = 0;
    quic.dgram_send_max_queue_len = 0;
    quic.max_idle_timeout = Some(Duration::from_secs(config.idle_timeout_seconds));
    quic.handshake_timeout = Some(Duration::from_secs(config.handshake_timeout_seconds));
    quic.listen_backlog = config.max_connections.min(16_384);
    quic.initial_max_streams_bidi = config.max_concurrent_streams;
    quic.initial_max_streams_uni = CONTROL_STREAM_LIMIT;
    quic.max_recv_udp_payload_size = MAX_UDP_PAYLOAD;
    quic.max_send_udp_payload_size = MAX_UDP_PAYLOAD;
    quic.discover_path_mtu = true;
    quic.pmtud_max_probes = 3;
    quic.enable_pacing = true;
    quic.enable_hystart = true;
    quic.send_capacity_factor = SEND_CAPACITY_FACTOR;
    quic.enable_early_data = false;
    quic.disable_active_migration = true;
    quic.active_connection_id_limit = 2;
    quic.max_path_challenge_recv_queue_len = 1;
    quic.grease = true;
    quic.disable_client_ip_validation = false;
    quic.max_amplification_factor = MAX_AMPLIFICATION_FACTOR;
    quic.cc_algorithm = congestion_control.clone();
    quic.qlog_dir = config
        .qlog_dir
        .as_deref()
        .filter(|path| !path.trim().is_empty())
        .map(str::to_owned);
    if let Some(qlog_dir) = &quic.qlog_dir {
        std::fs::create_dir_all(qlog_dir)
            .with_context(|| format!("failed to create qlog directory {qlog_dir}"))?;
    }

    let params = ConnectionParams::new_server(
        quic,
        TlsCertificatePaths {
            cert: &config.certificate,
            private_key: &config.private_key,
            kind: CertificateKind::X509,
        },
        Hooks {
            connection_hook: None,
        },
    );
    let mut listeners = listen_with_capabilities(vec![listener], params, DefaultMetrics)
        .context("failed to create tokio-quiche listener")?;
    let mut listener = listeners
        .pop()
        .ok_or_else(|| anyhow!("tokio-quiche returned no listener"))?;

    let token = HeaderValue::from_str(&config.internal_token).context("invalid bridge token")?;
    let public_port = public.port();
    let forwarded_port = HeaderValue::from_str(&public_port.to_string())
        .context("invalid public port forwarding value")?;
    let alt_svc = HeaderValue::from_str(&format!(
        "h3=\":{}\"; ma={}",
        public_port,
        config.alt_svc_max_age_seconds.max(0)
    ))
    .context("invalid Alt-Svc value")?;
    let connection_limit = Arc::new(Semaphore::new(config.max_connections));
    let request_limit = Arc::new(Semaphore::new(config.max_in_flight_requests));
    let admission = Arc::new(Admission::new(
        config.connection_rate_per_second,
        config.connection_burst,
        config.max_connections_per_ip,
    ));

    let (shutdown_tx, mut shutdown_rx) = oneshot::channel();
    let reader = control.try_clone()?;
    std::thread::Builder::new()
        .name("navidrome-h3-control".to_owned())
        .spawn(move || {
            let mut shutdown_tx = Some(shutdown_tx);
            for line in BufReader::new(reader).lines().map_while(Result::ok) {
                if line.trim() == "SHUTDOWN" {
                    if let Some(shutdown_tx) = shutdown_tx.take() {
                        let _ = shutdown_tx.send(());
                    }
                    return;
                }
            }
        })
        .context("failed to start control reader")?;
    control.write_all(b"READY\n")?;
    control.flush()?;
    info!(
        "HTTP/3 ready udp={} bridge=inherited-af_unix+h2 cc={} early_data=false retry=true pmtud=true pacing=true",
        public, congestion_control
    );

    let mut terminate = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
        .context("failed to listen for SIGTERM")?;
    let interrupt = tokio::signal::ctrl_c();
    tokio::pin!(interrupt);
    loop {
        tokio::select! {
            biased;
            _ = &mut shutdown_rx => break,
            signal = &mut interrupt => {
                signal.context("failed to listen for termination signal")?;
                break;
            }
            _ = terminate.recv() => break,
            bridge_result = &mut bridge_stopped_rx => {
                match bridge_result {
                    Ok(Ok(())) => bail!("inherited HTTP/2 bridge closed"),
                    Ok(Err(error)) => bail!("inherited HTTP/2 bridge stopped: {error}"),
                    Err(_) => bail!("inherited HTTP/2 bridge monitor stopped"),
                }
            },
            connection = listener.next() => {
                let Some(connection) = connection else { bail!("QUIC listener stopped") };
                match connection {
                    Ok(connection) => {
                        let permit = match connection_limit.clone().try_acquire_owned() {
                            Ok(permit) => permit,
                            Err(_) => {
                                let total = CONNECTION_REJECTIONS.fetch_add(1, Ordering::Relaxed) + 1;
                                if should_sample_log(total) {
                                    warn!("connection rejected: global admission limit reached total={total}");
                                }
                                continue;
                            }
                        };
                        let peer = connection.peer_addr();
                        let peer_permit = match admission.admit(peer.ip()) {
                            Ok(permit) => permit,
                            Err(rejection) => {
                                let total = CONNECTION_REJECTIONS.fetch_add(1, Ordering::Relaxed) + 1;
                                if should_sample_log(total) {
                                    warn!("connection rejected peer={peer}: {rejection:?} total={total}");
                                }
                                continue;
                            }
                        };
                        let bridge_headers = match BridgeHeaders::new(peer, &token, &forwarded_port) {
                            Ok(headers) => headers,
                            Err(error) => {
                                warn!("failed to prepare HTTP/3 forwarding headers peer={peer}: {error:#}");
                                continue;
                            }
                        };
                        let settings = Http3Settings {
                            max_header_list_size: Some(64 * 1024),
                            ..Http3Settings::default()
                        };
                        let (driver, controller) = ServerH3Driver::new(settings);
                        connection.start(driver);
                        tokio::spawn(handle_connection(
                            controller,
                            Arc::new(ConnectionContext {
                                peer,
                                bridge_headers,
                                client: client.clone(),
                                alt_svc: alt_svc.clone(),
                                request_limit: Arc::clone(&request_limit),
                            }),
                            permit,
                            peer_permit,
                        ));
                    }
                    Err(error) => warn!("HTTP/3 accept failed: {error}"),
                }
            }
        }
    }
    info!("HTTP/3 graceful shutdown requested");
    Ok(())
}

fn should_sample_log(total: u64) -> bool {
    total <= 8 || total.is_power_of_two()
}

fn tuned_udp_socket(address: SocketAddr) -> Result<std::net::UdpSocket> {
    let socket = Socket::new(
        Domain::for_address(address),
        Type::DGRAM,
        Some(Protocol::UDP),
    )
    .context("failed to create UDP socket")?;
    socket.set_reuse_address(true)?;
    socket.set_recv_buffer_size(SOCKET_BUFFER_SIZE)?;
    socket.set_send_buffer_size(SOCKET_BUFFER_SIZE)?;
    socket.set_nonblocking(true)?;
    socket
        .bind(&address.into())
        .with_context(|| format!("failed to bind HTTP/3 UDP listener {address}"))?;
    info!(
        "UDP buffers: requested={} recv={} send={}",
        SOCKET_BUFFER_SIZE,
        socket.recv_buffer_size().unwrap_or_default(),
        socket.send_buffer_size().unwrap_or_default()
    );
    Ok(socket.into())
}

#[derive(Debug, Clone, Copy, Eq, PartialEq)]
enum AdmissionRejection {
    RateLimited,
    TooManyConnections,
    TableFull,
}

struct PeerState {
    tokens: f64,
    updated: Instant,
    active: usize,
}

struct Admission {
    peers: Mutex<HashMap<IpAddr, PeerState>>,
    rate_per_second: f64,
    burst: u32,
    max_active: usize,
}

impl Admission {
    fn new(rate_per_second: f64, burst: u32, max_active: usize) -> Self {
        Self {
            peers: Mutex::new(HashMap::new()),
            rate_per_second,
            burst,
            max_active,
        }
    }

    fn admit(self: &Arc<Self>, ip: IpAddr) -> Result<AdmissionPermit, AdmissionRejection> {
        let now = Instant::now();
        let mut peers = self
            .peers
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        if peers.len() >= MAX_ADMISSION_PEERS && !peers.contains_key(&ip) {
            peers.retain(|_, state| {
                state.active > 0 || now.saturating_duration_since(state.updated) < ADMISSION_IDLE
            });
            if peers.len() >= MAX_ADMISSION_PEERS {
                return Err(AdmissionRejection::TableFull);
            }
        }
        let capacity = f64::from(self.burst) + 1.0;
        let state = peers.entry(ip).or_insert(PeerState {
            tokens: capacity,
            updated: now,
            active: 0,
        });
        let elapsed = now.saturating_duration_since(state.updated).as_secs_f64();
        state.tokens = (state.tokens + elapsed * self.rate_per_second).min(capacity);
        state.updated = now;
        if state.tokens < 1.0 {
            return Err(AdmissionRejection::RateLimited);
        }
        if state.active >= self.max_active {
            return Err(AdmissionRejection::TooManyConnections);
        }
        state.tokens -= 1.0;
        state.active += 1;
        Ok(AdmissionPermit {
            admission: Arc::clone(self),
            ip,
        })
    }
}

struct AdmissionPermit {
    admission: Arc<Admission>,
    ip: IpAddr,
}

impl Drop for AdmissionPermit {
    fn drop(&mut self) {
        let mut peers = self
            .admission
            .peers
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        if let Some(state) = peers.get_mut(&self.ip) {
            state.active = state.active.saturating_sub(1);
            state.updated = Instant::now();
        }
    }
}

struct BridgeHeaders {
    token: HeaderValue,
    remote_addr: HeaderValue,
    forwarded_for: HeaderValue,
    forwarded_port: HeaderValue,
}

impl BridgeHeaders {
    fn new(peer: SocketAddr, token: &HeaderValue, forwarded_port: &HeaderValue) -> Result<Self> {
        Ok(Self {
            token: token.clone(),
            remote_addr: HeaderValue::from_str(&peer.to_string())?,
            forwarded_for: HeaderValue::from_str(&peer.ip().to_string())?,
            forwarded_port: forwarded_port.clone(),
        })
    }
}

struct ConnectionContext {
    peer: SocketAddr,
    bridge_headers: BridgeHeaders,
    client: ProxyClient,
    alt_svc: HeaderValue,
    request_limit: Arc<Semaphore>,
}

async fn handle_connection(
    mut controller: ServerH3Controller,
    context: Arc<ConnectionContext>,
    _permit: tokio::sync::OwnedSemaphorePermit,
    _peer_permit: AdmissionPermit,
) {
    while let Some(event) = controller.event_receiver_mut().recv().await {
        match event {
            ServerH3Event::Headers {
                incoming_headers,
                is_in_early_data,
                ..
            } => {
                if *is_in_early_data {
                    // This is unreachable while quiche early-data is disabled.
                    // Never turn it into an application-visible HTTP 425.
                    warn!(
                        "discarding unexpected early-data stream peer={}",
                        context.peer
                    );
                    continue;
                }
                let request_permit = match context.request_limit.clone().try_acquire_owned() {
                    Ok(permit) => permit,
                    Err(_) => {
                        let total = REQUEST_REJECTIONS.fetch_add(1, Ordering::Relaxed) + 1;
                        if should_sample_log(total) {
                            warn!(
                                "HTTP/3 request rejected: bridge saturated peer={} total={total}",
                                context.peer
                            );
                        }
                        reject_overloaded(incoming_headers).await;
                        continue;
                    }
                };
                tokio::spawn(proxy_request(
                    incoming_headers,
                    Arc::clone(&context),
                    request_permit,
                ));
            }
            ServerH3Event::Core(H3Event::BodyBytesReceived { .. }) => {}
            ServerH3Event::Core(event) => log::debug!("HTTP/3 event: {event:?}"),
        }
    }
}

async fn reject_overloaded(incoming: IncomingH3Headers) {
    let mut send = incoming.send;
    if let Err(error) = send_overloaded(&mut send).await {
        log::debug!("failed to send HTTP/3 overload response: {error:#}");
    }
}

async fn proxy_request(
    incoming: IncomingH3Headers,
    context: Arc<ConnectionContext>,
    _request_permit: tokio::sync::OwnedSemaphorePermit,
) {
    let IncomingH3Headers {
        headers,
        send,
        recv,
        read_fin,
        ..
    } = incoming;
    if let Err(error) = proxy_request_inner(headers, send, recv, read_fin, &context).await {
        warn!(
            "HTTP/3 stream proxy failed peer={}: {error:#}",
            context.peer
        );
    }
}

async fn proxy_request_inner(
    headers: Vec<h3::Header>,
    mut send: OutboundFrameSender,
    recv: tokio_quiche::http3::driver::InboundFrameStream,
    read_fin: bool,
    context: &ConnectionContext,
) -> Result<()> {
    if is_connect_request(&headers) {
        send_error(
            &mut send,
            StatusCode::NOT_IMPLEMENTED,
            "HTTP/3 CONNECT is not supported; retry over HTTP/1.1",
        )
        .await?;
        return Ok(());
    }
    let decoded = match decode_request_headers(&headers, &context.bridge_headers) {
        Ok(decoded) => decoded,
        Err(error) => {
            send_error(&mut send, StatusCode::BAD_REQUEST, "invalid HTTP/3 request").await?;
            return Err(error);
        }
    };
    if decoded.method == Method::CONNECT {
        send_error(
            &mut send,
            StatusCode::NOT_IMPLEMENTED,
            "HTTP/3 CONNECT is not supported; retry over HTTP/1.1",
        )
        .await?;
        return Ok(());
    }

    let body = request_body(recv, read_fin);
    let mut request = Request::builder()
        .method(decoded.method)
        .uri(decoded.uri)
        .version(Version::HTTP_2)
        .body(body)
        .context("failed to build internal request")?;
    *request.headers_mut() = decoded.headers;

    let mut client = context.client.clone();
    tokio::time::timeout(BRIDGE_READY_TIMEOUT, client.ready())
        .await
        .context("timed out waiting for inherited HTTP/2 bridge capacity")?
        .context("inherited HTTP/2 bridge is not ready")?;
    let response =
        tokio::time::timeout(BRIDGE_RESPONSE_HEADER_TIMEOUT, client.send_request(request))
            .await
            .context("timed out waiting for inherited HTTP/2 response headers")?
            .context("inherited HTTP/2 bridge request failed")?;
    forward_response(response, &mut send, &context.alt_svc).await
}

fn is_connect_request(headers: &[h3::Header]) -> bool {
    headers.iter().any(|header| {
        header.name() == b":method" && header.value().eq_ignore_ascii_case(b"CONNECT")
    })
}

struct DecodedRequest {
    method: Method,
    uri: Uri,
    headers: HeaderMap,
}

fn decode_request_headers(
    headers: &[h3::Header],
    bridge_headers: &BridgeHeaders,
) -> Result<DecodedRequest> {
    let mut method = None;
    let mut scheme = None;
    let mut authority = None;
    let mut path = None;
    let mut regular_seen = false;
    // Four required pseudo-headers are replaced by seven private-hop headers.
    let mut output = HeaderMap::with_capacity(headers.len() + 3);

    for header in headers {
        let name = header.name();
        let value = header.value();
        if name.starts_with(b":") {
            if regular_seen {
                bail!("pseudo-header after regular header")
            }
            match name {
                b":method" if method.is_none() => method = Some(Method::from_bytes(value)?),
                b":scheme" if scheme.is_none() => scheme = Some(value),
                b":authority" if authority.is_none() => authority = Some(value),
                b":path" if path.is_none() => path = Some(value),
                _ => bail!("duplicate or unsupported pseudo-header"),
            }
            continue;
        }
        regular_seen = true;
        if name.iter().any(u8::is_ascii_uppercase) {
            bail!("uppercase field name")
        }
        let name = HeaderName::from_bytes(name)?;
        if forbidden_request_header(&name, value) {
            bail!("connection-specific field")
        }
        if name == HOST
            || name == TOKEN_HEADER
            || name == AUTHORITY_HEADER
            || name == REMOTE_ADDR_HEADER
            || name.as_str().starts_with("x-forwarded-")
            || matches!(
                name.as_str(),
                "forwarded" | "x-real-ip" | "true-client-ip" | "cf-connecting-ip"
            )
        {
            continue;
        }
        output.append(name, HeaderValue::from_bytes(value)?);
    }

    let method = method.ok_or_else(|| anyhow!("missing :method"))?;
    if !scheme
        .ok_or_else(|| anyhow!("missing :scheme"))?
        .eq_ignore_ascii_case(b"https")
    {
        bail!(":scheme must be https")
    }
    let authority = authority.ok_or_else(|| anyhow!("missing :authority"))?;
    let authority_text = std::str::from_utf8(authority)?;
    let authority_header = HeaderValue::from_bytes(authority)?;
    let path = std::str::from_utf8(path.ok_or_else(|| anyhow!("missing :path"))?)?;
    if !path.starts_with('/') {
        bail!(":path must be origin-form")
    }
    // Hyper's low-level HTTP/2 client derives :scheme and :authority from the
    // request URI. An origin-form URI only contains :path and is rejected by
    // SendRequest before it reaches the inherited bridge.
    let uri = Uri::builder()
        .scheme("https")
        .authority(authority_text)
        .path_and_query(path)
        .build()?;
    output.insert(HOST, authority_header.clone());
    output.insert(AUTHORITY_HEADER, authority_header);
    output.insert(TOKEN_HEADER, bridge_headers.token.clone());
    output.insert(REMOTE_ADDR_HEADER, bridge_headers.remote_addr.clone());
    output.insert("x-forwarded-for", bridge_headers.forwarded_for.clone());
    output.insert("x-forwarded-proto", HeaderValue::from_static("https"));
    output.insert("x-forwarded-port", bridge_headers.forwarded_port.clone());
    Ok(DecodedRequest {
        method,
        uri,
        headers: output,
    })
}

fn forbidden_request_header(name: &HeaderName, value: &[u8]) -> bool {
    name == CONNECTION
        || name.as_str() == "keep-alive"
        || name.as_str() == "proxy-connection"
        || name == TRANSFER_ENCODING
        || name == UPGRADE
        || (name == TE && !value.eq_ignore_ascii_case(b"trailers"))
}

fn request_body(
    recv: tokio_quiche::http3::driver::InboundFrameStream,
    read_fin: bool,
) -> ProxyBody {
    if read_fin {
        return Empty::<Bytes>::new()
            .map_err(infallible_to_box_error)
            .boxed_unsync();
    }
    let body = stream::unfold((recv, false), |(mut recv, finished)| async move {
        if finished {
            return None;
        }
        loop {
            match recv.recv().await {
                Some(InboundFrame::Body(data, fin)) => {
                    return Some((Ok::<_, BoxError>(Frame::data(data.freeze())), (recv, fin)));
                }
                Some(InboundFrame::Datagram(_)) => continue,
                None => return None,
            }
        }
    });
    StreamBody::new(body).boxed_unsync()
}

fn infallible_to_box_error(error: Infallible) -> BoxError {
    match error {}
}

async fn forward_response(
    response: hyper::Response<hyper::body::Incoming>,
    send: &mut OutboundFrameSender,
    alt_svc: &HeaderValue,
) -> Result<()> {
    let (parts, mut body) = response.into_parts();
    let mut headers = Vec::with_capacity(parts.headers.len() + 2);
    headers.push(h3::Header::new(
        b":status",
        parts.status.as_str().as_bytes(),
    ));
    let mut has_alt_svc = false;
    for (name, value) in &parts.headers {
        if forbidden_response_header(name) {
            continue;
        }
        has_alt_svc |= name.as_str() == "alt-svc";
        headers.push(h3::Header::new(name.as_str().as_bytes(), value.as_bytes()));
    }
    if !has_alt_svc {
        headers.push(h3::Header::new(b"alt-svc", alt_svc.as_bytes()))
    }
    send.send(OutboundFrame::Headers(headers, None)).await?;
    while let Some(frame) = body.frame().await {
        if let Ok(data) = frame?.into_data()
            && !data.is_empty()
        {
            send.send(OutboundFrame::Body(data, false)).await?;
        }
    }
    send.send(OutboundFrame::Body(Bytes::new(), true)).await?;
    Ok(())
}

fn forbidden_response_header(name: &HeaderName) -> bool {
    name == CONNECTION
        || name.as_str() == "keep-alive"
        || name.as_str() == "proxy-connection"
        || name == TRANSFER_ENCODING
        || name == UPGRADE
        || name == TRAILER
}

async fn send_error(
    send: &mut OutboundFrameSender,
    status: StatusCode,
    message: &'static str,
) -> Result<()> {
    let length = message.len().to_string();
    send.send(OutboundFrame::Headers(
        vec![
            h3::Header::new(b":status", status.as_str().as_bytes()),
            h3::Header::new(b"content-type", b"text/plain; charset=utf-8"),
            h3::Header::new(b"content-length", length.as_bytes()),
        ],
        None,
    ))
    .await?;
    send.send(OutboundFrame::Body(
        Bytes::from_static(message.as_bytes()),
        true,
    ))
    .await?;
    Ok(())
}

async fn send_overloaded(send: &mut OutboundFrameSender) -> Result<()> {
    const MESSAGE: &str = "HTTP/3 bridge is busy; retry shortly";
    let length = MESSAGE.len().to_string();
    send.send(OutboundFrame::Headers(
        vec![
            h3::Header::new(
                b":status",
                StatusCode::SERVICE_UNAVAILABLE.as_str().as_bytes(),
            ),
            h3::Header::new(b"content-type", b"text/plain; charset=utf-8"),
            h3::Header::new(b"content-length", length.as_bytes()),
            h3::Header::new(b"retry-after", b"1"),
        ],
        None,
    ))
    .await?;
    send.send(OutboundFrame::Body(
        Bytes::from_static(MESSAGE.as_bytes()),
        true,
    ))
    .await?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn peer() -> SocketAddr {
        "192.0.2.10:12345".parse().unwrap()
    }

    fn bridge_headers() -> BridgeHeaders {
        BridgeHeaders::new(
            peer(),
            &HeaderValue::from_static("test-token"),
            &HeaderValue::from_static("443"),
        )
        .unwrap()
    }

    #[test]
    fn congestion_control_is_normalized_and_validated() {
        assert_eq!(normalize_congestion_control(" BBR2 ").unwrap(), "bbr2");
        assert_eq!(normalize_congestion_control("CUBIC").unwrap(), "cubic");
        assert_eq!(normalize_congestion_control("reno").unwrap(), "reno");
        assert!(normalize_congestion_control("bbr3").is_err());
    }

    #[test]
    fn rejection_logs_are_sampled_after_the_initial_burst() {
        for total in 1..=8 {
            assert!(should_sample_log(total));
        }
        assert!(!should_sample_log(9));
        assert!(should_sample_log(16));
        assert!(!should_sample_log(17));
        assert!(should_sample_log(1024));
    }

    #[test]
    fn request_headers_are_sanitized_and_rebuilt() {
        let headers = vec![
            h3::Header::new(b":method", b"GET"),
            h3::Header::new(b":scheme", b"https"),
            h3::Header::new(b":authority", b"music.example"),
            h3::Header::new(b":path", b"/rest/ping?x=1"),
            h3::Header::new(b"x-forwarded-for", b"attacker"),
            h3::Header::new(b"x-real-ip", b"198.51.100.99"),
        ];
        let decoded = decode_request_headers(&headers, &bridge_headers()).unwrap();
        assert_eq!(decoded.method, Method::GET);
        assert_eq!(decoded.uri.scheme_str(), Some("https"));
        assert_eq!(
            decoded.uri.authority().map(|authority| authority.as_str()),
            Some("music.example")
        );
        assert_eq!(
            decoded.uri.path_and_query().unwrap().as_str(),
            "/rest/ping?x=1"
        );
        assert_eq!(decoded.headers["x-forwarded-for"], "192.0.2.10");
        assert_eq!(decoded.headers[TOKEN_HEADER], "test-token");
        assert_eq!(decoded.headers[REMOTE_ADDR_HEADER], "192.0.2.10:12345");
        assert!(!decoded.headers.contains_key("x-real-ip"));
    }

    #[test]
    fn connection_specific_headers_are_rejected() {
        let headers = vec![
            h3::Header::new(b":method", b"GET"),
            h3::Header::new(b":scheme", b"https"),
            h3::Header::new(b":authority", b"music.example"),
            h3::Header::new(b":path", b"/"),
            h3::Header::new(b"connection", b"close"),
        ];
        assert!(decode_request_headers(&headers, &bridge_headers()).is_err());
    }

    #[test]
    fn extended_connect_is_detected_before_pseudo_header_decoding() {
        let headers = vec![
            h3::Header::new(b":method", b"CONNECT"),
            h3::Header::new(b":protocol", b"websocket"),
            h3::Header::new(b":authority", b"music.example"),
            h3::Header::new(b":path", b"/ws"),
        ];
        assert!(is_connect_request(&headers));
    }

    #[test]
    fn admission_limits_rate_and_active_connections() {
        let ip = "192.0.2.20".parse().unwrap();
        let active = Arc::new(Admission::new(1000.0, 10, 1));
        let permit = active.admit(ip).unwrap();
        assert!(matches!(
            active.admit(ip),
            Err(AdmissionRejection::TooManyConnections)
        ));
        drop(permit);

        let rate = Arc::new(Admission::new(0.01, 0, 10));
        let _permit = rate.admit(ip).unwrap();
        assert!(matches!(
            rate.admit(ip),
            Err(AdmissionRejection::RateLimited)
        ));
    }
}
