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
		{"/rest/ping", true},
		{"/rest/ping.view", true},
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

func TestShouldSkipJWTVerifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"/rest/ping", true},
		{"/rest/getAlbumList2", true},
		{"/rest/stream", true},
		{"/api/keepalive/xyz", true},
		{"/api/album", false},
		{"/auth/login", false},
	}
	for _, test := range tests {
		req := httptest.NewRequest("GET", test.path, nil)
		if got := shouldSkipJWTVerifier(req); got != test.want {
			t.Fatalf("shouldSkipJWTVerifier(%q)=%v, want %v", test.path, got, test.want)
		}
	}
}

func TestIsNativeKeepAlivePath(t *testing.T) {
	t.Parallel()

	if !isNativeKeepAlivePath("/api/keepalive/1") {
		t.Fatal("expected keepalive path match")
	}
	if isNativeKeepAlivePath("/api/album") {
		t.Fatal("did not expect album path to match keepalive")
	}
}
