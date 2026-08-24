package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestDynamicAPIUsesZstdWhenAvailable(t *testing.T) {
	body := strings.Repeat(`{"id":"track","title":"Taylor Swift"}`, 64)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/song", nil)
	req.Header.Set("Accept-Encoding", "br, zstd, gzip")
	rec := httptest.NewRecorder()
	compressMiddleware()(handler).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "zstd" {
		t.Fatalf("Content-Encoding = %q, want zstd", got)
	}
	if got := decodeZstdPolicyTest(t, rec.Body.Bytes()); got != body {
		t.Fatalf("decoded body mismatch: got %d bytes, want %d", len(got), len(body))
	}
}

func TestRustHTTP3CompressionMarkerBypassesGoAndIsStripped(t *testing.T) {
	body := strings.Repeat(`{"ok":true}`, 40)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(rustHTTP3CompressionHeader); got != "" {
			t.Fatalf("private compression marker reached handler: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ping/details", nil)
	req.Header.Set("Accept-Encoding", "zstd")
	req.Header.Set(rustHTTP3CompressionHeader, "zstd")
	rec := httptest.NewRecorder()
	compressMiddleware()(handler).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want Rust bridge to handle compression", got)
	}
	if rec.Body.String() != body {
		t.Fatal("raw bridge body mismatch")
	}
}

func TestModuleJavaScriptUsesBrotliLikeWebUI(t *testing.T) {
	body := strings.Repeat("export const x = 1;\n", 80)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = io.WriteString(w, body)
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "br, zstd, gzip")
	rec := httptest.NewRecorder()
	compressMiddleware()(handler).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
}

func TestIdentityQZeroCompressesSmallJSON(t *testing.T) {
	body := `{"ok":true}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set("Accept-Encoding", "zstd, identity;q=0")
	rec := httptest.NewRecorder()
	compressMiddleware()(handler).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "zstd" {
		t.Fatalf("Content-Encoding = %q, want zstd when identity is forbidden", got)
	}
}

func TestLyricsAPIUsesBrotliForDenseText(t *testing.T) {
	body := strings.Repeat("[00:01.00] lyric line\n", 32)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"lyrics":"`+body+`"}`)
	})

	req := httptest.NewRequest(http.MethodGet, "/rest/getLyricsBySongId.view", nil)
	req.Header.Set("Accept-Encoding", "br, zstd, gzip")
	rec := httptest.NewRecorder()
	compressMiddleware()(handler).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
}

func TestKnownContentLengthAPICompressesWithoutLargeDecisionBuffer(t *testing.T) {
	body := strings.Repeat(`{"ok":true}`, 40)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = io.WriteString(w, body)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ping/details", nil)
	req.Header.Set("Accept-Encoding", "zstd")
	rec := httptest.NewRecorder()
	compressMiddleware()(handler).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "zstd" {
		t.Fatalf("Content-Encoding = %q, want zstd", got)
	}
	if rec.Header().Get("Content-Length") != "" {
		t.Fatal("compressed response must not retain original Content-Length")
	}
}

func TestCompressionSkipsAlreadyCompressedAndPartialBodies(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		status      int
	}{
		{name: "jpeg", contentType: "image/jpeg", status: http.StatusOK},
		{name: "zip", contentType: "application/zip", status: http.StatusOK},
		{name: "woff2", contentType: "font/woff2", status: http.StatusOK},
		{name: "partial-json", contentType: "application/json", status: http.StatusPartialContent},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				if tc.status == http.StatusPartialContent {
					w.Header().Set("Content-Range", "bytes 0-511/4096")
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, strings.Repeat("payload", 256))
			})

			req := httptest.NewRequest(http.MethodGet, "/api/blob", nil)
			req.Header.Set("Accept-Encoding", "br, zstd, gzip")
			rec := httptest.NewRecorder()
			compressMiddleware()(handler).ServeHTTP(rec, req)

			if got := rec.Header().Get("Content-Encoding"); got != "" {
				t.Fatalf("Content-Encoding = %q, want empty", got)
			}
		})
	}
}

