package errmap

import (
	"errors"
	"testing"

	"github.com/navidrome/navidrome/server/subsonic/responses"
	"google.golang.org/grpc/codes"
)

type stubCoded struct {
	code int32
}

func (s stubCoded) Error() string       { return "stub" }
func (s stubCoded) SubsonicCode() int32 { return s.code }

func TestGRPCCode(t *testing.T) {
	if GRPCCode(nil) != codes.OK {
		t.Fatal("nil must be OK")
	}
	if GRPCCode(errors.New(`endpoint "getAlbum" is not streamable via Open`)) != codes.FailedPrecondition {
		t.Fatal("whitelist errors must be FailedPrecondition")
	}
	if GRPCCode(stubCoded{code: responses.ErrorDataNotFound}) != codes.NotFound {
		t.Fatal("not found mapping")
	}
}
