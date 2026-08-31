package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/navidrome/navidrome/utils/ioutils"
)

func newHTTP2TestServer(handler http.Handler) *httptest.Server {
	srv := httptest.NewUnstartedServer(handler)
	srv.Config = newHTTPServer(handler)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	return srv
}

func newHTTP2Client(srv *httptest.Server) *http.Client {
	protocols := new(http.Protocols)
	protocols.SetHTTP2(true)
	transport := srv.Client().Transport.(*http.Transport).Clone()
	transport.Protocols = protocols
	return &http.Client{Transport: transport}
}

func BenchmarkHTTP2CompressedAPIResponse(b *testing.B) {
	body := make([]byte, largeCompressedResponseSize)
	for i := range body {
		body[i] = byte(i)
	}

	handler := compressMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(body); err != nil {
			panic(err)
		}
	}))

	srv := newHTTP2TestServer(handler)
	b.Cleanup(srv.Close)
	client := newHTTP2Client(srv)

	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		resp, err := client.Get(srv.URL + "/rest/getAlbumList2.view")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			_ = resp.Body.Close()
			b.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHTTP2StreamingResponse(b *testing.B) {
	chunk := make([]byte, ioutils.DefaultCopyBufferSize)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		for range 32 {
			if _, err := ioutils.Copy(w, bytes.NewReader(chunk)); err != nil {
				return
			}
		}
	})

	srv := newHTTP2TestServer(handler)
	b.Cleanup(srv.Close)
	client := newHTTP2Client(srv)

	b.SetBytes(int64(len(chunk) * 32))
	b.ReportAllocs()
	for b.Loop() {
		resp, err := client.Get(srv.URL + "/rest/stream.view")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			_ = resp.Body.Close()
			b.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
