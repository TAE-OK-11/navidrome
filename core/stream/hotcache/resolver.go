package hotcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

const (
	metadataVersion = 1
	defaultMaxSize  = "3GiB"
	copyBufferSize  = 256 * 1024
)

// File is the file-backed stream returned by a Resolver. Keeping syscall.Conn
// in the contract preserves net/http's sendfile path for cache hits.
type File interface {
	io.ReadSeekCloser
	syscall.Conn
}

// Resolver selects a verified hot-cache file or immediately opens the source.
type Resolver interface {
	Open(ctx context.Context, mf *model.MediaFile) (File, error)
	Stats() Stats
}

// IsHit reports whether a resolved file is backed by the original hot cache.
func IsHit(file File) bool {
	marked, ok := file.(interface{ HotCacheHit() bool })
	return ok && marked.HotCacheHit()
}

type Stats struct {
	Hits       uint64
	Misses     uint64
	Promotions uint64
	Failures   uint64
	Evictions  uint64
	Bytes      int64
	Entries    int
}

type Options struct {
	Enabled       bool
	Path          string
	MaxSize       int64
	PromoteOnPlay bool
}

type metadata struct {
	Version       int    `json:"version"`
	Key           string `json:"key"`
	SourceID      string `json:"sourceId"`
	SourcePath    string `json:"sourcePath"`
	SourceSize    int64  `json:"sourceSize"`
	SourceModTime int64  `json:"sourceModTime"`
	DataSize      int64  `json:"dataSize"`
	DataModTime   int64  `json:"dataModTime"`
	SHA256        string `json:"sha256"`
}

type entry struct {
	meta         metadata
	dataPath     string
	metadataPath string
	metadataSize int64
	lastUsed     time.Time
	active       int
	stale        bool
}

type resolver struct {
	mu            sync.Mutex
	enabled       bool
	promoteOnPlay bool
	path          string
	maxSize       int64
	used          int64
	reserved      int64
	entries       map[string]*entry
	promoting     map[string]struct{}
	promoteSem    chan struct{}
	copyFile      func(io.Writer, io.Reader, []byte) (int64, error)

	hits       atomic.Uint64
	misses     atomic.Uint64
	promotions atomic.Uint64
	failures   atomic.Uint64
	evictions  atomic.Uint64
}

var copyBufferPool = sync.Pool{
	New: func() any {
		buffer := make([]byte, copyBufferSize)
		return &buffer
	},
}

var (
	resolverOnce     sync.Once
	resolverInstance Resolver
)

// GetResolver returns the process-wide resolver. All HTTP routers must share
// one LRU, promotion registry, and pin set for a cache directory.
func GetResolver() Resolver {
	resolverOnce.Do(func() {
		resolverInstance = NewResolver()
	})
	return resolverInstance
}

func NewResolver() Resolver {
	if !conf.Server.HotCache.Enabled {
		return New(Options{})
	}
	maxSize, err := humanize.ParseBytes(conf.Server.HotCache.MaxSize)
	if err != nil || maxSize > uint64(^uint64(0)>>1) {
		log.Warn("Original hot cache disabled: invalid maximum size", "maxSize", conf.Server.HotCache.MaxSize, err)
		return New(Options{})
	}
	path := conf.Server.HotCache.Path.String()
	if path == "" {
		path = filepath.Join(conf.Server.CacheFolder.String(), "hot-music")
	}
	return New(Options{
		Enabled:       conf.Server.HotCache.Enabled,
		Path:          path,
		MaxSize:       int64(maxSize),
		PromoteOnPlay: conf.Server.HotCache.PromoteOnPlay,
	})
}

