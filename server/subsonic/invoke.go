package subsonic

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/subsonic/responses"
)

const pluginInvokeBodyLimit int64 = 10 * 1024 * 1024

type limitedBuffer struct {
	buf       bytes.Buffer
	header    http.Header
	status    int
	remaining int64
	err       error
}

func newLimitedBuffer(limit int64) *limitedBuffer {
	return &limitedBuffer{header: make(http.Header), remaining: limit, status: http.StatusOK}
}

func (w *limitedBuffer) Header() http.Header { return w.header }
func (w *limitedBuffer) WriteHeader(status int) {
	if w.status == http.StatusOK {
		w.status = status
	}
}
func (w *limitedBuffer) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if int64(len(p)) > w.remaining {
		w.err = fmt.Errorf("SubsonicAPI response body exceeds limit: %d bytes", pluginInvokeBodyLimit)
		p = p[:w.remaining]
	}
	n, err := w.buf.Write(p)
	w.remaining -= int64(n)
	if w.err != nil {
		return n, w.err
	}
	return n, err
}

func (api *Router) h(r chi.Router, endpoint string, f handler) {
	api.hr(r, endpoint, func(_ http.ResponseWriter, req *http.Request) (*responses.Subsonic, error) {
		return f(req)
	})
}

func (api *Router) hr(r chi.Router, endpoint string, f handlerRaw) {
	if api.internalHandlers == nil {
		api.internalHandlers = make(map[string]handlerRaw)
	}
	api.internalHandlers[endpoint] = f
	hr(r, endpoint, f)
}

// Invoke runs a Subsonic endpoint as a function call instead of an HTTP
// round-trip. Plugins use this so internal library access is not REST/P2P.
func (api *Router) Invoke(ctx context.Context, endpoint string, query url.Values, username string, asJSON bool) (string, []byte, error) {
	endpoint = strings.TrimSuffix(path.Base(endpoint), ".view")
	f, ok := api.internalHandlers[endpoint]
	if !ok {
		return "", nil, fmt.Errorf("unknown Subsonic endpoint %q", endpoint)
	}
	if query == nil {
		query = url.Values{}
	} else {
		query = cloneValues(query)
	}
	if asJSON {
		query.Set("f", "json")
	}
	u := &url.URL{Path: "/" + endpoint, RawQuery: query.Encode()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", nil, err
	}
	req = req.WithContext(request.WithInternalAuth(req.Context(), username))
	if api.ds != nil && username != "" {
		if usr, err := api.ds.User(ctx).FindByUsername(username); err == nil && usr != nil {
			req = req.WithContext(request.WithUser(req.Context(), *usr))
			req = req.WithContext(request.WithUsername(req.Context(), usr.UserName))
		}
	}
	if client := query.Get("c"); client != "" {
		req = req.WithContext(request.WithClient(req.Context(), client))
	}

	rec := newLimitedBuffer(pluginInvokeBodyLimit)
	res, err := f(rec, req)
	if rec.err != nil {
		return "", nil, rec.err
	}
	if err != nil {
		payload := errorResponse(err)
		body, encErr := encodeInvokeJSON(payload)
		if encErr != nil {
			return "", nil, encErr
		}
		return "application/json", body, nil
	}
	if req.Context().Err() != nil {
		return rec.header.Get("Content-Type"), rec.buf.Bytes(), nil
	}
	if rec.buf.Len() > 0 {
		ct := rec.header.Get("Content-Type")
		if ct == "" {
			ct = "application/octet-stream"
		}
		return ct, rec.buf.Bytes(), nil
	}
	if res == nil {
		return rec.header.Get("Content-Type"), nil, nil
	}
	buf := borrowResponseBuffer()
	defer recycleResponseBuffer(buf)
	if asJSON {
		if encErr := encodeJSON(buf, responses.JsonWrapper{Subsonic: *res}); encErr != nil {
			return "", nil, encErr
		}
		return "application/json", append([]byte(nil), buf.Bytes()...), nil
	}
	if encErr := xml.NewEncoder(buf).Encode(res); encErr != nil {
		return "", nil, encErr
	}
	return "application/xml", append([]byte(nil), buf.Bytes()...), nil
}

func encodeInvokeJSON(payload *responses.Subsonic) ([]byte, error) {
	buf := borrowResponseBuffer()
	defer recycleResponseBuffer(buf)
	if err := encodeJSON(buf, responses.JsonWrapper{Subsonic: *payload}); err != nil {
		return nil, err
	}
	return append([]byte(nil), buf.Bytes()...), nil
}

func cloneValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}
