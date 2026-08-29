pub mod compute_pid;
pub mod id_hash;
pub mod map_media;
pub mod tag_clean;

pub mod bench_support {
    use std::collections::HashMap;
    use std::path::Path;

    pub use super::map_media::map_to_json;

    pub fn bench_tags() -> HashMap<String, Vec<String>> {
        let mut tags = HashMap::new();
        tags.insert("title".to_owned(), vec!["Blue Monday".to_owned()]);
        tags.insert("album".to_owned(), vec!["Power, Corruption & Lies".to_owned()]);
        tags.insert("artist".to_owned(), vec!["New Order".to_owned()]);
        tags.insert("albumartist".to_owned(), vec!["New Order".to_owned()]);
        tags.insert("tracknumber".to_owned(), vec!["1/12".to_owned()]);
        tags.insert("discnumber".to_owned(), vec!["1/1".to_owned()]);
        tags.insert("date".to_owned(), vec!["1983-03-07".to_owned()]);
        tags.insert("musicbrainz_recordingid".to_owned(), vec!["rec-1".to_owned()]);
        tags
    }

    pub fn map_media_json(tags: &HashMap<String, Vec<String>>, path: &Path) -> Option<String> {
        map_to_json(tags, path, Some("[]"))
    }
}
