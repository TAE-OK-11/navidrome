use std::sync::LazyLock;

use regex::Regex;

static FTS_PUNCT_STRIP: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"[^\p{L}\p{N}]").expect("fts punct regex"));
static FTS5_SPECIAL_CHARS: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r#"[^\p{L}\p{N}\s*"\x{00}]"#).expect("fts5 special chars regex")
});
static FTS5_OPERATORS: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"(?i)\b(AND|OR|NOT|NEAR)\b").expect("fts5 operators regex"));
static FTS5_LEADING_STAR: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"(^|[\s])\*+").expect("fts5 leading star regex"));

const NAME_PUNCTUATION: &str = "-/.''";
const PHRASE_PLACEHOLDER_PREFIX: &str = "\u{00}PHRASE";
const PHRASE_PLACEHOLDER_SUFFIX: &str = "\u{00}";

pub struct Fts5Query {
    pub query: String,
    pub degraded: bool,
}

pub fn build_fts5_query(user_input: &str) -> Fts5Query {
    let q = user_input.trim();
    if q.is_empty() || q == "\"\"" {
        return Fts5Query {
            query: String::new(),
            degraded: false,
        };
    }

    let mut phrases = Vec::new();
    let mut result = q.to_owned();
    loop {
        let Some(start) = result.find('"') else {
            break;
        };
        let rest = &result[start + 1..];
        if let Some(end_rel) = rest.find('"') {
            let end = start + 1 + end_rel;
            let phrase = result[start..=end].to_owned();
            phrases.push(phrase);
            let placeholder = format!("{PHRASE_PLACEHOLDER_PREFIX}{}\u{00}", phrases.len() - 1);
            result.replace_range(start..=end, &placeholder);
        } else {
            result.remove(start);
            break;
        }
    }

    result = accent_fold(&result);
    result = FTS5_OPERATORS
        .replace_all(&result, |caps: &regex::Captures| caps[0].to_lowercase())
        .into_owned();
    let processed = process_punctuated_words(&result, &mut phrases);
    result = FTS5_SPECIAL_CHARS.replace_all(&processed, " ").into_owned();
    result = FTS5_LEADING_STAR.replace_all(&result, "$1").into_owned();
    let tokens: Vec<String> = result.split_whitespace().map(str::to_owned).collect();

    let mut prefix_tokens = Vec::with_capacity(tokens.len());
    let mut wrapped_tokens = Vec::with_capacity(tokens.len());
    for token in &tokens {
        if token.starts_with('\u{00}') || token.ends_with('*') {
            prefix_tokens.push(token.clone());
            wrapped_tokens.push(token.clone());
            continue;
        }
        prefix_tokens.push(format!("{token}*"));
        wrapped_tokens.push(format!("({token} OR {token}*)"));
    }

    let mut prefix_query = prefix_tokens.join(" ");
    let mut result = wrapped_tokens.join(" AND ");
    for (index, phrase) in phrases.iter().enumerate() {
        let placeholder = format!("{PHRASE_PLACEHOLDER_PREFIX}{index}{PHRASE_PLACEHOLDER_SUFFIX}");
        prefix_query = prefix_query.replace(&placeholder, phrase);
        result = result.replace(&placeholder, phrase);
    }

    Fts5Query {
        query: result,
        degraded: fts_query_degraded(user_input, &prefix_query),
    }
}

fn accent_fold(value: &str) -> String {
    deunicode::deunicode(value)
}

fn process_punctuated_words(input: &str, phrases: &mut Vec<String>) -> String {
    let mut result = Vec::new();
    for word in input.split_whitespace() {
        if word.starts_with('\u{00}')
            || word.contains(['*', '"'])
            || !word.chars().any(|ch| NAME_PUNCTUATION.contains(ch))
        {
            result.push(word.to_owned());
            continue;
        }
        let concat = FTS_PUNCT_STRIP.replace_all(word, "").into_owned();
        if concat.is_empty() || concat == word {
            result.push(word.to_owned());
            continue;
        }
        let sub_tokens: Vec<String> = FTS5_SPECIAL_CHARS
            .replace_all(word, " ")
            .split_whitespace()
            .map(str::to_owned)
            .collect();
        if sub_tokens.len() < 2 {
            result.push(concat);
            continue;
        }
        if is_dotted_abbreviation(word, &sub_tokens) {
            phrases.push(format!("\"{}\"", sub_tokens.join(" ")));
        } else {
            phrases.push(format!(
                "(\"{}\" OR {concat}*)",
                sub_tokens.join(" ")
            ));
        }
        result.push(format!(
            "{PHRASE_PLACEHOLDER_PREFIX}{}{PHRASE_PLACEHOLDER_SUFFIX}",
            phrases.len() - 1
        ));
    }
    result.join(" ")
}

fn is_dotted_abbreviation(word: &str, sub_tokens: &[String]) -> bool {
    for ch in word.chars() {
        if !ch.is_alphanumeric() && ch != '.' {
            return false;
        }
    }
    sub_tokens.iter().all(|token| is_single_unicode_letter(token))
}

fn is_single_unicode_letter(token: &str) -> bool {
    let mut chars = token.chars();
    let Some(first) = chars.next() else {
        return false;
    };
    first.is_alphabetic() && chars.next().is_none()
}

pub fn fts_query_degraded(original: &str, fts_query: &str) -> bool {
    let original = original.trim();
    if original.is_empty() || fts_query.is_empty() {
        return false;
    }
    let stripped = original.replace('"', "");
    let alpha_num = FTS_PUNCT_STRIP.replace_all(&stripped, "");
    if alpha_num.len() == stripped.len() {
        return false;
    }
    for token in fts_query.split_whitespace() {
        let token = token.trim_end_matches('*');
        if token.starts_with('\u{00}') {
            return false;
        }
        if token.starts_with('(') {
            return false;
        }
        if let Some(inner) = token.strip_prefix('"').and_then(|v| v.strip_suffix('"')) {
            for part in FTS_PUNCT_STRIP.replace_all(inner, " ").split_whitespace() {
                if part.chars().count() > 2 {
                    return false;
                }
            }
            continue;
        }
        if token.chars().count() > 2 {
            return false;
        }
    }
    true
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn wraps_single_word() {
        let out = build_fts5_query("beatles");
        assert_eq!(out.query, "(beatles OR beatles*)");
        assert!(!out.degraded);
    }

    #[test]
    fn handles_punctuated_name() {
        let out = build_fts5_query("a-ha");
        assert_eq!(out.query, "(\"a ha\" OR aha*)");
    }

    #[test]
    fn collapses_dotted_abbreviation() {
        let out = build_fts5_query("R.E.M.");
        assert_eq!(out.query, "\"R E M\"");
    }
}
