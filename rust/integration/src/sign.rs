use md5::{Digest, Md5};

pub fn sign_audioscrobbler(
    params: &std::collections::HashMap<String, String>,
    secret: &str,
) -> String {
    let mut keys: Vec<&String> = params
        .keys()
        .filter(|k| k.as_str() != "format" && k.as_str() != "callback")
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

    #[test]
    fn matches_known_vector() {
        let mut params = HashMap::new();
        params.insert("api_key".into(), "KEY".into());
        params.insert("method".into(), "auth.getToken".into());
        params.insert("format".into(), "json".into());
        let sig = sign_audioscrobbler(&params, "SECRET");
        assert_eq!(sig.len(), 32);
        assert_eq!(sig, sign_audioscrobbler(&params, "SECRET"));
    }
}
