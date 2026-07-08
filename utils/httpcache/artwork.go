package httpcache

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const ArtworkCacheControl = "public, max-age=315360000"

func SetArtworkHeaders(w http.ResponseWriter, r *http.Request, lastUpdate time.Time) bool {
	w.Header().Set("Cache-Control", ArtworkCacheControl)
	if lastUpdate.IsZero() {
		return false
	}

	etag := artworkETag(lastUpdate)
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", lastUpdate.Format(http.TimeFormat))
	if !notModified(r, lastUpdate, etag) {
		return false
	}

	w.WriteHeader(http.StatusNotModified)
	return true
}

func artworkETag(lastUpdate time.Time) string {
	return `"artwork-` + strconv.FormatInt(lastUpdate.UnixNano(), 36) + `"`
}

func notModified(r *http.Request, lastUpdate time.Time, etag string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if match := r.Header.Get("If-None-Match"); match != "" {
		return etagMatches(match, etag)
	}
	return notModifiedSince(r, lastUpdate)
}

func etagMatches(header, etag string) bool {
	for part := range strings.SplitSeq(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || part == etag {
			return true
		}
	}
	return false
}

func notModifiedSince(r *http.Request, lastUpdate time.Time) bool {
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
