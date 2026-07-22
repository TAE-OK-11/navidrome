package plugins

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/plugins/host"
)

const (
	httpClientDefaultTimeout     = 10 * time.Second
	httpClientMaxRedirects       = 5
	httpClientMaxResponseBodyLen = 10 * 1024 * 1024 // 10 MB
)

// contextKey is used for per-request redirect control via context.
type contextKey struct{}

// noFollowRedirectsKey signals the CheckRedirect callback to stop following redirects.
var noFollowRedirectsKey = contextKey{}

// httpServiceImpl implements host.HTTPService.
type httpServiceImpl struct {
	pluginName    string
	requiredHosts []string
	client        *http.Client
}

// newHTTPService creates a new HTTPService for a plugin.
func newHTTPService(pluginName string, permission *HTTPPermission) *httpServiceImpl {
	var requiredHosts []string
	if permission != nil {
		requiredHosts = permission.RequiredHosts
	}
	svc := &httpServiceImpl{
		pluginName:    pluginName,
		requiredHosts: requiredHosts,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if len(requiredHosts) == 0 {
		// A proxy resolves the target outside this process, so the default
		// internet-only policy cannot verify the selected dial IP. Explicit
		// allowlists retain the normal proxy behavior.
		transport.Proxy = nil
		transport.DialContext = svc.dialPublicContext
	}
	svc.client = &http.Client{
		Transport: transport,
		// Timeout is set per-request via context deadline, not here.
		// CheckRedirect validates hosts and enforces redirect limits.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.Context().Value(noFollowRedirectsKey) != nil {
				return http.ErrUseLastResponse
			}
			if len(via) >= httpClientMaxRedirects {
				log.Warn(req.Context(), "HTTP redirect limit exceeded", "plugin", svc.pluginName, "url", req.URL.String(), "redirectCount", len(via))
				return http.ErrUseLastResponse
			}
			if err := svc.validateHost(req.Context(), req.URL.Host); err != nil {
				log.Warn(req.Context(), "HTTP redirect blocked", "plugin", svc.pluginName, "url", req.URL.String(), "err", err)
				return err
			}
			return nil
		},
	}
	return svc
}

// dialPublicContext resolves and pins the destination before dialing. This
// prevents DNS aliases and rebinding from bypassing the default SSRF policy.
// It is used only when requiredHosts is empty; explicit allowlists preserve
// their existing semantics, including intentionally allowed private hosts.
func (s *httpServiceImpl) dialPublicContext(ctx context.Context, network, address string) (net.Conn, error) {
	hostname, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parsing dial address %q: %w", address, err)
	}
	lookupNetwork := "ip"
	if strings.HasSuffix(network, "4") {
		lookupNetwork = "ip4"
	} else if strings.HasSuffix(network, "6") {
		lookupNetwork = "ip6"
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, lookupNetwork, hostname)
	if err != nil {
		return nil, fmt.Errorf("resolving host %q: %w", hostname, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("resolving host %q: no addresses", hostname)
	}
	for _, ip := range ips {
		if isBlockedOutboundIP(ip) {
			log.Warn(ctx, "HTTP dial to special-use address blocked", "plugin", s.pluginName, "host", hostname, "ip", ip)
			return nil, fmt.Errorf("host %q resolves to disallowed address %s", hostname, ip)
		}
	}

	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, ip := range ips {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, lastErr
}

func (s *httpServiceImpl) Send(ctx context.Context, request host.HTTPRequest) (*host.HTTPResponse, error) {
	// Parse and validate URL
	parsedURL, err := url.Parse(request.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Validate URL scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("invalid URL scheme %q: must be http or https", parsedURL.Scheme)
	}

	// Validate host against allowed hosts and private IP restrictions
	if err := s.validateHost(ctx, parsedURL.Host); err != nil {
		return nil, err
	}

	// Apply per-request timeout via context deadline
	timeout := cmp.Or(time.Duration(request.TimeoutMs)*time.Millisecond, httpClientDefaultTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Signal CheckRedirect to not follow redirects for this request
	if request.NoFollowRedirects {
		ctx = context.WithValue(ctx, noFollowRedirectsKey, true)
	}

	// Build request body
	method := strings.ToUpper(request.Method)
	var body io.Reader
	if len(request.Body) > 0 {
		body = bytes.NewReader(request.Body)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, method, request.URL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	for k, v := range request.Headers {
		httpReq.Header.Set(k, v)
	}

	// Execute request
	resp, err := s.client.Do(httpReq) //nolint:gosec // URL is validated against requiredHosts
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	log.Trace(ctx, "HTTP request", "plugin", s.pluginName, "method", method, "url", request.URL, "status", resp.StatusCode)

	// Read response body (with size limit to prevent memory exhaustion)
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, httpClientMaxResponseBodyLen))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	// Flatten response headers (first value only)
	headers := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	return &host.HTTPResponse{
		StatusCode: int32(resp.StatusCode),
		Headers:    headers,
		Body:       respBody,
	}, nil
}

// validateHost checks whether a request to the given host is permitted.
// When requiredHosts is set, it checks against the allowlist.
// When requiredHosts is empty, it blocks private/loopback IPs to prevent SSRF.
func (s *httpServiceImpl) validateHost(ctx context.Context, hostStr string) error {
	hostname := extractHostname(hostStr)

	if len(s.requiredHosts) > 0 {
		if !s.isHostAllowed(hostname) {
			return fmt.Errorf("host %q is not allowed", hostStr)
		}
		return nil
	}

	// No explicit allowlist: block private/loopback IPs
	if isPrivateOrLoopback(hostname) {
		log.Warn(ctx, "HTTP request to private/loopback address blocked", "plugin", s.pluginName, "host", hostStr)
		return fmt.Errorf("host %q is not allowed: private/loopback addresses require explicit requiredHosts in manifest", hostStr)
	}
	return nil
}

func (s *httpServiceImpl) isHostAllowed(hostname string) bool {
	for _, pattern := range s.requiredHosts {
		if matchHostPattern(pattern, hostname) {
			return true
		}
	}
	return false
}

// extractHostname returns the hostname portion of a host string, stripping
// any port number and IPv6 brackets. It handles IPv6 addresses correctly
// (e.g. "[::1]:8080" → "::1", "[::1]" → "::1").
func extractHostname(hostStr string) string {
	if h, _, err := net.SplitHostPort(hostStr); err == nil {
		return h
	}
	// Strip IPv6 brackets when no port is present (e.g. "[::1]" → "::1")
	if strings.HasPrefix(hostStr, "[") && strings.HasSuffix(hostStr, "]") {
		return hostStr[1 : len(hostStr)-1]
	}
	return hostStr
}

// isPrivateOrLoopback rejects literal special-use addresses and localhost by
// name. Hostname resolution is checked and pinned separately at dial time.
func isPrivateOrLoopback(hostname string) bool {
	if strings.EqualFold(strings.TrimSuffix(hostname, "."), "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(hostname)
	if err != nil {
		return false
	}
	return isBlockedOutboundIP(ip)
}

var blockedOutboundPrefixes = []netip.Prefix{
	// IPv4 special-use networks (IANA IPv4 Special-Purpose Address Registry).
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"),
	netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	// IPv6 special-use networks (including IPv4 translation/tunneling ranges).
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2620:4f:8000::/48"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func isBlockedOutboundIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return true
	}
	for _, prefix := range blockedOutboundPrefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// Verify interface implementation
var _ host.HTTPService = (*httpServiceImpl)(nil)
