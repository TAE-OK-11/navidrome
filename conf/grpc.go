package conf

import "github.com/spf13/viper"

const publicGRPCConfigKey = "enablepublicgrpc"

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

func setPublicGRPCDefaults() {
	viper.SetDefault(publicGRPCConfigKey, true)
}

func init() {
	setPublicGRPCDefaults()
}
