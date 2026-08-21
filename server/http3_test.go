package server

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HTTP/3 support", func() {
	It("clears stale HTTP/3 advertisements when the companion is unavailable", func() {
		handler := clearHTTP3Advertisement(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.test/ping", nil))

		Expect(recorder.Code).To(Equal(http.StatusNoContent))
		Expect(recorder.Header().Get("Alt-Svc")).To(Equal("clear"))
	})
})
