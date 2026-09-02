package librefm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/navidrome/navidrome/core/agents"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("auth_router", func() {
	var (
		ds         *tests.MockDataStore
		userProps  *tests.MockedUserPropsRepo
		httpClient *tests.FakeHttpClient
		router     *Router
	)

	const userID = "user-1"

	BeforeEach(func() {
		userProps = &tests.MockedUserPropsRepo{}
		ds = &tests.MockDataStore{
			MockedProperty:  &tests.MockedPropertyRepo{},
			MockedUserProps: userProps,
		}
		auth.Init(ds)

		httpClient = &tests.FakeHttpClient{}
		router = &Router{
			ds:          ds,
			apiKey:      "API_KEY",
			secret:      "SECRET",
			authURL:     "https://libre.fm/api/auth/",
			sessionKeys: &agents.SessionKeys{DataStore: ds, KeyName: sessionKeyProperty},
		}
		router.client = newClient(router.apiKey, router.secret, "https://libre.fm/2.0/", httpClient)
		router.Handler = router.routes()
	})

	storedSessionKey := func(uid string) string {
		key, _ := userProps.Get(uid, sessionKeyProperty)
		return key
	}

	Describe("getLinkStatus", func() {
		It("returns apiKey, authUrl and linkToken", func() {
			req := httptest.NewRequest(http.MethodGet, "/link", nil)
			ctx := request.WithUser(req.Context(), model.User{ID: userID})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			router.getLinkStatus(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			var body map[string]any
			Expect(json.Unmarshal(rec.Body.Bytes(), &body)).To(Succeed())
			Expect(body["apiKey"]).To(Equal("API_KEY"))
			Expect(body["authUrl"]).To(Equal("https://libre.fm/api/auth/"))
			Expect(body["status"]).To(Equal(false))
			token, ok := body["linkToken"].(string)
			Expect(ok).To(BeTrue())
			Expect(token).ToNot(BeEmpty())
		})
	})

	Describe("link", func() {
		It("validates and stores a session key", func() {
			httpClient.Res = http.Response{
				Body:       io.NopCloser(bytes.NewBufferString(`{"user":{"name":"testuser"}}`)),
				StatusCode: 200,
			}

			req := httptest.NewRequest(http.MethodPut, "/link", strings.NewReader(`{"sessionKey":"SK-123"}`))
			ctx := request.WithUser(req.Context(), model.User{ID: userID})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			router.link(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(storedSessionKey(userID)).To(Equal("SK-123"))
			var body map[string]any
			Expect(json.Unmarshal(rec.Body.Bytes(), &body)).To(Succeed())
			Expect(body["user"]).To(Equal("testuser"))
		})

		It("rejects an invalid session key", func() {
			httpClient.Res = http.Response{
				Body:       io.NopCloser(bytes.NewBufferString(`{"error":9,"message":"Invalid session key"}`)),
				StatusCode: 400,
			}

			req := httptest.NewRequest(http.MethodPut, "/link", strings.NewReader(`{"sessionKey":"bad"}`))
			ctx := request.WithUser(req.Context(), model.User{ID: userID})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			router.link(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(storedSessionKey(userID)).To(BeEmpty())
		})
	})
})
