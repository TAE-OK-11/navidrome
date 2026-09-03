package rustworker

import (
	"errors"
	"testing"
)

func TestAllowLegacyNDJSONInTests(t *testing.T) {
	if !AllowLegacyNDJSON() {
		t.Fatal("go test must keep NDJSON as a gRPC-skip fallback")
	}
}

func TestPreferGRPC(t *testing.T) {
	unavailable := errors.New("unavailable")
	if !PreferGRPC(nil, unavailable) {
		t.Fatal("success must prefer gRPC")
	}
	if !PreferGRPC(errors.New("rpc failed"), unavailable) {
		t.Fatal("hard gRPC failure must not fall back to NDJSON")
	}
	if PreferGRPC(unavailable, unavailable) {
		t.Fatal("unavailable gRPC in tests must allow NDJSON fallback")
	}
}
