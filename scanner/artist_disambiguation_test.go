package scanner

import (
	"testing"

	"github.com/navidrome/navidrome/model"
)

func TestArtistDisambiguationSurvivesCreditDeduplication(t *testing.T) {
	credit := func(id, comment string) model.Participant {
		return model.Participant{Artist: model.Artist{ID: id, Name: "Death", Disambiguation: comment}}
	}
	entry := &folderEntry{tracks: model.MediaFiles{
		{Participants: model.Participants{model.RoleArtist: {credit("one", "")}}},
		{Participants: model.Participants{
			model.RoleArtist:      {credit("one", "US death metal band")},
			model.RoleAlbumArtist: {credit("two", "Detroit proto-punk band")},
		}},
	}}
	(&phaseFolders{}).createArtistsFromMediaFiles(entry)
	if len(entry.artists) != 2 {
		t.Fatalf("same-name artists with different IDs merged: %#v", entry.artists)
	}
	for _, artist := range entry.artists {
		want := map[string]string{"one": "US death metal band", "two": "Detroit proto-punk band"}[artist.ID]
		if artist.Disambiguation != want {
			t.Errorf("artist %s comment = %q, want %q", artist.ID, artist.Disambiguation, want)
		}
	}
}
