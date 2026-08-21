//go:build linux

package server

import (
	"errors"
	"net"
	"sync"
)

// inheritedConnListener lets net/http retain its mature HTTP/2 lifecycle and
// graceful-shutdown behavior while accepting only connections that Navidrome
// itself creates and passes to the tokio-quiche child with AF_UNIX socketpair.
// There is no bind/connect/accept path visible outside the process tree.
type inheritedConnListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newInheritedConnListener() *inheritedConnListener {
	return &inheritedConnListener{
		conns: make(chan net.Conn, 4),
		done:  make(chan struct{}),
	}
}

func (l *inheritedConnListener) add(conn net.Conn) error {
	select {
	case <-l.done:
		return net.ErrClosed
	case l.conns <- conn:
		return nil
	}
}

func (l *inheritedConnListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	case conn := <-l.conns:
		if conn == nil {
			return nil, errors.New("nil inherited HTTP/3 bridge connection")
		}
		return conn, nil
	}
}

func (l *inheritedConnListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *inheritedConnListener) Addr() net.Addr {
	return inheritedBridgeAddr{}
}

type inheritedBridgeAddr struct{}

func (inheritedBridgeAddr) Network() string { return "unix" }
func (inheritedBridgeAddr) String() string  { return "navidrome-h3-inherited" }
