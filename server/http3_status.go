package server

import (
	"sync/atomic"

	"github.com/navidrome/navidrome/conf"
)

// HTTP3Health summarizes the optional tokio-quiche transport for health checks.
type HTTP3Health struct {
	Enabled        bool   `json:"enabled"`
	Provider       string `json:"provider,omitempty"`
	CompanionReady bool   `json:"companionReady"`
}

var http3CompanionReady atomic.Bool

func setHTTP3CompanionReady(ready bool) {
	http3CompanionReady.Store(ready)
	if ready {
		http3CompanionUp.Set(1)
		return
	}
	http3CompanionUp.Set(0)
}

// HTTP3CompanionReady reports whether the tokio-quiche companion is accepting
// traffic. When HTTP/3 is disabled the value is always false.
func HTTP3CompanionReady() bool {
	return conf.HTTP3Enabled() && http3CompanionReady.Load()
}

// HTTP3HealthSnapshot returns a transport snapshot suitable for JSON APIs.
func HTTP3HealthSnapshot() HTTP3Health {
	health := HTTP3Health{
		Enabled:        conf.HTTP3Enabled(),
		CompanionReady: HTTP3CompanionReady(),
	}
	if health.Enabled {
		health.Provider = "tokio-quiche"
	}
	return health
}
