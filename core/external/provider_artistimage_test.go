package external_test

import (
	"bytes"
	"context"
	"net/url"
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

var _ = Describe("Provider - ArtistImage", func() {
	var ds *tests.MockDataStore
	var provider Provider
	var mockArtistRepo *mockArtistRepo
	var mockAlbumRepo *mockAlbumRepo
	var mockMediaFileRepo *mockMediaFileRepo
	var mockImageAgent *mockArtistImageAgent
	var agentsCombined *mockAgents
	var ctx context.Context

	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
		conf.Server.Agents = "mockImage" // Configure only the mock agent
		ctx = GinkgoT().Context()

		mockArtistRepo = newMockArtistRepo()
		mockAlbumRepo = newMockAlbumRepo()
		mockMediaFileRepo = newMockMediaFileRepo()

		ds = &tests.MockDataStore{
			MockedArtist:    mockArtistRepo,
			MockedAlbum:     mockAlbumRepo,
			MockedMediaFile: mockMediaFileRepo,
		}

		mockImageAgent = newMockArtistImageAgent()

		// Use the mockAgents from helper, setting the specific agent
		agentsCombined = &mockAgents{
			imageAgent: mockImageAgent,
		}
		agentsCombined.On("GetArtistMBID", mock.Anything, mock.Anything, mock.Anything).Return("", nil).Maybe()
		agentsCombined.On("GetArtistBiography", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil).Maybe()
		agentsCombined.On("GetArtistURL", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil).Maybe()
		agentsCombined.On("GetSimilarArtists", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]agents.Artist{}, nil).Maybe()

		provider = NewProvider(ds, agentsCombined, matcher.New(ds))
		DeferCleanup(provider.Close)

		// Default mocks for successful Get calls
		mockArtistRepo.On("Get", "artist-1").Return(&model.Artist{ID: "artist-1", Name: "Artist One"}, nil).Maybe()
		mockAlbumRepo.On("Get", "album-1").Return(&model.Album{ID: "album-1", Name: "Album One", AlbumArtistID: "artist-1"}, nil).Maybe()
		mockMediaFileRepo.On("Get", "mf-1").Return(&model.MediaFile{ID: "mf-1", Title: "Track One", ArtistID: "artist-1"}, nil).Maybe()
		// Default mock for non-existent entities
		mockArtistRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Maybe()
		mockAlbumRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Maybe()
		mockMediaFileRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Maybe()

		// Default successful image agent response (consumed by the background queue)
		mockImageAgent.On("GetArtistImages", mock.Anything, "artist-1", "Artist One", "").
			Return([]agents.ExternalImage{
				{URL: "http://example.com/large.jpg", Size: 1000},
				{URL: "http://example.com/medium.jpg", Size: 500},
				{URL: "http://example.com/small.jpg", Size: 200},
			}, nil).Maybe()
	})

	AfterEach(func() {
		mockArtistRepo.AssertExpectations(GinkgoT())
		mockAlbumRepo.AssertExpectations(GinkgoT())
		mockMediaFileRepo.AssertExpectations(GinkgoT())
		mockImageAgent.AssertExpectations(GinkgoT())
	})

	It("enqueues a background fetch and returns ErrNotFound on cache miss", func() {
		var logBuf bytes.Buffer
		log.SetOutput(&logBuf)
		defer log.SetOutput(GinkgoWriter)
		log.SetLevel(log.LevelDebug)

		imgURL, err := provider.ArtistImage(ctx, "artist-1")

		Expect(err).To(MatchError(model.ErrNotFound))
		Expect(imgURL).To(BeNil())
		mockArtistRepo.AssertCalled(GinkgoT(), "Get", "artist-1")
		Expect(logBuf.String()).To(ContainSubstring("Artist image not cached, enqueuing background fetch"))
		Eventually(func() bool { return artistImageAgentCalled(mockImageAgent) }).WithTimeout(2 * time.Second).Should(BeTrue())
	})

	It("fetches images asynchronously after a cache miss", func() {
		_, err := provider.ArtistImage(ctx, "artist-1")
		Expect(err).To(MatchError(model.ErrNotFound))

		Eventually(func() bool {
			for _, call := range mockImageAgent.Calls {
				if call.Method == "GetArtistImages" {
					return true
				}
			}
			return false
		}).WithTimeout(2 * time.Second).Should(BeTrue())
	})

	It("returns ErrNotFound if the artist is not found in the DB", func() {
		imgURL, err := provider.ArtistImage(ctx, "not-found")

		Expect(err).To(MatchError(model.ErrNotFound))
		Expect(imgURL).To(BeNil())
		mockArtistRepo.AssertCalled(GinkgoT(), "Get", "not-found")
		mockImageAgent.AssertNotCalled(GinkgoT(), "GetArtistImages", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	It("returns context error if context is canceled before enqueue", func() {
		cctx, cancelCtx := context.WithCancel(context.Background())
		mockArtistRepo.Mock = mock.Mock{}
		mockArtistRepo.On("Get", "artist-1").Return(&model.Artist{ID: "artist-1", Name: "Artist One"}, nil).Run(func(args mock.Arguments) {
			cancelCtx()
		}).Once()

		imgURL, err := provider.ArtistImage(cctx, "artist-1")

		Expect(err).To(MatchError(context.Canceled))
		Expect(imgURL).To(BeNil())
		mockArtistRepo.AssertCalled(GinkgoT(), "Get", "artist-1")
	})

	It("derives artist ID from MediaFile ID and still enqueues asynchronously", func() {
		mockArtistRepo.On("Get", "mf-1").Return(nil, model.ErrNotFound).Once()

		imgURL, err := provider.ArtistImage(ctx, "mf-1")

		Expect(err).To(MatchError(model.ErrNotFound))
		Expect(imgURL).To(BeNil())
		mockArtistRepo.AssertCalled(GinkgoT(), "Get", "mf-1")
		mockMediaFileRepo.AssertCalled(GinkgoT(), "Get", "mf-1")
		mockArtistRepo.AssertCalled(GinkgoT(), "Get", "artist-1")
		Eventually(func() bool { return artistImageAgentCalled(mockImageAgent) }).WithTimeout(2 * time.Second).Should(BeTrue())
	})

	It("derives artist ID from Album ID and still enqueues asynchronously", func() {
		mockArtistRepo.On("Get", "album-1").Return(nil, model.ErrNotFound).Once()
		mockMediaFileRepo.On("Get", "album-1").Return(nil, model.ErrNotFound).Once()

		imgURL, err := provider.ArtistImage(ctx, "album-1")

		Expect(err).To(MatchError(model.ErrNotFound))
		Expect(imgURL).To(BeNil())
		mockArtistRepo.AssertCalled(GinkgoT(), "Get", "album-1")
		mockMediaFileRepo.AssertCalled(GinkgoT(), "Get", "album-1")
		mockAlbumRepo.AssertCalled(GinkgoT(), "Get", "album-1")
		mockArtistRepo.AssertCalled(GinkgoT(), "Get", "artist-1")
		Eventually(func() bool { return artistImageAgentCalled(mockImageAgent) }).WithTimeout(2 * time.Second).Should(BeTrue())
	})

	It("returns ErrNotFound if derived artist is not found", func() {
		mockArtistRepo.On("Get", "mf-bad-artist").Return(nil, model.ErrNotFound).Once()
		mockMediaFileRepo.On("Get", "mf-bad-artist").Return(&model.MediaFile{ID: "mf-bad-artist", ArtistID: "not-found"}, nil).Once()
		mockMediaFileRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Maybe()
		mockAlbumRepo.On("Get", "not-found").Return(nil, model.ErrNotFound).Maybe()

		imgURL, err := provider.ArtistImage(ctx, "mf-bad-artist")

		Expect(err).To(MatchError(model.ErrNotFound))
		Expect(imgURL).To(BeNil())
		mockArtistRepo.AssertCalled(GinkgoT(), "Get", "mf-bad-artist")
		mockMediaFileRepo.AssertCalled(GinkgoT(), "Get", "mf-bad-artist")
		mockArtistRepo.AssertCalled(GinkgoT(), "Get", "not-found")
		mockImageAgent.AssertNotCalled(GinkgoT(), "GetArtistImages", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	It("returns cached URL and does not call agent when info is not expired", func() {
		cachedArtist := &model.Artist{
			ID:                    "artist-cached",
			Name:                  "Cached Artist",
			LargeImageUrl:         "http://example.com/cached-large.jpg",
			ExternalInfoUpdatedAt: new(time.Now().Add(-1 * time.Minute)),
		}
		mockArtistRepo.On("Get", "artist-cached").Return(cachedArtist, nil).Maybe()
		expectedURL, _ := url.Parse("http://example.com/cached-large.jpg")

		var logBuf bytes.Buffer
		log.SetOutput(&logBuf)
		defer log.SetOutput(GinkgoWriter)
		log.SetLevel(log.LevelDebug)

		imgURL, err := provider.ArtistImage(ctx, "artist-cached")

		Expect(err).ToNot(HaveOccurred())
		Expect(imgURL).To(Equal(expectedURL))
		mockImageAgent.AssertNotCalled(GinkgoT(), "GetArtistImages", mock.Anything, "artist-cached", mock.Anything, mock.Anything)
		Expect(logBuf.String()).ToNot(ContainSubstring("Artist image info expired, enqueuing background refresh"))
	})

	It("returns stale URL and enqueues refresh when info is expired", func() {
		conf.Server.DevArtistInfoTimeToLive = 1 * time.Nanosecond
		staleArtist := &model.Artist{
			ID:                    "artist-expired",
			Name:                  "Expired Artist",
			LargeImageUrl:         "http://example.com/expired-large.jpg",
			ExternalInfoUpdatedAt: new(time.Now().Add(-1 * time.Hour)),
		}
		mockArtistRepo.On("Get", "artist-expired").Return(staleArtist, nil).Maybe()
		mockImageAgent.On("GetArtistImages", mock.Anything, "artist-expired", mock.Anything, mock.Anything).
			Return([]agents.ExternalImage{{URL: "http://example.com/expired-large.jpg", Size: 1000}}, nil).Maybe()
		expectedURL, _ := url.Parse("http://example.com/expired-large.jpg")

		var logBuf bytes.Buffer
		log.SetOutput(&logBuf)
		defer log.SetOutput(GinkgoWriter)
		log.SetLevel(log.LevelDebug)

		imgURL, err := provider.ArtistImage(ctx, "artist-expired")

		Expect(err).ToNot(HaveOccurred())
		Expect(imgURL).To(Equal(expectedURL))
		Expect(logBuf.String()).To(ContainSubstring("Artist image info expired, enqueuing background refresh"))
		Eventually(func() bool { return artistImageAgentCalled(mockImageAgent) }).WithTimeout(2 * time.Second).Should(BeTrue())
	})

	Context("Unicode handling in artist names", func() {
		var artistWithEnDash *model.Artist

		const (
			originalArtistName   = "Run–D.M.C." // Artist name with en dash
			normalizedArtistName = "Run-D.M.C." // Normalized version with hyphen
		)

		BeforeEach(func() {
			artistWithEnDash = &model.Artist{ID: "artist-endash", Name: originalArtistName}
			mockArtistRepo.Mock = mock.Mock{}
			mockArtistRepo.On("Get", "artist-endash").Return(artistWithEnDash, nil).Once()
			mockImageAgent.On("GetArtistImages", mock.Anything, "artist-endash", mock.AnythingOfType("string"), "").
				Return([]agents.ExternalImage{
					{URL: "http://example.com/rundmc.jpg", Size: 1000},
				}, nil).Once()
		})

		When("DevPreserveUnicodeInExternalCalls is true", func() {
			BeforeEach(func() {
				conf.Server.DevPreserveUnicodeInExternalCalls = true
			})
			It("preserves Unicode characters in artist names", func() {
				imgURL, err := provider.ArtistImage(ctx, "artist-endash")
				Expect(err).To(MatchError(model.ErrNotFound))
				Expect(imgURL).To(BeNil())
				mockArtistRepo.AssertCalled(GinkgoT(), "Get", "artist-endash")
				Eventually(func() bool {
					return agentCalledWithName(mockImageAgent, originalArtistName)
				}).WithTimeout(2 * time.Second).Should(BeTrue())
			})
		})

		When("DevPreserveUnicodeInExternalCalls is false", func() {
			BeforeEach(func() {
				conf.Server.DevPreserveUnicodeInExternalCalls = false
			})

			It("normalizes Unicode characters", func() {
				imgURL, err := provider.ArtistImage(ctx, "artist-endash")
				Expect(err).To(MatchError(model.ErrNotFound))
				Expect(imgURL).To(BeNil())
				mockArtistRepo.AssertCalled(GinkgoT(), "Get", "artist-endash")
				Eventually(func() bool {
					return agentCalledWithName(mockImageAgent, normalizedArtistName)
				}).WithTimeout(2 * time.Second).Should(BeTrue())
			})
		})
	})
})

func artistImageAgentCalled(agent *mockArtistImageAgent) bool {
	for _, call := range agent.Calls {
		if call.Method == "GetArtistImages" {
			return true
		}
	}
	return false
}

func agentCalledWithName(agent *mockArtistImageAgent, name string) bool {
	for _, call := range agent.Calls {
		if call.Method != "GetArtistImages" || len(call.Arguments) < 3 {
			continue
		}
		got, _ := call.Arguments[2].(string)
		if got == name {
			return true
		}
	}
	return false
}

// mockArtistImageAgent implementation using testify/mock
// This remains local as it's specific to testing the ArtistImage functionality
type mockArtistImageAgent struct {
	mock.Mock
	agents.ArtistImageRetriever // Embed interface
}

// Constructor for the mock agent
func newMockArtistImageAgent() *mockArtistImageAgent {
	mock := new(mockArtistImageAgent)
	// Set default AgentName if needed, although usually called via mockAgents
	mock.On("AgentName").Return("mockImage").Maybe()
	return mock
}

func (m *mockArtistImageAgent) AgentName() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockArtistImageAgent) GetArtistImages(ctx context.Context, id, artistName, mbid string) ([]agents.ExternalImage, error) {
	args := m.Called(ctx, id, artistName, mbid)
	// Need careful type assertion for potentially nil slice
	var res []agents.ExternalImage
	if args.Get(0) != nil {
		res = args.Get(0).([]agents.ExternalImage)
	}
	return res, args.Error(1)
}

// Ensure mockAgent implements the interface
var _ agents.ArtistImageRetriever = (*mockArtistImageAgent)(nil)
