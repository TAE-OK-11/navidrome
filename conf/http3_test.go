package conf

import "testing"

func TestHTTP3MigrationDefaults(t *testing.T) {
	// The configuration suite resets Viper's process-global state. Restore the
	// HTTP/3 defaults so this test remains independent of top-level test order.
	setHTTP3Defaults()

	if HTTP3Provider() != HTTP3ProviderTokioQuiche {
		t.Fatalf("HTTP3Provider()=%q, want %q", HTTP3Provider(), HTTP3ProviderTokioQuiche)
	}
	if HTTP3Allow0RTT() {
		t.Fatal("HTTP3Allow0RTT()=true, early request data must remain disabled")
	}
	if HTTP3MaxConnections() < HTTP3MaxConnectionsPerIP() {
		t.Fatal("per-IP HTTP/3 connection limit exceeds the global limit")
	}
	if HTTP3ConnectionRatePerSecond() <= 0 || HTTP3ConnectionBurst() < 0 {
		t.Fatal("invalid default HTTP/3 connection admission policy")
	}
}
