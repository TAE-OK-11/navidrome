package stream

import (
	"context"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

func BenchmarkLegacyStreamDecision(b *testing.B) {
	ds := &tests.MockDataStore{MockedTranscoding: &tests.MockTranscodingRepo{}}
	decider := NewTranscodeDecider(ds, nil)
	mediaFile := &model.MediaFile{
		ID: "benchmark", Title: "Track", Suffix: "flac", Codec: "flac",
		BitRate: 1000, SampleRate: 48000, Channels: 2, Duration: 240,
		UpdatedAt: time.Unix(1, 0),
	}
	ctx := context.Background()

	b.Run("direct", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			request := decider.ResolveRequest(ctx, mediaFile, "", 0, 0)
			if request.Format != "raw" {
				b.Fatalf("format=%q", request.Format)
			}
		}
	})
	b.Run("transcode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			request := decider.ResolveRequest(ctx, mediaFile, "mp3", 192, 0)
			if request.Format != "mp3" {
				b.Fatalf("format=%q", request.Format)
			}
		}
	})
}
