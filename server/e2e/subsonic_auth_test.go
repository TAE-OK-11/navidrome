package e2e

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/navidrome/navidrome/core/apikeys"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subsonic API key authentication", func() {
	BeforeEach(func() {
		setupTestDB()
	})

	It("authenticates with a dedicated API key", func() {
		service := apikeys.New(ds)
		_, token, err := service.Create(context.Background(), adminUser.ID, apikeys.CreateInput{Name: "e2e"})
		Expect(err).ToNot(HaveOccurred())

		w := httptest.NewRecorder()
		q := url.Values{}
		q.Add("apiKey", token)
		q.Add("v", "1.16.1")
		q.Add("c", "test-client")
		q.Add("f", "json")
		r := httptest.NewRequest("GET", "/tokenInfo?"+q.Encode(), nil)
		router.ServeHTTP(w, r)

		resp := parseJSONResponse(w)
		Expect(resp.Status).To(Equal(responses.StatusOK))
		Expect(resp.TokenInfo.Username).To(Equal(adminUser.UserName))
	})

	It("authenticates with login JWT apiKey", func() {
		token, err := auth.CreateToken(&adminUser)
		Expect(err).ToNot(HaveOccurred())

		w := httptest.NewRecorder()
		q := url.Values{}
		q.Add("apiKey", token)
		q.Add("v", "1.16.1")
		q.Add("c", "test-client")
		q.Add("f", "json")
		r := httptest.NewRequest("GET", "/tokenInfo?"+q.Encode(), nil)
		router.ServeHTTP(w, r)

		resp := parseJSONResponse(w)
		Expect(resp.Status).To(Equal(responses.StatusOK))
		Expect(resp.TokenInfo).ToNot(BeNil())
		Expect(resp.TokenInfo.Username).To(Equal(adminUser.UserName))
	})

	It("rejects conflicting apiKey and username parameters", func() {
		token, err := auth.CreateToken(&adminUser)
		Expect(err).ToNot(HaveOccurred())

		w := httptest.NewRecorder()
		q := url.Values{}
		q.Add("apiKey", token)
		q.Add("u", adminUser.UserName)
		q.Add("v", "1.16.1")
		q.Add("c", "test-client")
		q.Add("f", "json")
		r := httptest.NewRequest("GET", "/ping?"+q.Encode(), nil)
		router.ServeHTTP(w, r)

		Expect(w.Body.String()).To(ContainSubstring(`"code":43`))
	})

	It("rejects invalid apiKey", func() {
		w := httptest.NewRecorder()
		q := url.Values{}
		q.Add("apiKey", "not-a-valid-token")
		q.Add("v", "1.16.1")
		q.Add("c", "test-client")
		q.Add("f", "json")
		r := httptest.NewRequest("GET", "/tokenInfo?"+q.Encode(), nil)
		router.ServeHTTP(w, r)

		Expect(w.Body.String()).To(ContainSubstring(`"code":44`))
		Expect(w.Body.String()).To(ContainSubstring(`helpUrl`))
	})
})

var _ = Describe("Subsonic formPost", func() {
	BeforeEach(func() {
		setupTestDB()
	})

	It("accepts application/x-www-form-urlencoded POST bodies", func() {
		w := httptest.NewRecorder()
		body := strings.NewReader("u=" + adminUser.UserName + "&p=password&v=1.16.1&c=test-client&f=json")
		r := httptest.NewRequest("POST", "/ping", body)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(w, r)

		resp := parseJSONResponse(w)
		Expect(resp.Status).To(Equal(responses.StatusOK))
	})
})
