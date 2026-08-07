package conf

import "github.com/spf13/viper"

const (
	http3ConfigKey         = "enablehttp3"
	http3Allow0RTTConfigKey = "http3allow0rtt"
)

// HTTP3Enabled reports whether the optional HTTP/3 listener should be started.
// It can be configured with EnableHTTP3 in the config file or ND_ENABLEHTTP3
// in the environment.
func HTTP3Enabled() bool {
	return viper.GetBool(http3ConfigKey)
}

// HTTP3Allow0RTT reports whether QUIC session resumption may accept replayable
// early data. The HTTP/3 server still applies a strict request-level replay
// guard, allowing only explicitly read-only API requests before the handshake
// completes.
func HTTP3Allow0RTT() bool {
	return viper.GetBool(http3Allow0RTTConfigKey)
}

func init() {
	viper.SetDefault(http3ConfigKey, false)
	viper.SetDefault(http3Allow0RTTConfigKey, true)
}
