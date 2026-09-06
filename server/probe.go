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
	path = strings.TrimSuffix(strings.ToLower(path), ".view")
	pingPath := strings.ToLower(base + "/ping")
	healthPath := strings.ToLower(base + consts.URLPathNativeAPI + "/health")
	restPingPath := strings.ToLower(base + consts.URLPathSubsonicAPI + "/ping")
	// Subsonic clients poll /rest/ping while streaming; treat it as a probe so
	// compression/JWT middleware stay off the hot path (auth still runs in
	// the Subsonic stack).
	return path == pingPath || path == healthPath || path == restPingPath
}

// shouldSkipJWTVerifier returns true for requests that authenticate outside the
// outer JWT middleware (Subsonic has its own stack) or that are pure probes.
func shouldSkipJWTVerifier(r *http.Request) bool {
	if isProbeRequest(r) || isNativeKeepAlivePath(r.URL.Path) {
		return true
	}
	if r == nil || r.URL == nil {
		return false
	}
	base := strings.TrimSuffix(conf.Server.BasePath, "/")
	path := strings.ToLower(strings.TrimSuffix(r.URL.Path, "/"))
	restPrefix := strings.ToLower(base + consts.URLPathSubsonicAPI)
	return path == restPrefix || strings.HasPrefix(path, restPrefix+"/")
}

func isNativeKeepAlivePath(path string) bool {
	path = strings.ToLower(strings.TrimSuffix(path, "/"))
	base := strings.ToLower(strings.TrimSuffix(conf.Server.BasePath, "/"))
	prefix := base + strings.ToLower(consts.URLPathNativeAPI) + "/keepalive"
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}
