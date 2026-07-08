package httpcache

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetArtworkHeadersNotModified(t *testing.T) {
	lastUpdate := time.Date(2026, 7, 6, 12, 30, 0, 456, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/cover", nil)
	req.Header.Set("If-Modified-Since", lastUpdate.Format(http.TimeFormat))
	rec := httptest.NewRecorder()

	notModified := SetArtworkHeaders(rec, req, lastUpdate)

	if !notModified {
		t.Fatal("expected not modified")
	}
	if rec.Code != http.StatusNotModified {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusNotModified)
	}
	if got := rec.Header().Get("Cache-Control"); got != ArtworkCacheControl {
		t.Fatalf("Cache-Control=%q, want %q", got, ArtworkCacheControl)
	}
	if got := rec.Header().Get("Last-Modified"); got != lastUpdate.Format(http.TimeFormat) {
		t.Fatalf("Last-Modified=%q, want %q", got, lastUpdate.Format(http.TimeFormat))
	}
	if got, want := rec.Header().Get("ETag"), artworkETag(lastUpdate); got != want {
		t.Fatalf("ETag=%q, want %q", got, want)
	}
}

func TestSetArtworkHeadersModified(t *testing.T) {
	lastUpdate := time.Date(2026, 7, 6, 12, 30, 1, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/cover", nil)
	req.Header.Set("If-Modified-Since", time.Date(2026, 7, 6, 12, 30, 0, 0, time.UTC).Format(http.TimeFormat))
	rec := httptest.NewRecorder()

	notModified := SetArtworkHeaders(rec, req, lastUpdate)

	if notModified {
		t.Fatal("expected modified response")
	}
	if got := rec.Header().Get("Cache-Control"); got != ArtworkCacheControl {
		t.Fatalf("Cache-Control=%q, want %q", got, ArtworkCacheControl)
	}
}

func TestSetArtworkHeadersETagNotModified(t *testing.T) {
	lastUpdate := time.Date(2026, 7, 6, 12, 30, 0, 456, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/cover", nil)
	req.Header.Set("If-None-Match", artworkETag(lastUpdate))
	rec := httptest.NewRecorder()

	notModified := SetArtworkHeaders(rec, req, lastUpdate)

	if !notModified {
		t.Fatal("expected not modified")
	}
	if rec.Code != http.StatusNotModified {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusNotModified)
	}
}

func TestSetArtworkHeadersETagTakesPrecedence(t *testing.T) {
	lastUpdate := time.Date(2026, 7, 6, 12, 30, 0, 456, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/cover", nil)
	req.Header.Set("If-None-Match", `"stale"`)
	req.Header.Set("If-Modified-Since", lastUpdate.Format(http.TimeFormat))
	rec := httptest.NewRecorder()

	notModified := SetArtworkHeaders(rec, req, lastUpdate)

	if notModified {
		t.Fatal("expected modified response when If-None-Match does not match")
	}
}
