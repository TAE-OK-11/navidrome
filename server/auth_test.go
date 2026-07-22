package server

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auth", func() {
	Describe("User login", func() {
		var ds model.DataStore
		var req *http.Request
		var resp *httptest.ResponseRecorder

		BeforeEach(func() {
			ds = &tests.MockDataStore{}
			auth.Init(ds)
		})

		Describe("createAdmin", func() {
			var createdAt time.Time
			BeforeEach(func() {
				req = httptest.NewRequest("POST", "/createAdmin", strings.NewReader(`{"username":"johndoe", "password":"secret"}`))
				resp = httptest.NewRecorder()
				createdAt = time.Now()
				createAdmin(ds)(resp, req)
			})

			It("creates an admin user with the specified password", func() {
				usr := ds.User(context.Background())
				u, err := usr.FindByUsername("johndoe")
				Expect(err).To(BeNil())
				Expect(u.Password).ToNot(BeEmpty())
				Expect(u.IsAdmin).To(BeTrue())
				Expect(*u.LastLoginAt).To(BeTemporally(">=", createdAt, time.Second))
			})

			It("returns the expected payload", func() {
				Expect(resp.Code).To(Equal(http.StatusOK))
				var parsed map[string]any
				Expect(json.Unmarshal(resp.Body.Bytes(), &parsed)).To(BeNil())
				Expect(parsed["isAdmin"]).To(Equal(true))
				Expect(parsed["username"]).To(Equal("johndoe"))
				Expect(parsed["name"]).To(Equal("Johndoe"))
				Expect(parsed["id"]).ToNot(BeEmpty())
				Expect(parsed["token"]).ToNot(BeEmpty())
			})
		})

		Describe("bootstrap security controls", func() {
			It("rejects oversized login and create-admin credential bodies before reading them fully", func() {
				for path, handler := range map[string]func(model.DataStore) func(http.ResponseWriter, *http.Request){
					"/login":       login,
					"/createAdmin": createAdmin,
				} {
					req := httptest.NewRequest("POST", path, strings.NewReader(`{"username":"`+strings.Repeat("a", maxAuthRequestBodySize)+`","password":"secret"}`))
					resp := httptest.NewRecorder()

					handler(&tests.MockDataStore{})(resp, req)

					Expect(resp.Code).To(Equal(http.StatusRequestEntityTooLarge), path)
				}
			})

			It("returns the repository error when creating the initial admin fails", func() {
				repo := tests.CreateMockUserRepo()
				repo.Error = errors.New("put failed")
				ds := &tests.MockDataStore{MockedUser: repo}

				err := createAdminUser(context.Background(), ds, "johndoe", "secret")

				Expect(err).To(MatchError("put failed"))
			})

			It("serializes concurrent initial-admin creation", func() {
				repo := newSynchronizedUserRepo()
				base := &tests.MockDataStore{MockedUser: repo}
				ds := &serializedImmediateDataStore{DataStore: base}
				start := make(chan struct{})
				codes := make(chan int, 2)
				var wg sync.WaitGroup

				for i, username := range []string{"legitimate", "racer"} {
					wg.Add(1)
					go func(i int, username string) {
						defer wg.Done()
						<-start
						req := httptest.NewRequest("POST", "/createAdmin", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, username, fmt.Sprintf("secret-%d", i))))
						resp := httptest.NewRecorder()
						createAdmin(ds)(resp, req)
						codes <- resp.Code
					}(i, username)
				}
				close(start)
				wg.Wait()
				close(codes)

				var gotCodes []int
				for code := range codes {
					gotCodes = append(gotCodes, code)
				}
				Expect(gotCodes).To(ConsistOf(http.StatusOK, http.StatusForbidden))
				Expect(repo.userCount()).To(Equal(1))
				Expect(repo.adminCount()).To(Equal(1))
				Expect(ds.immediateCalls).To(Equal(2))
			})
		})

		Describe("Login from HTTP headers", func() {
			const (
				trustedIpv4   = "192.168.0.42"
				untrustedIpv4 = "8.8.8.8"
				trustedIpv6   = "2001:4860:4860:1234:5678:0000:4242:8888"
				untrustedIpv6 = "5005:0:3003"
			)

			fs := os.DirFS("tests/fixtures")

			BeforeEach(func() {
				usr := ds.User(context.Background())
				_ = usr.Put(&model.User{ID: "111", UserName: "janedoe", NewPassword: "abc123", Name: "Jane", IsAdmin: false})
				req = httptest.NewRequest("GET", "/index.html", nil)
				req.Header.Add("Remote-User", "janedoe")
				resp = httptest.NewRecorder()
				conf.Server.UILoginBackgroundURL = ""
				conf.Server.ExtAuth.TrustedSources = "192.168.0.0/16,2001:4860:4860::/48"
			})

			It("sets auth data if IPv4 matches whitelist", func() {
				req = req.WithContext(request.WithReverseProxyIp(req.Context(), trustedIpv4))
				serveIndex(ds, fs, nil)(resp, req)

				config := extractAppConfig(resp.Body.String())
				parsed := config["auth"].(map[string]any)

				Expect(parsed["id"]).To(Equal("111"))
			})

			It("sets no auth data if IPv4 does not match whitelist", func() {
				req = req.WithContext(request.WithReverseProxyIp(req.Context(), untrustedIpv4))
				serveIndex(ds, fs, nil)(resp, req)

				config := extractAppConfig(resp.Body.String())
				Expect(config["auth"]).To(BeNil())
			})

			It("sets auth data if IPv6 matches whitelist", func() {
				req = req.WithContext(request.WithReverseProxyIp(req.Context(), trustedIpv6))
				serveIndex(ds, fs, nil)(resp, req)

				config := extractAppConfig(resp.Body.String())
				parsed := config["auth"].(map[string]any)

				Expect(parsed["id"]).To(Equal("111"))
			})

			It("sets no auth data if IPv6 does not match whitelist", func() {
				req = req.WithContext(request.WithReverseProxyIp(req.Context(), untrustedIpv6))
				serveIndex(ds, fs, nil)(resp, req)

				config := extractAppConfig(resp.Body.String())
				Expect(config["auth"]).To(BeNil())
			})

			It("creates user and sets auth data if user does not exist", func() {
				newUser := "NEW_USER_" + id.NewRandom()

				req = req.WithContext(request.WithReverseProxyIp(req.Context(), trustedIpv4))
				req.Header.Set("Remote-User", newUser)
				serveIndex(ds, fs, nil)(resp, req)

				config := extractAppConfig(resp.Body.String())
				parsed := config["auth"].(map[string]any)

				Expect(parsed["username"]).To(Equal(newUser))
			})

			It("sets auth data if user exists", func() {
				req = req.WithContext(request.WithReverseProxyIp(req.Context(), trustedIpv4))
				serveIndex(ds, fs, nil)(resp, req)

				config := extractAppConfig(resp.Body.String())
				parsed := config["auth"].(map[string]any)

				Expect(parsed["id"]).To(Equal("111"))
				Expect(parsed["isAdmin"]).To(BeFalse())
				Expect(parsed["name"]).To(Equal("Jane"))
				Expect(parsed["username"]).To(Equal("janedoe"))
				Expect(parsed["subsonicSalt"]).ToNot(BeEmpty())
				Expect(parsed["subsonicToken"]).ToNot(BeEmpty())
				salt := parsed["subsonicSalt"].(string)
				token := fmt.Sprintf("%x", md5.Sum([]byte("abc123"+salt)))
				Expect(parsed["subsonicToken"]).To(Equal(token))

				// Request Header authentication should not generate a JWT token
				Expect(parsed).ToNot(HaveKey("token"))
			})

			It("does not set auth data when listening on unix socket without whitelist", func() {
				conf.Server.Address = "unix:/tmp/navidrome-test"
				conf.Server.ExtAuth.TrustedSources = ""

				// No ReverseProxyIp in request context
				serveIndex(ds, fs, nil)(resp, req)

				config := extractAppConfig(resp.Body.String())
				Expect(config["auth"]).To(BeNil())
			})

			It("does not set auth data when listening on unix socket with incorrect whitelist", func() {
				conf.Server.Address = "unix:/tmp/navidrome-test"

				req = req.WithContext(request.WithReverseProxyIp(req.Context(), "@"))
				serveIndex(ds, fs, nil)(resp, req)

				config := extractAppConfig(resp.Body.String())
				Expect(config["auth"]).To(BeNil())
			})

			It("sets auth data when listening on unix socket with correct whitelist", func() {
				conf.Server.Address = "unix:/tmp/navidrome-test"
				conf.Server.ExtAuth.TrustedSources = conf.Server.ExtAuth.TrustedSources + ",@"

				req = req.WithContext(request.WithReverseProxyIp(req.Context(), "@"))
				serveIndex(ds, fs, nil)(resp, req)

				config := extractAppConfig(resp.Body.String())
				parsed := config["auth"].(map[string]any)

				Expect(parsed["id"]).To(Equal("111"))
			})
		})

		Describe("login", func() {
			BeforeEach(func() {
				req = httptest.NewRequest("POST", "/login", strings.NewReader(`{"username":"janedoe", "password":"abc123"}`))
				resp = httptest.NewRecorder()
			})

			It("fails if user does not exist", func() {
				login(ds)(resp, req)
				Expect(resp.Code).To(Equal(http.StatusUnauthorized))
			})

			It("logs in successfully if user exists", func() {
				usr := ds.User(context.Background())
				_ = usr.Put(&model.User{ID: "111", UserName: "janedoe", NewPassword: "abc123", Name: "Jane", IsAdmin: false})

				login(ds)(resp, req)
				Expect(resp.Code).To(Equal(http.StatusOK))

				var parsed map[string]any
				Expect(json.Unmarshal(resp.Body.Bytes(), &parsed)).To(BeNil())
				Expect(parsed["isAdmin"]).To(Equal(false))
				Expect(parsed["username"]).To(Equal("janedoe"))
				Expect(parsed["name"]).To(Equal("Jane"))
				Expect(parsed["id"]).ToNot(BeEmpty())
				Expect(parsed["token"]).ToNot(BeEmpty())
			})
		})
	})

	Describe("tokenFromHeader", func() {
		It("returns the token when the Authorization header is set correctly", func() {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(consts.UIAuthorizationHeader, "Bearer testtoken")

			token := tokenFromHeader(req)
			Expect(token).To(Equal("testtoken"))
		})

		It("returns an empty string when the Authorization header is not set", func() {
			req := httptest.NewRequest("GET", "/", nil)

			token := tokenFromHeader(req)
			Expect(token).To(BeEmpty())
		})

		It("returns an empty string when the Authorization header is not a Bearer token", func() {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(consts.UIAuthorizationHeader, "Basic testtoken")

			token := tokenFromHeader(req)
			Expect(token).To(BeEmpty())
		})

		It("returns an empty string when the Bearer token is too short", func() {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(consts.UIAuthorizationHeader, "Bearer")

			token := tokenFromHeader(req)
			Expect(token).To(BeEmpty())
		})
	})

	Describe("validateIPAgainstList", func() {
		Context("when provided with empty inputs", func() {
			It("should return false", func() {
				Expect(validateIPAgainstList("", "")).To(BeFalse())
				Expect(validateIPAgainstList("192.168.1.1", "")).To(BeFalse())
				Expect(validateIPAgainstList("", "192.168.0.0/16")).To(BeFalse())
			})
		})

		Context("when provided with invalid IP inputs", func() {
			It("should return false", func() {
				Expect(validateIPAgainstList("invalidIP", "192.168.0.0/16")).To(BeFalse())
			})
		})

		Context("when provided with valid inputs", func() {
			It("should return true when IP is in the list", func() {
				Expect(validateIPAgainstList("192.168.1.1", "192.168.0.0/16,10.0.0.0/8")).To(BeTrue())
				Expect(validateIPAgainstList("10.0.0.1", "192.168.0.0/16,10.0.0.0/8")).To(BeTrue())
			})

			It("should return false when IP is not in the list", func() {
				Expect(validateIPAgainstList("172.16.0.1", "192.168.0.0/16,10.0.0.0/8")).To(BeFalse())
			})
		})

		Context("when provided with invalid CIDR notation in the list", func() {
			It("should ignore invalid CIDR and return the correct result", func() {
				Expect(validateIPAgainstList("192.168.1.1", "192.168.0.0/16,invalidCIDR")).To(BeTrue())
				Expect(validateIPAgainstList("10.0.0.1", "invalidCIDR,10.0.0.0/8")).To(BeTrue())
				Expect(validateIPAgainstList("172.16.0.1", "192.168.0.0/16,invalidCIDR")).To(BeFalse())
			})
		})

		Context("when provided with IP:port format", func() {
			It("should handle IP:port format correctly", func() {
				Expect(validateIPAgainstList("192.168.1.1:8080", "192.168.0.0/16,10.0.0.0/8")).To(BeTrue())
				Expect(validateIPAgainstList("10.0.0.1:1234", "192.168.0.0/16,10.0.0.0/8")).To(BeTrue())
				Expect(validateIPAgainstList("172.16.0.1:9999", "192.168.0.0/16,10.0.0.0/8")).To(BeFalse())
			})
		})
	})

	Describe("handleLoginFromHeaders", func() {
		var ds model.DataStore
		var req *http.Request
		const trustedIP = "192.168.0.42"

		BeforeEach(func() {
			ds = &tests.MockDataStore{}
			req = httptest.NewRequest("GET", "/", nil)
			req = req.WithContext(request.WithReverseProxyIp(req.Context(), trustedIP))
			conf.Server.ExtAuth.TrustedSources = "192.168.0.0/16"
			conf.Server.ExtAuth.UserHeader = "Remote-User"
		})

		It("makes the first user an admin", func() {
			// No existing users
			req.Header.Set("Remote-User", "firstuser")
			result := handleLoginFromHeaders(ds, req)

			Expect(result).ToNot(BeNil())
			Expect(result["isAdmin"]).To(BeTrue())

			// Verify user was created as admin
			u, err := ds.User(context.Background()).FindByUsername("firstuser")
			Expect(err).To(BeNil())
			Expect(u.IsAdmin).To(BeTrue())
		})

		It("does not make subsequent users admins", func() {
			// Create the first user
			_ = ds.User(context.Background()).Put(&model.User{
				ID:       "existing-user-id",
				UserName: "existinguser",
				Name:     "Existing User",
				IsAdmin:  true,
			})

			// Try to create a second user via proxy header
			req.Header.Set("Remote-User", "seconduser")
			result := handleLoginFromHeaders(ds, req)

			Expect(result).ToNot(BeNil())
			Expect(result["isAdmin"]).To(BeFalse())

			// Verify user was created as non-admin
			u, err := ds.User(context.Background()).FindByUsername("seconduser")
			Expect(err).To(BeNil())
			Expect(u.IsAdmin).To(BeFalse())
		})

		It("serializes concurrent external-auth first-user creation", func() {
			repo := newSynchronizedUserRepo()
			base := &tests.MockDataStore{MockedUser: repo}
			ds := &serializedImmediateDataStore{DataStore: base}
			start := make(chan struct{})
			results := make(chan map[string]any, 2)
			var wg sync.WaitGroup

			for _, username := range []string{"external-one", "external-two"} {
				wg.Add(1)
				go func(username string) {
					defer wg.Done()
					<-start
					req := httptest.NewRequest("GET", "/", nil)
					req = req.WithContext(request.WithReverseProxyIp(req.Context(), trustedIP))
					req.Header.Set("Remote-User", username)
					results <- handleLoginFromHeaders(ds, req)
				}(username)
			}
			close(start)
			wg.Wait()
			close(results)

			var adminResults int
			for result := range results {
				Expect(result).ToNot(BeNil())
				if result["isAdmin"] == true {
					adminResults++
				}
			}
			Expect(adminResults).To(Equal(1))
			Expect(repo.userCount()).To(Equal(2))
			Expect(repo.adminCount()).To(Equal(1))
			Expect(ds.immediateCalls).To(Equal(2))
		})
	})
})

