use std::collections::BTreeMap;
use std::hint::black_box;

use criterion::{Criterion, criterion_group, criterion_main};
use navidrome_search::bench_support::{BenchEngine, bench_documents};

fn search_hotpaths(c: &mut Criterion) {
    let documents = bench_documents(2_000);
    c.bench_function("search_replace_2000_docs", |b| {
        b.iter(|| {
            let mut engine = BenchEngine::new().expect("engine");
            engine.replace(black_box(documents.clone())).expect("replace");
        });
    });

    let mut engine = BenchEngine::new().expect("engine");
    engine.replace(documents).expect("replace");
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