// New creates an isolated original-file hot cache. Initialization failures
// disable only this cache; direct source streaming remains available.
func New(options Options) Resolver {
	hardMaxSize := mustParseDefaultSize()
	if options.MaxSize <= 0 || options.MaxSize > hardMaxSize {
		options.MaxSize = hardMaxSize
	}
	r := &resolver{
		enabled:       options.Enabled,
		promoteOnPlay: options.PromoteOnPlay,
		path:          options.Path,
		maxSize:       options.MaxSize,
		entries:       make(map[string]*entry),
		promoting:     make(map[string]struct{}),
		promoteSem:    make(chan struct{}, 1),
		copyFile: func(dst io.Writer, src io.Reader, buffer []byte) (int64, error) {
			return io.CopyBuffer(dst, src, buffer)
		},
	}
	if !r.enabled {
		return r
	}
	if err := r.initialize(); err != nil {
		r.enabled = false
		log.Warn("Original hot cache disabled; direct streaming will be used", "path", r.path, err)
		return r
	}
	log.Info("Initialized original hot cache", "path", r.path, "maxSize", r.maxSize, "entries", len(r.entries), "size", r.used)
	return r
}

func mustParseDefaultSize() int64 {
	size, _ := humanize.ParseBytes(defaultMaxSize)
	return int64(size)
}

func (r *resolver) initialize() error {
	if r.path == "" {
		return errors.New("cache path is empty")
	}
	if err := os.MkdirAll(r.path, 0o750); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	probe, err := os.CreateTemp(r.path, ".write-probe-*.tmp")
	if err != nil {
		return fmt.Errorf("cache directory is not writable: %w", err)
	}
	probeName := probe.Name()
	if closeErr := probe.Close(); closeErr != nil {
		_ = os.Remove(probeName)
		return fmt.Errorf("closing cache write probe: %w", closeErr)
	}
	if err := os.Remove(probeName); err != nil {
		return fmt.Errorf("removing cache write probe: %w", err)
	}

	dirEntries, err := os.ReadDir(r.path)
	if err != nil {
		return fmt.Errorf("reading cache directory: %w", err)
	}
	dataFiles := make(map[string]string)
	metadataFiles := make(map[string]string)
	for _, item := range dirEntries {
		if item.IsDir() {
			continue
		}
		name := item.Name()
		path := filepath.Join(r.path, name)
		switch {
		case strings.HasSuffix(name, ".tmp"):
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("removing incomplete cache file %q: %w", path, err)
			}
		case strings.HasSuffix(name, ".data"):
			dataFiles[strings.TrimSuffix(name, ".data")] = path
		case strings.HasSuffix(name, ".json"):
			metadataFiles[strings.TrimSuffix(name, ".json")] = path
		default:
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("removing unknown cache file %q: %w", path, err)
			}
		}
	}

	for key, metadataPath := range metadataFiles {
		dataPath, ok := dataFiles[key]
		if !ok {
			if err := os.Remove(metadataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("removing orphan metadata %q: %w", metadataPath, err)
			}
			continue
		}
		loaded, err := loadEntry(key, dataPath, metadataPath)
		if err != nil {
			if removeErr := removePair(dataPath, metadataPath); removeErr != nil {
				return fmt.Errorf("removing invalid cache entry %q: %w", key, removeErr)
			}
			continue
		}
		r.entries[key] = loaded
		r.used += loaded.meta.DataSize + loaded.metadataSize
		delete(dataFiles, key)
	}
	for _, dataPath := range dataFiles {
		if err := os.Remove(dataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing orphan cache data %q: %w", dataPath, err)
		}
	}

	r.mu.Lock()
	err = r.evictUntilLocked(0)
	r.mu.Unlock()
	if err != nil {
		return fmt.Errorf("enforcing cache limit: %w", err)
	}
	return nil
}

func loadEntry(key, dataPath, metadataPath string) (*entry, error) {
	raw, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}
	var meta metadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	if meta.Version != metadataVersion || meta.Key != key || keyFor(meta.SourceID, meta.SourcePath) != key {
		return nil, errors.New("metadata identity mismatch")
	}
	sourceInfo, err := os.Stat(meta.SourcePath)
	if err != nil || !sourceMatches(meta, sourceInfo) {
		return nil, errors.New("source is missing or changed")
	}
	dataInfo, err := os.Stat(dataPath)
	if err != nil || !dataInfo.Mode().IsRegular() || dataInfo.Size() != meta.DataSize || dataInfo.ModTime().UnixNano() != meta.DataModTime {
		return nil, errors.New("cached data metadata mismatch")
	}
	digest, err := fileSHA256(dataPath)
	if err != nil || digest != meta.SHA256 {
		return nil, errors.New("cached data checksum mismatch")
	}
	metadataInfo, err := os.Stat(metadataPath)
	if err != nil {
		return nil, err
	}
	return &entry{
		meta:         meta,
		dataPath:     dataPath,
		metadataPath: metadataPath,
		metadataSize: metadataInfo.Size(),
		lastUsed:     metadataInfo.ModTime(),
	}, nil
}

