package external_test

import (
	"bytes"
	"context"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/core/agents"
	. "github.com/navidrome/navidrome/core/external"
	"github.com/navidrome/navidrome/core/matcher"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Provider - AlbumImage", func() {
	var ds *tests.MockDataStore
	var provider Provider
	var mockArtistRepo *mockArtistRepo
	var mockAlbumRepo *mockAlbumRepo
	var mockMediaFileRepo *mockMediaFileRepo
	var mockAlbumAgent *mockAlbumInfoAgent
	var ctx context.Context

	BeforeEach(func() {
		ctx = GinkgoT().Context()
		DeferCleanup(configtest.SetupConfig())
		conf.Server.Agents = "mockAlbum" // Configure mock agent

		mockArtistRepo = newMockArtistRepo()
		mockAlbumRepo = newMockAlbumRepo()
		mockMediaFileRepo = newMockMediaFileRepo()

		ds = &tests.MockDataStore{
			MockedArtist:    mockArtistRepo,
			MockedAlbum:     mockAlbumRepo,
			MockedMediaFile: mockMediaFileRepo,
		}

		mockAlbumAgent = newMockAlbumInfoAgent()
		mockAlbumAgent.On("GetAlbumInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&agents.AlbumInfo{}, nil).Maybe()
		mockAlbumAgent.On("GetAlbumImages", mock.Anything, "Album One", mock.Anything, mock.Anything).
			Return([]agents.ExternalImage{
				{URL: "http://example.com/large.jpg", Size: 1000},
				{URL: "http://example.com/medium.jpg", Size: 500},
				{URL: "http://example.com/small.jpg", Size: 200},
			}, nil).Maybe()

		agentsCombined := &mockAgents{albumInfoAgent: mockAlbumAgent}
		provider = NewProvider(ds, agentsCombined, matcher.New(ds))
		DeferCleanup(provider.Close)

		mockAlbumRepo.On("Get", "mf-1").Return(nil, model.ErrNotFound).Maybe()
		mockMediaFileRepo.On("Get", "album-1").Return(nil, model.ErrNotFound).Maybe()
		mockArtistRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Maybe()
		mockAlbumRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Maybe()
		mockMediaFileRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Maybe()
	})

	It("enqueues a background fetch and returns ErrNotFound on cache miss", func() {
		mockAlbumRepo.On("Get", "album-1").Return(&model.Album{ID: "album-1", Name: "Album One", AlbumArtistID: "artist-1"}, nil).Once()
		var logBuf bytes.Buffer
		log.SetOutput(&logBuf)
		defer log.SetOutput(GinkgoWriter)
		log.SetLevel(log.LevelDebug)

		imgURL, err := provider.AlbumImage(ctx, "album-1")

		Expect(err).To(MatchError(model.ErrNotFound))
		Expect(imgURL).To(BeNil())
		mockAlbumRepo.AssertCalled(GinkgoT(), "Get", "album-1")
		mockArtistRepo.AssertNotCalled(GinkgoT(), "Get", "artist-1")
		Expect(logBuf.String()).To(ContainSubstring("Album image not cached, enqueuing background fetch"))
		Eventually(func() bool { return albumImageAgentCalled(mockAlbumAgent) }).WithTimeout(2 * time.Second).Should(BeTrue())
	})

	It("fetches images asynchronously after a cache miss", func() {
		mockAlbumRepo.On("Get", "album-1").Return(&model.Album{ID: "album-1", Name: "Album One", AlbumArtistID: "artist-1"}, nil).Once()

		_, err := provider.AlbumImage(ctx, "album-1")
		Expect(err).To(MatchError(model.ErrNotFound))

		Eventually(func() bool { return albumImageAgentCalled(mockAlbumAgent) }).WithTimeout(2 * time.Second).Should(BeTrue())
	})

	It("returns ErrNotFound if the album is not found in the DB", func() {
		mockAlbumRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Once()
		mockMediaFileRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Once()

		imgURL, err := provider.AlbumImage(ctx, "not-found")

		Expect(err).To(MatchError("data not found"))
		Expect(imgURL).To(BeNil())
		mockAlbumRepo.AssertCalled(GinkgoT(), "Get", "not-found")
		mockMediaFileRepo.AssertCalled(GinkgoT(), "Get", "not-found")
		mockAlbumAgent.AssertNotCalled(GinkgoT(), "GetAlbumImages", mock.Anything, mock.Anything, mock.Anything)
	})

	It("returns context error if context is canceled before enqueue", func() {
		cctx, cancelCtx := context.WithCancel(ctx)
		mockAlbumRepo.On("Get", "album-1").Return(&model.Album{ID: "album-1", Name: "Album One", AlbumArtistID: "artist-1"}, nil).Once()
		cancelCtx()

		imgURL, err := provider.AlbumImage(cctx, "album-1")

		Expect(err).To(MatchError("context canceled"))
		Expect(imgURL).To(BeNil())
		mockAlbumRepo.AssertCalled(GinkgoT(), "Get", "album-1")
		mockAlbumAgent.AssertNotCalled(GinkgoT(), "GetAlbumImages", mock.Anything, "Album One", mock.Anything, mock.Anything)
	})

	It("derives album ID from MediaFile ID and still enqueues asynchronously", func() {
		mockAlbumRepo.On("Get", "mf-1").Return(nil, model.ErrNotFound).Once()
		mockMediaFileRepo.On("Get", "mf-1").Return(&model.MediaFile{ID: "mf-1", Title: "Track One", ArtistID: "artist-1", AlbumID: "album-1"}, nil).Once()
		mockAlbumRepo.On("Get", "album-1").Return(&model.Album{ID: "album-1", Name: "Album One", AlbumArtistID: "artist-1"}, nil).Once()

		imgURL, err := provider.AlbumImage(ctx, "mf-1")

		Expect(err).To(MatchError(model.ErrNotFound))
		Expect(imgURL).To(BeNil())
		mockAlbumRepo.AssertCalled(GinkgoT(), "Get", "mf-1")
		mockMediaFileRepo.AssertCalled(GinkgoT(), "Get", "mf-1")
		mockAlbumRepo.AssertCalled(GinkgoT(), "Get", "album-1")
		mockArtistRepo.AssertNotCalled(GinkgoT(), "Get", "artist-1")
		Eventually(func() bool { return albumImageAgentCalled(mockAlbumAgent) }).WithTimeout(2 * time.Second).Should(BeTrue())
	})

	It("returns ErrNotFound if deriving album ID fails", func() {
		mockAlbumRepo.On("Get", "mf-no-album").Return(nil, model.ErrNotFound).Once()
		mockMediaFileRepo.On("Get", "mf-no-album").Return(&model.MediaFile{ID: "mf-no-album", Title: "Track No Album", ArtistID: "artist-1", AlbumID: "not-found"}, nil).Once()
		mockAlbumRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Once()

		imgURL, err := provider.AlbumImage(ctx, "mf-no-album")

		Expect(err).To(MatchError("data not found"))
		Expect(imgURL).To(BeNil())
		mockAlbumRepo.AssertCalled(GinkgoT(), "Get", "mf-no-album")
		mockMediaFileRepo.AssertCalled(GinkgoT(), "Get", "mf-no-album")
		mockAlbumRepo.AssertCalled(GinkgoT(), "Get", "not-found")
		mockAlbumAgent.AssertNotCalled(GinkgoT(), "GetAlbumImages", mock.Anything, mock.Anything, mock.Anything)
	})

	It("returns cached URL and does not call agent when info is not expired", func() {
		cached := &model.Album{
			ID:                    "album-cached",
			Name:                  "Cached Album",
			LargeImageUrl:         "http://example.com/cached-large.jpg",
			ExternalInfoUpdatedAt: new(time.Now().Add(-1 * time.Minute)),
		}
		mockAlbumRepo.On("Get", "album-cached").Return(cached, nil).Once()
		expectedURL, _ := url.Parse("http://example.com/cached-large.jpg")

		imgURL, err := provider.AlbumImage(ctx, "album-cached")

		Expect(err).ToNot(HaveOccurred())
		Expect(imgURL).To(Equal(expectedURL))
		mockAlbumAgent.AssertNotCalled(GinkgoT(), "GetAlbumImages", mock.Anything, "Cached Album", mock.Anything, mock.Anything)
	})

	It("returns stale URL and enqueues refresh when info is expired", func() {
		conf.Server.DevAlbumInfoTimeToLive = 1 * time.Nanosecond
		stale := &model.Album{
			ID:                    "album-expired",
			Name:                  "Expired Album",
			LargeImageUrl:         "http://example.com/expired-large.jpg",
			ExternalInfoUpdatedAt: new(time.Now().Add(-1 * time.Hour)),
		}
		mockAlbumRepo.On("Get", "album-expired").Return(stale, nil).Once()
		mockAlbumAgent.On("GetAlbumImages", mock.Anything, "Expired Album", mock.Anything, mock.Anything).
			Return([]agents.ExternalImage{{URL: "http://example.com/expired-large.jpg", Size: 1000}}, nil).Maybe()
		expectedURL, _ := url.Parse("http://example.com/expired-large.jpg")

		var logBuf bytes.Buffer
		log.SetOutput(&logBuf)
		defer log.SetOutput(GinkgoWriter)
		log.SetLevel(log.LevelDebug)

		imgURL, err := provider.AlbumImage(ctx, "album-expired")

		Expect(err).ToNot(HaveOccurred())
		Expect(imgURL).To(Equal(expectedURL))
		Expect(logBuf.String()).To(ContainSubstring("Album image info expired, enqueuing background refresh"))
		Eventually(func() bool { return albumImageAgentCalled(mockAlbumAgent) }).WithTimeout(2 * time.Second).Should(BeTrue())
	})

	Context("Unicode handling in album names", func() {
		var albumWithEnDash *model.Album

		const (
			originalAlbumName   = "Raising Hell–Deluxe" // Album name with en dash
			normalizedAlbumName = "Raising Hell-Deluxe" // Normalized version with hyphen
		)

		BeforeEach(func() {
			albumWithEnDash = &model.Album{ID: "album-endash", Name: originalAlbumName, AlbumArtistID: "artist-1"}
			mockArtistRepo.Mock = mock.Mock{}
			mockAlbumRepo.Mock = mock.Mock{}
			mockAlbumRepo.On("Get", "album-endash").Return(albumWithEnDash, nil).Once()
			mockAlbumAgent.On("GetAlbumImages", mock.Anything, mock.AnythingOfType("string"), "", "").
				Return([]agents.ExternalImage{
					{URL: "http://example.com/album.jpg", Size: 1000},
				}, nil).Once()
		})

		When("DevPreserveUnicodeInExternalCalls is true", func() {
			BeforeEach(func() {
				conf.Server.DevPreserveUnicodeInExternalCalls = true
			})

			It("preserves Unicode characters in album names", func() {
				imgURL, err := provider.AlbumImage(ctx, "album-endash")
				Expect(err).To(MatchError(model.ErrNotFound))
				Expect(imgURL).To(BeNil())
				mockAlbumRepo.AssertCalled(GinkgoT(), "Get", "album-endash")
				Eventually(func() bool {
					return albumAgentCalledWithName(mockAlbumAgent, originalAlbumName)
				}).WithTimeout(2 * time.Second).Should(BeTrue())
			})
		})

		When("DevPreserveUnicodeInExternalCalls is false", func() {
			BeforeEach(func() {
				conf.Server.DevPreserveUnicodeInExternalCalls = false
			})

			It("normalizes Unicode characters", func() {
				imgURL, err := provider.AlbumImage(ctx, "album-endash")
				Expect(err).To(MatchError(model.ErrNotFound))
				Expect(imgURL).To(BeNil())
				mockAlbumRepo.AssertCalled(GinkgoT(), "Get", "album-endash")
				Eventually(func() bool {
					return albumAgentCalledWithName(mockAlbumAgent, normalizedAlbumName)
				}).WithTimeout(2 * time.Second).Should(BeTrue())
			})
		})
	})
})

