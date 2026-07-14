package stream

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/navidrome/navidrome/core/stream/hotcache"
	"github.com/navidrome/navidrome/model"
	"github.com/stretchr/testify/require"
)

type playbackObserverFile struct {
	*os.File
	begins   int
	observes int
}

func (f *playbackObserverFile) BeginPlayback(hotcache.PlaybackObservation) {
	f.begins++
}

func (f *playbackObserverFile) ObservePlayback(context.Context, hotcache.PlaybackObservation) {
	f.observes++
}

type panicResponseWriter struct {
	header http.Header
}

func (w *panicResponseWriter) Header() http.Header { return w.header }
func (*panicResponseWriter) WriteHeader(int)       {}
func (*panicResponseWriter) Write([]byte) (int, error) {
	panic("response writer failed")
}

func TestSeekableStreamObservesPlaybackWhenResponseWriterPanics(t *testing.T) {
	path := t.TempDir() + "/track.flac"
	require.NoError(t, os.WriteFile(path, []byte("audio payload"), 0o600))
	file, err := os.Open(path)
	require.NoError(t, err)
	observed := &playbackObserverFile{File: file}
	t.Cleanup(func() { _ = observed.Close() })

	stream := &Stream{
		mf:         &model.MediaFile{ID: "track", Title: "Track", Suffix: "flac"},
		format:     "raw",
		ReadCloser: observed,
		Seeker:     observed,
	}
	stream.TrackPlayback()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/stream", nil)
	require.NoError(t, err)
	writer := &panicResponseWriter{header: make(http.Header)}

	require.Panics(t, func() {
		_, _ = stream.Serve(context.Background(), writer, request)
	})
	require.Equal(t, 1, observed.begins)
	require.Equal(t, 1, observed.observes)
}

func TestMediaStreamerBypassesDisabledHotCache(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(directory+"/track.flac", []byte("audio payload"), 0o600))
	mf := &model.MediaFile{ID: "track", LibraryPath: directory, Path: "track.flac", Suffix: "flac"}
	streamer := NewMediaStreamer(nil, nil, nil, hotcache.New(hotcache.Options{})).(*mediaStreamer)

	require.Nil(t, streamer.resolver)
	result, err := streamer.NewStream(context.Background(), mf, Request{Format: "raw"})
	require.NoError(t, err)
	require.IsType(t, &os.File{}, result.ReadCloser)
	require.NoError(t, result.Close())
}
