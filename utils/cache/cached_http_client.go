package cache

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/navidrome/navidrome/log"
)

const cacheSizeLimit = 100

type HTTPClient struct {
	cache SimpleCache[string, string]
	hc    httpDoer
	ttl   time.Duration
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewHTTPClient(wrapped httpDoer, ttl time.Duration) *HTTPClient {
	c := &HTTPClient{hc: wrapped, ttl: ttl}
	c.cache = NewSimpleCache[string, string](Options{
		SizeLimit:  cacheSizeLimit,
		DefaultTTL: ttl,
	})
	return c
}

func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	key, cachedReq, err := c.cacheKeyAndRequest(req)
	if err != nil {
		return nil, err
	}
	cached := true
	start := time.Now()
	respStr, err := c.cache.GetWithLoader(key, func(key string) (string, time.Duration, error) {
		cached = false
		resp, err := c.hc.Do(cachedReq)
		if err != nil {
			log.Trace(req.Context(), "CachedHTTPClient.Do", "req", req, err)
			return "", 0, err
		}
		defer resp.Body.Close()
		return c.serializeResponse(resp), c.ttl, nil
	})
	log.Trace(req.Context(), "CachedHTTPClient.Do", "key", key, "cached", cached, "elapsed", time.Since(start), err)
	if err != nil {
		return nil, err
	}
	return c.deserializeResponse(req, respStr)
}

func (c *HTTPClient) cacheKeyAndRequest(req *http.Request) (string, *http.Request, error) {
	cachedReq := req.Clone(req.Context())
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return "", nil, err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
		cachedReq.Body = io.NopCloser(bytes.NewReader(body))
	}

	var key strings.Builder
	key.Grow(len(req.Method) + len(req.URL.String()) + 64)
	key.WriteString(req.Method)
	key.WriteByte(' ')
	key.WriteString(req.URL.String())
	key.WriteByte('\n')

	if len(req.Header) > 0 {
		headerNames := make([]string, 0, len(req.Header))
		for name := range req.Header {
			headerNames = append(headerNames, name)
		}
		sort.Strings(headerNames)
		for _, name := range headerNames {
			key.WriteString(strings.ToLower(name))
			key.WriteByte(':')
			values := append([]string(nil), req.Header[name]...)
			sort.Strings(values)
			key.WriteString(strings.Join(values, "\x00"))
			key.WriteByte('\n')
		}
	}

	if len(body) > 0 {
		sum := sha256.Sum256(body)
		key.WriteString("body-sha256:")
		key.WriteString(hex.EncodeToString(sum[:]))
	}
	return key.String(), cachedReq, nil
}

func (c *HTTPClient) serializeResponse(resp *http.Response) string {
	var b = &bytes.Buffer{}
	_ = resp.Write(b)
	return b.String()
}

func (c *HTTPClient) deserializeResponse(req *http.Request, respStr string) (*http.Response, error) {
	r := bufio.NewReader(strings.NewReader(respStr))
	return http.ReadResponse(r, req)
}