func (r *resolver) Open(ctx context.Context, mf *model.MediaFile) (File, error) {
	sourcePath := mf.AbsolutePath()
	if !r.enabled {
		return os.Open(sourcePath)
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil || !sourceInfo.Mode().IsRegular() {
		r.invalidate(keyFor(mf.ID, sourcePath))
		return os.Open(sourcePath)
	}
	key := keyFor(mf.ID, sourcePath)
	now := time.Now()

	r.mu.Lock()
	if cached := r.entries[key]; cached != nil && !cached.stale && sourceMatchesMedia(cached.meta, mf, sourcePath, sourceInfo) {
		dataInfo, statErr := os.Stat(cached.dataPath)
		if statErr == nil && dataInfo.Mode().IsRegular() && dataInfo.Size() == cached.meta.DataSize && dataInfo.ModTime().UnixNano() == cached.meta.DataModTime {
			file, openErr := os.Open(cached.dataPath)
			if openErr == nil {
				cached.active++
				cached.lastUsed = now
				r.mu.Unlock()
				if touchErr := os.Chtimes(cached.metadataPath, now, now); touchErr != nil {
					_ = file.Close()
					r.release(key, cached)
					r.misses.Add(1)
					log.Debug(ctx, "Could not persist original hot-cache access time; using source", "id", mf.ID, touchErr)
					return os.Open(sourcePath)
				}
				r.hits.Add(1)
				log.Trace(ctx, "Original hot cache HIT", "id", mf.ID, "path", cached.dataPath)
				return &pinnedFile{File: file, release: func() { r.release(key, cached) }}, nil
			}
		}
		r.markStaleLocked(cached)
	}
	r.mu.Unlock()

	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	r.misses.Add(1)
	log.Trace(ctx, "Original hot cache MISS", "id", mf.ID, "path", sourcePath)
	if r.promoteOnPlay {
		sourceID := mf.ID
		return &promotionFile{
			File: file,
			promote: func() {
				r.schedulePromotion(context.Background(), sourceID, sourcePath, sourceInfo)
			},
		}, nil
	}
	return file, nil
}

func (r *resolver) Stats() Stats {
	r.mu.Lock()
	bytes := r.used
	entries := len(r.entries)
	r.mu.Unlock()
	return Stats{
		Hits:       r.hits.Load(),
		Misses:     r.misses.Load(),
		Promotions: r.promotions.Load(),
		Failures:   r.failures.Load(),
		Evictions:  r.evictions.Load(),
		Bytes:      bytes,
		Entries:    entries,
	}
}

func (r *resolver) schedulePromotion(ctx context.Context, sourceID, sourcePath string, sourceInfo os.FileInfo) {
	key := keyFor(sourceID, sourcePath)
	r.mu.Lock()
	if _, exists := r.promoting[key]; exists {
		r.mu.Unlock()
		return
	}
	if cached := r.entries[key]; cached != nil && cached.active > 0 {
		r.mu.Unlock()
		return
	}
	r.promoting[key] = struct{}{}
	r.mu.Unlock()

	identity := sourceIdentity{id: sourceID, path: sourcePath, size: sourceInfo.Size(), modTime: sourceInfo.ModTime().UnixNano()}
	go func() {
		r.promoteSem <- struct{}{}
		err := r.promote(ctx, key, identity)
		<-r.promoteSem

		r.mu.Lock()
		delete(r.promoting, key)
		r.mu.Unlock()
		if err != nil {
			r.failures.Add(1)
			log.Debug(ctx, "Original hot-cache promotion failed; source streaming was unaffected", "id", identity.id, err)
		}
	}()
}

type sourceIdentity struct {
	id      string
	path    string
	size    int64
	modTime int64
}

func (r *resolver) promote(ctx context.Context, key string, source sourceIdentity) error {
	if source.size <= 0 || source.size > r.maxSize {
		return nil
	}
	sourceFile, err := os.Open(source.path)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	openedInfo, err := sourceFile.Stat()
	if err != nil || openedInfo.Size() != source.size || openedInfo.ModTime().UnixNano() != source.modTime {
		return errors.New("source changed before promotion")
	}

	meta := metadata{
		Version:       metadataVersion,
		Key:           key,
		SourceID:      source.id,
		SourcePath:    source.path,
		SourceSize:    source.size,
		SourceModTime: source.modTime,
		DataSize:      source.size,
		DataModTime:   time.Now().UnixNano(),
		SHA256:        strings.Repeat("0", sha256.Size*2),
	}
	estimatedMetadata, _ := json.Marshal(meta)
	required := source.size + int64(len(estimatedMetadata)) + 512
	r.mu.Lock()
	if err := r.evictUntilLocked(required); err != nil {
		r.mu.Unlock()
		return err
	}
	r.reserved += required
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.reserved -= required
		r.mu.Unlock()
	}()

	tempData, err := os.CreateTemp(r.path, key+".*.tmp")
	if err != nil {
		return err
	}
	tempDataPath := tempData.Name()
	defer func() {
		_ = tempData.Close()
		_ = os.Remove(tempDataPath)
	}()

	hasher := sha256.New()
	buffer := copyBufferPool.Get().(*[]byte)
	written, copyErr := r.copyFile(io.MultiWriter(tempData, hasher), io.LimitReader(sourceFile, source.size+1), *buffer)
	copyBufferPool.Put(buffer)
	if copyErr != nil {
		return copyErr
	}
	if written != source.size {
		return fmt.Errorf("source size changed during promotion: copied %d, expected %d", written, source.size)
	}
	if err := tempData.Sync(); err != nil {
		return err
	}
	if err := tempData.Chmod(0o440); err != nil {
		return err
	}
	dataInfo, err := tempData.Stat()
	if err != nil {
		return err
	}
	if err := tempData.Close(); err != nil {
		return err
	}

	currentInfo, err := os.Stat(source.path)
	if err != nil || currentInfo.Size() != source.size || currentInfo.ModTime().UnixNano() != source.modTime {
		return errors.New("source changed during promotion")
	}
	meta.DataModTime = dataInfo.ModTime().UnixNano()
	meta.SHA256 = hex.EncodeToString(hasher.Sum(nil))
	metadataBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if int64(len(metadataBytes)) > required-source.size {
		return errors.New("cache metadata exceeded reserved capacity")
	}
	tempMetadata, err := os.CreateTemp(r.path, key+".*.tmp")
	if err != nil {
		return err
	}
	tempMetadataPath := tempMetadata.Name()
	defer func() {
		_ = tempMetadata.Close()
		_ = os.Remove(tempMetadataPath)
	}()
	if _, err := tempMetadata.Write(metadataBytes); err != nil {
		return err
	}
	if err := tempMetadata.Sync(); err != nil {
		return err
	}
	if err := tempMetadata.Close(); err != nil {
		return err
	}

	dataPath := filepath.Join(r.path, key+".data")
	metadataPath := filepath.Join(r.path, key+".json")
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing := r.entries[key]; existing != nil {
		if existing.active > 0 {
			return errors.New("cache entry became active during replacement")
		}
		if err := r.removeEntryLocked(key, existing); err != nil {
			return err
		}
	}
	if err := os.Rename(tempDataPath, dataPath); err != nil {
		return err
	}
	if err := os.Rename(tempMetadataPath, metadataPath); err != nil {
		_ = os.Remove(dataPath)
		return err
	}
	metadataInfo, err := os.Stat(metadataPath)
	if err != nil {
		_ = removePair(dataPath, metadataPath)
		return err
	}
	loaded := &entry{
		meta:         meta,
		dataPath:     dataPath,
		metadataPath: metadataPath,
		metadataSize: metadataInfo.Size(),
		lastUsed:     metadataInfo.ModTime(),
	}
	r.entries[key] = loaded
	r.used += loaded.meta.DataSize + loaded.metadataSize
	r.promotions.Add(1)
	_ = syncDirectory(r.path)
	log.Trace(ctx, "Promoted original file to hot cache", "id", source.id, "size", source.size)
	return nil
}

