package subsonic

import (
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("request pipeline routes", func() {
	It("exposes system, catalog, and media endpoints on one router", func() {
		api := &Router{ds: &tests.MockDataStore{}}
		handler := api.routes().(chi.Router)

		found := map[string]bool{}
		_ = chi.Walk(handler, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			found[route] = true
			return nil
		})

		Expect(found["/ping"]).To(BeTrue())
		Expect(found["/getAlbum"]).To(BeTrue())
		Expect(found["/stream"]).To(BeTrue())
		Expect(found["/download"]).To(BeTrue())
		Expect(found["/getCoverArt"]).To(BeTrue())
	})

	It("keeps ping reachable without a registered player", func() {
		api := &Router{ds: &tests.MockDataStore{}}
		req := httptest.NewRequest(http.MethodGet, "/ping?u=user&p=pass&v=1.16.1&c=test", nil)
		rec := httptest.NewRecorder()
		api.routes().ServeHTTP(rec, req)
		Expect(rec.Code).NotTo(Equal(http.StatusNotFound))
	})
})
