use std::collections::HashMap;
use std::hint::black_box;

use criterion::{Criterion, criterion_group, criterion_main};
use navidrome_integration::sign_audioscrobbler;

#[derive(serde::Deserialize)]
struct SignVector {
    secret: String,
    params: HashMap<String, String>,
}

fn integration_hotpaths(c: &mut Criterion) {
    let raw = include_str!("../../../integration/testdata/sign_vector.json");
    let vector: SignVector = serde_json::from_str(raw).expect("parse sign vector");

    c.bench_function("sign_audioscrobbler", |b| {
        b.iter(|| {
            black_box(sign_audioscrobbler(
                black_box(&vector.params),
                black_box(&vector.secret),
            ))
        });
    });
}

criterion_group!(benches, integration_hotpaths);
criterion_main!(benches);
