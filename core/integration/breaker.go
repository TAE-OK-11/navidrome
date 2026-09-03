package integration

import (
	"sync"
	"time"
)

const (
	breakerFailThreshold = 5
	breakerOpenFor       = 30 * time.Second
	breakerHalfOpenMax   = 1
)

type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

type circuitBreaker struct {
	mu               sync.Mutex
	state            breakerState
	consecutiveFails int
	openedAt         time.Time
	halfOpenInFlight int
}

func (b *circuitBreaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case breakerOpen:
		if time.Since(b.openedAt) >= breakerOpenFor {
			b.state = breakerHalfOpen
			b.halfOpenInFlight = 1
			return true
		}
		return false
	case breakerHalfOpen:
		if b.halfOpenInFlight >= breakerHalfOpenMax {
			return false
		}
		b.halfOpenInFlight++
		return true
	default:
		return true
	}
}

func (b *circuitBreaker) success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecutiveFails = 0
	b.halfOpenInFlight = 0
	b.state = breakerClosed
}

func (b *circuitBreaker) failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecutiveFails++
	if b.state == breakerHalfOpen || b.consecutiveFails >= breakerFailThreshold {
		b.state = breakerOpen
		b.openedAt = time.Now()
		b.halfOpenInFlight = 0
	}
}
