package server

import (
	"net/http/httptest"
	"testing"

	"github.com/navidrome/navidrome/consts"
)

func TestIsProbeRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"/ping", true},
		{"/api/health", true},
		{"/api/health/", true},
		{"/rest/ping", false},
		{"/api/album", false},
	}
	for _, test := range tests {
		req := httptest.NewRequest("GET", test.path, nil)
		if got := isProbeRequest(req); got != test.want {
			t.Fatalf("isProbeRequest(%q)=%v, want %v", test.path, got, test.want)
		}
	}
	_ = consts.URLPathNativeAPI
}
