package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTP3EarlyDataRejectsRequestBodies(t *testing.T) {
	handler := guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, contentLength := range []int64{1, -1} {
		req := httptest.NewRequest(http.MethodGet, "https://example.test/rest/getAlbum.view?id=1", strings.NewReader("x"))
		req.TLS = &tls.ConnectionState{HandshakeComplete: false}
		req.ContentLength = contentLength
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusTooEarly {
			t.Fatalf("ContentLength=%d status=%d, want 425", contentLength, rec.Code)
		}
	}
}

func TestHTTP3EarlyDataAllowsBodylessMetadataRead(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.test/rest/getAlbum.view?id=1", nil)
	req.TLS = &tls.ConnectionState{HandshakeComplete: false}
	rec := httptest.NewRecorder()
	guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", rec.Code)
	}
}

func TestHTTP3EarlyDataAllowsHotIdempotentReads(t *testing.T) {
	handler := guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, target := range []string{
		"https://example.test/",
		"https://example.test/assets/app.js",
		"https://example.test/assets/app.css",
		"https://example.test/rest/getCoverArt.view?id=album-1",
		"https://example.test/rest/getLyricsBySongId.view?id=song-1",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.TLS = &tls.ConnectionState{HandshakeComplete: false}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("target=%s status=%d, want 204", target, rec.Code)
		}
	}
}

func TestHTTP3EarlyDataStillRejectsLargeOrStateChangingReads(t *testing.T) {
	handler := guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, target := range []string{
		"https://example.test/rest/stream.view?id=song-1",
		"https://example.test/rest/star.view?id=song-1",
		"https://example.test/api/lastfm/callback?token=x",
		"https://example.test/share/s/token",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.TLS = &tls.ConnectionState{HandshakeComplete: false}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusTooEarly {
			t.Fatalf("target=%s status=%d, want 425", target, rec.Code)
		}
	}
}

func TestHTTP3TrustedProxyStreamLimitIsRaisedConservatively(t *testing.T) {
	if serverQUICMaxIncomingStreams <= 100 {
		t.Fatalf("MaxIncomingStreams=%d, want above quic-go default 100", serverQUICMaxIncomingStreams)
	}
	if serverQUICMaxIncomingStreams > 512 {
		t.Fatalf("MaxIncomingStreams=%d, unexpectedly high", serverQUICMaxIncomingStreams)
	}
}
