package rustworker

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/navidrome/navidrome/log"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
)

const DefaultMaxWorkerRestarts = 3

// restartBudgetWindow resets the crash-restart counter when the previous
// worker lived at least this long, so infrequent crashes do not permanently
// disable the companion after three lifetime failures.
const restartBudgetWindow = time.Minute

const (
	restartBaseDelay = 50 * time.Millisecond
	restartMaxDelay  = 2 * time.Second
)

// ErrWorkerUnavailable is returned when the Rust gRPC worker cannot be started.
var ErrWorkerUnavailable = errors.New("gRPC worker unavailable")

// ManagedGRPCConfig describes how to launch and health-check a Rust gRPC worker.
type ManagedGRPCConfig struct {
	Name       string
	Listen     string
	ExtraEnv   []string
	ExtraEnvFn func() []string
	Resolve    func() (string, error)
	Health     func(context.Context, *grpc.ClientConn) error
}

// ManagedGRPC hosts one Rust gRPC worker process and recreates it after crashes.
type ManagedGRPC struct {
	cfg ManagedGRPCConfig

	mu        sync.Mutex
	proc      *GRPCProcess
	restarts  int
	lastStart time.Time
	closed    bool
	startSF   singleflight.Group
}

// NewManagedGRPC returns a worker host that is not started until Conn is called.
func NewManagedGRPC(cfg ManagedGRPCConfig) *ManagedGRPC {
	return &ManagedGRPC{cfg: cfg}
}

// Conn returns a live gRPC connection, starting the worker on first use.
// Concurrent callers share a single start via singleflight; the mutex is not
// held across process spawn or the health RPC.
func (m *ManagedGRPC) Conn() (*grpc.ClientConn, error) {
	if m == nil {
		return nil, errors.New("managed gRPC worker is nil")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("Rust %s gRPC worker is closed", m.cfg.Name)
	}
	if m.proc != nil {
		if !processGone(m.proc) {
			conn := m.proc.Conn
			m.mu.Unlock()
			return conn, nil
		}
		// watchProcess has not cleared yet; drop the dead handle so we respawn.
		m.closeLocked()
	}
	m.mu.Unlock()

	v, err, _ := m.startSF.Do("start", func() (any, error) {
		return m.ensureStarted()
	})
	if err != nil {
		return nil, err
	}
	return v.(*grpc.ClientConn), nil
}

// Warm starts the worker in the background so the first RPC avoids cold spawn.
// Safe to call concurrently; errors are logged and retried on the next Conn.
// No-op when a process was already Adopted or Conn-started.
func (m *ManagedGRPC) Warm() {
	if m == nil {
		return
	}
	go func() {
		if _, err := m.Conn(); err != nil && !errors.Is(err, ErrWorkerUnavailable) && !errors.Is(err, ErrSkippedInTests) {
			log.Debug("Rust "+m.cfg.Name+" gRPC warm failed", err)
		}
	}()
}

// Adopt installs an already-started, health-checked process into the manager.
// Returns true when ownership transfers; on false the caller must Close(proc).
// Used to reuse the startup preflight worker instead of spawn-kill-respawn.
func (m *ManagedGRPC) Adopt(proc *GRPCProcess) bool {
	if m == nil || proc == nil || proc.Conn == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.proc != nil {
		return false
	}
	m.proc = proc
	m.lastStart = time.Now()
	go m.watchProcess()
	if proc.Cmd != nil && proc.Cmd.Process != nil {
		log.Info("Rust "+m.cfg.Name+" gRPC worker adopted", "pid", proc.Cmd.Process.Pid, "listen", proc.Addr)
	} else {
		log.Info("Rust "+m.cfg.Name+" gRPC worker adopted", "listen", proc.Addr)
	}
	return true
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

// ensureStarted launches the worker when none is live. Must only run under
// startSF so concurrent Conn callers coalesce.
func (m *ManagedGRPC) ensureStarted() (*grpc.ClientConn, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("Rust %s gRPC worker is closed", m.cfg.Name)
	}
	if m.proc != nil {
		if !processGone(m.proc) {
			conn := m.proc.Conn
			m.mu.Unlock()
			return conn, nil
		}
		m.closeLocked()
	}
	m.mu.Unlock()

	proc, err := m.launch()
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		proc.Close()
		return nil, fmt.Errorf("Rust %s gRPC worker is closed", m.cfg.Name)
	}
	if m.proc != nil {
		// Another installer won (e.g. watchProcess); drop ours.
		proc.Close()
		return m.proc.Conn, nil
	}
	m.proc = proc
	m.lastStart = time.Now()
	go m.watchProcess()
	if proc.Cmd != nil && proc.Cmd.Process != nil {
		log.Info("Rust "+m.cfg.Name+" gRPC worker ready", "pid", proc.Cmd.Process.Pid, "listen", proc.Addr)
	} else {
		log.Info("Rust "+m.cfg.Name+" gRPC worker ready", "listen", proc.Addr)
	}
	return m.proc.Conn, nil
}

