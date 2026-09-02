package subsonic

import (
	"net/http/httptest"

	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("tokenInfo", func() {
	var router *Router
	var ds model.DataStore
	var user model.User

	BeforeEach(func() {
		ds = &tests.MockDataStore{}
		auth.Init(ds)
		user = model.User{ID: "user-1", UserName: "demo"}
		router = New(ds, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	})

	It("returns the authenticated username", func() {
		r := httptest.NewRequest("GET", "/rest/tokenInfo", nil)
		r = r.WithContext(request.WithUser(r.Context(), user))

		resp, err := router.GetTokenInfo(r)

		Expect(err).ToNot(HaveOccurred())
		Expect(resp.TokenInfo).ToNot(BeNil())
		Expect(resp.TokenInfo.Username).To(Equal("demo"))
	})
})
