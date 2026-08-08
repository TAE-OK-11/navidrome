package main

import (
	"testing"
	"time"
)

func TestQuantile(t *testing.T) {
	values := []time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond, 4 * time.Millisecond}
	if got := quantile(values, 0.50); got != 2*time.Millisecond {
		t.Fatalf("p50=%s, want 2ms", got)
	}
	if got := quantile(values, 0.99); got != 4*time.Millisecond {
		t.Fatalf("p99=%s, want 4ms", got)
	}
}

func TestRegressionFailures(t *testing.T) {
	baseline := result{Requests: 1000, ErrorRate: 0.001, RequestsPerSecond: 100, P95Milliseconds: 10, P99Milliseconds: 20}
	candidate := result{Requests: 1000, ErrorRate: 0.001, RequestsPerSecond: 95, P95Milliseconds: 10.5, P99Milliseconds: 21}
	if failures := regressionFailures(baseline, candidate); len(failures) != 0 {
		t.Fatalf("unexpected failures: %v", failures)
	}

	candidate.TooEarly = 1
	candidate.RangeFailures = 1
	candidate.ErrorRate = 0.01
	candidate.RequestsPerSecond = 80
	candidate.P95Milliseconds = 12
	candidate.P99Milliseconds = 24
	if failures := regressionFailures(baseline, candidate); len(failures) != 6 {
		t.Fatalf("failure count=%d, want 6: %v", len(failures), failures)
	}
}
