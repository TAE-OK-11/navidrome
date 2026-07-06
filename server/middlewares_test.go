package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("middlewares", func() {
	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
	})
	Describe("robotsTXT", func() {
		var nextCalled bool
		next := func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		}
		BeforeEach(func() {
			nextCalled = false
		})

		It("returns the robot.txt when requested from root", func() {
			r := httptest.NewRequest("GET", "/robots.txt", nil)
			w := httptest.NewRecorder()

			robotsTXT(os.DirFS("tests/fixtures"))(http.HandlerFunc(next)).ServeHTTP(w, r)

			Expect(nextCalled).To(BeFalse())
			Expect(w.Body.String()).To(HavePrefix("User-agent:"))
		})

		It("allows prefixes", func() {
			r := httptest.NewRequest("GET", "/app/robots.txt", nil)
			w := httptest.NewRecorder()

			robotsTXT(os.DirFS("tests/fixtures"))(http.HandlerFunc(next)).ServeHTTP(w, r)

			Expect(nextCalled).To(BeFalse())
			Expect(w.Body.String()).To(HavePrefix("User-agent:"))
		})

		It("passes through requests for other files", func() {
			r := httptest.NewRequest("GET", "/this_is_not_a_robots.txt_file", nil)
			w := httptest.NewRecorder()

			robotsTXT(os.DirFS("tests/fixtures"))(http.HandlerFunc(next)).ServeHTTP(w, r)

			Expect(nextCalled).To(BeTrue())
		})
	})

	Describe("serverAddressMiddleware", func() {
		var (
			nextHandler http.Handler
			middleware  http.Handler
			recorder    *httptest.ResponseRecorder
			req         *http.Request
		)

		BeforeEach(func() {
			nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			middleware = serverAddressMiddleware(nextHandler)
			recorder = httptest.NewRecorder()
		})

		Context("with no X-Forwarded headers", func() {
			BeforeEach(func() {
				req, _ = http.NewRequest("GET", "http://example.com", nil)
			})

			It("should not modify the request", func() {
				middleware.ServeHTTP(recorder, req)
				Expect(req.Host).To(Equal("example.com"))
				Expect(req.URL.Scheme).To(Equal("http"))
			})
		})

		Context("with X-Forwarded-Host header", func() {
			BeforeEach(func() {
				req, _ = http.NewRequest("GET", "http://example.com", nil)
				req.Header.Set("X-Forwarded-Host", "forwarded.example.com")
			})

			It("should modify the request with the X-Forwarded-Host header value", func() {
				middleware.ServeHTTP(recorder, req)
				Expect(req.Host).To(Equal("forwarded.example.com"))
				Expect(req.URL.Scheme).To(Equal("http"))
			})
		})

		Context("with X-Forwarded-Proto header", func() {
			BeforeEach(func() {
				req, _ = http.NewRequest("GET", "http://example.com", nil)
				req.Header.Set("X-Forwarded-Proto", "https")
			})

			It("should modify the request with the X-Forwarded-Proto header value", func() {
				middleware.ServeHTTP(recorder, req)
				Expect(req.Host).To(Equal("example.com"))
				Expect(req.URL.Scheme).To(Equal("https"))
			})
		})

		Context("with X-Forwarded-Scheme header", func() {
			BeforeEach(func() {
				req, _ = http.NewRequest("GET", "http://example.com", nil)
				req.Header.Set("X-Forwarded-Scheme", "https")
			})

			It("should modify the request with the X-Forwarded-Scheme header value", func() {
				middleware.ServeHTTP(recorder, req)
				Expect(req.Host).To(Equal("example.com"))
				Expect(req.URL.Scheme).To(Equal("https"))
			})
		})

		Context("with multiple X-Forwarded headers", func() {
			BeforeEach(func() {
				req, _ = http.NewRequest("GET", "http://example.com", nil)
				req.Header.Set("X-Forwarded-Host", "forwarded.example.com")
				req.Header.Set("X-Forwarded-Proto", "https")
				req.Header.Set("X-Forwarded-Scheme", "http")
			})

			It("should modify the request with the first non-empty X-Forwarded header value", func() {
				middleware.ServeHTTP(recorder, req)
				Expect(req.Host).To(Equal("forwarded.example.com"))
				Expect(req.URL.Scheme).To(Equal("https"))
			})
		})

		Context("with multiple values in X-Forwarded-Host header", func() {
			BeforeEach(func() {
				req, _ = http.NewRequest("GET", "http://example.com", nil)
				req.Header.Set("X-Forwarded-Host", "forwarded1.example.com, forwarded2.example.com")
			})

			It("should modify the request with the first value in X-Forwarded-Host header", func() {
				middleware.ServeHTTP(recorder, req)
				Expect(req.Host).To(Equal("forwarded1.example.com"))
				Expect(req.URL.Scheme).To(Equal("http"))
			})
		})
	})

	Describe("clientUniqueIDMiddleware", func() {
		var (
			nextHandler http.Handler
			middleware  http.Handler
			req         *http.Request
			nextReq     *http.Request
			rec         *httptest.ResponseRecorder
		)

		BeforeEach(func() {
			nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				nextReq = r
			})
			middleware = clientUniqueIDMiddleware(nextHandler)
			req, _ = http.NewRequest(http.MethodGet, "/", nil)
			rec = httptest.NewRecorder()
		})

		Context("when the request header has the unique client ID", func() {
			BeforeEach(func() {
				req.Header.Set(consts.UIClientUniqueIDHeader, "123456")
				conf.Server.BasePath = "/music"
			})

			It("sets the unique client ID as a cookie and adds it to the request context", func() {
				middleware.ServeHTTP(rec, req)

				Expect(rec.Result().Cookies()).To(HaveLen(1))
				Expect(rec.Result().Cookies()[0].Name).To(Equal(consts.UIClientUniqueIDHeader))
				Expect(rec.Result().Cookies()[0].Value).To(Equal("123456"))
				Expect(rec.Result().Cookies()[0].MaxAge).To(Equal(consts.CookieExpiry))
				Expect(rec.Result().Cookies()[0].HttpOnly).To(BeTrue())
				Expect(rec.Result().Cookies()[0].Secure).To(BeTrue())
				Expect(rec.Result().Cookies()[0].SameSite).To(Equal(http.SameSiteStrictMode))
				Expect(rec.Result().Cookies()[0].Path).To(Equal("/music"))
				clientUniqueId, _ := request.ClientUniqueIdFrom(nextReq.Context())
				Expect(clientUniqueId).To(Equal("123456"))
			})
		})

		Context("when the request header does not have the unique client ID", func() {
			Context("when the request has the unique client ID in a cookie", func() {
				BeforeEach(func() {
					req.AddCookie(&http.Cookie{
						Name:  consts.UIClientUniqueIDHeader,
						Value: "123456",
					})
				})

				It("adds the unique client ID to the request context", func() {
					middleware.ServeHTTP(rec, req)

					Expect(rec.Result().Cookies()).To(HaveLen(0))

					clientUniqueId, _ := request.ClientUniqueIdFrom(nextReq.Context())
					Expect(clientUniqueId).To(Equal("123456"))
				})
			})

			Context("when the request does not have the unique client ID in a cookie", func() {
				It("does not add the unique client ID to the request context", func() {
					middleware.ServeHTTP(rec, req)

					Expect(rec.Result().Cookies()).To(HaveLen(0))

					clientUniqueId, _ := request.ClientUniqueIdFrom(nextReq.Context())
					Expect(clientUniqueId).To(BeEmpty())
				})
			})
		})
	})

	Describe("compressMiddleware", func() {
		const responseBody = `{"data":"` + "0123456789abcdef0123456789abcdef" + `"}`

		var handler http.Handler
		var largeHandler http.Handler

		BeforeEach(func() {
			body := strings.Repeat(responseBody, 32)
			handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			})

			largeBody := strings.Repeat(responseBody, 512)
			largeHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(largeBody))
			})
		})

		It("uses zstd for small compressed responses", func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "gzip, zstd, br")
			rec := httptest.NewRecorder()

			compressMiddleware()(handler).ServeHTTP(rec, req)

			Expect(rec.Header().Get("Content-Encoding")).To(Equal("zstd"))
			Expect(rec.Header().Values("Vary")).To(ContainElement("Accept-Encoding"))
			Expect(decodeZstd(rec.Body.Bytes())).To(Equal(strings.Repeat(responseBody, 32)))
		})

		It("uses brotli for large compressed responses", func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "gzip, zstd, br")
			rec := httptest.NewRecorder()

			compressMiddleware()(largeHandler).ServeHTTP(rec, req)

			Expect(rec.Header().Get("Content-Encoding")).To(Equal("br"))
			Expect(rec.Header().Values("Vary")).To(ContainElement("Accept-Encoding"))
			Expect(decodeBrotli(rec.Body.Bytes())).To(Equal(strings.Repeat(responseBody, 512)))
		})

		It("uses zstd when brotli is unavailable", func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "zstd, gzip")
			rec := httptest.NewRecorder()

			compressMiddleware()(handler).ServeHTTP(rec, req)

			Expect(rec.Header().Get("Content-Encoding")).To(Equal("zstd"))
			Expect(decodeZstd(rec.Body.Bytes())).To(Equal(strings.Repeat(responseBody, 32)))
		})

		It("honors Accept-Encoding q values", func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "br;q=0, zstd;q=0.5, gzip;q=0.1")
			rec := httptest.NewRecorder()

			compressMiddleware()(handler).ServeHTTP(rec, req)

			Expect(rec.Header().Get("Content-Encoding")).To(Equal("zstd"))
			Expect(decodeZstd(rec.Body.Bytes())).To(Equal(strings.Repeat(responseBody, 32)))
		})

		It("keeps zstd for small responses even when brotli has a higher q value", func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "br;q=1, zstd;q=0.1, gzip;q=0.5")
			rec := httptest.NewRecorder()

			compressMiddleware()(handler).ServeHTTP(rec, req)

			Expect(rec.Header().Get("Content-Encoding")).To(Equal("zstd"))
			Expect(decodeZstd(rec.Body.Bytes())).To(Equal(strings.Repeat(responseBody, 32)))
		})

		It("does not compress small responses", func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "br")
			rec := httptest.NewRecorder()
			smallHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			})

			compressMiddleware()(smallHandler).ServeHTTP(rec, req)

			Expect(rec.Header().Get("Content-Encoding")).To(BeEmpty())
			Expect(rec.Body.String()).To(Equal(`{"ok":true}`))
		})

		It("does not compress range requests or audio responses", func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "br")
			req.Header.Set("Range", "bytes=0-10")
			rec := httptest.NewRecorder()

			compressMiddleware()(handler).ServeHTTP(rec, req)
			Expect(rec.Header().Get("Content-Encoding")).To(BeEmpty())

			req = httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "br")
			rec = httptest.NewRecorder()
			audioHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "audio/mpeg")
				_, _ = w.Write([]byte(strings.Repeat("audio", 256)))
			})

			compressMiddleware()(audioHandler).ServeHTTP(rec, req)
			Expect(rec.Header().Get("Content-Encoding")).To(BeEmpty())
		})

		It("does not wrap known media endpoints", func() {
			req := httptest.NewRequest(http.MethodGet, "/rest/stream.view", nil)
			req.Header.Set("Accept-Encoding", "br")
			rec := httptest.NewRecorder()

			compressMiddleware()(handler).ServeHTTP(rec, req)

			Expect(rec.Header().Get("Content-Encoding")).To(BeEmpty())
			Expect(rec.Body.String()).To(Equal(strings.Repeat(responseBody, 32)))
		})

		It("does not break streaming flush support", func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "br")
			rec := httptest.NewRecorder()
			streamingHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, ok := w.(http.Flusher)
				Expect(ok).To(BeTrue())
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache, no-transform")
				_, _ = w.Write([]byte(strings.Repeat("event: ping\n", 64)))
				w.(http.Flusher).Flush()
			})

			compressMiddleware()(streamingHandler).ServeHTTP(rec, req)

			Expect(rec.Header().Get("Content-Encoding")).To(BeEmpty())
			Expect(rec.Body.String()).To(ContainSubstring("event: ping"))
		})
	})

	Describe("URLParamsMiddleware", func() {
		var (
			router      *chi.Mux
			middleware  http.Handler
			recorder    *httptest.ResponseRecorder
			testHandler http.HandlerFunc
		)

		BeforeEach(func() {
			router = chi.NewRouter()
			recorder = httptest.NewRecorder()
			testHandler = func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("OK"))
			}
		})

		Context("when request has no query parameters", func() {
			It("adds URL parameters to the request", func() {
				middleware = URLParamsMiddleware(testHandler)
				router.Mount("/", middleware)

				req, _ := http.NewRequest("GET", "/?user=1", nil)
				router.ServeHTTP(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusOK))
				Expect(recorder.Body.String()).To(Equal("OK"))
				Expect(req.URL.RawQuery).To(ContainSubstring("user=1"))
			})

			It("does not re-encode query params without route params", func() {
				middleware = URLParamsMiddleware(testHandler)

				req, _ := http.NewRequest("GET", "/?z=2&a=1", nil)
				middleware.ServeHTTP(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusOK))
				Expect(req.URL.RawQuery).To(Equal("z=2&a=1"))
			})
		})

		Context("when request has query parameters", func() {
			It("merges URL parameters and query parameters", func() {
				router.Route("/{key}", func(r chi.Router) {
					r.Use(URLParamsMiddleware)
					r.Get("/", testHandler)
				})

				req, _ := http.NewRequest("GET", "/test?key=value", nil)
				router.ServeHTTP(recorder, req)
				Expect(recorder.Code).To(Equal(http.StatusOK))
				Expect(recorder.Body.String()).To(Equal("OK"))
				Expect(req.URL.RawQuery).To(ContainSubstring("key=value"))
				Expect(req.URL.RawQuery).To(ContainSubstring("%3Akey=test"))
			})
		})

		Context("when URL parameter has wildcard key", func() {
			It("does not include wildcard key in query parameters", func() {
				router.Route("/{t*}", func(r chi.Router) {
					r.Use(URLParamsMiddleware)
					r.Get("/", testHandler)
				})

				req, _ := http.NewRequest("GET", "/test?key=value", nil)
				router.ServeHTTP(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusOK))
				Expect(recorder.Body.String()).To(Equal("OK"))
				Expect(req.URL.RawQuery).To(ContainSubstring("key=value"))
			})
		})

		Context("when URL parameters require encoding", func() {
			It("encodes URL parameters correctly", func() {
				router.Route("/{key}", func(r chi.Router) {
					r.Use(URLParamsMiddleware)
					r.Get("/", testHandler)
				})

				req, _ := http.NewRequest("GET", "/test with space?key=another value", nil)
				router.ServeHTTP(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusOK))
				Expect(recorder.Body.String()).To(Equal("OK"))
				queryValues, _ := url.ParseQuery(req.URL.RawQuery)
				Expect(queryValues.Get(":key")).To(Equal("test with space"))
				Expect(queryValues.Get("key")).To(Equal("another value"))
			})
		})

		Context("when there are multiple URL parameters", func() {
			It("includes all URL parameters in the query string", func() {
				router.Route("/{key}/{value}", func(r chi.Router) {
					r.Use(URLParamsMiddleware)
					r.Get("/", testHandler)
				})

				req, _ := http.NewRequest("GET", "/test/value?key=other_value", nil)
				router.ServeHTTP(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusOK))
				Expect(recorder.Body.String()).To(Equal("OK"))

				queryValues, _ := url.ParseQuery(req.URL.RawQuery)
				Expect(queryValues.Get(":key")).To(Equal("test"))
				Expect(queryValues.Get(":value")).To(Equal("value"))
				Expect(queryValues.Get("key")).To(Equal("other_value"))
			})
		})
	})

	Describe("UpdateLastAccessMiddleware", func() {
		var (
			middleware     func(next http.Handler) http.Handler
			req            *http.Request
			ctx            context.Context
			ds             *tests.MockDataStore
			id             string
			lastAccessTime time.Time
		)

		callMiddleware := func(req *http.Request) {
			middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(nil, req)
		}

		BeforeEach(func() {
			id = uuid.NewString()
			ds = &tests.MockDataStore{}
			lastAccessTime = time.Now()
			Expect(ds.User(ctx).Put(&model.User{ID: id, UserName: "johndoe", LastAccessAt: &lastAccessTime})).
				To(Succeed())

			middleware = UpdateLastAccessMiddleware(ds)
			ctx = request.WithUser(
				context.Background(),
				model.User{ID: id, UserName: "johndoe"},
			)
			req, _ = http.NewRequest(http.MethodGet, "/", nil)
			req = req.WithContext(ctx)
		})

		Context("when the request has a user", func() {
			It("does calls the next handler", func() {
				called := false
				middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
				})).ServeHTTP(nil, req)
				Expect(called).To(BeTrue())
			})

			It("updates the last access time", func() {
				time.Sleep(3 * time.Millisecond)

				callMiddleware(req)

				user, _ := ds.MockedUser.FindByUsername("johndoe")
				Expect(*user.LastAccessAt).To(BeTemporally(">", lastAccessTime, time.Second))
			})

			It("skip fast successive requests", func() {
				// First request
				callMiddleware(req)
				user, _ := ds.MockedUser.FindByUsername("johndoe")
				lastAccessTime = *user.LastAccessAt // Store the last access time

				// Second request
				time.Sleep(3 * time.Millisecond)
				callMiddleware(req)

				// The second request should not have changed the last access time
				user, _ = ds.MockedUser.FindByUsername("johndoe")
				Expect(user.LastAccessAt).To(Equal(&lastAccessTime))
			})
		})
		Context("when the request has no user", func() {
			It("does not update the last access time", func() {
				req = req.WithContext(context.Background())
				callMiddleware(req)

				usr, _ := ds.MockedUser.FindByUsername("johndoe")
				Expect(usr.LastAccessAt).To(Equal(&lastAccessTime))
			})
		})
	})
})

func decodeBrotli(data []byte) string {
	out, err := io.ReadAll(brotli.NewReader(bytes.NewReader(data)))
	Expect(err).NotTo(HaveOccurred())
	return string(out)
}

func decodeZstd(data []byte) string {
	reader, err := zstd.NewReader(bytes.NewReader(data))
	Expect(err).NotTo(HaveOccurred())
	defer reader.Close()
	out, err := io.ReadAll(reader)
	Expect(err).NotTo(HaveOccurred())
	return string(out)
}
