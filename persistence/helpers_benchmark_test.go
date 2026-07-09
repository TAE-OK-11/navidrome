package persistence

import (
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
)

var benchmarkSQLArgs map[string]any

func BenchmarkToSQLArgsMediaFile(b *testing.B) {
	bpm := 120
	mf := &dbMediaFile{MediaFile: &model.MediaFile{
		ID:          "track-id",
		PID:         "persistent-id",
		LibraryID:   1,
		FolderID:    "folder-id",
		Path:        "artist/album/01-track.flac",
		Title:       "Track",
		Album:       "Album",
		Artist:      "Artist",
		AlbumArtist: "Artist",
		Duration:    240,
		BitRate:     1000,
		SampleRate:  48000,
		Channels:    2,
		BPM:         &bpm,
		CreatedAt:   time.Unix(1, 0),
		UpdatedAt:   time.Unix(2, 0),
	}}

	b.ReportAllocs()
	for b.Loop() {
		args, err := toSQLArgs(mf)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkSQLArgs = args
	}
}
