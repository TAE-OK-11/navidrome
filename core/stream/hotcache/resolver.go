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
	"strings"
	"sync"
	"sync/atomic"
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
	Title         string `json:"title,omitempty"`
	Artist        string `json:"artist,omitempty"`
	Album         string `json:"album,omitempty"`
	Codec         string `json:"codec,omitempty"`
	Container     string `json:"container,omitempty"`
	Extension     string `json:"extension,omitempty"`
	BitRate       int    `json:"bitRate,omitempty"`
}

type entry struct {
	meta             metadata
	dataPath         string
	metadataPath     string
	metadataSize     int64
	createdAt        time.Time
	lastUsed         time.Time
	lastPersisted    time.Time
	lastSessionHit   time.Time
	lastRequestHit   time.Time
	latestTTFB       time.Duration
	latestRangeCount uint64
	requestHits      uint64
	sessionHits      uint64
	active           int
	touchPending     bool
	stale            bool
}

type resolver struct {
	mu             sync.Mutex
	enabled        bool
	promoteOnPlay  bool
	path           string
	maxSize        int64
	used           int64
	reserved       int64
	entries        map[string]*entry
	playing        map[string]int
	queue          chan *promotionTask
	promoting      map[string]*promotionTask
	current        *promotionTask
	paused         atomic.Bool
	shuttingDown   atomic.Bool
	stopOnce       sync.Once
	stop           chan struct{}
	control        chan struct{}
	workers        sync.WaitGroup
	workerCtx      context.Context
	workerCancel   context.CancelFunc
	queueMax       int
	promotionDelay time.Duration
	maxRetries     int
	touchInterval  time.Duration
	statsFlush     time.Duration
	statsPath      string
	touchQueue     chan string
	copyFile       func(context.Context, io.Writer, io.Reader, []byte, *promotionTask) (int64, error)
	sourceStreams  atomic.Int64
	initializedIn  time.Duration

	hits       atomic.Uint64
	misses     atomic.Uint64
	promotions atomic.Uint64
	failures   atomic.Uint64
	evictions  atomic.Uint64

	runtime            runtimeStats
	events             *eventRing
	sessions           *sessionTracker
	eventSampleEvery   uint64
	eventSampleCounter atomic.Uint64
}

var _ Manager = (*resolver)(nil)

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

// GetResolver returns the process-wide resolver. All HTTP routers share one
// LRU, promotion queue, session tracker, and pin registry.
func GetResolver() Resolver {
	resolverOnce.Do(func() {
		resolverInstance = NewResolver()
	})
	return resolverInstance
}

func GetManager() Manager {
	manager, _ := GetResolver().(Manager)
	return manager
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
		Enabled:                 true,
		Path:                    path,
		MaxSize:                 int64(maxSize),
		PromoteOnPlay:           conf.Server.HotCache.PromoteOnPlay,
		SessionWindow:           conf.Server.HotCache.SessionWindow,
		SessionIdleTimeout:      conf.Server.HotCache.SessionIdleTimeout,
		MaxSessions:             conf.Server.HotCache.MaxSessions,
		MinPlaySeconds:          conf.Server.HotCache.MinPlaySeconds,
		MinPlayPercent:          conf.Server.HotCache.MinPlayPercent,
		PromotionConcurrency:    conf.Server.HotCache.PromotionConcurrency,
		QueueMax:                conf.Server.HotCache.QueueMax,
		PromotionDelayAfterPlay: conf.Server.HotCache.PromotionDelayAfterPlay,
		PromotionMaxRetries:     conf.Server.HotCache.PromotionMaxRetries,
		TouchInterval:           conf.Server.HotCache.TouchInterval,
		StatsEnabled:            conf.Server.HotCache.StatsEnabled,
		EventsMax:               conf.Server.HotCache.EventsMax,
		StatsFlushInterval:      conf.Server.HotCache.StatsFlushInterval,
		EventSampleRate:         conf.Server.HotCache.EventSampleRate,
	})
}

