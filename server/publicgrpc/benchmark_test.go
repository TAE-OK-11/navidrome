package publicgrpc

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/core/eventbus"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/publicgrpc/gen"
	"github.com/navidrome/navidrome/tests"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const benchBufSize = 1 << 20

type benchStub struct{}

func (benchStub) Invoke(_ context.Context, _ string, _ url.Values, _ string, _ bool) (int, string, []byte, error) {
	body := []byte(`{"subsonic-response":{"status":"ok","version":"1.16.1"}}`)
	return http.StatusOK, "application/json", body, nil
}

func (benchStub) Open(_ context.Context, _ string, _ url.Values, _ string, _ bool, w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "audio/mpeg")
	chunk := make([]byte, openChunkSize)
	for range 8 {
		if _, err := w.Write(chunk); err != nil {
			return err
		}
	}
	return nil
}

type bearerPerRPC struct{ token string }

func (b bearerPerRPC) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + b.token}, nil
}

func (bearerPerRPC) RequireTransportSecurity() bool { return false }

func benchGRPCClient(b *testing.B, svc *Service, token string) gen.PublicClient {
	b.Helper()
	lis := bufconn.Listen(benchBufSize)
	gs := grpc.NewServer(
		grpc.MaxRecvMsgSize(publicGRPCMaxMsgBytes),
		grpc.MaxSendMsgSize(publicGRPCMaxMsgBytes),
	)
	gen.RegisterPublicServer(gs, svc)
	go func() { _ = gs.Serve(lis) }()
	b.Cleanup(func() {
		gs.Stop()
		_ = lis.Close()
	})

	opts := []grpc.DialOption{
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	if token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(bearerPerRPC{token: token}))
	}
	conn, err := grpc.NewClient("passthrough:///bufnet", opts...)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = conn.Close() })
	return gen.NewPublicClient(conn)
}

func benchAuthToken(b *testing.B, username string) string {
	b.Helper()
	token, err := auth.CreateExpiringPublicToken(time.Now().Add(time.Hour), auth.Claims{
		Subject: username,
		UserID:  "bench",
	})
	if err != nil {
		b.Fatal(err)
	}
	return token
}

func benchService(b *testing.B) *Service {
	b.Helper()
	conf.SetPublicGRPCEnabledForTest(true)
	ds := &tests.MockDataStore{}
	userRepo := tests.CreateMockUserRepo()
	_ = userRepo.Put(&model.User{ID: "bench", UserName: "bench"})
	ds.MockedUser = userRepo
	auth.TokenAuth = jwtauth.New("HS256", []byte("benchmark-grpc-pgo-secret"), nil)
	return NewService(ds, benchStub{}, nil, eventbus.New())
}

func BenchmarkPublicGRPCPing(b *testing.B) {
	client := benchGRPCClient(b, benchService(b), "")
	b.ReportAllocs()
	for b.Loop() {
		if _, err := client.Ping(context.Background(), &gen.PingRequest{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPublicGRPCInvoke(b *testing.B) {
	svc := benchService(b)
	token := benchAuthToken(b, "bench")
	client := benchGRPCClient(b, svc, token)
	ctx := context.Background()
	req := &gen.InvokeRequest{Endpoint: "getAlbumList2", Json: true}
	b.SetBytes(64)
	b.ReportAllocs()
	for b.Loop() {
		resp, err := client.Invoke(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if len(resp.GetBody()) == 0 {
			b.Fatal("empty invoke body")
		}
	}
}

func BenchmarkPublicGRPCOpenStream(b *testing.B) {
	svc := benchService(b)
	token := benchAuthToken(b, "bench")
	client := benchGRPCClient(b, svc, token)
	ctx := context.Background()
	req := &gen.OpenRequest{Path: "stream", Json: true}
	b.SetBytes(openChunkSize * 8)
	b.ReportAllocs()
	for b.Loop() {
		stream, err := client.Open(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		for {
			chunk, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
			if chunk.GetFinal() {
				break
			}
		}
	}
}

func newHTTP2BenchServer(handler http.Handler) *httptest.Server {
	srv := httptest.NewUnstartedServer(handler)
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	srv.Config = &http.Server{
		Handler:           handler,
		TLSConfig:         srv.TLS,
		Protocols:         protocols,
		ReadHeaderTimeout: 5 * time.Second,
	}
	srv.EnableHTTP2 = true
	srv.StartTLS()
	return srv
}

func BenchmarkPublicGRPCHTTP2Ping(b *testing.B) {
	conf.SetPublicGRPCEnabledForTest(true)
	gs := NewServer(nil, nil, nil)
	handler := Mux(gs, http.NotFoundHandler())
	srv := newHTTP2BenchServer(handler)
	b.Cleanup(srv.Close)

	client := srv.Client()
	protocols := new(http.Protocols)
	protocols.SetHTTP2(true)
	transport := client.Transport.(*http.Transport).Clone()
	transport.Protocols = protocols
	h2Client := &http.Client{Transport: transport}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/navidrome.public.v1.Public/Ping", http.NoBody)
	if err != nil {
		b.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("TE", "trailers")

	b.ReportAllocs()
	for b.Loop() {
		resp, err := h2Client.Do(req)
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
