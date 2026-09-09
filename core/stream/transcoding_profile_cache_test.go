package stream

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

type blockingTranscodingRepository struct {
	model.TranscodingRepository
	started chan struct{}
	finish  chan struct{}
}

func (r *blockingTranscodingRepository) FindByFormat(format string) (*model.Transcoding, error) {
	close(r.started)
	<-r.finish
	return &model.Transcoding{TargetFormat: format, DefaultBitRate: 128}, nil
}

func TestTranscodingProfileLookupCanBeCanceledWithoutCancelingSharedLoad(t *testing.T) {
	repository := &blockingTranscodingRepository{started: make(chan struct{}), finish: make(chan struct{})}
	ds := &tests.MockDataStore{MockedTranscoding: repository}
	cache := newTranscodingProfileCache()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	release := sync.OnceFunc(func() { close(repository.finish) })
	defer release()
	done := make(chan *model.Transcoding, 1)
	go func() { done <- cache.get(ctx, ds, "opus") }()
	select {
	case <-repository.started:
	case <-time.After(time.Second):
		t.Fatal("lookup did not start")
	}
	cancel()
	select {
	case value := <-done:
		if value != nil {
			t.Fatal("canceled lookup returned a profile")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled lookup remained blocked on the database")
	}
	release()
	if profile := cache.get(t.Context(), ds, "opus"); profile == nil || profile.DefaultBitRate != 128 {
		t.Fatalf("replacement request could not reuse shared lookup: %+v", profile)
	}
}
