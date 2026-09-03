package plugins

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/plugins/host"
	"github.com/navidrome/navidrome/utils/slice"
)

const (
	defaultMaxKVStoreSize = 1 * 1024 * 1024 // 1MB default
	maxKeyLength          = 256             // Max key length in bytes
)

const cleanupInterval = 1 * time.Hour

// kvstoreServiceImpl implements the host.KVStoreService interface.
// Data lives in the shared DataStore, namespaced by plugin ID.
type kvstoreServiceImpl struct {
	pluginName string
	store      model.PluginKVRepository
	maxSize    int64
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	closed     atomic.Bool
}

func newKVStoreService(ctx context.Context, ds model.DataStore, pluginName string, perm *KVStorePermission) (*kvstoreServiceImpl, error) {
	if ds == nil {
		return nil, fmt.Errorf("plugin datastore is required")
	}
	store := ds.PluginKV(ctx)
	if store == nil {
		return nil, fmt.Errorf("plugin kv store unavailable")
	}

	maxSize := int64(defaultMaxKVStoreSize)
	if perm != nil && perm.MaxSize != nil && *perm.MaxSize != "" {
		parsed, err := humanize.ParseBytes(*perm.MaxSize)
		if err != nil {
			return nil, fmt.Errorf("invalid maxSize %q: %w", *perm.MaxSize, err)
		}
		maxSize = int64(parsed)
	}

	log.Debug("Initialized plugin kvstore", "plugin", pluginName, "maxSize", humanize.Bytes(uint64(maxSize)))

	cleanupCtx, cancel := context.WithCancel(ctx)
	svc := &kvstoreServiceImpl{
		pluginName: pluginName,
		store:      store,
		maxSize:    maxSize,
		cancel:     cancel,
	}
	svc.wg.Add(1)
	go svc.cleanupLoop(cleanupCtx)
	return svc, nil
}

func (s *kvstoreServiceImpl) checkClosed() error {
	if s.closed.Load() {
		return fmt.Errorf("kvstore is closed")
	}
	return nil
}

func (s *kvstoreServiceImpl) checkStorageLimit(ctx context.Context, delta int64) error {
	if delta <= 0 {
		return nil
	}
	used, err := s.store.StorageUsed(ctx, s.pluginName)
	if err != nil {
		return err
	}
	newTotal := used + delta
	if newTotal > s.maxSize {
		return fmt.Errorf("storage limit exceeded: would use %s of %s allowed",
			humanize.Bytes(uint64(newTotal)), humanize.Bytes(uint64(s.maxSize)))
	}
	return nil
}

func (s *kvstoreServiceImpl) setValue(ctx context.Context, key string, value []byte, ttlSeconds int64) error {
	if err := s.checkClosed(); err != nil {
		return err
	}
	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	if len(key) > maxKeyLength {
		return fmt.Errorf("key exceeds maximum length of %d bytes", maxKeyLength)
	}

	oldSize, err := s.store.ExistingSize(ctx, s.pluginName, key)
	if err != nil {
		return fmt.Errorf("checking existing key: %w", err)
	}
	if err := s.checkStorageLimit(ctx, int64(len(value))-oldSize); err != nil {
		return err
	}
	if err := s.store.Put(ctx, s.pluginName, key, value, ttlSeconds); err != nil {
		return err
	}
	log.Trace(ctx, "KVStore.Set", "plugin", s.pluginName, "key", key, "size", len(value), "ttlSeconds", ttlSeconds)
	return nil
}

func (s *kvstoreServiceImpl) Set(ctx context.Context, key string, value []byte) error {
	return s.setValue(ctx, key, value, 0)
}

func (s *kvstoreServiceImpl) SetWithTTL(ctx context.Context, key string, value []byte, ttlSeconds int64) error {
	if ttlSeconds <= 0 {
		return fmt.Errorf("ttlSeconds must be greater than 0")
	}
	return s.setValue(ctx, key, value, ttlSeconds)
}

func (s *kvstoreServiceImpl) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := s.checkClosed(); err != nil {
		return nil, false, err
	}
	value, found, err := s.store.Get(ctx, s.pluginName, key)
	if err != nil {
		return nil, false, err
	}
	if found {
		log.Trace(ctx, "KVStore.Get", "plugin", s.pluginName, "key", key, "found", true)
	}
	return value, found, nil
}

func (s *kvstoreServiceImpl) Delete(ctx context.Context, key string) error {
	if err := s.checkClosed(); err != nil {
		return err
	}
	if err := s.store.Delete(ctx, s.pluginName, key); err != nil {
		return fmt.Errorf("deleting value: %w", err)
	}
	log.Trace(ctx, "KVStore.Delete", "plugin", s.pluginName, "key", key)
	return nil
}

func (s *kvstoreServiceImpl) Has(ctx context.Context, key string) (bool, error) {
	if err := s.checkClosed(); err != nil {
		return false, err
	}
	return s.store.Has(ctx, s.pluginName, key)
}

func (s *kvstoreServiceImpl) List(ctx context.Context, prefix string) ([]string, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	keys, err := s.store.List(ctx, s.pluginName, prefix)
	if err != nil {
		return nil, err
	}
	log.Trace(ctx, "KVStore.List", "plugin", s.pluginName, "prefix", prefix, "count", len(keys))
	return keys, nil
}

func (s *kvstoreServiceImpl) GetStorageUsed(ctx context.Context) (int64, error) {
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	used, err := s.store.StorageUsed(ctx, s.pluginName)
	if err != nil {
		return 0, err
	}
	log.Trace(ctx, "KVStore.GetStorageUsed", "plugin", s.pluginName, "bytes", used)
	return used, nil
}

func (s *kvstoreServiceImpl) DeleteByPrefix(ctx context.Context, prefix string) (int64, error) {
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	if prefix == "" {
		return 0, fmt.Errorf("prefix cannot be empty")
	}
	count, err := s.store.DeleteByPrefix(ctx, s.pluginName, prefix)
	if err != nil {
		return 0, err
	}
	log.Trace(ctx, "KVStore.DeleteByPrefix", "plugin", s.pluginName, "prefix", prefix, "deletedCount", count)
	return count, nil
}

func (s *kvstoreServiceImpl) GetMany(ctx context.Context, keys []string) (map[string][]byte, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return map[string][]byte{}, nil
	}
	const batchSize = 200
	result := make(map[string][]byte)
	for chunk := range slice.CollectChunks(slices.Values(keys), batchSize) {
		part, err := s.store.GetMany(ctx, s.pluginName, chunk)
		if err != nil {
			return nil, err
		}
		for key, value := range part {
			result[key] = value
		}
	}
	log.Trace(ctx, "KVStore.GetMany", "plugin", s.pluginName, "requested", len(keys), "found", len(result))
	return result, nil
}

func (s *kvstoreServiceImpl) cleanupLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupExpired(ctx)
		}
	}
}

func (s *kvstoreServiceImpl) cleanupExpired(ctx context.Context) {
	count, err := s.store.CleanupExpired(ctx, s.pluginName)
	if err != nil {
		log.Error(ctx, "KVStore cleanup: failed to delete expired keys", "plugin", s.pluginName, err)
		return
	}
	if count > 0 {
		log.Debug("KVStore cleanup completed", "plugin", s.pluginName, "deletedKeys", count)
	}
}

func (s *kvstoreServiceImpl) Close() error {
	log.Debug("Closing plugin kvstore", "plugin", s.pluginName)
	s.closed.Store(true)
	s.cancel()
	s.wg.Wait()
	return nil
}

var _ host.KVStoreService = (*kvstoreServiceImpl)(nil)
