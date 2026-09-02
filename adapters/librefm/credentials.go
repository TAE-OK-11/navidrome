package librefm

import (
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
)

func effectiveApiKey() string {
	if conf.Server.LibreFM.ApiKey != "" {
		return conf.Server.LibreFM.ApiKey
	}
	return consts.DefaultLibreFMApiKey
}

func effectiveSecret() string {
	if conf.Server.LibreFM.Secret != "" {
		return conf.Server.LibreFM.Secret
	}
	return consts.DefaultLibreFMSecret
}
