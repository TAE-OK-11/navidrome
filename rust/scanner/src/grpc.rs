use std::collections::BTreeMap;

use tonic::{Request, Response, Status};

use crate::proto::folder_hash_server::FolderHash;
use crate::proto::{
    HashRequest, HashResponse, HealthRequest, HealthResponse,
};
use crate::scan::{folder_hash_from_input, FileEntry, FolderHashInput};

#[derive(Default, Clone)]
pub struct FolderHashService;

#[tonic::async_trait]
impl FolderHash for FolderHashService {
    async fn hash(
        &self,
        request: Request<HashRequest>,
    ) -> Result<Response<HashResponse>, Status> {
        let req = request.into_inner();
        let input = FolderHashInput {
            path: req.path,
            mod_time_ns: req.mod_time_ns,
            images_updated_at_ns: req.images_updated_at_ns,
            num_playlists: req.num_playlists.max(0) as usize,
            num_subfolders: req.num_subfolders.max(0) as usize,
            audio_files: to_files(req.audio_files),
            image_files: to_files(req.image_files),
        };
        let hash = tokio::task::spawn_blocking(move || folder_hash_from_input(&input))
            .await
            .map_err(|err| Status::internal(format!("folder hash join: {err}")))?;
        Ok(Response::new(HashResponse {
            ok: true,
            hash,
            error: String::new(),
        }))
    }

    async fn health(
        &self,
        _request: Request<HealthRequest>,
    ) -> Result<Response<HealthResponse>, Status> {
        Ok(Response::new(HealthResponse { ok: true }))
    }
}

fn to_files(files: std::collections::HashMap<String, crate::proto::FileMeta>) -> BTreeMap<String, FileEntry> {
    files
        .into_iter()
        .map(|(key, meta)| {
            let name = if meta.name.is_empty() {
                key.clone()
            } else {
                meta.name
            };
            (key, FileEntry::new(name, meta.size, meta.mod_time_ns))
        })
        .collect()
}
