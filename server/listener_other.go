//go:build !unix

package server

import "syscall"

func configureTCPListenerSocket(_network, _address string, _c syscall.RawConn) error {
	return nil
}
