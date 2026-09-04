use std::collections::{BTreeMap, HashMap};
use std::path::PathBuf;

use anyhow::Result;
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::{Request, Response, Status};

use crate::proto::folder_hash_server::FolderHash;
use crate::proto::scanner_server::Scanner;
use crate::proto::{
    FileMeta, HashRequest, HashResponse, HealthRequest, HealthResponse, WalkEvent, WalkEventKind,
    WalkFolder, WalkRequest,
};
use crate::scan::{
    Event, EventSink, FileEntry, Folder, FolderHashInput, ScanRequest, folder_hash_from_input,
    run_scan_into, validate_request,
};

#[derive(Default, Clone)]
pub struct FolderHashService;

#[derive(Default, Clone)]
pub struct ScannerService;

#[tonic::async_trait]
impl FolderHash for FolderHashService {
    async fn hash(&self, request: Request<HashRequest>) -> Result<Response<HashResponse>, Status> {
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

#[tonic::async_trait]
impl Scanner for ScannerService {
    type WalkStream = ReceiverStream<Result<WalkEvent, Status>>;

    async fn walk(
        &self,
        request: Request<WalkRequest>,
    ) -> Result<Response<Self::WalkStream>, Status> {
        let req = request.into_inner();
        let (tx, rx) = mpsc::channel(64);
        tokio::task::spawn_blocking(move || {
            if let Err(err) = walk_sync(req, tx.clone()) {
                let _ = tx.blocking_send(Err(Status::internal(err.to_string())));
            }
        });
        Ok(Response::new(ReceiverStream::new(rx)))
    }
}

struct ChannelSink {
    tx: mpsc::Sender<Result<WalkEvent, Status>>,
}

impl EventSink for ChannelSink {
    fn emit(&mut self, event: &Event<'_>) -> Result<()> {
        self.tx
            .blocking_send(Ok(to_walk_event(event)))
            .map_err(|_| anyhow::anyhow!("scanner walk client disconnected"))
    }
}

fn walk_sync(req: WalkRequest, tx: mpsc::Sender<Result<WalkEvent, Status>>) -> Result<()> {
    let mut sink = ChannelSink { tx };
    let request = to_scan_request(req);
    if let Err(error) = validate_request(&request) {
        sink.emit(&Event::Error {
            message: &format!("{error:#}"),
        })?;
        return Ok(());
    }
    if let Err(error) = run_scan_into(request, &mut sink) {
        let _ = sink.emit(&Event::Error {
            message: &format!("{error:#}"),
        });
    }
    Ok(())
}

fn to_scan_request(req: WalkRequest) -> ScanRequest {
    ScanRequest {
        root: PathBuf::from(req.root),
        targets: req.targets,
        follow_symlinks: req.follow_symlinks,
        ignore_dot_folders: req.ignore_dot_folders,
        known_hashes: req.known_hashes,
        walk_threads: req.walk_threads.max(0) as usize,
    }
}

fn to_walk_event(event: &Event<'_>) -> WalkEvent {
    match event {
        Event::Folder { folder } => WalkEvent {
            kind: WalkEventKind::Folder.into(),
            folder: Some(folder_to_proto(folder)),
            message: String::new(),
            folders: 0,
            files: 0,
        },
        Event::FolderSummary { folder } => WalkEvent {
            kind: WalkEventKind::FolderSummary.into(),
            folder: Some(WalkFolder {
                path: folder.path.clone(),
                hash: folder.hash.clone(),
                ..WalkFolder::default()
            }),
            message: String::new(),
            folders: 0,
            files: 0,
        },
        Event::Warning { message } => WalkEvent {
            kind: WalkEventKind::Warning.into(),
            folder: None,
            message: (*message).to_owned(),
            folders: 0,
            files: 0,
        },
        Event::Error { message } => WalkEvent {
            kind: WalkEventKind::Error.into(),
            folder: None,
            message: (*message).to_owned(),
            folders: 0,
            files: 0,
        },
        Event::Done { folders, files } => WalkEvent {
            kind: WalkEventKind::Done.into(),
            folder: None,
            message: String::new(),
            folders: *folders as u64,
            files: *files as u64,
        },
    }
}

fn folder_to_proto(folder: &Folder) -> WalkFolder {
    WalkFolder {
        path: folder.path.clone(),
        mod_time_ns: folder.mod_time_ns,
        images_updated_at_ns: folder.images_updated_at_ns,
        num_playlists: folder.num_playlists as i32,
        num_subfolders: folder.num_subfolders as i32,
        audio_files: files_to_proto(&folder.audio_files),
        image_files: files_to_proto(&folder.image_files),
        hash: folder.hash.clone(),
    }
}

fn files_to_proto(files: &BTreeMap<String, FileEntry>) -> HashMap<String, FileMeta> {
    files
        .iter()
        .map(|(key, file)| {
            (
                key.clone(),
                FileMeta {
                    name: file.name.clone(),
                    size: file.size,
                    mod_time_ns: file.mod_time_ns,
                },
            )
        })
        .collect()
}

fn to_files(files: HashMap<String, FileMeta>) -> BTreeMap<String, FileEntry> {
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
