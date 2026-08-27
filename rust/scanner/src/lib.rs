mod scan;

pub use scan::run;

pub mod bench_support {
    pub use super::scan::{bench_folder, folder_content_hash};
}
