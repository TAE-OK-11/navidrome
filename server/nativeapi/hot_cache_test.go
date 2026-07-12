package nativeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/core/stream/hotcache"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hot Cache administrator API", func() {
	var handler http.Handler

	BeforeEach(func() {
		router := chi.NewRouter()
		api := &Router{}
		router.With(adminOnlyMiddleware).Group(func(r chi.Router) {
			api.addHotCacheRoutes(r)
		})
		handler = router
	})

	requestAs := func(path string, user model.User) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(request.WithUser(context.Background(), user))
		handler.ServeHTTP(recorder, req)
		return recorder
	}

	It("rejects regular users", func() {
		response := requestAs("/admin/hot-cache/status", model.User{ID: "user"})
		Expect(response.Code).To(Equal(http.StatusForbidden))
	})

	It("returns a stable status snapshot to administrators", func() {
		response := requestAs("/admin/hot-cache/status", model.User{ID: "admin", IsAdmin: true})
		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Header().Get("Cache-Control")).To(Equal("no-store"))
		Expect(response.Body.String()).To(ContainSubstring(`"enabled":false`))
	})

	It("returns one normalized dashboard response", func() {
		response := requestAs("/admin/hot-cache/dashboard", model.User{ID: "admin", IsAdmin: true})
		Expect(response.Code).To(Equal(http.StatusOK))
		var body map[string]any
		Expect(json.Unmarshal(response.Body.Bytes(), &body)).To(Succeed())
		for _, key := range []string{"sessions", "queue", "formats", "events", "errors", "artwork"} {
			Expect(body[key]).ToNot(BeNil(), key)
		}
	})

	It("requires an independent purge confirmation", func() {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin/hot-cache/purge", nil)
		req = req.WithContext(request.WithUser(context.Background(), model.User{ID: "admin", IsAdmin: true}))
		handler.ServeHTTP(recorder, req)
		Expect(recorder.Code).To(Equal(http.StatusPreconditionFailed))
	})

	It("searches cache candidates and includes their current state", func() {
		repo := tests.CreateMockMediaFileRepo()
		repo.SetData(model.MediaFiles{
			{ID: "cached", Title: "Cached Song", Artist: "Artist", Album: "Album", Size: 123, Suffix: "flac"},
			{ID: "available", Title: "Available Song", Artist: "Artist", Album: "Album", Size: 456, Suffix: "m4a"},
		})
		api := &Router{ds: &tests.MockDataStore{MockedMediaFile: repo}}
		manager := &hotCacheManagerStub{states: map[string]string{"cached": "cached"}}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/candidates?search=song&limit=25", nil)
		api.hotCacheCandidates(manager).ServeHTTP(recorder, request)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		var page hotCacheCandidatePage
		Expect(json.Unmarshal(recorder.Body.Bytes(), &page)).To(Succeed())
		Expect(page.Items).To(HaveLen(2))
		Expect(page.Items[0].CacheState).To(Equal("available"))
		Expect(page.Items[1].CacheState).To(Equal("cached"))
	})

	It("queues selected tracks once in a single request", func() {
		repo := tests.CreateMockMediaFileRepo()
		repo.SetData(model.MediaFiles{{ID: "song-1", AlbumID: "album-1"}, {ID: "song-2", AlbumID: "album-2"}})
		warmer := &hotCacheWarmerStub{}
		api := &Router{ds: &tests.MockDataStore{MockedMediaFile: repo}, cacheWarmer: warmer}
		manager := &hotCacheManagerStub{}
		body := bytes.NewBufferString(`{"mediaIds":["song-1","song-1","song-2"]}`)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/promote", body)
		api.hotCachePromoteMany(manager).ServeHTTP(recorder, request)

		Expect(recorder.Code).To(Equal(http.StatusAccepted))
		Expect(manager.promoted).To(Equal([]string{"song-1", "song-2"}))
		var result hotCachePromoteResult
		Expect(json.Unmarshal(recorder.Body.Bytes(), &result)).To(Succeed())
		Expect(result.Accepted).To(Equal([]string{"song-1", "song-2"}))
		Expect(result.Rejected).ToNot(BeNil())
		Expect(warmer.ids).To(ConsistOf("album-1", "album-2"))
	})

	It("rejects trailing JSON in a promotion request", func() {
		api := &Router{ds: &tests.MockDataStore{MockedMediaFile: tests.CreateMockMediaFileRepo()}}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/promote", bytes.NewBufferString(`{"mediaIds":["song-1"]}{}`))
		api.hotCachePromoteMany(&hotCacheManagerStub{}).ServeHTTP(recorder, request)

		Expect(recorder.Code).To(Equal(http.StatusBadRequest))
	})
})

type hotCacheManagerStub struct {
	hotcache.Manager
	states   map[string]string
	promoted []string
}

type hotCacheWarmerStub struct {
	ids []string
}

func (w *hotCacheWarmerStub) PreCache(id model.ArtworkID) {
	w.ids = append(w.ids, id.ID)
}

func (w *hotCacheWarmerStub) PreCacheOnDemand(id model.ArtworkID) {
	w.ids = append(w.ids, id.ID)
}

func (m *hotCacheManagerStub) MediaStates([]string) map[string]string {
	result := make(map[string]string, len(m.states))
	maps.Copy(result, m.states)
	return result
}

func (m *hotCacheManagerStub) Promote(_ context.Context, mediaFile *model.MediaFile) error {
	m.promoted = append(m.promoted, mediaFile.ID)
	return nil
}
