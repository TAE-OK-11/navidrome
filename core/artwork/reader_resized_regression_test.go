package artwork

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/tests"
)

type countedArtworkSource struct {
	data   []byte
	path   string
	opens  int
	closes int
	cancel context.CancelFunc
}

func (s *countedArtworkSource) Key() string            { return "test-cover" }
func (s *countedArtworkSource) LastUpdated() time.Time { return time.Time{} }
func (s *countedArtworkSource) Reader(context.Context) (io.ReadCloser, string, error) {
	s.opens++
	if s.cancel != nil {
		s.cancel()
	}
	return &countedArtworkStream{Reader: bytes.NewReader(s.data), source: s}, s.path, nil
}

type countedArtworkStream struct {
	io.Reader
	source *countedArtworkSource
}

func (s *countedArtworkStream) Close() error { s.source.closes++; return nil }

func TestResizedArtworkReusesSmallOriginal(t *testing.T) {
	for _, embedded := range []bool{false, true} {
		t.Run(map[bool]string{false: "remote", true: "embedded"}[embedded], func(t *testing.T) {
			source := &countedArtworkSource{data: generatePNG(t, 2, 2), path: "https://example.test/cover.png"}
			if embedded {
				source.path = filepath.Join(t.TempDir(), "song.mp3")
				if err := os.WriteFile(source.path, []byte("audio container, not image bytes"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			r := &resizedArtworkReader{original: source, size: 300}
			out, _, err := r.Reader(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			got, err := io.ReadAll(out)
			if err != nil || !bytes.Equal(got, source.data) {
				t.Fatalf("original image changed: %v", err)
			}
			if source.opens != 1 || source.closes != 1 {
				t.Fatalf("opens=%d closes=%d, want one each", source.opens, source.closes)
			}
		})
	}
}

func TestResizedArtworkPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &countedArtworkSource{data: generatePNG(t, 2, 2), cancel: cancel}
	r := &resizedArtworkReader{original: source, size: 300}
	out, _, err := r.Reader(ctx)
	if out != nil {
		out.Close()
		t.Fatal("canceled request returned artwork")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want cancellation, got %v", err)
	}
	if source.opens != 1 || source.closes != 1 {
		t.Fatalf("opens=%d closes=%d", source.opens, source.closes)
	}
}

func TestResizedArtworkEmbeddedThumbnail(t *testing.T) {
	tests.Init(t, false)
	source := &countedArtworkSource{data: generatePNG(t, 600, 600), path: filepath.Join(t.TempDir(), "song.mp3")}
	if err := os.WriteFile(source.path, []byte("audio container, not image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	r := &resizedArtworkReader{original: source, size: 30}
	out, _, err := r.Reader(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	config, _, err := image.DecodeConfig(out)
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 30 || config.Height != 30 {
		t.Fatalf("embedded thumbnail is %dx%d, want 30x30", config.Width, config.Height)
	}
	if source.opens != 1 || source.closes != 1 {
		t.Fatalf("opens=%d closes=%d, want one each", source.opens, source.closes)
	}
}

func TestResizedArtworkGIFPreservesWorkerError(t *testing.T) {
	tests.Init(t, false)
	r := &resizedArtworkReader{size: 0}
	_, _, err := r.resizeImage(t.Context(), bytes.NewReader(createAnimatedGIF(3)))
	if err == nil || !strings.Contains(err.Error(), "size") || strings.Contains(err.Error(), "%!w") {
		t.Fatalf("lost worker validation error: %v", err)
	}
}

func TestResizedArtworkRejectsTruncatedJPEG(t *testing.T) {
	r := &resizedArtworkReader{size: 300}
	_, _, err := r.resizeImage(t.Context(), bytes.NewReader([]byte{0xff, 0xd8, 0xff}))
	if err == nil {
		t.Fatal("JPEG fast path accepted a truncated header")
	}
}

func BenchmarkResizedArtworkPassthrough(b *testing.B) {
	b.Setenv("ND_GRPCWORKERINTESTS", "1")
	if err := metadataworker.EnsureTestBinary(); err != nil {
		b.Fatal(err)
	}
	source := &countedArtworkSource{data: generateJPEG(b, 600, 600, 90), path: "https://example.test/cover.jpg"}
	// Warm a real worker so both versions measure the production IPC path
	// without including process startup in the timed workload.
	if _, err := persistentImageWorkers.sniffAnimation(b.Context(), source.data); err != nil {
		b.Fatal(err)
	}
	r := &resizedArtworkReader{original: source, size: 1200}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, _, err := r.Reader(b.Context())
		if err != nil {
			b.Fatal(err)
		}
		_, err = io.Copy(io.Discard, out)
		out.Close()
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(source.opens)/float64(b.N), "source_reads/op")
}
