use md5::{Digest, Md5};

const BASE62: &[u8; 62] = b"0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ";
const HASH_SEPARATOR: [u8; 3] = [0xe2, 0x80, 0x8b];

/// Matches Go `id.NewHash`: MD5 over segments separated by U+200B, base62-encoded to 22 chars.
pub fn new_hash(data: &[&str]) -> String {
    let mut hasher = Md5::new();
    for segment in data {
        hasher.update(segment.as_bytes());
        hasher.update(HASH_SEPARATOR);
    }
    encode_md5(hasher.finalize().into())
}

fn encode_md5(bytes: [u8; 16]) -> String {
    let mut value = u128::from_be_bytes(bytes);
    if value == 0 {
        return "0".repeat(22);
    }
    let mut digits = Vec::with_capacity(22);
    while value > 0 {
        digits.push(BASE62[(value % 62) as usize]);
        value /= 62;
    }
    digits.reverse();
    format!("{:0>22}", unsafe { String::from_utf8_unchecked(digits) })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn matches_go_reference_vectors() {
        assert_eq!(new_hash(&["test"]), "5cLJPkLA5DK2BADhoeotPk");
        assert_eq!(new_hash(&["hello", "world"]), "0QzS8wlDQspvekMXI12llZ");
    }
}