// New creates an isolated original-file hot cache. Initialization failures
// disable only this cache; direct source streaming remains available.
func New(options Options) Resolver {
	options = normalizeOptions(options)
	r := &resolver{
		enabled:        options.Enabled,
		promoteOnPlay:  options.PromoteOnPlay,
		path:           options.Path,
		maxSize:        options.MaxSize,
		entries:        make(map[string]*entry),
		playing:        make(map[string]int),
		queueMax:       options.QueueMax,
		promotionDelay: options.PromotionDelayAfterPlay,
		maxRetries:     options.PromotionMaxRetries,
		touchInterval:  options.TouchInterval,
		statsFlush:     options.StatsFlushInterval,
		stop:           make(chan struct{}),
		control:        make(chan struct{}, 1),
		runtime:        runtimeStats{enabled: options.StatsEnabled},
		events:         newEventRing(options.EventsMax),
	}
	if options.EventSampleRate > 0 {
		r.eventSampleEvery = max(1, uint64(1/options.EventSampleRate))
	}
	if !r.enabled {
		return r
	}
	r.statsPath = filepath.Join(filepath.Dir(r.path), ".hot-cache-stats-v1.json")
	r.queue = make(chan *promotionTask, options.QueueMax)
	r.promoting = make(map[string]*promotionTask)
	r.touchQueue = make(chan string, min(options.QueueMax*2, 512))
	r.workerCtx, r.workerCancel = context.WithCancel(context.Background())
	r.copyFile = r.copyWithYield
	r.sessions = newSessionTracker(r, options)

	started := time.Now()
	cleanup, err := r.initialize()
	r.initializedIn = time.Since(started)
	if err != nil {
		r.workerCancel()
		r.enabled = false
		log.Warn("Original hot cache disabled; direct streaming will be used", "path", r.path, err)
		return r
	}
	if r.runtime.enabled {
		if err := r.loadRuntimeStats(); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Warn("Ignoring invalid Hot Cache statistics snapshot", "path", r.statsPath, err)
		}
	}
	r.startWorkers(options.PromotionConcurrency)
	r.runtime.orphanCleanup.Add(uint64(cleanup.OrphansRemoved + cleanup.TemporaryRemoved))
	r.runtime.corruptCleanup.Add(uint64(cleanup.CorruptRemoved))
	log.Info("Initialized original hot cache", "path", r.path, "maxSize", r.maxSize, "entries", len(r.entries),
		"size", r.used, "reserved", r.reserved, "elapsed", r.initializedIn,
		"temporaryRemoved", cleanup.TemporaryRemoved, "orphansRemoved", cleanup.OrphansRemoved,
		"corruptRemoved", cleanup.CorruptRemoved)
	r.addEvent(Event{Type: "recovery_cleanup", Category: "recovery", PlaybackSuccess: true,
		Message: fmt.Sprintf("temporary=%d orphan=%d corrupt=%d", cleanup.TemporaryRemoved, cleanup.OrphansRemoved, cleanup.CorruptRemoved)})
	return r
}

func normalizeOptions(options Options) Options {
	hardMaxSize := mustParseDefaultSize()
	if options.MaxSize <= 0 || options.MaxSize > hardMaxSize {
		options.MaxSize = hardMaxSize
	}
	if options.SessionWindow <= 0 {
		options.SessionWindow = 30 * time.Second
	}
	if options.SessionIdleTimeout < options.SessionWindow {
		options.SessionIdleTimeout = max(60*time.Second, options.SessionWindow)
	}
	if options.MaxSessions <= 0 {
		options.MaxSessions = 1024
	}
	if options.MinPlaySeconds <= 0 {
		options.MinPlaySeconds = 20
	}
	if options.MinPlayPercent <= 0 || options.MinPlayPercent > 100 {
		options.MinPlayPercent = 25
	}
	// A single HDD copy is intentional: additional workers increase seek
	// contention on the source volume and hurt first-play latency.
	options.PromotionConcurrency = 1
	if options.QueueMax <= 0 {
		options.QueueMax = 128
	}
	options.QueueMax = min(options.QueueMax, 1024)
	if options.PromotionDelayAfterPlay < 0 {
		options.PromotionDelayAfterPlay = time.Second
	}
	if options.PromotionMaxRetries < 0 {
		options.PromotionMaxRetries = 2
	}
	options.PromotionMaxRetries = min(options.PromotionMaxRetries, 5)
	if options.TouchInterval <= 0 {
		options.TouchInterval = 30 * time.Second
	}
	if options.StatsFlushInterval <= 0 {
		options.StatsFlushInterval = 30 * time.Second
	}
	options.StatsFlushInterval = min(max(options.StatsFlushInterval, 5*time.Second), 24*time.Hour)
	if options.EventsMax <= 0 {
		options.EventsMax = 5000
	}
	options.EventsMax = min(options.EventsMax, 20_000)
	if options.EventSampleRate < 0 || options.EventSampleRate > 1 {
		options.EventSampleRate = 0.01
	}
	return options
}

func mustParseDefaultSize() int64 {
	size, _ := humanize.ParseBytes(defaultMaxSize)
	return int64(size)
}

