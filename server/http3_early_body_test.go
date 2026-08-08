package server

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTP3EarlyDataRejectsRequestBodiesWithoutConnectionGate(t *testing.T) {
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

func TestHTTP3EarlyDataAllowsCoverArtRead(t *testing.T) {
	for _, path := range []string{
		"https://example.test/rest/getCoverArt?id=al-1&size=600",
		"https://example.test/rest/getCoverArt.view?id=al-1&size=600",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.TLS = &tls.ConnectionState{HandshakeComplete: false}
		rec := httptest.NewRecorder()
		guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("path=%s status=%d, want 204", path, rec.Code)
		}
	}
}

func TestHTTP3EarlyDataWaitsForHandshakeInsteadOfRetrying(t *testing.T) {
	complete := make(chan struct{})
	req := httptest.NewRequest(http.MethodGet, "https://example.test/rest/getAlbumInfo.view?id=1", nil)
	req.TLS = &tls.ConnectionState{HandshakeComplete: false}
	req = req.WithContext(context.WithValue(req.Context(), http3HandshakeGateKey{}, http3HandshakeGate{
		complete:  complete,
		completed: func() bool { return true },
	}))

	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("request completed before QUIC handshake gate opened")
	case <-time.After(20 * time.Millisecond):
	}

	close(complete)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not resume after QUIC handshake completed")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", rec.Code)
	}
}

func TestHTTP3EarlyDataFallsBackTo425WhenHandshakeFails(t *testing.T) {
	complete := make(chan struct{})
	close(complete)
	req := httptest.NewRequest(http.MethodPost, "https://example.test/rest/scrobble.view", nil)
	req.TLS = &tls.ConnectionState{HandshakeComplete: false}
	req = req.WithContext(context.WithValue(req.Context(), http3HandshakeGateKey{}, http3HandshakeGate{
		complete:  complete,
		completed: func() bool { return false },
	}))
	rec := httptest.NewRecorder()
	guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusTooEarly {
		t.Fatalf("status=%d, want 425", rec.Code)
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
