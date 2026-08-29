package nativeapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/server"
)

func TestHealthHandler(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	healthHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusOK)
	}

	var response struct {
		Status  string             `json:"status"`
		Version string             `json:"version"`
		HTTP3   server.HTTP3Health `json:"http3"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "ok" {
		t.Fatalf("status=%q, want ok", response.Status)
	}
	if response.Version != consts.Version {
		t.Fatalf("version=%q, want %q", response.Version, consts.Version)
	}
	if response.HTTP3.CompanionReady {
		t.Fatal("companionReady should be false when HTTP/3 is disabled in tests")
	}
}

func TestHealthBodyForUsesCache(t *testing.T) {
	t.Parallel()

	first := healthBodyFor(false)
	second := healthBodyFor(false)
	if string(first) != string(second) {
		t.Fatal("expected cached health body")
	}
}
