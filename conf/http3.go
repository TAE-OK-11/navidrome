package conf

import "github.com/spf13/viper"

const http3ConfigKey = "enablehttp3"

// HTTP3Enabled reports whether the optional HTTP/3 listener should be started.
// It can be configured with EnableHTTP3 in the config file or ND_ENABLEHTTP3
// in the environment.
func HTTP3Enabled() bool {
	return viper.GetBool(http3ConfigKey)
}

func init() {
	viper.SetDefault(http3ConfigKey, false)
}
