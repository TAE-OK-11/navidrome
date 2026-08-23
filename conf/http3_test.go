package conf

import "testing"

func TestHTTP3QuicheDefaults(t *testing.T) {
	// The configuration suite resets Viper's process-global state. Restore the
	// HTTP/3 defaults so this test remains independent of top-level test order.
	setHTTP3Defaults()

	if HTTP3Allow0RTT() {
		t.Fatal("HTTP3Allow0RTT()=true, early request data must remain disabled")
	}
	if HTTP3MaxConnections() < HTTP3MaxConnectionsPerIP() {
		t.Fatal("per-IP HTTP/3 connection limit exceeds the global limit")
	}
	if HTTP3MaxInFlightRequests() < 128 || HTTP3MaxInFlightRequests() > 4096 {
		t.Fatalf("HTTP3MaxInFlightRequests()=%d outside safe default range", HTTP3MaxInFlightRequests())
	}
	if HTTP3ConnectionRatePerSecond() <= 0 || HTTP3ConnectionBurst() < 0 {
		t.Fatal("invalid default HTTP/3 connection admission policy")
	}
	if HTTP3CongestionControl() != HTTP3CongestionControlBBR2 {
		t.Fatalf("HTTP3CongestionControl()=%q, want %q", HTTP3CongestionControl(), HTTP3CongestionControlBBR2)
	}
}

func TestValidHTTP3CongestionControl(t *testing.T) {
	for _, value := range []string{"bbr2", " BBR2 ", "cubic", "reno"} {
		if !ValidHTTP3CongestionControl(value) {
			t.Fatalf("ValidHTTP3CongestionControl(%q)=false", value)
		}
	}
	for _, value := range []string{"", "bbr3", "bbr2_gcongestion", "anything"} {
		if ValidHTTP3CongestionControl(value) {
			t.Fatalf("ValidHTTP3CongestionControl(%q)=true", value)
		}
	}
}
