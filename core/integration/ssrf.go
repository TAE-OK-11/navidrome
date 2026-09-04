package integration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

var specialUsePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

type ssrfIPLookup func(context.Context, string, string) ([]net.IP, error)
type ssrfDialFunc func(context.Context, string, string) (net.Conn, error)

type ssrfDialer struct {
	lookup ssrfIPLookup
	dial   ssrfDialFunc
}

func isSafeArtworkIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range specialUsePrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func (d ssrfDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid artwork destination %q: %w", address, err)
	}
	ips, err := d.lookup(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolving artwork destination %q: %w", host, err)
	}
	var lastErr error
	for _, ip := range ips {
		if !isSafeArtworkIP(ip) {
			lastErr = fmt.Errorf("artwork destination %q resolved to disallowed address %s", host, ip)
			continue
		}
		conn, dialErr := d.dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("artwork destination %q has no usable addresses", host)
	}
	return nil, lastErr
}

func newSSRFTransport() *http.Transport {
	netDialer := &net.Dialer{Timeout: 5 * time.Second}
	dialer := ssrfDialer{
		lookup: net.DefaultResolver.LookupIP,
		dial:   netDialer.DialContext,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = dialer.DialContext
	return transport
}

func artworkRedirectCheck(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("unsupported artwork redirect scheme %q", req.URL.Scheme)
	}
	return nil
}
