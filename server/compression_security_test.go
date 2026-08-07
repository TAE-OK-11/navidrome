package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSensitiveAuthResponsesAreNeverCompressed(t *testing.T) {
	body := strings.Repeat(`{"token":"secret","subsonicToken":"secret2","salt":"salt"}`, 16)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	for _, target := range []string{
		"/auth/login",
		"/auth/createAdmin",
		"/music/auth/login",
	} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, target, nil)
			req.Header.Set("Accept-Encoding", "zstd, br, gzip")
			rec := httptest.NewRecorder()
			compressMiddleware()(handler).ServeHTTP(rec, req)
			if got := rec.Header().Get("Content-Encoding"); got != "" {
				t.Fatalf("Content-Encoding = %q, want identity", got)
			}
			if got := rec.Body.String(); got != body {
				t.Fatalf("body changed on auth response: got %d bytes, want %d", len(got), len(body))
			}
		})
	}
}
