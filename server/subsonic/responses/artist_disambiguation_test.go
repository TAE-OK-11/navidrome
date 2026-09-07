package responses_test

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/navidrome/navidrome/server/subsonic/responses"
)

func TestArtistDisambiguationPresence(t *testing.T) {
	artist := responses.ArtistID3{Id: "one", Name: "Death"}
	for _, legacy := range []bool{true, false} {
		if !legacy {
			artist.OpenSubsonicArtistID3 = &responses.OpenSubsonicArtistID3{}
		}
		for _, marshal := range []func(any) ([]byte, error){json.Marshal, xml.Marshal} {
			encoded, err := marshal(artist)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "disambiguation") == legacy {
				t.Errorf("disambiguation presence for legacy=%v: %s", legacy, encoded)
			}
		}
	}
}
