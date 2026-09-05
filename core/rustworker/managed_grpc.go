package rustworker

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/navidrome/navidrome/log"
	"google.golang.org/grpc"
)

const DefaultMaxWorkerRestarts = 3

// ErrWorkerUnavailable is returned when the Rust gRPC worker cannot be started.
var ErrWorkerUnavailable = errors.New("gRPC worker unavailable")

// ManagedGRPCConfig describes how to launch and health-check a Rust gRPC worker.
type ManagedGRPCConfig struct {
	Name     string
	Listen   string
	ExtraEnv []string
	Resolve  func() (string, error)
	Health   func(context.Context, *grpc.ClientConn) error
}

// ManagedGRPC hosts one Rust gRPC worker process and recreates it after crashes.
type ManagedGRPC struct {
	cfg ManagedGRPCConfig

	mu       sync.Mutex
	proc     *GRPCProcess
	restarts int
	closed   bool
}

// NewManagedGRPC returns a worker host that is not started until Conn is called.
func NewManagedGRPC(cfg ManagedGRPCConfig) *ManagedGRPC {
	return &ManagedGRPC{cfg: cfg}
}

// Conn returns a live gRPC connection, starting the worker on first use.
func (m *ManagedGRPC) Conn() (*grpc.ClientConn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, fmt.Errorf("Rust %s gRPC worker is closed", m.cfg.Name)
	}
	if m.proc != nil {
		return m.proc.Conn, nil
	}
	if err := m.startLocked(); err != nil {
		return nil, err
	}
	return m.proc.Conn, nil
}

// Invalidate closes the current worker so the next Conn call starts a fresh process.
func (m *ManagedGRPC) Invalidate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeLocked()
}

// Close shuts down the worker permanently.
func (m *ManagedGRPC) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.closeLocked()
}

func (m *ManagedGRPC) closeLocked() {
	if m.proc == nil {
		return
	}
	m.proc.Close()
	m.proc = nil
}

func (m *ManagedGRPC) startLocked() error {
	if m.cfg.Resolve == nil {
		return fmt.Errorf("Rust %s gRPC worker has no binary resolver", m.cfg.Name)
	}
	binary, err := m.cfg.Resolve()
	if err != nil {
		return fmt.Errorf("resolving Rust %s worker: %w", m.cfg.Name, err)
	}
	listen := m.cfg.Listen
	if listen == "" {
		listen = DefaultListenAddr("navidrome-" + m.cfg.Name)
	}
	proc, err := StartGRPC(context.Background(), binary, listen, m.cfg.ExtraEnv)
	if err != nil {
		if errors.Is(err, ErrSkippedInTests) {
			return ErrWorkerUnavailable
		}
		LogGRPCUnavailable(m.cfg.Name, err)
		return err
	}
	if m.cfg.Health != nil {
		healthCtx, cancel := context.WithTimeout(context.Background(), DefaultGRPCDialTimeout)
		err := m.cfg.Health(healthCtx, proc.Conn)
		cancel()
		if err != nil {
			proc.Close()
			LogGRPCUnavailable(m.cfg.Name, err)
			return fmt.Errorf("Rust %s worker health: %w", m.cfg.Name, err)
		}
	}
	m.proc = proc
	go m.watchProcess()
	if proc.Cmd != nil && proc.Cmd.Process != nil {
		log.Info("Rust "+m.cfg.Name+" gRPC worker ready", "pid", proc.Cmd.Process.Pid, "listen", proc.Addr)
	} else {
		log.Info("Rust "+m.cfg.Name+" gRPC worker ready", "listen", proc.Addr)
	}
	return nil
}

func (m *ManagedGRPC) watchProcess() {
	m.mu.Lock()
	proc := m.proc
	m.mu.Unlock()
	if proc == nil || proc.Cmd == nil {
		return
	}
	_ = proc.Cmd.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.proc != proc {
		return
	}
	m.proc = nil
	if m.restarts >= DefaultMaxWorkerRestarts {
		log.Error("Rust "+m.cfg.Name+" gRPC worker restart limit reached", "attempts", m.restarts)
		return
	}
	m.restarts++
	if err := m.startLocked(); err != nil {
		log.Error("Rust "+m.cfg.Name+" gRPC worker restart failed", "attempt", m.restarts, err)
		return
	}
	log.Info("Rust "+m.cfg.Name+" gRPC worker restarted", "attempt", m.restarts)
}

// CallGRPC executes fn against the managed connection, retrying once after a
// transport failure invalidates the worker.
func CallGRPC[T any](m *ManagedGRPC, ctx context.Context, fn func(context.Context, *grpc.ClientConn) (T, error)) (T, error) {
	var zero T
	if m == nil {
		return zero, errors.New("managed gRPC worker is nil")
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		conn, err := m.Conn()
		if err != nil {
			return zero, err
		}
		result, err := fn(ctx, conn)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !IsTransportFailure(err) || attempt > 0 {
			return zero, err
		}
		m.Invalidate()
	}
	return zero, lastErr
}
