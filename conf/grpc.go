package conf

import "github.com/spf13/viper"

const publicGRPCConfigKey = "enablepublicgrpc"

const (
	publicGRPCAddressKey    = "publicgrpcaddress"
	publicGRPCPortKey       = "publicgrpcport"
	publicGRPCReflectionKey = "enablepublicgrpcreflection"
)

// PublicGRPCEnabled reports whether the public gRPC service is multiplexed
// onto the HTTP/2 (and HTTP/3-bridged) listener. Config: EnablePublicGRPC
// or ND_ENABLEPUBLICGRPC.
func PublicGRPCEnabled() bool {
	return viper.GetBool(publicGRPCConfigKey)
}

// SetPublicGRPCEnabledForTest is for unit tests only.
func SetPublicGRPCEnabledForTest(enabled bool) {
	viper.Set(publicGRPCConfigKey, enabled)
}

// PublicGRPCReflectionEnabled reports whether gRPC server reflection is
// registered on the public API. Off by default: reflection exposes RPC
// schemas without authentication. Enable for grpcurl/proxy discovery on
// trusted overlays only. Config: EnablePublicGRPCReflection or
// ND_ENABLEPUBLICGRPCREFLECTION.
func PublicGRPCReflectionEnabled() bool {
	return viper.GetBool(publicGRPCReflectionKey)
}

// SetPublicGRPCReflectionForTest is for unit tests only.
func SetPublicGRPCReflectionForTest(enabled bool) {
	viper.Set(publicGRPCReflectionKey, enabled)
}

// PublicGRPCAddress returns the bind address for the optional plaintext H2C
// listener that serves gRPC only (no REST) without TLS.
// It exists for WireGuard / private-overlay origins: WireGuard already
// encrypts, so a second TLS layer (H2/H3) is pure double-encryption overhead.
// Plaintext H2C (prior-knowledge, protocol grpc on the proxy side) removes
// it while the main ND_PORT listener keeps serving public TLS.
// Config: PublicGRPCAddress or ND_PUBLICGRPCADDRESS. Empty disables the
// listener; by default it binds loopback only.
func PublicGRPCAddress() string {
	return viper.GetString(publicGRPCAddressKey)
}

// PublicGRPCPort returns the TCP port for the plaintext H2C listener.
// Config: PublicGRPCPort or ND_PUBLICGRPCPORT. 0 disables the listener.
func PublicGRPCPort() int {
	return viper.GetInt(publicGRPCPortKey)
}

// PublicGRPCPlaintextEnabled reports whether the dedicated H2C listener
// should be started alongside the main (possibly TLS) listener.
func PublicGRPCPlaintextEnabled() bool {
	return PublicGRPCEnabled() && PublicGRPCPort() > 0 && PublicGRPCAddress() != ""
}

// SetPublicGRPCPlaintextForTest is for unit tests only.
func SetPublicGRPCPlaintextForTest(address string, port int) {
	viper.Set(publicGRPCAddressKey, address)
	viper.Set(publicGRPCPortKey, port)
}

func setPublicGRPCDefaults() {
	viper.SetDefault(publicGRPCConfigKey, true)
	// Loopback-only by default: safe, and maximally open for local
	// proxies/sidecars. WireGuard deployments override the address with
	// ND_PUBLICGRPCADDRESS=10.77.0.1 (or the overlay IP).
	viper.SetDefault(publicGRPCAddressKey, "127.0.0.1")
	viper.SetDefault(publicGRPCPortKey, 50051)
	viper.SetDefault(publicGRPCReflectionKey, false)
}

func init() {
	setPublicGRPCDefaults()
}
