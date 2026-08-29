pub mod folder_hash_worker;
mod scan;

pub use scan::run;

pub mod bench_support {
    pub use super::scan::{bench_folder, folder_content_hash, folder_hash_from_input, FolderHashInput};
}
