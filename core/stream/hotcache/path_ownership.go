package hotcache

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/navidrome/navidrome/conf"
)

const (
	hotCacheOwnershipDirName        = "zzzz-navidrome-hot-cache-owner-v1"
	hotCacheOwnershipMarkerName     = "owner"
	hotCacheOwnershipMarkerContents = "navidrome-original-hot-cache-v1\n"
	maxLegacyMetadataSize           = 64 << 10
)

// PrepareConfiguredPath establishes that the configured hot-cache directory is
// dedicated to Navidrome before the resolver is allowed to scan or clean it.
// Empty directories and directories with a strong legacy hot-cache signature
// are adopted by creating an ownership marker. Other non-empty directories are
// rejected without modifying their contents.
func PrepareConfiguredPath() error {
	if !conf.Server.HotCache.Enabled {
		return nil
	}
	path := conf.Server.HotCache.Path.String()
	if path == "" {
		path = filepath.Join(conf.Server.CacheFolder.String(), "hot-music")
	}
	return prepareOwnedCachePath(path)
}

func prepareOwnedCachePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("hot-cache path is empty")
	}

	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(path, 0o750); err != nil {
			return fmt.Errorf("creating hot-cache directory %q: %w", path, err)
		}
	case err != nil:
		return fmt.Errorf("inspecting hot-cache directory %q: %w", path, err)
	case !info.IsDir():
		return fmt.Errorf("hot-cache path %q is not a directory", path)
	}

	if err := verifyHotCacheOwnership(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("reading hot-cache directory %q: %w", path, err)
	}
	if len(entries) > 0 {
		legacy, err := isRecognizedLegacyHotCache(path, entries)
		if err != nil {
			return err
		}
		if !legacy {
			return fmt.Errorf("refusing non-empty hot-cache directory %q without a Navidrome ownership marker", path)
		}
	}
	return createHotCacheOwnership(path)
}

func ownershipDirectory(path string) string {
	return filepath.Join(path, hotCacheOwnershipDirName)
}

func ownershipMarker(path string) string {
	return filepath.Join(ownershipDirectory(path), hotCacheOwnershipMarkerName)
}

func verifyHotCacheOwnership(path string) error {
	dirPath := ownershipDirectory(path)
	dirInfo, err := os.Lstat(dirPath)
	if err != nil {
		return err
	}
	if !dirInfo.IsDir() {
		return fmt.Errorf("invalid hot-cache ownership directory %q", dirPath)
	}

	markerPath := ownershipMarker(path)
	markerInfo, err := os.Lstat(markerPath)
	if err != nil {
		return err
	}
	if !markerInfo.Mode().IsRegular() {
		return fmt.Errorf("invalid hot-cache ownership marker %q", markerPath)
	}
	contents, err := os.ReadFile(markerPath)
	if err != nil {
		return fmt.Errorf("reading hot-cache ownership marker %q: %w", markerPath, err)
	}
	if string(contents) != hotCacheOwnershipMarkerContents {
		return fmt.Errorf("invalid hot-cache ownership marker contents in %q", markerPath)
	}
	return nil
}

func createHotCacheOwnership(path string) error {
	dirPath := ownershipDirectory(path)
	createdDir := false
	if err := os.Mkdir(dirPath, 0o700); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("creating hot-cache ownership directory %q: %w", dirPath, err)
		}
		dirInfo, statErr := os.Lstat(dirPath)
		if statErr != nil {
			return fmt.Errorf("inspecting hot-cache ownership directory %q: %w", dirPath, statErr)
		}
		if !dirInfo.IsDir() {
			return fmt.Errorf("invalid hot-cache ownership directory %q", dirPath)
		}
	} else {
		createdDir = true
	}

	markerPath := ownershipMarker(path)
	marker, err := os.OpenFile(markerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return verifyHotCacheOwnership(path)
		}
		if createdDir {
			_ = os.Remove(dirPath)
		}
		return fmt.Errorf("creating hot-cache ownership marker %q: %w", markerPath, err)
	}
	cleanup := true
	defer func() {
		_ = marker.Close()
		if cleanup {
			_ = os.Remove(markerPath)
			if createdDir {
				_ = os.Remove(dirPath)
			}
		}
	}()

	if _, err := marker.WriteString(hotCacheOwnershipMarkerContents); err != nil {
		return fmt.Errorf("writing hot-cache ownership marker %q: %w", markerPath, err)
	}
	if err := marker.Sync(); err != nil {
		return fmt.Errorf("syncing hot-cache ownership marker %q: %w", markerPath, err)
	}
	if err := marker.Close(); err != nil {
		return fmt.Errorf("closing hot-cache ownership marker %q: %w", markerPath, err)
	}
	cleanup = false
	return nil
}

func isRecognizedLegacyHotCache(path string, entries []os.DirEntry) (bool, error) {
	dataKeys := make(map[string]struct{})
	metadataKeys := make(map[string]struct{})
	for _, item := range entries {
		if item.IsDir() {
			return false, nil
		}
		info, err := item.Info()
		if err != nil {
			return false, fmt.Errorf("inspecting legacy hot-cache entry %q: %w", filepath.Join(path, item.Name()), err)
		}
		if !info.Mode().IsRegular() {
			return false, nil
		}

		name := item.Name()
		switch {
		case strings.HasSuffix(name, ".data"):
			key := strings.TrimSuffix(name, ".data")
			if !validHotCacheKey(key) {
				return false, nil
			}
			dataKeys[key] = struct{}{}
		case strings.HasSuffix(name, ".json"):
			key := strings.TrimSuffix(name, ".json")
			if !validHotCacheKey(key) {
				return false, nil
			}
			valid, err := validLegacyMetadata(filepath.Join(path, name), key)
			if err != nil {
				return false, err
			}
			if !valid {
				return false, nil
			}
			metadataKeys[key] = struct{}{}
		case strings.HasSuffix(name, ".tmp"):
			key, _, ok := strings.Cut(name, ".")
			if !ok || !validHotCacheKey(key) {
				return false, nil
			}
		default:
			return false, nil
		}
	}
	if len(metadataKeys) == 0 || len(dataKeys) != len(metadataKeys) {
		return false, nil
	}
	for key := range dataKeys {
		if _, ok := metadataKeys[key]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func validHotCacheKey(key string) bool {
	if len(key) != hex.EncodedLen(32) || strings.ToLower(key) != key {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

func validLegacyMetadata(path, key string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("opening legacy hot-cache metadata %q: %w", path, err)
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, maxLegacyMetadataSize+1))
	if err != nil {
		return false, fmt.Errorf("reading legacy hot-cache metadata %q: %w", path, err)
	}
	if len(contents) > maxLegacyMetadataSize {
		return false, nil
	}
	var meta metadata
	if err := json.Unmarshal(contents, &meta); err != nil {
		return false, nil
	}
	return meta.Version == metadataVersion && meta.Key == key && keyFor(meta.SourceID, meta.SourcePath) == key, nil
}