func (r *resolver) initialize() (CleanupResult, error) {
	var result CleanupResult
	if r.path == "" {
		return result, errors.New("cache path is empty")
	}
	if err := os.MkdirAll(r.path, 0o750); err != nil {
		return result, fmt.Errorf("creating cache directory: %w", err)
	}
	probe, err := os.CreateTemp(r.path, ".write-probe-*.tmp")
	if err != nil {
		return result, fmt.Errorf("cache directory is not writable: %w", err)
	}
	probeName := probe.Name()
	if closeErr := probe.Close(); closeErr != nil {
		_ = os.Remove(probeName)
		return result, fmt.Errorf("closing cache write probe: %w", closeErr)
	}
	if err := os.Remove(probeName); err != nil {
		return result, fmt.Errorf("removing cache write probe: %w", err)
	}

	dirEntries, err := os.ReadDir(r.path)
	if err != nil {
		return result, fmt.Errorf("reading cache directory: %w", err)
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
				return result, fmt.Errorf("removing incomplete cache file %q: %w", path, err)
			}
			result.TemporaryRemoved++
		case strings.HasSuffix(name, ".data"):
			dataFiles[strings.TrimSuffix(name, ".data")] = path
		case strings.HasSuffix(name, ".json"):
			metadataFiles[strings.TrimSuffix(name, ".json")] = path
		default:
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return result, fmt.Errorf("removing unknown cache file %q: %w", path, err)
			}
			result.OrphansRemoved++
		}
	}

	for key, metadataPath := range metadataFiles {
		dataPath, ok := dataFiles[key]
		if !ok {
			if err := os.Remove(metadataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return result, fmt.Errorf("removing orphan metadata %q: %w", metadataPath, err)
			}
			result.OrphansRemoved++
			continue
		}
		loaded, err := loadEntry(key, dataPath, metadataPath)
		if err != nil {
			if removeErr := removePair(dataPath, metadataPath); removeErr != nil {
				return result, fmt.Errorf("removing invalid cache entry %q: %w", key, removeErr)
			}
			result.CorruptRemoved++
			delete(dataFiles, key)
			continue
		}
		r.entries[key] = loaded
		r.used += loaded.meta.DataSize + loaded.metadataSize
		delete(dataFiles, key)
	}
	for _, dataPath := range dataFiles {
		if err := os.Remove(dataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, fmt.Errorf("removing orphan cache data %q: %w", dataPath, err)
		}
		result.OrphansRemoved++
	}
	if err := r.evictUntilLocked(0); err != nil {
		return result, fmt.Errorf("enforcing cache limit: %w", err)
	}
	return result, nil
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
		meta: meta, dataPath: dataPath, metadataPath: metadataPath, metadataSize: metadataInfo.Size(),
		createdAt: dataInfo.ModTime(), lastUsed: metadataInfo.ModTime(), lastPersisted: metadataInfo.ModTime(),
		lastRequestHit: metadataInfo.ModTime(),
	}, nil
}

func (r *resolver) Open(ctx context.Context, mf *model.MediaFile) (File, error) {
	sourcePath := mf.AbsolutePath()
	if !r.enabled {
		return os.Open(sourcePath)
	}

	openedAt := time.Now()
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil || !sourceInfo.Mode().IsRegular() {
		r.invalidate(keyFor(mf.ID, sourcePath))
		r.runtime.sourceInvalidations.Add(1)
		r.addEvent(Event{Type: "source_deleted", Category: "integrity", MediaID: mf.ID,
			SourcePath: sourcePath, Message: fmt.Sprint(err), PlaybackSuccess: false})
		return os.Open(sourcePath)
	}
	identity := newStreamIdentity(ctx, mf, sourcePath, sourceInfo.Size(), sourceInfo.ModTime().UnixNano())
	key := keyFor(mf.ID, sourcePath)
	now := time.Now()

	fallbackReason := ""
	r.mu.Lock()
	if cached := r.entries[key]; cached != nil && !cached.stale {
		if !sourceMatchesMedia(cached.meta, mf, sourcePath, sourceInfo) {
			fallbackReason = "source-changed"
			r.runtime.sourceInvalidations.Add(1)
			r.markStaleLocked(cached)
		} else {
			file, openErr := os.Open(cached.dataPath)
			if openErr == nil {
				dataInfo, statErr := file.Stat()
				if statErr == nil && dataInfo.Mode().IsRegular() && dataInfo.Size() == cached.meta.DataSize && dataInfo.ModTime().UnixNano() == cached.meta.DataModTime {
					cached.active++
					cached.lastUsed = now
					cached.lastRequestHit = now
					cached.requestHits++
					queueTouch := !cached.touchPending && now.Sub(cached.lastPersisted) >= r.touchInterval
					if queueTouch {
						cached.touchPending = true
					}
					r.mu.Unlock()
					if queueTouch {
						r.scheduleTouch(key)
					}
					r.hits.Add(1)
					return &observedFile{File: file, owner: r, key: key, entry: cached, identity: identity,
						cached: true, openedAt: openedAt}, nil
				}
				_ = file.Close()
			}
			fallbackReason = "cache-open-or-metadata-validation-failed"
			r.markStaleLocked(cached)
		}
	}
	r.mu.Unlock()
	if fallbackReason != "" {
		r.runtime.fallbacks.Add(1)
		r.runtime.fallbackSuccesses.Add(1)
		r.runtime.format[validFormatIndex(identity.format)].fallbacks.Add(1)
		r.addEvent(Event{Type: "fallback_to_source", Category: "hot_cache", MediaID: mf.ID,
			Title: mf.Title, Artist: mf.Artist, SourcePath: sourcePath, Reason: fallbackReason,
			FallbackSuccess: true, PlaybackSuccess: true, Resolved: true})
		if fallbackReason == "source-changed" {
			r.addEvent(Event{Type: "source_changed", Category: "integrity", MediaID: mf.ID,
				Title: mf.Title, Artist: mf.Artist, SourcePath: sourcePath, PlaybackSuccess: true, Resolved: true})
		}
	}

	file, err := os.Open(sourcePath)
	if err != nil {
		r.runtime.sourceReadErrors.Add(1)
		r.runtime.fallbackFailures.Add(1)
		return nil, err
	}
	r.sourceStreams.Add(1)
	r.misses.Add(1)
	return &observedFile{File: file, owner: r, key: key, identity: identity, openedAt: openedAt}, nil
}

