package responses_test

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/navidrome/navidrome/server/subsonic/responses"
)

func TestArtistDisambiguationPresence(t *testing.T) {
	for _, tc := range []struct {
		name      string
		extension *responses.OpenSubsonicArtistID3
	}{
		{name: "legacy"},
		{name: "empty", extension: &responses.OpenSubsonicArtistID3{Disambiguation: new("")}},
		{name: "populated", extension: &responses.OpenSubsonicArtistID3{Disambiguation: new("US death metal band")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			artist := responses.ArtistID3{Id: "one", Name: "Death", OpenSubsonicArtistID3: tc.extension}
			for _, value := range []any{artist, responses.ArtistWithAlbumsID3{
				ArtistID3: artist,
				Album:     []responses.AlbumID3{{Id: "album-one"}},
			}} {
				for _, marshal := range []func(any) ([]byte, error){json.Marshal, xml.Marshal} {
					encoded, err := marshal(value)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(encoded), "disambiguation") != (tc.extension != nil) {
						t.Errorf("incorrect disambiguation presence: %s", encoded)
					}
					if tc.extension != nil && !strings.Contains(string(encoded), *tc.extension.Disambiguation) {
						t.Errorf("comment missing: %s", encoded)
					}
					if _, withAlbums := value.(responses.ArtistWithAlbumsID3); withAlbums && !strings.Contains(string(encoded), "album-one") {
						t.Errorf("nested album missing: %s", encoded)
					}
				}
			}
		})
	}
}
