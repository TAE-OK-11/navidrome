package nativeapi

import (
	"encoding/json"
	"net/http"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/server"
)

type healthResponse struct {
	Status  string         `json:"status"`
	Version string         `json:"version"`
	HTTP3   server.HTTP3Health `json:"http3"`
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	response := healthResponse{
		Status:  "ok",
		Version: consts.Version,
		HTTP3:   server.HTTP3HealthSnapshot(),
	}
	if !conf.HTTP3Enabled() {
		response.HTTP3.Enabled = false
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(response)
}
