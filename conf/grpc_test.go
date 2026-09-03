package conf

import "testing"

func TestPublicGRPCEnabledByDefault(t *testing.T) {
	setPublicGRPCDefaults()
	if !PublicGRPCEnabled() {
		t.Fatal("public gRPC should be on by default alongside HTTP/2")
	}
	SetPublicGRPCEnabledForTest(false)
	if PublicGRPCEnabled() {
		t.Fatal("SetPublicGRPCEnabledForTest(false) did not disable")
	}
	SetPublicGRPCEnabledForTest(true)
}
