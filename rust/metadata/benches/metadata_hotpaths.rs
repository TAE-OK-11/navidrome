use std::collections::HashMap;
use std::hint::black_box;
use std::path::Path;

use criterion::{Criterion, criterion_group, criterion_main};
use navidrome_metadata::bench_support::{bench_tags, map_media_json};

fn metadata_hotpaths(c: &mut Criterion) {
    let tags = bench_tags();
    c.bench_function("map_media_json", |b| {
        b.iter(|| {
            black_box(map_media_json(black_box(&tags), black_box(Path::new("music/song.mp3"))))
        });
    });
}

criterion_group!(benches, metadata_hotpaths);
criterion_main!(benches);
