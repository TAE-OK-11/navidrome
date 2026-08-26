package lyrics

import (
	"sync"
	"time"

	"github.com/navidrome/navidrome/model"
)

type sidecarCacheEntry struct {
	modTime time.Time
	list    model.LyricList
}

var sidecarLyricsCache sync.Map

func sidecarCacheKey(libraryPath, relPath, suffix string) string {
	return libraryPath + "\x00" + relPath + "\x00" + suffix
}

func loadSidecarCache(libraryPath, relPath, suffix string, modTime time.Time) (model.LyricList, bool) {
	key := sidecarCacheKey(libraryPath, relPath, suffix)
	if cached, ok := sidecarLyricsCache.Load(key); ok {
		entry := cached.(sidecarCacheEntry)
		if entry.modTime.Equal(modTime) {
			return entry.list, true
		}
	}
	return nil, false
}

func storeSidecarCache(libraryPath, relPath, suffix string, modTime time.Time, list model.LyricList) {
	if len(list) == 0 {
		return
	}
	key := sidecarCacheKey(libraryPath, relPath, suffix)
	sidecarLyricsCache.Store(key, sidecarCacheEntry{modTime: modTime, list: list})
}
