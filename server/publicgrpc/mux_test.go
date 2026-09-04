package publicgrpc

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
)

func TestGRPCOnlyRejectsREST(t *testing.T) {
	h := GRPCOnly(grpc.NewServer())
	req := httptest.NewRequest(http.MethodGet, "/rest/ping.view", nil)
	req.Proto = "HTTP/2.0"
	req.ProtoMajor = 2
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestGRPCOnlyAcceptsGRPC(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/navidrome.public.v1.Public/Ping", nil)
	req.Proto = "HTTP/2.0"
	req.ProtoMajor = 2
	req.Header.Set("Content-Type", "application/grpc")
	if !isGRPCRequest(req) {
		t.Fatal("HTTP/2 application/grpc must be recognized")
	}
}
