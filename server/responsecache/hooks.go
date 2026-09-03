package responsecache

import "sync"

// Invalidator coordinates Subsonic response cache invalidation across API layers.
type Invalidator struct {
	mu sync.RWMutex

	invalidatePlaylists func()
	invalidateEntity    func(string)
	invalidateCatalog   func()
}

var defaultInvalidator Invalidator

// RegisterPlaylistsInvalidator registers the callback used to clear getPlaylists cache.
func RegisterPlaylistsInvalidator(fn func()) {
	defaultInvalidator.mu.Lock()
	defaultInvalidator.invalidatePlaylists = fn
	defaultInvalidator.mu.Unlock()
}

// RegisterEntityInvalidator registers the callback used to clear entity response cache.
func RegisterEntityInvalidator(fn func(string)) {
	defaultInvalidator.mu.Lock()
	defaultInvalidator.invalidateEntity = fn
	defaultInvalidator.mu.Unlock()
}

// InvalidatePlaylists clears cached getPlaylists responses.
func InvalidatePlaylists() {
	defaultInvalidator.mu.RLock()
	fn := defaultInvalidator.invalidatePlaylists
	defaultInvalidator.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// InvalidateEntity clears cached getArtist/getAlbum/getSong responses for an entity ID
// across all users.
func InvalidateEntity(entityID string) {
	if entityID == "" {
		return
	}
	defaultInvalidator.mu.RLock()
	fn := defaultInvalidator.invalidateEntity
	defaultInvalidator.mu.RUnlock()
	if fn != nil {
		fn(entityID)
	}
}

// InvalidateEntities clears cached entity responses for multiple IDs.
func InvalidateEntities(entityIDs ...string) {
	for _, entityID := range entityIDs {
		InvalidateEntity(entityID)
	}
}

// RegisterCatalogInvalidator registers the callback used to clear catalog
// response caches after library scans or other bulk metadata changes.
func RegisterCatalogInvalidator(fn func()) {
	defaultInvalidator.mu.Lock()
	defaultInvalidator.invalidateCatalog = fn
	defaultInvalidator.mu.Unlock()
}

// InvalidateCatalog clears cached catalog responses (entity lists, album lists,
// stream metadata) across all users.
func InvalidateCatalog() {
	defaultInvalidator.mu.RLock()
	fn := defaultInvalidator.invalidateCatalog
	defaultInvalidator.mu.RUnlock()
	if fn != nil {
		fn()
	}
}