type serializedImmediateDataStore struct {
	model.DataStore
	mu             sync.Mutex
	immediateCalls int
}

func (s *serializedImmediateDataStore) WithTxImmediate(block func(tx model.DataStore) error, _ ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.immediateCalls++
	return block(s)
}

type synchronizedUserRepo struct {
	model.UserRepository
	mu    sync.Mutex
	users map[string]model.User
}

func newSynchronizedUserRepo() *synchronizedUserRepo {
	return &synchronizedUserRepo{users: map[string]model.User{}}
}

func (r *synchronizedUserRepo) CountAll(...model.QueryOptions) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.users)), nil
}

func (r *synchronizedUserRepo) Put(user *model.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *user
	if copy.NewPassword != "" {
		copy.Password = copy.NewPassword
	}
	r.users[strings.ToLower(copy.UserName)] = copy
	return nil
}

func (r *synchronizedUserRepo) FindByUsername(username string) (*model.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.users[strings.ToLower(username)]
	if !ok {
		return nil, model.ErrNotFound
	}
	copy := user
	return &copy, nil
}

func (r *synchronizedUserRepo) FindByUsernameWithPassword(username string) (*model.User, error) {
	return r.FindByUsername(username)
}

func (r *synchronizedUserRepo) UpdateLastLoginAt(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, user := range r.users {
		if user.ID == id {
			user.LastLoginAt = new(time.Now())
			r.users[key] = user
			return nil
		}
	}
	return model.ErrNotFound
}

func (r *synchronizedUserRepo) userCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.users)
}

func (r *synchronizedUserRepo) adminCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, user := range r.users {
		if user.IsAdmin {
			count++
		}
	}
	return count
}
