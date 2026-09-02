package librefm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

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

	const (
		userID     = "user-1"
		victimID   = "victim-user-id"
		attackerID = "attacker-user-id"
	)

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

	stubGetSessionOK := func(sessionKey string) {
		httpClient.Res = http.Response{
			Body:       io.NopCloser(bytes.NewBufferString(`{"session":{"name":"testuser","key":"` + sessionKey + `"}}`)),
			StatusCode: 200,
		}
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

	Describe("callback", func() {
		It("stores the session key under the user encoded in the signed token", func() {
			stubGetSessionOK("LEGIT_SESSION")
			linkToken, err := createLinkToken(victimID)
			Expect(err).ToNot(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/link/callback?uid="+linkToken+"&token=LIBREFM_TOKEN", nil)
			rec := httptest.NewRecorder()
			router.callback(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(storedSessionKey(victimID)).To(Equal("LEGIT_SESSION"))
		})

		It("rejects a raw (unsigned) uid value", func() {
			req := httptest.NewRequest(http.MethodGet, "/link/callback?uid="+victimID+"&token=LIBREFM_TOKEN", nil)
			rec := httptest.NewRecorder()
			router.callback(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(storedSessionKey(victimID)).To(BeEmpty())
			Expect(httpClient.SavedRequest).To(BeNil())
		})

		It("rejects an expired link token", func() {
			expiredToken, err := auth.EncodeToken(map[string]any{
				"uid":   victimID,
				"scope": linkTokenScope,
				"exp":   time.Now().Add(-1 * time.Minute).UTC().Unix(),
			})
			Expect(err).ToNot(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/link/callback?uid="+expiredToken+"&token=LIBREFM_TOKEN", nil)
			rec := httptest.NewRecorder()
			router.callback(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(storedSessionKey(victimID)).To(BeEmpty())
			Expect(httpClient.SavedRequest).To(BeNil())
		})

		It("writes only under the user encoded in the token", func() {
			stubGetSessionOK("ATTACKER_SESSION")
			attackerToken, err := createLinkToken(attackerID)
			Expect(err).ToNot(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/link/callback?uid="+attackerToken+"&token=LIBREFM_TOKEN&user="+victimID, nil)
			rec := httptest.NewRecorder()
			router.callback(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(storedSessionKey(attackerID)).To(Equal("ATTACKER_SESSION"))
			Expect(storedSessionKey(victimID)).To(BeEmpty())
		})

		It("returns 400 when uid is missing", func() {
			req := httptest.NewRequest(http.MethodGet, "/link/callback?token=LIBREFM_TOKEN", nil)
			rec := httptest.NewRecorder()
			router.callback(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})

		It("returns 400 when token is missing", func() {
			linkToken, err := createLinkToken(victimID)
			Expect(err).ToNot(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/link/callback?uid="+linkToken, nil)
			rec := httptest.NewRecorder()
			router.callback(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})
})
