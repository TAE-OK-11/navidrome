package publicurl

import (
	"net/http"
	"testing"

	"github.com/go-chi/jwtauth/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/model"
)

func BenchmarkImageURL(b *testing.B) {
	auth.TokenAuth = jwtauth.New("HS256", []byte("benchmark secret"), nil)
	conf.Server.ShareURL = ""
	req, err := http.NewRequest(http.MethodGet, "http://localhost/app", nil)
	if err != nil {
		b.Fatal(err)
	}
	artID := model.NewArtworkID(model.KindArtistArtwork, "benchmark-artist", nil)

	b.ReportAllocs()
	for b.Loop() {
		_ = ImageURL(req, artID, 600)
	}
}
