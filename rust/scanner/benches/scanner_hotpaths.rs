use std::hint::black_box;

use criterion::{Criterion, criterion_group, criterion_main};
use navidrome_scanner::bench_support::{bench_folder, folder_content_hash};

fn scanner_hotpaths(c: &mut Criterion) {
    let folder = bench_folder(128);
    c.bench_function("scanner_folder_hash_128_files", |b| {
        b.iter(|| black_box(folder_content_hash(black_box(&folder))));
    });
}

criterion_group!(benches, scanner_hotpaths);
criterion_main!(benches);
