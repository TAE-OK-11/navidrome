use std::sync::{Arc, RwLock};

use tonic::{Request, Response, Status};

use crate::proto::search_server::Search;
use crate::proto::{
    HealthRequest, HealthResponse, Hit as ProtoHit, IndexRequest, IndexResponse,
    SearchGroup as ProtoGroup, index_request,
};
use crate::{Engine, Request as SearchOp, SearchDocument, SearchSpec, handle_request_locked};

pub struct SearchService {
    engine: Arc<RwLock<Option<Engine>>>,
}

impl SearchService {
    pub fn new() -> Self {
        Self {
            engine: Arc::new(RwLock::new(None)),
        }
    }
}

#[tonic::async_trait]
impl Search for SearchService {
    async fn apply(
        &self,
        request: Request<IndexRequest>,
    ) -> Result<Response<IndexResponse>, Status> {
        let engine = Arc::clone(&self.engine);
        let req = request.into_inner();
        let result = tokio::task::spawn_blocking(move || apply_sync(&engine, req))
            .await
            .map_err(|err| Status::internal(format!("search worker join: {err}")))?;
        Ok(Response::new(result))
    }

    async fn health(
        &self,
        _request: Request<HealthRequest>,
    ) -> Result<Response<HealthResponse>, Status> {
        Ok(Response::new(HealthResponse { ok: true }))
    }
}

fn apply_sync(engine: &RwLock<Option<Engine>>, request: IndexRequest) -> IndexResponse {
    let Some(op) = request.op else {
        return failed("op is required");
    };
    let search_op = match to_search_op(op) {
        Ok(op) => op,
        Err(error) => return failed(&error),
    };
    match handle_request_locked(engine, search_op) {
        Ok(response) => IndexResponse {
            protocol: response.protocol,
            ok: response.ok,
            groups: response
                .groups
                .into_iter()
                .map(|group| ProtoGroup {
                    kind: group.kind,
                    hits: group
                        .hits
                        .into_iter()
                        .map(|hit| ProtoHit {
                            id: hit.id,
                            score: hit.score,
                        })
                        .collect(),
                })
                .collect(),
            indexed: response.indexed,
            error: response.error.unwrap_or_default(),
            normalized: response.normalized.unwrap_or_default(),
        },
        Err(error) => failed(&format!("{error:#}")),
    }
}

fn to_search_op(op: index_request::Op) -> Result<SearchOp, String> {
    match op {
        index_request::Op::BeginReplace(_) => Ok(SearchOp::BeginReplace),
        index_request::Op::Append(append) => Ok(SearchOp::Append {
            documents: to_docs(append.documents),
        }),
        index_request::Op::CommitReplace(_) => Ok(SearchOp::CommitReplace),
        index_request::Op::AbortReplace(_) => Ok(SearchOp::AbortReplace),
        index_request::Op::Upsert(upsert) => Ok(SearchOp::Upsert {
            documents: to_docs(upsert.documents),
        }),
        index_request::Op::Delete(delete) => Ok(SearchOp::Delete { keys: delete.keys }),
        index_request::Op::Commit(_) => Ok(SearchOp::Commit),
        index_request::Op::SearchAll(search) => Ok(SearchOp::SearchAll {
            query: search.query,
            library_ids: search.library_ids,
            searches: search
                .searches
                .into_iter()
                .map(|spec| SearchSpec {
                    kind: spec.kind,
                    offset: spec.offset.max(0) as usize,
                    limit: spec.limit.max(0) as usize,
                })
                .collect(),
        }),
        index_request::Op::NormalizeFts(norm) => Ok(SearchOp::NormalizeFts { values: norm.values }),
    }
}

fn to_docs(docs: Vec<crate::proto::Document>) -> Vec<SearchDocument> {
    docs.into_iter()
        .map(|doc| SearchDocument {
            key: doc.key,
            id: doc.id,
            kind: doc.kind,
            library_ids: doc.library_ids,
            primary: doc.primary,
            secondary: doc.secondary,
        })
        .collect()
}

fn failed(error: &str) -> IndexResponse {
    IndexResponse {
        protocol: crate::PROTOCOL_VERSION,
        ok: false,
        error: error.to_string(),
        ..Default::default()
    }
}