func (r *resolver) Stats() Stats {
	r.mu.Lock()
	bytes := r.used
	entries := len(r.entries)
	r.mu.Unlock()
	return Stats{Hits: r.hits.Load(), Misses: r.misses.Load(), Promotions: r.promotions.Load(),
		Failures: r.failures.Load(), Evictions: r.evictions.Load(), Bytes: bytes, Entries: entries}
}

type observedFile struct {
	*os.File
	owner    *resolver
	key      string
	entry    *entry
	identity streamIdentity
	cached   bool
	openedAt time.Time
	once     sync.Once
}

func (f *observedFile) Close() error {
	err := f.File.Close()
	f.once.Do(func() {
		if f.cached {
			f.owner.release(f.key, f.entry)
		} else {
			f.owner.sourceStreams.Add(-1)
		}
	})
	return err
}

func (f *observedFile) HotCacheHit() bool { return f.cached }

func (f *observedFile) BeginPlayback(observation PlaybackObservation) {
	f.owner.sessions.begin(f.identity, observation)
}

func (f *observedFile) ObservePlayback(ctx context.Context, observation PlaybackObservation) {
	if observation.TTFB <= 0 {
		observation.TTFB = time.Since(f.openedAt)
	}
	f.owner.sessions.observe(ctx, f.identity, f.cached, observation)
	f.owner.sampleRequestEvent(f.identity, f.cached, observation)
	if f.cached {
		f.owner.mu.Lock()
		if current := f.owner.entries[f.key]; current == f.entry {
			current.latestTTFB = observation.TTFB
			if observation.RangeHeader != "" {
				current.latestRangeCount++
			}
		}
		f.owner.mu.Unlock()
	}
}

func (r *resolver) sampleRequestEvent(identity streamIdentity, cached bool, observation PlaybackObservation) {
	if r.eventSampleEvery == 0 || r.eventSampleCounter.Add(1)%r.eventSampleEvery != 0 {
		return
	}
	eventType := "request_cache_miss"
	if cached {
		eventType = "request_cache_hit"
	}
	r.addEvent(Event{Type: eventType, Category: "request_sample", MediaID: identity.mediaID,
		Title: identity.title, Artist: identity.artist, PlaybackSuccess: !observation.Cancelled,
		Reason: observation.RangeHeader})
}

func (r *resolver) scheduleTouch(key string) {
	select {
	case r.touchQueue <- key:
	default:
		r.mu.Lock()
		if cached := r.entries[key]; cached != nil {
			cached.touchPending = false
		}
		r.mu.Unlock()
	}
}

func (r *resolver) persistTouch(key string) {
	r.mu.Lock()
	cached := r.entries[key]
	if cached == nil {
		r.mu.Unlock()
		return
	}
	path, touchedAt := cached.metadataPath, cached.lastUsed
	r.mu.Unlock()
	err := os.Chtimes(path, touchedAt, touchedAt)
	r.mu.Lock()
	if current := r.entries[key]; current == cached {
		current.touchPending = false
		if err == nil {
			current.lastPersisted = touchedAt
		}
	}
	r.mu.Unlock()
	if err != nil {
		r.addEvent(Event{Type: "lru_touch_failed", Category: "storage", MediaID: cached.meta.SourceID,
			CachePath: cached.metadataPath, Message: err.Error(), PlaybackSuccess: true, Resolved: true})
		log.Debug("Could not persist original hot-cache LRU access time", "mediaID", cached.meta.SourceID, err)
	}
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
