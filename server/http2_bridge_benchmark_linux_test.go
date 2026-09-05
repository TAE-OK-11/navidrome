//go:build linux

package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/utils/ioutils"
	"golang.org/x/net/http2"
)

func BenchmarkHTTP2InheritedBridgeRoundTrip(b *testing.B) {
	token := "benchmark-bridge-token"
	body := make([]byte, largeCompressedResponseSize)
	for i := range body {
		body[i] = byte(i)
	}

	handler := authenticatedHTTP3Bridge(token, compressMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(body); err != nil {
			panic(err)
		}
	})))

	client := newInheritedBridgeBenchClient(b, handler)

	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		req, err := http.NewRequest(http.MethodGet, "http://navidrome-h3-inherited/rest/getAlbumList2.view", nil)
		if err != nil {
			b.Fatal(err)
		}
		req.Header.Set(rustHTTP3TokenHeader, token)
		req.Header.Set(rustHTTP3AuthorityHeader, "music.example")
		req.Header.Set(rustHTTP3RemoteAddrHeader, "192.0.2.10:54321")
		req.Header.Set("Accept-Encoding", "zstd, br, gzip")

		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := ioutils.Copy(io.Discard, resp.Body); err != nil {
			_ = resp.Body.Close()
			b.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHTTP2InheritedBridgeMediaStream measures the inherited HTTP/2 bridge
// path used by the tokio-quiche HTTP/3 companion for transcoded audio delivery.
func BenchmarkHTTP2InheritedBridgeMediaStream(b *testing.B) {
	token := "benchmark-bridge-stream-token"
	chunk := make([]byte, ioutils.DefaultCopyBufferSize)
	for i := range chunk {
		chunk[i] = byte(i)
	}
	const chunks = 32

	handler := authenticatedHTTP3Bridge(token, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Accept-Ranges", "none")
		for range chunks {
			if _, err := ioutils.Copy(w, bytes.NewReader(chunk)); err != nil {
				return
			}
		}
	}))

	client := newInheritedBridgeBenchClient(b, handler)
	total := int64(len(chunk) * chunks)
	b.SetBytes(total)
	b.ReportAllocs()
	for b.Loop() {
		req, err := http.NewRequest(http.MethodGet, "http://navidrome-h3-inherited/rest/stream.view", nil)
		if err != nil {
			b.Fatal(err)
		}
		req.Header.Set(rustHTTP3TokenHeader, token)
		req.Header.Set(rustHTTP3AuthorityHeader, "music.example")
		req.Header.Set(rustHTTP3RemoteAddrHeader, "192.0.2.10:54321")

		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		n, err := ioutils.Copy(io.Discard, resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			b.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			b.Fatal(err)
		}
		if n != total {
			b.Fatalf("copied %d bytes, want %d", n, total)
		}
	}
}

func newInheritedBridgeBenchClient(b *testing.B, handler http.Handler) *http.Client {
	b.Helper()

	listener := newInheritedConnListener()
	protocols := new(http.Protocols)
	protocols.SetHTTP1(false)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)
	internalServer := &http.Server{
		ReadHeaderTimeout: consts.ServerReadHeaderTimeout,
		IdleTimeout:       serverH3BridgeIdleTimeout,
		MaxHeaderBytes:    serverMaxHeaderBytes,
		Protocols:         protocols,
		HTTP2: &http.HTTP2Config{
			MaxConcurrentStreams:          serverH3BridgeMaxStreams,
			MaxReceiveBufferPerConnection: serverH2ConnectionWindow,
			MaxReceiveBufferPerStream:     serverH2StreamWindow,
			SendPingTimeout:               serverH2SendPingTimeout,
			PingTimeout:                   serverH2PingTimeout,
			WriteByteTimeout:              serverH2WriteByteTimeout,
		},
		Handler: handler,
	}

	go func() {
		_ = internalServer.Serve(listener)
	}()
	b.Cleanup(func() {
		_ = internalServer.Close()
		_ = listener.Close()
	})

	parent, childFile, err := inheritedSocketpair("benchmark-h2-parent", "benchmark-h2-child", serverH2StreamWindow)
	if err != nil {
		b.Fatal(err)
	}
	if err := listener.add(parent); err != nil {
		b.Fatal(err)
	}

	childConn, err := net.FileConn(childFile)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = childConn.Close()
		_ = childFile.Close()
	})

	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(context.Context, string, string, *tls.Config) (net.Conn, error) {
			return childConn, nil
		},
	}
	return &http.Client{Transport: transport}
}
