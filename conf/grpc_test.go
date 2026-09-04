package conf

import "testing"

func TestPublicGRPCEnabledByDefault(t *testing.T) {
	setPublicGRPCDefaults()
	if !PublicGRPCEnabled() {
		t.Fatal("public gRPC should be on by default alongside HTTP/2")
	}
	if !PublicGRPCPlaintextEnabled() {
		t.Fatal("plaintext H2C gRPC should be on by default for local proxies")
	}
	if PublicGRPCAddress() != "127.0.0.1" {
		t.Fatalf("PublicGRPCAddress()=%q, want loopback-only default", PublicGRPCAddress())
	}
	if PublicGRPCPort() != 50051 {
		t.Fatalf("PublicGRPCPort()=%d, want 50051", PublicGRPCPort())
	}
	SetPublicGRPCEnabledForTest(false)
	if PublicGRPCEnabled() {
		t.Fatal("SetPublicGRPCEnabledForTest(false) did not disable")
	}
	if PublicGRPCPlaintextEnabled() {
		t.Fatal("plaintext H2C must follow EnablePublicGRPC=false")
	}
	SetPublicGRPCEnabledForTest(true)
	SetPublicGRPCPlaintextForTest("127.0.0.1", 50051)
}
