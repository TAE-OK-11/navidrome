package server

import (
	"context"

	"github.com/navidrome/navidrome/utils/neterr"
)

// IsExpectedTransportError reports whether a response ended because the
// client disconnected. Callers can keep normal disconnects out of warning logs.
func IsExpectedTransportError(ctx context.Context, err error) bool {
	return neterr.IsExpectedClientDisconnect(ctx, err)
}
