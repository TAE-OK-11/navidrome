package persistence

import (
	"testing"

	"github.com/navidrome/navidrome/model"
)

var benchmarkParticipantsJSON = marshalParticipants(model.Participants{
	model.RoleArtist: {
		{Artist: model.Artist{ID: "artist-1", Name: "Primary Artist"}},
		{Artist: model.Artist{ID: "artist-2", Name: "Featured Artist"}},
	},
	model.RoleAlbumArtist: {
		{Artist: model.Artist{ID: "artist-1", Name: "Primary Artist"}},
	},
	model.RoleComposer: {
		{Artist: model.Artist{ID: "artist-3", Name: "Composer"}},
	},
	model.RolePerformer: {
		{Artist: model.Artist{ID: "artist-4", Name: "Guitarist"}, SubRole: "guitar"},
		{Artist: model.Artist{ID: "artist-5", Name: "Drummer"}, SubRole: "drums"},
	},
})

func BenchmarkUnmarshalParticipants(b *testing.B) {
	b.ReportAllocs()
	b.Run("decode", func(b *testing.B) {
		for b.Loop() {
			if _, err := decodeParticipants(benchmarkParticipantsJSON); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("cached", func(b *testing.B) {
		for b.Loop() {
			if _, err := unmarshalParticipants(benchmarkParticipantsJSON); err != nil {
				b.Fatal(err)
			}
		}
	})
}
