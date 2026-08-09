package model

import (
	"fmt"
	"testing"
	"time"
)

var benchmarkArtworkIDString string

func BenchmarkArtworkIDString(b *testing.B) {
	id := ArtworkID{
		Kind:       KindAlbumArtwork,
		ID:         "18690de0-151b-4d86-81cb-f418a907315a",
		LastUpdate: time.Unix(0x65f2a100, 0),
	}

	b.Run("builder", func(b *testing.B) {
		for b.Loop() {
			benchmarkArtworkIDString = id.String()
		}
	})
	b.Run("legacy-fmt", func(b *testing.B) {
		for b.Loop() {
			benchmarkArtworkIDString = legacyArtworkIDString(id)
		}
	})
}

func legacyArtworkIDString(id ArtworkID) string {
	if id.ID == "" {
		return ""
	}
	s := fmt.Sprintf("%s-%s", id.Kind.prefix, id.ID)
	if lu := id.LastUpdate.Unix(); lu > 0 {
		return fmt.Sprintf("%s_%x", s, lu)
	}
	return s + "_0"
}
