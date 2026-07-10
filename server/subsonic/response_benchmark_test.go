package subsonic

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/navidrome/navidrome/server/subsonic/responses"
)

var benchmarkJSONResponse = buildBenchmarkJSONResponse()

func buildBenchmarkJSONResponse() *responses.JsonWrapper {
	created := time.Unix(1_700_000_000, 0).UTC()
	songs := make([]responses.Child, 100)
	for i := range songs {
		songs[i] = responses.Child{
			Id:          "song-id",
			Parent:      "album-id",
			Title:       "A representative song title",
			Album:       "A representative album",
			Artist:      "A representative artist",
			Track:       int32(i + 1),
			Year:        2026,
			Genre:       "Alternative Rock",
			CoverArt:    "al-album-id_0",
			Size:        12_000_000,
			ContentType: "audio/flac",
			Suffix:      "flac",
			Duration:    240,
			BitRate:     900,
			Path:        "Artist/Album/01 - Song.flac",
			Created:     &created,
			AlbumId:     "album-id",
			ArtistId:    "artist-id",
			Type:        "music",
			OpenSubsonicChild: &responses.OpenSubsonicChild{
				MediaType:          responses.MediaTypeSong,
				MusicBrainzId:      "00000000-0000-0000-0000-000000000000",
				Genres:             responses.Array[responses.ItemGenre]{{Name: "Alternative Rock"}},
				Moods:              responses.Array[string]{"Energetic"},
				Artists:            responses.Array[responses.ArtistID3Ref]{{Id: "artist-id", Name: "A representative artist"}},
				AlbumArtists:       responses.Array[responses.ArtistID3Ref]{{Id: "artist-id", Name: "A representative artist"}},
				DisplayArtist:      "A representative artist",
				DisplayAlbumArtist: "A representative artist",
			},
		}
	}
	return &responses.JsonWrapper{Subsonic: responses.Subsonic{
		Status:        responses.StatusOK,
		Version:       "1.16.1",
		Type:          "navidrome",
		ServerVersion: "benchmark",
		OpenSubsonic:  true,
		AlbumWithSongsID3: &responses.AlbumWithSongsID3{
			AlbumID3: responses.AlbumID3{Id: "album-id", Name: "A representative album", SongCount: 100, Created: created},
			Song:     songs,
		},
	}}
}

func BenchmarkSubsonicJSONMarshal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(benchmarkJSONResponse); err != nil {
			b.Fatal(err)
		}
	}
}
