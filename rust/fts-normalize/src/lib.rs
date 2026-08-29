mod fts5_query;

use std::collections::HashSet;

use regex::Regex;
use std::sync::LazyLock;

static FTS_PUNCT_STRIP: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"[^\p{L}\p{N}]").expect("fts punct regex"));

/// Matches Go `str.NormalizeForFTS`: punctuation-stripped and accent-transliterated
/// word variants for secondary search fields.
pub fn normalize_for_fts(values: &[String]) -> String {
    let mut seen = HashSet::new();
    let mut result = Vec::new();
    let mut add = |orig: &str, variant: &str| {
        if variant.is_empty() || variant == orig {
            return;
        }
        let lower = variant.to_lowercase();
        if seen.insert(lower) {
            result.push(variant.to_owned());
        }
    };

    for value in values {
        for word in value.split_whitespace() {
            let transliterated = deunicode::deunicode(word);
            add(
                word,
                &FTS_PUNCT_STRIP
                    .replace_all(&transliterated, "")
                    .into_owned(),
            );
            add(word, &transliterated);
        }
    }
    result.join(" ")
}

pub use fts5_query::{build_fts5_query, fts_query_degraded, Fts5Query};

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn normalizes_punctuation_and_accents() {
        let out = normalize_for_fts(&["R.E.M.".to_owned(), "Bjørk".to_owned()]);
        assert!(out.contains("REM"));
        assert!(out.contains("Bjork"));
    }
}