func TestCompressionHonorsAcceptEncodingQualityAndWildcard(t *testing.T) {
	body := strings.Repeat(`{"ok":true,"name":"metadata"}`, 64)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "client prefers gzip", header: "zstd;q=0.2, gzip;q=1, br;q=0.5", want: "gzip"},
		{name: "server zstd wins equal quality", header: "br;q=1, zstd;q=1, gzip;q=1", want: "zstd"},
		{name: "wildcard includes zstd", header: "br;q=0, *;q=0.8", want: "zstd"},
		{name: "explicit zstd zero overrides wildcard", header: "zstd;q=0, br;q=0, *;q=0.7", want: "gzip"},
		{name: "invalid q does not outrank valid encoding", header: "zstd;q=2, gzip;q=0.5", want: "gzip"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("Accept-Encoding", tc.header)
			rec := httptest.NewRecorder()
			compressMiddleware()(handler).ServeHTTP(rec, req)
			if got := rec.Header().Get("Content-Encoding"); got != tc.want {
				t.Fatalf("Content-Encoding = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCompressionWeakensRepresentationValidators(t *testing.T) {
	body := strings.Repeat(`{"id":"track"}`, 64)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"strong-tag"`)
		w.Header().Set("Content-MD5", "old-md5")
		w.Header().Set("Content-Digest", "sha-256=:old:")
		w.Header().Set("Digest", "sha-256=old")
		_, _ = io.WriteString(w, body)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/song", nil)
	req.Header.Set("Accept-Encoding", "zstd")
	rec := httptest.NewRecorder()
	compressMiddleware()(handler).ServeHTTP(rec, req)

	if got := rec.Header().Get("ETag"); got != `W/"strong-tag"` {
		t.Fatalf("ETag = %q, want weak validator", got)
	}
	for _, header := range []string{"Content-MD5", "Content-Digest", "Digest"} {
		if got := rec.Header().Get(header); got != "" {
			t.Fatalf("%s = %q, want empty after compression", header, got)
		}
	}
}

func TestCompressionKeepsExistingWeakETag(t *testing.T) {
	body := strings.Repeat(`{"id":"track"}`, 64)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `W/"semantic-tag"`)
		_, _ = io.WriteString(w, body)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/song", nil)
	req.Header.Set("Accept-Encoding", "zstd")
	rec := httptest.NewRecorder()
	compressMiddleware()(handler).ServeHTTP(rec, req)
	if got := rec.Header().Get("ETag"); got != `W/"semantic-tag"` {
		t.Fatalf("ETag = %q, want existing weak validator", got)
	}
}

type informationalPolicyWriter struct {
	header   http.Header
	statuses []int
	body     strings.Builder
}

func (w *informationalPolicyWriter) Header() http.Header { return w.header }
func (w *informationalPolicyWriter) WriteHeader(status int) {
	w.statuses = append(w.statuses, status)
}
func (w *informationalPolicyWriter) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func TestCompressionDoesNotTreatEarlyHintsAsFinalStatus(t *testing.T) {
	body := strings.Repeat(`{"ok":true}`, 64)
	underlying := &informationalPolicyWriter{header: make(http.Header)}
	w := &compressResponseWriter{
		ResponseWriter: underlying,
		accepted:       acceptedCompressions{zstd: true},
		path:           "/api/test",
	}

	w.Header().Set("Link", "</app.js>; rel=preload")
	w.WriteHeader(http.StatusEarlyHints)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if len(underlying.statuses) != 2 || underlying.statuses[0] != http.StatusEarlyHints || underlying.statuses[1] != http.StatusOK {
		t.Fatalf("statuses = %v, want [103 200]", underlying.statuses)
	}
	if got := underlying.header.Get("Content-Encoding"); got != "zstd" {
		t.Fatalf("Content-Encoding = %q, want zstd", got)
	}
}

func TestAPICompressionUsesSmallDecisionBufferPool(t *testing.T) {
	buf, pool := getCompressionBuffer(apiCompressionDecisionBufferSize)
	if pool != &apiCompressionBufferPool {
		t.Fatal("API decision buffer did not use the API-sized pool")
	}
	if cap(buf) != apiCompressionDecisionBufferSize {
		t.Fatalf("API buffer capacity = %d, want %d", cap(buf), apiCompressionDecisionBufferSize)
	}
	pool.Put(buf[:0])
}

func decodeZstdPolicyTest(t *testing.T, data []byte) string {
	t.Helper()
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	out, err := decoder.DecodeAll(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
