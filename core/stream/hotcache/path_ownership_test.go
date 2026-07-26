package hotcache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareOwnedCachePathRejectsUnrelatedNonEmptyDirectory(t *testing.T) {
	path := t.TempDir()
	importantPath := filepath.Join(path, "important-library-track.flac")
	importantData := []byte("must not be deleted")
	require.NoError(t, os.WriteFile(importantPath, importantData, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "application.json"), []byte(`{"owner":"another-service"}`), 0o600))

	err := prepareOwnedCachePath(path)
	require.ErrorContains(t, err, "refusing non-empty hot-cache directory")
	actual, readErr := os.ReadFile(importantPath)
	require.NoError(t, readErr)
	require.Equal(t, importantData, actual)
	_, markerErr := os.Stat(ownershipDirectory(path))
	require.ErrorIs(t, markerErr, os.ErrNotExist)
}

func TestPrepareOwnedCachePathCreatesMarkerAndResolverPreservesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, prepareOwnedCachePath(path))
	require.NoError(t, verifyHotCacheOwnership(path))

	resolver := newTestResolver(t, path, 1<<20)
	require.True(t, resolver.enabled)
	require.NoError(t, verifyHotCacheOwnership(path))
}

func TestPrepareOwnedCachePathAdoptsRecognizedLegacyCache(t *testing.T) {
	path := t.TempDir()
	key := keyFor("legacy-track", "")
	meta := metadata{
		Version: metadataVersion,
		Key: key,
		SourceID: "legacy-track",
		SourcePath: "/music/legacy-track.flac",
	}
	encoded, err := json.Marshal(meta)
	require.NoError(t, err)
	dataPath := filepath.Join(path, key+".data")
	metadataPath := filepath.Join(path, key+".json")
	require.NoError(t, os.WriteFile(dataPath, []byte("legacy-cache-data"), 0o440))
	require.NoError(t, os.WriteFile(metadataPath, encoded, 0o600))

	require.NoError(t, prepareOwnedCachePath(path))
	require.NoError(t, verifyHotCacheOwnership(path))
	actual, err := os.ReadFile(dataPath)
	require.NoError(t, err)
	require.Equal(t, []byte("legacy-cache-data"), actual)
}

func TestPrepareOwnedCachePathRejectsTamperedMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, prepareOwnedCachePath(path))
	importantPath := filepath.Join(path, "important.data")
	require.NoError(t, os.WriteFile(importantPath, []byte("unrelated"), 0o600))
	require.NoError(t, os.WriteFile(ownershipMarker(path), []byte("not-a-valid-marker\n"), 0o600))

	err := prepareOwnedCachePath(path)
	require.ErrorContains(t, err, "invalid hot-cache ownership marker contents")
	actual, readErr := os.ReadFile(importantPath)
	require.NoError(t, readErr)
	require.Equal(t, []byte("unrelated"), actual)
}
