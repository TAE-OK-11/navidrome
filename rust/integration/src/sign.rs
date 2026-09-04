use md5::{Digest, Md5};

pub fn sign_audioscrobbler(
    params: &std::collections::HashMap<String, String>,
    secret: &str,
) -> String {
    let mut keys: Vec<&String> = params
        .keys()
        .filter(|k| {
            let key = k.as_str();
            key != "format" && key != "callback" && key != "api_sig"
        })
        .collect();
    keys.sort();
    let mut msg = String::new();
    for key in keys {
        msg.push_str(key);
        if let Some(value) = params.get(key) {
            msg.push_str(value);
        }
    }
    msg.push_str(secret);
    let digest = Md5::digest(msg.as_bytes());
    hex_encode(&digest)
}

fn hex_encode(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[derive(serde::Deserialize)]
    struct SignVector {
        secret: String,
        params: HashMap<String, String>,
        expected: String,
    }

    #[test]
    fn matches_shared_vector() {
        let raw = include_str!("../../../tests/fixtures/integration/sign_vector.json");
        let vector: SignVector = serde_json::from_str(raw).expect("parse sign vector");
        let sig = sign_audioscrobbler(&vector.params, &vector.secret);
        assert_eq!(sig, vector.expected);
    }

    #[test]
    fn ignores_api_sig() {
        let raw = include_str!("../../../tests/fixtures/integration/sign_vector.json");
        let mut vector: SignVector = serde_json::from_str(raw).expect("parse sign vector");
        vector.params.insert("api_sig".into(), "stale".into());
        let sig = sign_audioscrobbler(&vector.params, &vector.secret);
        assert_eq!(sig, vector.expected);
    }
}
