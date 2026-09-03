use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use tokio::sync::Mutex;
use tonic::{Request, Response, Status};

use crate::breaker::CircuitBreaker;
use crate::proto::outbound_server::Outbound;
use crate::proto::{
    HealthRequest, HealthResponse, HttpRequest, HttpResponse, SignRequest, SignResponse,
};
use crate::sign::sign_audioscrobbler;

const DEFAULT_TIMEOUT: Duration = Duration::from_secs(10);
const MAX_BODY: usize = 8 * 1024 * 1024;
const USER_AGENT: &str = "NavidromeIntegration/1.0";

#[derive(Clone)]
pub struct OutboundService {
    http: reqwest::Client,
    breakers: Arc<Mutex<HashMap<String, CircuitBreaker>>>,
}

impl OutboundService {
    pub fn new() -> anyhow::Result<Self> {
        let http = reqwest::Client::builder()
            .user_agent(USER_AGENT)
            .http2_adaptive_window(true)
            .pool_idle_timeout(Duration::from_secs(30))
            .pool_max_idle_per_host(8)
            .connect_timeout(Duration::from_secs(5))
            .timeout(DEFAULT_TIMEOUT)
            .build()?;
        Ok(Self {
            http,
            breakers: Arc::new(Mutex::new(HashMap::new())),
        })
    }

    async fn breaker_allow(&self, dest: &str) -> bool {
        let mut map = self.breakers.lock().await;
        map.entry(dest.to_string()).or_default().allow()
    }

    async fn breaker_success(&self, dest: &str) {
        let mut map = self.breakers.lock().await;
        map.entry(dest.to_string()).or_default().success();
    }

    async fn breaker_failure(&self, dest: &str) {
        let mut map = self.breakers.lock().await;
        map.entry(dest.to_string()).or_default().failure();
    }
}

#[tonic::async_trait]
impl Outbound for OutboundService {
    async fn call(&self, request: Request<HttpRequest>) -> Result<Response<HttpResponse>, Status> {
        let req = request.into_inner();
        if req.url.is_empty() {
            return Err(Status::invalid_argument("url is required"));
        }
        if req.body.len() > MAX_BODY {
            return Err(Status::resource_exhausted("request body too large"));
        }
        let dest = if req.destination.is_empty() {
            "unknown"
        } else {
            req.destination.as_str()
        };
        if !self.breaker_allow(dest).await {
            return Ok(Response::new(HttpResponse {
                error: format!("circuit open for {dest}"),
                ..Default::default()
            }));
        }

        let method =
            reqwest::Method::from_bytes(req.method.as_bytes()).unwrap_or(reqwest::Method::GET);
        let timeout = if req.timeout_ms > 0 {
            Duration::from_millis(req.timeout_ms as u64)
        } else {
            DEFAULT_TIMEOUT
        };
        let mut builder = self.http.request(method, &req.url).timeout(timeout);
        for (key, value) in &req.headers {
            if key.eq_ignore_ascii_case("host") || key.eq_ignore_ascii_case("content-length") {
                continue;
            }
            builder = builder.header(key, value);
        }
        if !req.body.is_empty() {
            builder = builder.body(req.body);
        }

        match builder.send().await {
            Ok(resp) => {
                let status = resp.status().as_u16() as i32;
                let retry_after_ms = parse_retry_after(resp.headers());
                let headers = header_map(resp.headers());
                let body = resp.bytes().await.unwrap_or_default();
                if status >= 500 {
                    self.breaker_failure(dest).await;
                } else {
                    self.breaker_success(dest).await;
                }
                Ok(Response::new(HttpResponse {
                    status,
                    headers,
                    body: body.to_vec(),
                    error: String::new(),
                    retry_after_ms,
                }))
            }
            Err(err) => {
                self.breaker_failure(dest).await;
                Ok(Response::new(HttpResponse {
                    error: err.to_string(),
                    ..Default::default()
                }))
            }
        }
    }

    async fn sign(&self, request: Request<SignRequest>) -> Result<Response<SignResponse>, Status> {
        let req = request.into_inner();
        if req.secret.is_empty() {
            return Ok(Response::new(SignResponse {
                error: "secret is required".into(),
                ..Default::default()
            }));
        }
        Ok(Response::new(SignResponse {
            api_sig: sign_audioscrobbler(&req.params, &req.secret),
            error: String::new(),
        }))
    }

    async fn health(
        &self,
        _request: Request<HealthRequest>,
    ) -> Result<Response<HealthResponse>, Status> {
        Ok(Response::new(HealthResponse {
            ok: true,
            version: env!("CARGO_PKG_VERSION").into(),
        }))
    }
}

fn header_map(headers: &reqwest::header::HeaderMap) -> HashMap<String, String> {
    let mut out = HashMap::new();
    for (key, value) in headers {
        if let Ok(v) = value.to_str() {
            out.insert(key.to_string(), v.to_string());
        }
    }
    out
}

fn parse_retry_after(headers: &reqwest::header::HeaderMap) -> i32 {
    let Some(value) = headers.get("retry-after").and_then(|v| v.to_str().ok()) else {
        return 0;
    };
    value.parse::<i32>().ok().unwrap_or(0).saturating_mul(1000)
}
