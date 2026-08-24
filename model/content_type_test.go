package model

import (
	"mime"
	"testing"
)

var (
	benchmarkContentType   string
	benchmarkContentSuffix = "flac"
)

func TestContentTypeForSuffix(t *testing.T) {
	t.Parallel()

	for suffix, want := range map[string]string{
		"flac": "audio/flac",
		"m4a":  "audio/mp4",
		"mp3":  "audio/mpeg",
		"opus": "audio/ogg",
		"wav":  "audio/wav",
		"txt":  "text/plain; charset=utf-8",
	} {
		if got := ContentTypeForSuffix(suffix); got != want {
			t.Errorf("ContentTypeForSuffix(%q) = %q, want %q", suffix, got, want)
		}
	}
}

func BenchmarkContentTypeForSuffix(b *testing.B) {
	b.Run("fast-path", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchmarkContentType = ContentTypeForSuffix(benchmarkContentSuffix)
			if benchmarkContentType != "audio/flac" {
				b.Fatal(benchmarkContentType)
			}
		}
	})
	b.Run("registry", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchmarkContentType = mime.TypeByExtension("." + benchmarkContentSuffix)
			if benchmarkContentType != "audio/flac" {
				b.Fatal(benchmarkContentType)
			}
		}
	})
}
