package hotcache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareOwnedCachePathRejectsUnrelatedNonEmptyDirectory(t *testing.T) {
	path := t.TempDir()
	importantPath := filepath.Join(path, "important-library-track.flac")
	importantData := []byte("must not be deleted")
	require.NoError(t, os.WriteFile(importantPath, importantData, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "application.json"), []byte(`{"owner":"another-service"}`), 0o600))

	_, err := prepareOwnedCachePath(path)
	require.ErrorContains(t, err, "refusing non-empty hot-cache directory")
	actual, readErr := os.ReadFile(importantPath)
	require.NoError(t, readErr)
	require.Equal(t, importantData, actual)
	_, markerErr := os.Stat(ownershipDirectory(path))
	require.ErrorIs(t, markerErr, os.ErrNotExist)
}

func TestPrepareOwnedCachePathCreatesMarkerAndResolverPreservesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache")
	resolved, err := prepareOwnedCachePath(path)
	require.NoError(t, err)
	require.NoError(t, verifyHotCacheOwnership(resolved))

	resolver := newTestResolver(t, resolved, 1<<20)
	require.True(t, resolver.enabled)
	require.NoError(t, verifyHotCacheOwnership(resolved))
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

	resolved, err := prepareOwnedCachePath(path)
	require.NoError(t, err)
	require.NoError(t, verifyHotCacheOwnership(resolved))
	actual, err := os.ReadFile(dataPath)
	require.NoError(t, err)
	require.Equal(t, []byte("legacy-cache-data"), actual)
}

func TestPrepareOwnedCachePathRejectsTamperedMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache")
	resolved, err := prepareOwnedCachePath(path)
	require.NoError(t, err)
	importantPath := filepath.Join(resolved, "important.data")
	require.NoError(t, os.WriteFile(importantPath, []byte("unrelated"), 0o600))
	require.NoError(t, os.WriteFile(ownershipMarker(resolved), []byte("not-a-valid-marker\n"), 0o600))

	_, err = prepareOwnedCachePath(path)
	require.ErrorContains(t, err, "invalid hot-cache ownership marker contents")
	actual, readErr := os.ReadFile(importantPath)
	require.NoError(t, readErr)
	require.Equal(t, []byte("unrelated"), actual)
}

func TestPrepareOwnedCachePathPinsSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires additional privileges on Windows")
	}
	base := t.TempDir()
	target := filepath.Join(base, "target")
	link := filepath.Join(base, "cache-link")
	require.NoError(t, os.Mkdir(target, 0o750))
	require.NoError(t, os.Symlink(target, link))

	resolved, err := prepareOwnedCachePath(link)
	require.NoError(t, err)
	expected, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)
	require.Equal(t, expected, resolved)
	require.NoError(t, verifyHotCacheOwnership(resolved))
}
