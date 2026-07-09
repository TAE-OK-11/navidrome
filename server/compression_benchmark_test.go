package server

import (
	"io"
	"net/http"
	"testing"
)

type benchmarkResponseWriter struct {
	header http.Header
}

func (w *benchmarkResponseWriter) Header() http.Header {
	return w.header
}

func (*benchmarkResponseWriter) WriteHeader(int) {}

func (*benchmarkResponseWriter) Write(p []byte) (int, error) {
	return io.Discard.Write(p)
}

func BenchmarkCompressionLargeSingleWrite(b *testing.B) {
	body := make([]byte, 256<<10)
	for i := range body {
		body[i] = byte(i)
	}

	b.Run("raw", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		for b.Loop() {
			underlying := &benchmarkResponseWriter{header: make(http.Header)}
			underlying.header.Set("Content-Type", "application/octet-stream")
			w := &compressResponseWriter{
				ResponseWriter: underlying,
				accepted:       acceptedCompressions{zstd: true},
				path:           "/rest/example",
			}
			if _, err := w.Write(body); err != nil {
				b.Fatal(err)
			}
			if err := w.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("zstd", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		for b.Loop() {
			underlying := &benchmarkResponseWriter{header: make(http.Header)}
			underlying.header.Set("Content-Type", "application/json")
			w := &compressResponseWriter{
				ResponseWriter: underlying,
				accepted:       acceptedCompressions{zstd: true},
				path:           "/rest/example",
			}
			if _, err := w.Write(body); err != nil {
				b.Fatal(err)
			}
			if err := w.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
