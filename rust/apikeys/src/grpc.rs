use tonic::{Request, Response, Status};

use crate::proto::api_keys_server::ApiKeys;
use crate::proto::{
    GenerateRequest, GenerateResponse, HashRequest, HashResponse, HealthRequest, HealthResponse,
    VerifyRequest, VerifyResponse,
};
use crate::{generate, hash_token, verify_token};

#[derive(Default, Clone)]
pub struct ApiKeysService;

#[tonic::async_trait]
impl ApiKeys for ApiKeysService {
    async fn generate(
        &self,
        request: Request<GenerateRequest>,
    ) -> Result<Response<GenerateResponse>, Status> {
        let pepper = request.into_inner().pepper;
        match generate(&pepper) {
            Ok((token, lookup_prefix, hash)) => Ok(Response::new(GenerateResponse {
                ok: true,
                token,
                lookup_prefix,
                hash,
                error: String::new(),
            })),
            Err(error) => Ok(Response::new(GenerateResponse {
                ok: false,
                error: error.to_string(),
                ..Default::default()
            })),
        }
    }

    async fn hash(
        &self,
        request: Request<HashRequest>,
    ) -> Result<Response<HashResponse>, Status> {
        let req = request.into_inner();
        match hash_token(&req.token, &req.pepper) {
            Ok(hash) => Ok(Response::new(HashResponse {
                ok: true,
                lookup_prefix: crate::lookup_prefix(&req.token),
                hash,
                error: String::new(),
            })),
            Err(error) => Ok(Response::new(HashResponse {
                ok: false,
                error: error.to_string(),
                ..Default::default()
            })),
        }
    }

    async fn verify(
        &self,
        request: Request<VerifyRequest>,
    ) -> Result<Response<VerifyResponse>, Status> {
        let req = request.into_inner();
        match verify_token(&req.token, &req.hash, &req.pepper) {
            Ok(valid) => Ok(Response::new(VerifyResponse {
                ok: true,
                valid,
                error: String::new(),
            })),
            Err(error) => Ok(Response::new(VerifyResponse {
                ok: false,
                error: error.to_string(),
                ..Default::default()
            })),
        }
    }

    async fn health(
        &self,
        _request: Request<HealthRequest>,
    ) -> Result<Response<HealthResponse>, Status> {
        Ok(Response::new(HealthResponse { ok: true }))
    }
}
