package librefm

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/core/scrobbler"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const librefmScrobbleOK = `{"scrobbles":{"scrobble":{"ignoredMessage":{"code":"0"}},"@attr":{"accepted":1}}}`

var _ = Describe("librefmAgent", func() {
	var ds model.DataStore
	var ctx context.Context
	var agent *librefmAgent
	var httpClient *tests.FakeHttpClient
	var track *model.MediaFile

	BeforeEach(func() {
		ds = &tests.MockDataStore{}
		ctx = context.Background()
		DeferCleanup(configtest.SetupConfig())
		conf.Server.LibreFM.Enabled = true
		conf.Server.LibreFM.ApiKey = "key"
		conf.Server.LibreFM.Secret = "secret"
		_ = ds.UserProps(ctx).Put("user-1", sessionKeyProperty, "SK-1")
		httpClient = &tests.FakeHttpClient{}
		agent = librefmConstructor(ds)
		agent.client = newClient("key", "secret", "https://libre.fm/2.0/", httpClient)
		track = &model.MediaFile{
			ID:          "123",
			Title:       "Track Title",
			Album:       "Track Album",
			Artist:      "Track Artist",
			AlbumArtist: "Album Artist",
			TrackNumber: 1,
			Duration:    180,
		}
	})

	Describe("Scrobble", func() {
		It("sends a scrobble successfully", func() {
			httpClient.Res = http.Response{Body: io.NopCloser(bytes.NewBufferString(librefmScrobbleOK)), StatusCode: 200}
			err := agent.Scrobble(ctx, "user-1", scrobbler.Scrobble{MediaFile: *track, TimeStamp: time.Now()})
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns ErrNotAuthorized if user is not linked", func() {
			err := agent.Scrobble(ctx, "user-2", scrobbler.Scrobble{MediaFile: *track, TimeStamp: time.Now()})
			Expect(err).To(MatchError(scrobbler.ErrNotAuthorized))
		})

		It("returns ErrRetryLater on service offline", func() {
			httpClient.Res = http.Response{
				Body:       io.NopCloser(bytes.NewBufferString(`{"error":11,"message":"Service Offline"}`)),
				StatusCode: 400,
			}
			err := agent.Scrobble(ctx, "user-1", scrobbler.Scrobble{MediaFile: *track, TimeStamp: time.Now()})
			Expect(err).To(MatchError(scrobbler.ErrRetryLater))
		})

		It("returns ErrUnrecoverable when the service rejects the scrobble", func() {
			httpClient.Res = http.Response{
				Body: io.NopCloser(bytes.NewBufferString(
					`{"scrobbles":{"scrobble":{"ignoredMessage":{"code":"4","#text":"too old"}},"@attr":{"accepted":0}}}`,
				)),
				StatusCode: 200,
			}
			err := agent.Scrobble(ctx, "user-1", scrobbler.Scrobble{MediaFile: *track, TimeStamp: time.Now()})
			Expect(err).To(MatchError(scrobbler.ErrUnrecoverable))
		})

		It("skips songs with less than 31 seconds", func() {
			track.Duration = 20
			httpClient.Res = http.Response{Body: io.NopCloser(bytes.NewBufferString(librefmScrobbleOK)), StatusCode: 200}
			err := agent.Scrobble(ctx, "user-1", scrobbler.Scrobble{MediaFile: *track, TimeStamp: time.Now()})
			Expect(err).ToNot(HaveOccurred())
			Expect(httpClient.SavedRequest).To(BeNil())
		})
	})
})
