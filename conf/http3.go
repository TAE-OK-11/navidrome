package conf

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	http3ConfigKey            = "enablehttp3"
	http3Allow0RTTConfigKey   = "http3allow0rtt"
	http3ProviderConfigKey    = "http3provider"
	http3GatewayPathKey       = "http3gatewaypath"
	http3AltSvcMaxAgeKey      = "http3altsvcmaxage"
	http3QlogDirKey           = "http3qlogdir"
	http3MaxConnectionsKey    = "http3maxconnections"
	http3MaxPerIPKey          = "http3maxconnectionsperip"
	http3RatePerSecondKey     = "http3connectionratepersecond"
	http3ConnectionBurstKey   = "http3connectionburst"
	http3CongestionControlKey = "http3congestioncontrol"
)

const (
	HTTP3ProviderQuicGo      = "quic-go"
	HTTP3ProviderTokioQuiche = "tokio-quiche"
)

const (
	HTTP3CongestionControlBBR2  = "bbr2"
	HTTP3CongestionControlCubic = "cubic"
	HTTP3CongestionControlReno  = "reno"
)

// HTTP3Enabled reports whether the optional HTTP/3 listener should be started.
// It can be configured with EnableHTTP3 in the config file or ND_ENABLEHTTP3
// in the environment.
func HTTP3Enabled() bool {
	return viper.GetBool(http3ConfigKey)
}

// HTTP3Allow0RTT reports whether QUIC session resumption may accept replayable
// early data. It deliberately remains false during and after the provider
// migration: resumed handshakes are supported, but request bytes are never
// exposed to Navidrome before the TLS handshake completes.
func HTTP3Allow0RTT() bool {
	return false
}

// HTTP3Provider selects the HTTP/3 provider. tokio-quiche is the production
// default; quic-go is retained only as a rollback target during validation.
func HTTP3Provider() string {
	return viper.GetString(http3ProviderConfigKey)
}

// HTTP3GatewayPath returns an explicit path to the Rust companion. An empty
// value resolves to a navidrome-h3 binary next to the Navidrome executable.
func HTTP3GatewayPath() string {
	return viper.GetString(http3GatewayPathKey)
}

func HTTP3AltSvcMaxAge() time.Duration {
	return viper.GetDuration(http3AltSvcMaxAgeKey)
}

func HTTP3QlogDir() string {
	return viper.GetString(http3QlogDirKey)
}

func HTTP3MaxConnections() int {
	return viper.GetInt(http3MaxConnectionsKey)
}

func HTTP3MaxConnectionsPerIP() int {
	return viper.GetInt(http3MaxPerIPKey)
}

func HTTP3ConnectionRatePerSecond() float64 {
	return viper.GetFloat64(http3RatePerSecondKey)
}

func HTTP3ConnectionBurst() int {
	return viper.GetInt(http3ConnectionBurstKey)
}

// HTTP3CongestionControl returns the congestion controller requested for the
// tokio-quiche transport. quiche 0.29.x maps both "bbr" and "bbr2" to its
// gcongestion BBRv2 implementation; Navidrome exposes the unambiguous "bbr2"
// spelling and keeps Cubic/Reno as explicit rollback choices.
func HTTP3CongestionControl() string {
	return strings.ToLower(strings.TrimSpace(viper.GetString(http3CongestionControlKey)))
}

func ValidHTTP3CongestionControl(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case HTTP3CongestionControlBBR2, HTTP3CongestionControlCubic, HTTP3CongestionControlReno:
		return true
	default:
		return false
	}
}

func setHTTP3Defaults() {
	viper.SetDefault(http3ConfigKey, false)
	// 0-RTT request data is intentionally disabled. Keep the legacy key at
	// false during the migration so existing configurations remain parseable.
	viper.SetDefault(http3Allow0RTTConfigKey, false)
	viper.SetDefault(http3ProviderConfigKey, HTTP3ProviderTokioQuiche)
	viper.SetDefault(http3GatewayPathKey, "")
	viper.SetDefault(http3AltSvcMaxAgeKey, 5*time.Minute)
	viper.SetDefault(http3QlogDirKey, "")
	viper.SetDefault(http3MaxConnectionsKey, 4096)
	viper.SetDefault(http3MaxPerIPKey, 128)
	viper.SetDefault(http3RatePerSecondKey, 50.0)
	viper.SetDefault(http3ConnectionBurstKey, 100)
	viper.SetDefault(http3CongestionControlKey, HTTP3CongestionControlBBR2)
}

func init() {
	setHTTP3Defaults()
}
