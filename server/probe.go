package server

import (
	"net/http"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
)

func isProbeRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		return false
	}
	base := strings.TrimSuffix(conf.Server.BasePath, "/")
	pingPath := base + "/ping"
	healthPath := base + consts.URLPathNativeAPI + "/health"
	return path == pingPath || path == healthPath
}
