package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/quic-go/quic-go/http3"
)

var _ = Describe("HTTP/3 support", func() {
	const testDataDir = "testdata"

	It("serves repeated requests over a reusable QUIC connection", func() {
		certFile := filepath.Join(testDataDir, "test_cert.pem")
		keyFile := filepath.Join(testDataDir, "test_key.pem")

		runtime, err := newHTTP3Runtime(
			GinkgoT().Context(),
			"127.0.0.1:0",
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}),
			certFile,
			keyFile,
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(runtime.server.QUICConfig.Allow0RTT).To(BeTrue())
		Expect(runtime.server.QUICConfig.DisablePathMTUDiscovery).To(BeFalse())
		Expect(runtime.server.IdleTimeout).To(Equal(serverHTTP3IdleTimeout))

		serveErrC := make(chan error, 1)
		go func() {
			serveErrC <- runtime.serve()
		}()
		DeferCleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = runtime.shutdown(ctx)
			select {
			case <-serveErrC:
			case <-time.After(time.Second):
			}
		})

		certPEM, err := os.ReadFile(certFile)
		Expect(err).ToNot(HaveOccurred())
		roots := x509.NewCertPool()
		Expect(roots.AppendCertsFromPEM(certPEM)).To(BeTrue())

		transport := &http3.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    roots,
				MinVersion: tls.VersionTLS13,
			},
		}
		DeferCleanup(func() {
			_ = transport.Close()
		})

		client := &http.Client{
			Transport: transport,
			Timeout:   5 * time.Second,
		}
		url := fmt.Sprintf("https://%s/ping", runtime.packetConn.LocalAddr().String())

		for range 3 {
			var protoMajor int
			Eventually(func() error {
				resp, err := client.Get(url)
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusNoContent {
					return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				}
				protoMajor = resp.ProtoMajor
				return nil
			}, 2*time.Second, 100*time.Millisecond).Should(Succeed())
			Expect(protoMajor).To(Equal(3))
		}
	})

	Describe("0-RTT replay guard", func() {
		newEarlyRequest := func(method, target string) *http.Request {
			req := httptest.NewRequest(method, target, nil)
			req.TLS = &tls.ConnectionState{HandshakeComplete: false}
			return req
		}

		It("allows read-only native API requests before handshake completion", func() {
			recorder := httptest.NewRecorder()
			handler := guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			handler.ServeHTTP(recorder, newEarlyRequest(http.MethodGet, "https://example.test/api/album"))
			Expect(recorder.Code).To(Equal(http.StatusNoContent))
		})

		It("allows explicitly read-only Subsonic endpoint families", func() {
			for _, target := range []string{
				"https://example.test/rest/getAlbum.view?id=1",
				"https://example.test/rest/search3.view?query=test",
				"https://example.test/rest/ping.view",
			} {
				recorder := httptest.NewRecorder()
				handler := guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}))
				handler.ServeHTTP(recorder, newEarlyRequest(http.MethodGet, target))
				Expect(recorder.Code).To(Equal(http.StatusNoContent), target)
			}
		})

		It("rejects replayable state-changing Subsonic GET endpoints", func() {
			for _, target := range []string{
				"https://example.test/rest/scrobble.view?id=1",
				"https://example.test/rest/star.view?id=1",
				"https://example.test/rest/unstar.view?id=1",
			} {
				recorder := httptest.NewRecorder()
				handler := guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}))
				handler.ServeHTTP(recorder, newEarlyRequest(http.MethodGet, target))
				Expect(recorder.Code).To(Equal(http.StatusTooEarly), target)
				Expect(recorder.Header().Get("Cache-Control")).To(Equal("no-store"))
			}
		})

		It("rejects non-idempotent methods during early data", func() {
			for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
				recorder := httptest.NewRecorder()
				handler := guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}))
				handler.ServeHTTP(recorder, newEarlyRequest(method, "https://example.test/api/playlists/1"))
				Expect(recorder.Code).To(Equal(http.StatusTooEarly), method)
			}
		})

		It("allows every request after the TLS handshake completes", func() {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "https://example.test/auth/login", nil)
			req.TLS = &tls.ConnectionState{HandshakeComplete: true}
			handler := guardHTTP3EarlyData(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			handler.ServeHTTP(recorder, req)
			Expect(recorder.Code).To(Equal(http.StatusNoContent))
		})
	})

	It("shuts down idempotently", func() {
		certFile := filepath.Join(testDataDir, "test_cert.pem")
		keyFile := filepath.Join(testDataDir, "test_key.pem")
		runtime, err := newHTTP3Runtime(
			GinkgoT().Context(),
			"127.0.0.1:0",
			http.NotFoundHandler(),
			certFile,
			keyFile,
		)
		Expect(err).ToNot(HaveOccurred())

		serveErrC := make(chan error, 1)
		go func() { serveErrC <- runtime.serve() }()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		Expect(runtime.shutdown(ctx)).To(Succeed())
		Expect(runtime.shutdown(ctx)).To(Succeed())
		Eventually(serveErrC, time.Second).Should(Receive())
	})

	It("advertises the HTTP/3 endpoint to HTTP/1.1 and HTTP/2 clients", func() {
		runtime := &http3Runtime{altSvcHeader: `h3=":4533"; ma=2592000`}
		handler := runtime.advertise(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.test/ping", nil))

		Expect(recorder.Code).To(Equal(http.StatusNoContent))
		Expect(recorder.Header().Get("Alt-Svc")).To(Equal(`h3=":4533"; ma=2592000`))
	})
})
