package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestOrderedPrecompressedEncodingsHonorsQuality(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   []compressionEncoding
	}{
		{
			name:   "brotli preferred on equal quality",
			header: "br, zstd, gzip",
			want:   []compressionEncoding{compressionBrotli, compressionZstd, compressionGzip},
		},
		{
			name:   "client zstd preference wins",
			header: "br;q=0.5, zstd;q=1, gzip;q=0.2",
			want:   []compressionEncoding{compressionZstd, compressionBrotli, compressionGzip},
		},
		{
			name:   "wildcard includes zstd and gzip",
			header: "br;q=0, *;q=0.8",
			want:   []compressionEncoding{compressionZstd, compressionGzip},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := orderedPrecompressedEncodings(acceptedCompressionEncodings(tc.header))
			if len(got) != len(tc.want) {
				t.Fatalf("order length = %d, want %d (%v)", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("order[%d] = %q, want %q (all=%v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

func TestPrecompressedFileServerFallsBackToNextAcceptedEncoding(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/app.js", []byte("plain"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/app.js.zst", []byte("zstd-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	// Brotli is preferred by quality but intentionally absent on disk. The
	// server should continue to the next acceptable representation, Zstd.
	req.Header.Set("Accept-Encoding", "br;q=1, zstd;q=0.9, gzip;q=0.5")
	rec := httptest.NewRecorder()
	PrecompressedFileServer(os.DirFS(dir)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "zstd" {
		t.Fatalf("Content-Encoding = %q, want zstd", got)
	}
	if got := rec.Body.String(); got != "zstd-bytes" {
		t.Fatalf("body = %q, want zstd-bytes", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
}

func TestPrecompressedFileServerDoesNotServeEncodingWithQZero(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/app.js", []byte("plain"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/app.js.br", []byte("brotli-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	req.Header.Set("Accept-Encoding", "br;q=0")
	rec := httptest.NewRecorder()
	PrecompressedFileServer(os.DirFS(dir)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want identity", got)
	}
	if got := rec.Body.String(); got != "plain" {
		t.Fatalf("body = %q, want plain", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding on identity fallback", got)
	}
}

func TestPrecompressedFileServerIdentityFallbackVariesByAcceptEncoding(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/app.js", []byte("plain-only"), 0o600); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	req.Header.Set("Accept-Encoding", "br, zstd, gzip")
	rec := httptest.NewRecorder()
	PrecompressedFileServer(os.DirFS(dir)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want identity", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	if got := rec.Body.String(); got != "plain-only" {
		t.Fatalf("body = %q, want plain-only", got)
	}
}
