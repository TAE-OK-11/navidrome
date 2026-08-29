package nativeapi

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/server"
)

type healthResponse struct {
	Status  string             `json:"status"`
	Version string             `json:"version"`
	HTTP3   server.HTTP3Health `json:"http3"`
}

var (
	healthBodies     map[bool][]byte
	healthBodiesOnce sync.Once
)

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	ready := conf.HTTP3Enabled() && server.HTTP3CompanionReady()
	_, _ = w.Write(healthBodyFor(ready))
}

func healthBodyFor(companionReady bool) []byte {
	healthBodiesOnce.Do(func() {
		healthBodies = map[bool][]byte{
			false: mustMarshalHealth(false),
			true:  mustMarshalHealth(true),
		}
	})
	return healthBodies[companionReady]
}

func mustMarshalHealth(companionReady bool) []byte {
	enabled := conf.HTTP3Enabled()
	health := server.HTTP3Health{
		Enabled:        enabled,
		CompanionReady: enabled && companionReady,
	}
	if enabled {
		health.Provider = "tokio-quiche"
	}
	body, err := json.Marshal(healthResponse{
		Status:  "ok",
		Version: consts.Version,
		HTTP3:   health,
	})
	if err != nil {
		panic(err)
	}
	return body
}
