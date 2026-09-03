package responsecache

import (
	"sync/atomic"
	"testing"
)

func TestInvalidatePlaylistsNoOpWithoutRegistration(t *testing.T) {
	InvalidatePlaylists()
}

func TestInvalidateEntityNoOpWithoutRegistration(t *testing.T) {
	InvalidateEntity("song-1")
	InvalidateEntities()
}

func TestInvalidatePlaylistsCallsRegisteredHook(t *testing.T) {
	var calls atomic.Int32
	RegisterPlaylistsInvalidator(func() {
		calls.Add(1)
	})
	InvalidatePlaylists()
	if calls.Load() != 1 {
		t.Fatalf("expected 1 invalidation call, got %d", calls.Load())
	}
}

func TestInvalidateEntityCallsRegisteredHook(t *testing.T) {
	var got string
	RegisterEntityInvalidator(func(entityID string) {
		got = entityID
	})
	InvalidateEntity("album-1")
	if got != "album-1" {
		t.Fatalf("expected album-1, got %q", got)
	}
}

func TestInvalidateCatalogNoOpWithoutRegistration(t *testing.T) {
	InvalidateCatalog()
}

func TestInvalidateCatalogCallsRegisteredHook(t *testing.T) {
	var calls atomic.Int32
	RegisterCatalogInvalidator(func() {
		calls.Add(1)
	})
	InvalidateCatalog()
	if calls.Load() != 1 {
		t.Fatalf("expected 1 invalidation call, got %d", calls.Load())
	}
}
