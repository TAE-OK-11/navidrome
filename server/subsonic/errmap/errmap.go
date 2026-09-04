package errmap

import (
	"errors"
	"net/http"
	"strings"

	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/req"
	"google.golang.org/grpc/codes"
)

type codedSubsonicError interface {
	error
	SubsonicCode() int32
}

// GRPCCode maps Subsonic handler errors to gRPC status codes for the public API.
func GRPCCode(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	if errors.Is(err, req.ErrMissingParam) {
		return codes.InvalidArgument
	}
	var coded codedSubsonicError
	if errors.As(err, &coded) {
		switch coded.SubsonicCode() {
		case responses.ErrorDataNotFound:
			return codes.NotFound
		case responses.ErrorAuthenticationFail, responses.ErrorAuthorizationFail,
			responses.ErrorTokenAuthLDAP, responses.ErrorAuthNotSupported,
			responses.ErrorConflictingAuth, responses.ErrorInvalidAPIKey:
			return codes.PermissionDenied
		case responses.ErrorMissingParameter:
			return codes.InvalidArgument
		case responses.ErrorClientTooOld, responses.ErrorServerTooOld:
			return codes.FailedPrecondition
		}
	}
	if strings.Contains(err.Error(), "unknown Subsonic endpoint") {
		return codes.NotFound
	}
	if strings.Contains(err.Error(), "not streamable") {
		return codes.FailedPrecondition
	}
	if strings.Contains(err.Error(), "authenticated user") && strings.Contains(err.Error(), "not found") {
		return codes.PermissionDenied
	}
	if errors.Is(err, http.ErrBodyNotAllowed) {
		return codes.FailedPrecondition
	}
	return codes.Internal
}
