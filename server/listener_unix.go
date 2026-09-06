//go:build unix

package server

import (
	"syscall"

	"golang.org/x/sys/unix"
)

const tcpListenerSocketBuffer = 1 << 20 // 1 MiB

// configureTCPListenerSocket tunes accepted TCP sockets for low-latency local
// reverse-proxy hops (same host as nginx/caddy/middleware) while remaining
// safe for remote clients.
func configureTCPListenerSocket(_network, _address string, c syscall.RawConn) error {
	var sockErr error
	if err := c.Control(func(fd uintptr) {
		if err := unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_NODELAY, 1); err != nil {
			sockErr = err
			return
		}
		// Best-effort buffer bumps; ignore failures on constrained hosts.
		_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF, tcpListenerSocketBuffer)
		_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUF, tcpListenerSocketBuffer)
	}); err != nil {
		return err
	}
	return sockErr
}
