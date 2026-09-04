package subsonic

import "testing"

func TestOpenEndpointAllowed(t *testing.T) {
	if !openEndpointAllowed("stream") {
		t.Fatal("stream must be openable")
	}
	if !openEndpointAllowed("getCoverArt.view") {
		t.Fatal("getCoverArt must be openable")
	}
	if openEndpointAllowed("getAlbum") {
		t.Fatal("getAlbum must use Invoke, not Open")
	}
}
