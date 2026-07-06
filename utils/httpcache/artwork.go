package httpcache

import (
	"net/http"
	"time"
)

const ArtworkCacheControl = "public, max-age=315360000"

func SetArtworkHeaders(w http.ResponseWriter, r *http.Request, lastUpdate time.Time) bool {
	w.Header().Set("Cache-Control", ArtworkCacheControl)
	if lastUpdate.IsZero() {
		return false
	}

	w.Header().Set("Last-Modified", lastUpdate.Format(http.TimeFormat))
	if !notModifiedSince(r, lastUpdate) {
		return false
	}

	w.WriteHeader(http.StatusNotModified)
	return true
}

func notModifiedSince(r *http.Request, lastUpdate time.Time) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	value := r.Header.Get("If-Modified-Since")
	if value == "" {
		return false
	}
	since, err := http.ParseTime(value)
	if err != nil {
		return false
	}
	return !lastUpdate.Truncate(time.Second).After(since)
}
