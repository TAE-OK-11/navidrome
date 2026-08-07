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

func TestLyricsAPIUsesZstdForLowLatencyDynamicCompression(t *testing.T) {
	body := strings.Repeat("[00:01.00] lyric line\n", 32)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"lyrics":"`+body+`"}`)
	})

	req := httptest.NewRequest(http.MethodGet, "/rest/getLyricsBySongId.view", nil)
	req.Header.Set("Accept-Encoding", "br, zstd, gzip")
	rec := httptest.NewRecorder()
	compressMiddleware()(handler).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "zstd" {
		t.Fatalf("Content-Encoding = %q, want zstd", got)
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
