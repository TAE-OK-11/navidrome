package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type signBenchVector struct {
	Secret   string            `json:"secret"`
	Params   map[string]string `json:"params"`
	Expected string            `json:"expected"`
}

func loadSignBenchVector(b *testing.B) signBenchVector {
	b.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		b.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "../../rust/integration/testdata/sign_vector.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var vector signBenchVector
	if err := json.Unmarshal(raw, &vector); err != nil {
		b.Fatal(err)
	}
	return vector
}

// BenchmarkIntegrationGatewaySign exercises the outbound integration gateway
// signing path (gRPC worker when ND_INTEGRATIONWORKERPATH is set, else Go fallback).
func BenchmarkIntegrationGatewaySign(b *testing.B) {
	vector := loadSignBenchVector(b)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		sig := Sign(ctx, vector.Params, vector.Secret)
		if sig != vector.Expected {
			b.Fatalf("sig=%s want=%s", sig, vector.Expected)
		}
	}
}