func (r *resolver) evictUntilLocked(required int64) error {
	if required > r.maxSize {
		return errors.New("file is larger than hot-cache capacity")
	}
	if r.used+r.reserved+required <= r.maxSize {
		return nil
	}
	candidates := make([]*entry, 0, len(r.entries))
	for _, cached := range r.entries {
		if cached.active == 0 {
			candidates = append(candidates, cached)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].lastUsed.Before(candidates[j].lastUsed) })
	for _, cached := range candidates {
		if err := r.removeEntryLocked(cached.meta.Key, cached); err != nil {
			continue
		}
		r.evictions.Add(1)
		if r.used+r.reserved+required <= r.maxSize {
			return nil
		}
	}
	return errors.New("not enough evictable hot-cache space")
}

func (r *resolver) removeEntryLocked(key string, cached *entry) error {
	if cached.active > 0 {
		cached.stale = true
		return errors.New("cache entry is active")
	}
	if err := removePair(cached.dataPath, cached.metadataPath); err != nil {
		return err
	}
	delete(r.entries, key)
	r.used -= cached.meta.DataSize + cached.metadataSize
	if r.used < 0 {
		r.used = 0
	}
	return nil
}

func (r *resolver) invalidate(key string) {
	r.mu.Lock()
	if cached := r.entries[key]; cached != nil {
		r.markStaleLocked(cached)
	}
	r.mu.Unlock()
}