// launch starts the process and runs Health without holding m.mu.
func (m *ManagedGRPC) launch() (*GRPCProcess, error) {
	if m.cfg.Resolve == nil {
		return nil, fmt.Errorf("Rust %s gRPC worker has no binary resolver", m.cfg.Name)
	}
	binary, err := m.cfg.Resolve()
	if err != nil {
		return nil, fmt.Errorf("resolving Rust %s worker: %w", m.cfg.Name, err)
	}
	listen := m.cfg.Listen
	if listen == "" {
		listen = DefaultListenAddr("navidrome-" + m.cfg.Name)
	}
	proc, err := StartGRPC(context.Background(), binary, listen, m.workerEnv())
	if err != nil {
		if errors.Is(err, ErrSkippedInTests) {
			return nil, ErrWorkerUnavailable
		}
		LogGRPCUnavailable(m.cfg.Name, err)
		return nil, err
	}
	if m.cfg.Health != nil {
		healthCtx, cancel := context.WithTimeout(context.Background(), DefaultGRPCHealthTimeout)
		err := m.cfg.Health(healthCtx, proc.Conn)
		cancel()
		if err != nil {
			proc.Close()
			LogGRPCUnavailable(m.cfg.Name, err)
			return nil, fmt.Errorf("Rust %s worker health: %w", m.cfg.Name, err)
		}
	}
	return proc, nil
}

func (m *ManagedGRPC) workerEnv() []string {
	if m.cfg.ExtraEnvFn == nil {
		return m.cfg.ExtraEnv
	}
	return append(append([]string(nil), m.cfg.ExtraEnv...), m.cfg.ExtraEnvFn()...)
}

func (m *ManagedGRPC) watchProcess() {
	m.mu.Lock()
	proc := m.proc
	m.mu.Unlock()
	if proc == nil || proc.Cmd == nil {
		return
	}
	_ = proc.Wait()

	m.mu.Lock()
	if m.closed || m.proc != proc {
		m.mu.Unlock()
		return
	}
	m.proc = nil
	if !m.lastStart.IsZero() && time.Since(m.lastStart) >= restartBudgetWindow {
		m.restarts = 0
	}
	if m.restarts >= DefaultMaxWorkerRestarts {
		log.Error("Rust "+m.cfg.Name+" gRPC worker restart limit reached", "attempts", m.restarts)
		m.mu.Unlock()
		return
	}
	m.restarts++
	attempt := m.restarts
	m.mu.Unlock()

	delay := restartDelay(attempt)
	if delay > 0 {
		time.Sleep(delay)
	}

	m.mu.Lock()
	if m.closed || m.proc != nil {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	_, err, _ := m.startSF.Do("start", func() (any, error) {
		return m.ensureStarted()
	})
	if err != nil {
		log.Error("Rust "+m.cfg.Name+" gRPC worker restart failed", "attempt", attempt, err)
		return
	}
	log.Info("Rust "+m.cfg.Name+" gRPC worker restarted", "attempt", attempt, "backoff", delay)
}

// processGone reports whether a managed process can no longer serve RPCs.
// ProcessState is set only after Wait returns, so this catches the window
// between exit and watchProcess clearing m.proc without probing signals.
func processGone(p *GRPCProcess) bool {
	if p == nil || p.Conn == nil {
		return true
	}
	if p.Cmd != nil && p.Cmd.ProcessState != nil {
		return true
	}
	return false
}

// restartDelay returns a jittered exponential backoff for crash restarts.
// attempt is 1-based (first restart after the initial start).
func restartDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := restartBaseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= restartMaxDelay {
			delay = restartMaxDelay
			break
		}
	}
	if delay > restartMaxDelay {
		delay = restartMaxDelay
	}
	// +/- 20% jitter avoids synchronized reconnects across workers.
	jitter := 1 + (rand.Float64()*0.4 - 0.2) //nolint:gosec // non-crypto reconnect jitter
	return time.Duration(float64(delay) * jitter)
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
		// Soft-fail: do not respawn if the caller already gave up.
		if ctx.Err() != nil {
			return zero, err
		}
		m.Invalidate()
	}
	return zero, lastErr
}
