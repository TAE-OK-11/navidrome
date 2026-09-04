use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

/// Mirrors core/integration/ssrf.go so DestArtwork fetches cannot target
/// loopback, RFC1918, CGNAT, link-local, or documentation ranges.
pub fn is_safe_artwork_ip(ip: IpAddr) -> bool {
    match unmap(ip) {
        IpAddr::V4(v4) => is_safe_v4(v4),
        IpAddr::V6(v6) => is_safe_v6(v6),
    }
}

fn unmap(ip: IpAddr) -> IpAddr {
    match ip {
        IpAddr::V6(v6) => v6.to_ipv4_mapped().map(IpAddr::V4).unwrap_or(ip),
        other => other,
    }
}

fn is_safe_v4(ip: Ipv4Addr) -> bool {
    if ip.is_unspecified()
        || ip.is_loopback()
        || ip.is_private()
        || ip.is_link_local()
        || ip.is_multicast()
        || ip.is_broadcast()
    {
        return false;
    }
    let octets = ip.octets();
    // 0.0.0.0/8 (Go special-use; unspecified 0.0.0.0 already rejected)
    if octets[0] == 0 {
        return false;
    }
    // 100.64.0.0/10 CGNAT
    if octets[0] == 100 && octets[1] & 0b1100_0000 == 64 {
        return false;
    }
    // 192.0.0.0/24 IETF protocol assignments
    if octets[0] == 192 && octets[1] == 0 && octets[2] == 0 {
        return false;
    }
    // 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24 documentation
    if ip.is_documentation() {
        return false;
    }
    // 198.18.0.0/15 benchmarking
    if octets[0] == 198 && (octets[1] == 18 || octets[1] == 19) {
        return false;
    }
    // 240.0.0.0/4 reserved
    if octets[0] >= 240 {
        return false;
    }
    true
}

fn is_safe_v6(ip: Ipv6Addr) -> bool {
    if ip.is_unspecified()
        || ip.is_loopback()
        || ip.is_multicast()
        || ip.is_unicast_link_local()
        || ip.is_unique_local()
    {
        return false;
    }
    let segments = ip.segments();
    // 2001:db8::/32 documentation
    if segments[0] == 0x2001 && segments[1] == 0x0db8 {
        return false;
    }
    true
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::IpAddr;

    #[test]
    fn rejects_special_use_addresses() {
        for raw in [
            "0.0.0.0",
            "127.0.0.1",
            "10.0.0.1",
            "100.64.0.1",
            "169.254.169.254",
            "192.0.2.1",
            "198.18.0.1",
            "224.0.0.1",
            "240.0.0.1",
            "::1",
            "fc00::1",
            "fe80::1",
            "2001:db8::1",
        ] {
            let ip: IpAddr = raw.parse().unwrap();
            assert!(!is_safe_artwork_ip(ip), "{raw} should be rejected");
        }
    }

    #[test]
    fn allows_public_unicast() {
        assert!(is_safe_artwork_ip("8.8.8.8".parse().unwrap()));
        assert!(is_safe_artwork_ip("2606:4700:4700::1111".parse().unwrap()));
    }

    #[test]
    fn unmaps_ipv4_mapped_loopback() {
        let mapped: IpAddr = "::ffff:127.0.0.1".parse().unwrap();
        assert!(!is_safe_artwork_ip(mapped));
    }
}
