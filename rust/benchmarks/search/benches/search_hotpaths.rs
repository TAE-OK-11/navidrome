use std::hint::black_box;

use criterion::{Criterion, criterion_group, criterion_main};
use navidrome_search::bench_support::BenchEngine;

fn search_hotpaths(c: &mut Criterion) {
    c.bench_function("search_replace_2000_docs", |b| {
        b.iter(|| {
            let mut engine = BenchEngine::new().expect("engine");
            engine.load_documents(black_box(2_000)).expect("replace");
        });
    });

    let mut engine = BenchEngine::new().expect("engine");
    engine.load_documents(2_000).expect("replace");
    c.bench_function("search_query_latin_3_groups", |b| {
        b.iter(|| {
            engine
                .search_all(black_box("blue monday"), black_box(&[1]))
                .expect("search");
        });
    });
}

criterion_group!(benches, search_hotpaths);
criterion_main!(benches);
