package server

import (
	"testing"

	"github.com/navidrome/navidrome/conf"
)

func TestHTTP3HealthSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := HTTP3HealthSnapshot()
	if snapshot.Enabled != conf.HTTP3Enabled() {
		t.Fatalf("enabled=%v, want %v", snapshot.Enabled, conf.HTTP3Enabled())
	}
	if snapshot.CompanionReady {
		t.Fatal("companionReady should be false in unit tests")
	}
	if snapshot.Enabled && snapshot.Provider != "tokio-quiche" {
		t.Fatalf("provider=%q, want tokio-quiche", snapshot.Provider)
	}
}

func TestSetHTTP3CompanionReady(t *testing.T) {
	t.Parallel()

	setHTTP3CompanionReady(true)
	if !http3CompanionReady.Load() {
		t.Fatal("companion ready flag was not set")
	}
	setHTTP3CompanionReady(false)
	if http3CompanionReady.Load() {
		t.Fatal("companion ready flag was not cleared")
	}
}
