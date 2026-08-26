package stream

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ErrTooManyTranscodes is returned by TranscodeLimiter.Acquire when the
// configured concurrency cap has been reached. Callers should translate this
// into an HTTP 429 response so well-behaved clients back off and retry.
var ErrTooManyTranscodes = errors.New("too many concurrent transcodes")

// RetryAfterSeconds is the value returned in the HTTP Retry-After header when
// a request is rejected with ErrTooManyTranscodes. Most transcodes finish well
// within this window, so retrying after this delay typically succeeds.
const RetryAfterSeconds = 5

// A retiring transcode normally releases its slot within a few milliseconds
// after the client closes the old stream. Briefly waiting for that handoff
// avoids turning an ordinary track change into a five-second 429 retry.
const transcodeSlotHandoffWait = 100 * time.Millisecond

// TranscodeLimiter gates the number of concurrent ffmpeg transcodes. It enforces
// both a global cap (to protect the host from process exhaustion) and an optional
// per-user cap (to keep one client from starving the others). Acquire permits a
// short, context-aware handoff from a retiring stream before returning 429.
type TranscodeLimiter interface {
	// Acquire reserves a slot for the given user. On success it returns a release
	// function that must be called exactly once when the transcode is done.
	// Calling release more than once is safe and idempotent.
	Acquire(ctx context.Context, user string) (release func(), err error)

	// Enabled reports whether the limiter actually enforces any cap. Callers
	// can use it to decide whether to bind ffmpeg's lifetime to the request
	// context so disconnects free slots quickly, rather than letting the
	// process drain to completion in the background.
	Enabled() bool
}

// NewTranscodeLimiter returns a limiter enforcing the given caps. Each cap is
// independent: a value of zero or less disables that cap. When both caps are
// disabled the limiter is a no-op.
func NewTranscodeLimiter(maxConcurrent, maxPerUser int) TranscodeLimiter {
	if maxConcurrent <= 0 && maxPerUser <= 0 {
		return noopLimiter{}
	}
	l := &transcodeLimiter{
		maxGlobal:  maxConcurrent,
		maxPerUser: maxPerUser,
		changed:    make(chan struct{}),
	}
	if maxPerUser > 0 {
		l.perUser = make(map[string]int)
	}
	return l
}

// releasingReadCloser wraps an io.ReadCloser so that closing it also releases
// the limiter slot exactly once. release must be the function returned by
// TranscodeLimiter.Acquire; its own idempotency makes double-Close safe too.
type releasingReadCloser struct {
	io.ReadCloser
	release func()
}

func (r *releasingReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.release()
	return err
}

type noopLimiter struct{}

func (noopLimiter) Acquire(context.Context, string) (func(), error) {
	return func() {}, nil
}

func (noopLimiter) Enabled() bool { return false }

type transcodeLimiter struct {
	maxGlobal  int
	maxPerUser int

	mu           sync.Mutex
	globalActive int
	perUser      map[string]int
	changed      chan struct{}
}

func (*transcodeLimiter) Enabled() bool { return true }

func (l *transcodeLimiter) Acquire(ctx context.Context, user string) (func(), error) {
	var timer *time.Timer
	var timeout <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		l.mu.Lock()
		if l.availableLocked(user) {
			l.reserveLocked(user)
			l.mu.Unlock()
			return l.releaseFunc(user), nil
		}
		changed := l.changed
		l.mu.Unlock()
		if timer == nil {
			timer = time.NewTimer(transcodeSlotHandoffWait)
			timeout = timer.C
		}

		select {
		case <-changed:
			// A release broadcasts to every waiter. Recheck both limits under
			// the mutex so exactly the allowed number can reserve a slot.
		case <-timeout:
			return nil, ErrTooManyTranscodes
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (l *transcodeLimiter) availableLocked(user string) bool {
	perUserActive := l.maxPerUser > 0 && user != ""
	if perUserActive && l.perUser[user] >= l.maxPerUser {
		return false
	}
	return l.maxGlobal <= 0 || l.globalActive < l.maxGlobal
}

func (l *transcodeLimiter) reserveLocked(user string) {
	if l.maxGlobal > 0 {
		l.globalActive++
	}
	if l.maxPerUser > 0 && user != "" {
		l.perUser[user]++
	}
}

func (l *transcodeLimiter) releaseFunc(user string) func() {
	var released atomic.Bool
	return func() {
		if !released.CompareAndSwap(false, true) {
			return
		}
		l.mu.Lock()
		if l.maxGlobal > 0 {
			l.globalActive--
		}
		if l.maxPerUser > 0 && user != "" {
			l.perUser[user]--
			if l.perUser[user] <= 0 {
				delete(l.perUser, user)
			}
		}
		close(l.changed)
		l.changed = make(chan struct{})
		l.mu.Unlock()
	}
}