func (r *resolver) markStaleLocked(cached *entry) {
	cached.stale = true
	if cached.active == 0 {
		_ = r.removeEntryLocked(cached.meta.Key, cached)
	}
}

func (r *resolver) release(key string, cached *entry) {
	r.mu.Lock()
	if cached.active > 0 {
		cached.active--
	}
	if cached.active == 0 && cached.stale {
		_ = r.removeEntryLocked(key, cached)
	}
	r.mu.Unlock()
}

type pinnedFile struct {
	*os.File
	once    sync.Once
	release func()
}

func (f *pinnedFile) Close() error {
	err := f.File.Close()
	f.once.Do(f.release)
	return err
}

func (f *pinnedFile) HotCacheHit() bool { return true }

// promotionFile defers the second source read until direct playback releases
// the source. This protects first-play TTFB and throughput on slow HDDs and
// remote filesystems while keeping promotion fully asynchronous afterwards.
type promotionFile struct {
	*os.File
	once    sync.Once
	promote func()
}

func (f *promotionFile) Close() error {
	err := f.File.Close()
	f.once.Do(f.promote)
	return err
}

func keyFor(sourceID, sourcePath string) string {
	identity := sourceID
	if identity == "" {
		identity = filepath.Clean(sourcePath)
	}
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func sourceMatches(meta metadata, info os.FileInfo) bool {
	return info != nil && info.Mode().IsRegular() && info.Size() == meta.SourceSize && info.ModTime().UnixNano() == meta.SourceModTime
}

func sourceMatchesMedia(meta metadata, mf *model.MediaFile, path string, info os.FileInfo) bool {
	return meta.SourceID == mf.ID && meta.SourcePath == path && sourceMatches(meta, info)
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	buffer := copyBufferPool.Get().(*[]byte)
	_, err = io.CopyBuffer(hasher, file, *buffer)
	copyBufferPool.Put(buffer)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func removePair(dataPath, metadataPath string) error {
	var errs []error
	if err := os.Remove(dataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	if err := os.Remove(metadataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
