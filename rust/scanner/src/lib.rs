pub mod proto {
    tonic::include_proto!("navidrome.scanner.v1");
}

pub mod folder_hash_worker;
pub mod grpc;
pub mod scan;

pub use scan::run;

pub mod bench_support {
    pub use super::scan::{bench_folder, folder_content_hash, folder_hash_from_input, FolderHashInput};
}
