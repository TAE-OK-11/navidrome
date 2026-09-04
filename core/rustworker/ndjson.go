package rustworker

import (
	"errors"
	"testing"

	"github.com/navidrome/navidrome/log"
)

// AllowLegacyNDJSON is true only inside `go test`. Production internal IPC is
// gRPC-only. NDJSON stdin/stdout remains as a test fallback because StartGRPC
// is skipped in tests to avoid hanging on leftover stdout pipes.
var forceLegacyNDJSON *bool

// SetLegacyNDJSONForTest overrides AllowLegacyNDJSON. Pass nil to reset.
func SetLegacyNDJSONForTest(force *bool) {
	forceLegacyNDJSON = force
}

func AllowLegacyNDJSON() bool {
	if forceLegacyNDJSON != nil {
		return *forceLegacyNDJSON
	}
	return testing.Testing()
}

// PreferGRPC reports whether a gRPC result should be used as-is (success or
// hard failure) instead of falling back to NDJSON.
func PreferGRPC(err, unavailable error) bool {
	if !errors.Is(err, unavailable) {
		return true
	}
	return !AllowLegacyNDJSON()
}

// LogGRPCUnavailable records that a Rust gRPC worker could not be started.
func LogGRPCUnavailable(name string, err error) {
	if AllowLegacyNDJSON() {
		log.Warn("Rust "+name+" gRPC worker unavailable; using NDJSON fallback", err)
		return
	}
	log.Error("Rust "+name+" gRPC worker unavailable", err)
}
