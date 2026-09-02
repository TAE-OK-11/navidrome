package apikeys_test

import (
	"context"
	"crypto/sha256"
	"cmp"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/apikeys"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	"github.com/navidrome/navidrome/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("API keys service", func() {
	var (
		ds      *tests.MockDataStore
		service *apikeys.Service
		user    model.User
	)

	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
		ds = &tests.MockDataStore{}
		auth.Init(ds)
		_ = ds.Property(context.Background()).Put(consts.JWTSecretKey, mustEncrypt("test-pepper-with-enough-length"))
		user = model.User{ID: "user-1", UserName: "demo"}
		_ = ds.User(context.Background()).Put(&user)
		service = apikeys.New(ds)
	})

	It("creates and authenticates a dedicated API key", func() {
		key, token, err := service.Create(context.Background(), user.ID, apikeys.CreateInput{Name: "mobile"})
		Expect(err).ToNot(HaveOccurred())
		Expect(key.Name).To(Equal("mobile"))
		Expect(token).To(HavePrefix("nd_"))

		authenticated, err := service.Authenticate(context.Background(), token)
		Expect(err).ToNot(HaveOccurred())
		Expect(authenticated.ID).To(Equal(user.ID))
	})

	It("still accepts login JWT tokens", func() {
		jwt, err := auth.CreateToken(&user)
		Expect(err).ToNot(HaveOccurred())

		authenticated, err := service.Authenticate(context.Background(), jwt)
		Expect(err).ToNot(HaveOccurred())
		Expect(authenticated.UserName).To(Equal(user.UserName))
	})
})

func mustEncrypt(value string) string {
	enc, err := utils.Encrypt(context.Background(), encryptionKey(), value)
	if err != nil {
		panic(err)
	}
	return enc
}

func encryptionKey() []byte {
	key := cmp.Or(conf.Server.PasswordEncryptionKey, consts.DefaultEncryptionKey)
	sum := sha256.Sum256([]byte(key))
	return sum[:]
}
