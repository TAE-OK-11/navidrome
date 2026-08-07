package server

import (
	"net/http"
	"testing"
)

func TestBrotliFallbackNeverUsesZstdNumericLevel(t *testing.T) {
	profile := selectCompressionProfile(
		acceptedCompressions{brotli: true},
		"/api/album",
		make(http.Header),
		"application/json",
		4096,
	)
	if profile.encoding != compressionBrotli {
		t.Fatalf("encoding = %q, want br", profile.encoding)
	}
	if profile.level != brotliLargeLevel {
		t.Fatalf("Brotli fallback level = %d, want %d", profile.level, brotliLargeLevel)
	}
}

func TestHugeBrotliResponseUsesDedicatedHugePoolLevel(t *testing.T) {
	header := make(http.Header)
	header.Set("Content-Length", "524288")
	profile := selectCompressionProfile(
		acceptedCompressions{brotli: true},
		"/export/metadata",
		header,
		"application/json",
		524288,
	)
	if profile.encoding != compressionBrotli {
		t.Fatalf("encoding = %q, want br", profile.encoding)
	}
	if profile.level != brotliHugeLevel {
		t.Fatalf("huge Brotli level = %d, want %d", profile.level, brotliHugeLevel)
	}
}

func TestBrotliPoolMappingMatchesConfiguredLevels(t *testing.T) {
	if brotliPool(brotliLargeLevel) != &brotliLargeWriterPool {
		t.Fatal("Brotli level 5 must map to large-writer pool")
	}
	if brotliPool(brotliHugeLevel) != &brotliHugeWriterPool {
		t.Fatal("Brotli level 6 must map to huge-writer pool")
	}
}