func albumImageAgentCalled(agent *mockAlbumInfoAgent) bool {
	return agent.imagesCalled.Load()
}

func albumAgentCalledWithName(agent *mockAlbumInfoAgent, name string) bool {
	got, _ := agent.lastName.Load().(string)
	return got == name
}

type mockAlbumInfoAgent struct {
	mock.Mock
	agents.AlbumInfoRetriever
	agents.AlbumImageRetriever
	imagesCalled atomic.Bool
	lastName     atomic.Value
}

func newMockAlbumInfoAgent() *mockAlbumInfoAgent {
	m := new(mockAlbumInfoAgent)
	m.On("AgentName").Return("mockAlbum").Maybe()
	return m
}

func (m *mockAlbumInfoAgent) AgentName() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockAlbumInfoAgent) GetAlbumInfo(ctx context.Context, name, artist, mbid string) (*agents.AlbumInfo, error) {
	args := m.Called(ctx, name, artist, mbid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*agents.AlbumInfo), args.Error(1)
}

func (m *mockAlbumInfoAgent) GetAlbumImages(ctx context.Context, name, artist, mbid string) ([]agents.ExternalImage, error) {
	m.lastName.Store(name)
	m.imagesCalled.Store(true)
	args := m.Called(ctx, name, artist, mbid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]agents.ExternalImage), args.Error(1)
}

// Ensure mockAgent implements the interfaces
var _ agents.AlbumInfoRetriever = (*mockAlbumInfoAgent)(nil)
var _ agents.AlbumImageRetriever = (*mockAlbumInfoAgent)(nil)
