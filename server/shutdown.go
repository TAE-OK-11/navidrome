package server

import (
	"context"
	"errors"
	"net/http"
)

func shutdownHTTPServer(ctx context.Context, server *http.Server) error {
	shutdownErr := server.Shutdown(ctx)
	if shutdownErr == nil {
		return nil
	}

	closeErr := server.Close()
	if errors.Is(closeErr, http.ErrServerClosed) {
		closeErr = nil
	}
	return errors.Join(shutdownErr, closeErr)
}
