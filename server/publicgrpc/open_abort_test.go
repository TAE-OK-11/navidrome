package publicgrpc

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/navidrome/navidrome/server/publicgrpc/gen"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type abortingOpener struct{ stubInvoker }

func (abortingOpener) Open(context.Context, string, url.Values, string, bool, http.ResponseWriter) error {
	panic(http.ErrAbortHandler)
}

func TestOpenConvertsHTTPAbortToGRPCFailure(t *testing.T) {
	s := &Service{invoker: abortingOpener{}}
	// A nil stream catches any attempt to send a successful Final after abort.
	err := s.openSubsonic(context.Background(), &gen.OpenRequest{Path: "stream"}, nil, "user")
	if status.Code(err) != codes.Internal {
		t.Fatalf("got %v, want Internal", err)
	}
}
