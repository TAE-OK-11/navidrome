package nativeapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/navidrome/navidrome/consts"
)

const invokeBodyLimit int64 = 32 * 1024 * 1024

type limitedInvokeBuffer struct {
	buf       bytes.Buffer
	header    http.Header
	status    int
	remaining int64
	err       error
}

func newLimitedInvokeBuffer(limit int64) *limitedInvokeBuffer {
	return &limitedInvokeBuffer{header: make(http.Header), remaining: limit, status: http.StatusOK}
}

func (w *limitedInvokeBuffer) Header() http.Header { return w.header }
func (w *limitedInvokeBuffer) WriteHeader(status int) {
	if w.status == http.StatusOK {
		w.status = status
	}
}
func (w *limitedInvokeBuffer) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if int64(len(p)) > w.remaining {
		w.err = fmt.Errorf("native API response exceeds limit: %d bytes", invokeBodyLimit)
		p = p[:w.remaining]
	}
	n, err := w.buf.Write(p)
	w.remaining -= int64(n)
	if w.err != nil {
		return n, w.err
	}
	return n, err
}

// Invoke runs a Native REST handler in-process. Path is relative to /api (e.g.
// "/song", "/playlist/abc"). The bearer token is forwarded as X-ND-Authorization.
func (api *Router) Invoke(ctx context.Context, method, path string, query url.Values, contentType string, body []byte, token string) (int, http.Header, []byte, error) {
	if api.Handler == nil {
		return 0, nil, nil, fmt.Errorf("native API router is not configured")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, nil, nil, fmt.Errorf("path is required")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if query == nil {
		query = url.Values{}
	} else {
		query = cloneNativeValues(query)
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, path+"?"+query.Encode(), bodyReader)
	if err != nil {
		return 0, nil, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set(consts.UIAuthorizationHeader, "Bearer "+token)
	}

	rec := newLimitedInvokeBuffer(invokeBodyLimit)
	api.Handler.ServeHTTP(rec, req)
	if rec.err != nil {
		return 0, nil, nil, rec.err
	}
	return rec.status, rec.header.Clone(), rec.buf.Bytes(), nil
}

// Open streams a Native REST handler response to w (for gRPC Open/media proxy).
func (api *Router) Open(ctx context.Context, method, path string, query url.Values, contentType string, body []byte, token string, w http.ResponseWriter) error {
	if api.Handler == nil {
		return fmt.Errorf("native API router is not configured")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if query == nil {
		query = url.Values{}
	} else {
		query = cloneNativeValues(query)
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, path+"?"+query.Encode(), bodyReader)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set(consts.UIAuthorizationHeader, "Bearer "+token)
	}

	api.Handler.ServeHTTP(w, req)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func cloneNativeValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}
