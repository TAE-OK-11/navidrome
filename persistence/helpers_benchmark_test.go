package persistence

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
)

var benchmarkSQLArgs map[string]any

var (
	benchmarkFirstCap   = regexp.MustCompile("(.)([A-Z][a-z]+)")
	benchmarkAllCap     = regexp.MustCompile("([a-z0-9])([A-Z])")
	benchmarkUnderscore = regexp.MustCompile("_([A-Za-z])")
)

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

func BenchmarkCaseConversion(b *testing.B) {
	b.Run("legacy_snake", func(b *testing.B) {
		for b.Loop() {
			snake := benchmarkFirstCap.ReplaceAllString("SortAlbumArtistName", "${1}_${2}")
			_ = strings.ToLower(benchmarkAllCap.ReplaceAllString(snake, "${1}_${2}"))
		}
	})
	b.Run("snake", func(b *testing.B) {
		for b.Loop() {
			_ = toSnakeCase("SortAlbumArtistName")
		}
	})
	b.Run("legacy_camel", func(b *testing.B) {
		for b.Loop() {
			_ = benchmarkUnderscore.ReplaceAllStringFunc("sort_album_artist_name", func(s string) string {
				return strings.ToUpper(strings.ReplaceAll(s, "_", ""))
			})
		}
	})
	b.Run("camel", func(b *testing.B) {
		for b.Loop() {
			_ = toCamelCase("sort_album_artist_name")
		}
	})
}
