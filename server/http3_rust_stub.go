//go:build !linux

package server

import (
	"context"
	"errors"
	"net/http"
)

func newRustHTTP3Runtime(context.Context, string, http.Handler, string, string) (http3Service, error) {
	return nil, errors.New("the tokio-quiche HTTP/3 provider currently supports Linux only")
}
