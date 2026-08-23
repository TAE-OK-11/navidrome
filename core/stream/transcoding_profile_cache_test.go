package stream

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

type countingTranscodingRepository struct {
	model.TranscodingRepository
	calls atomic.Int32
}

func (r *countingTranscodingRepository) FindByFormat(format string) (*model.Transcoding, error) {
	r.calls.Add(1)
	if format == "opus" {
		return &model.Transcoding{ID: "opus", TargetFormat: format, DefaultBitRate: 128}, nil
	}
	return nil, model.ErrNotFound
}

func TestTranscodingProfileCacheCollapsesRepeatedReads(t *testing.T) {
	repository := &countingTranscodingRepository{}
	ds := &tests.MockDataStore{MockedTranscoding: repository}
	cache := newTranscodingProfileCache()

	first := cache.get(context.Background(), ds, "opus")
	if first == nil || first.DefaultBitRate != 128 {
		t.Fatalf("first lookup=%+v", first)
	}
	first.DefaultBitRate = 1
	second := cache.get(context.Background(), ds, "opus")
	if second == nil || second.DefaultBitRate != 128 {
		t.Fatalf("cached lookup was mutated: %+v", second)
	}
	if got := repository.calls.Load(); got != 1 {
		t.Fatalf("FindByFormat calls=%d, want 1", got)
	}

	if missing := cache.get(context.Background(), ds, "missing"); missing != nil {
		t.Fatalf("missing lookup=%+v", missing)
	}
	if missing := cache.get(context.Background(), ds, "missing"); missing != nil {
		t.Fatalf("cached missing lookup=%+v", missing)
	}
	if got := repository.calls.Load(); got != 2 {
		t.Fatalf("FindByFormat calls after negative cache=%d, want 2", got)
	}
}
