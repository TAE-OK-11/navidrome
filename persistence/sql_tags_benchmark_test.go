package persistence

import (
	"testing"

	"github.com/navidrome/navidrome/model"
)

var benchmarkTagsJSON = marshalTags(model.Tags{
	model.TagGenre:        {"Rock", "Alternative Rock"},
	model.TagMood:         {"Energetic", "Bright"},
	model.TagGrouping:     {"Favorites"},
	model.TagISRC:         {"USRC17607839"},
	model.TagAlbumVersion: {"Deluxe Edition"},
})

func BenchmarkUnmarshalTags(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := unmarshalTags(benchmarkTagsJSON); err != nil {
			b.Fatal(err)
		}
	}
}
