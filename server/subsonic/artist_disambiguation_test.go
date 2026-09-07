package subsonic

import (
	"context"
	"testing"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

func TestArtistDisambiguationHonorsLegacyClients(t *testing.T) {
	old := conf.Server.Subsonic.LegacyClients
	conf.Server.Subsonic.LegacyClients = "legacy-test"
	defer func() { conf.Server.Subsonic.LegacyClients = old }()
	artist := model.Artist{Name: "Death", Disambiguation: "US death metal band"}
	result := toOSArtistID3(context.Background(), artist)
	if result == nil || result.Disambiguation != artist.Disambiguation {
		t.Fatal("artist comment missing from OpenSubsonic response")
	}
	if toOSArtistID3(request.WithClient(context.Background(), "legacy-test"), artist) != nil {
		t.Fatal("extended artist fields emitted to a legacy client")
	}
}
