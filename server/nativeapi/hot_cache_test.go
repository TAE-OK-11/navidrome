package nativeapi

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
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

	It("requires an independent purge confirmation", func() {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin/hot-cache/purge", nil)
		req = req.WithContext(request.WithUser(context.Background(), model.User{ID: "admin", IsAdmin: true}))
		handler.ServeHTTP(recorder, req)
		Expect(recorder.Code).To(Equal(http.StatusPreconditionFailed))
	})
})
