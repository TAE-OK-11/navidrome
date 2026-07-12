package hotcache

import (
	"context"
	"io"
	"syscall"
	"time"

	"github.com/navidrome/navidrome/model"
)

// File is the file-backed stream returned by a Resolver. Keeping syscall.Conn
// in the contract preserves net/http's sendfile path for cache hits and misses.
type File interface {
	io.ReadSeekCloser
	syscall.Conn
}

// Resolver selects a verified hot-cache file or immediately opens the source.
// The small interface keeps Hot Cache disabled mode effectively equivalent to
// opening the source directly.
type Resolver interface {
	Open(ctx context.Context, mf *model.MediaFile) (File, error)
	Stats() Stats
}

// Manager is the control-plane surface used by the administrator API. None of
// these methods are called from direct streaming unless explicitly documented.
type Manager interface {
	Resolver
	Status() StatusSnapshot
	Sessions() []SessionSnapshot
	Entries(query EntryQuery) EntryPage
	Queue() []QueueItemSnapshot
	CurrentPromotion() *PromotionSnapshot
	Events(after uint64, limit int) []Event
	Errors(after uint64, limit int) []Event
	Formats() []FormatSnapshot
	MediaStates(mediaIDs []string) map[string]string
	Promote(ctx context.Context, mf *model.MediaFile) error
	Remove(mediaID string) error
	Retry(ctx context.Context, mf *model.MediaFile) error
	Cancel(mediaID string) error
	Pause()
	Resume()
	Verify(ctx context.Context) VerificationResult
	Cleanup(ctx context.Context) CleanupResult
	Purge(ctx context.Context) PurgeResult
	ResetEvents()
	ResetStats()
	Shutdown(ctx context.Context) error
}

// PlaybackObservation is emitted once after a direct HTTP request finishes.
// RangeBytes is the normalized requested interval, not an allocation-heavy
// copy of the HTTP request. Elapsed and unique byte coverage are both used to
// avoid treating fast downloads or seeks as actual listening time.
type PlaybackObservation struct {
	Playback      bool
	RangeHeader   string
	Method        string
	RemoteAddr    string
	UserAgent     string
	Elapsed       time.Duration
	TTFB          time.Duration
	Cancelled     bool
	BytesExpected int64
	Sendfile      bool
}

// BeginPlayback reports an active direct-play request. Tracking starts before
// ServeContent so a cancelled range request is not mistaken for a stopped
// player when its replacement request is already active.
func BeginPlayback(file io.ReadCloser, observation PlaybackObservation) {
	if observer, ok := file.(interface{ BeginPlayback(PlaybackObservation) }); ok {
		observer.BeginPlayback(observation)
	}
}

// ObservePlayback reports a completed HTTP request without exposing the
// resolver implementation to the streaming package.
func ObservePlayback(file io.ReadCloser, ctx context.Context, observation PlaybackObservation) {
	if observer, ok := file.(interface {
		ObservePlayback(context.Context, PlaybackObservation)
	}); ok {
		observer.ObservePlayback(ctx, observation)
	}
}

// IsHit reports whether a resolved file is backed by the original hot cache.
func IsHit(file File) bool {
	marked, ok := file.(interface{ HotCacheHit() bool })
	return ok && marked.HotCacheHit()
}

type Stats struct {
	Hits       uint64 `json:"hits"`
	Misses     uint64 `json:"misses"`
	Promotions uint64 `json:"promotions"`
	Failures   uint64 `json:"failures"`
	Evictions  uint64 `json:"evictions"`
	Bytes      int64  `json:"bytes"`
	Entries    int    `json:"entries"`
}

type Options struct {
	Enabled                 bool
	Path                    string
	MaxSize                 int64
	PromoteOnPlay           bool
	SessionWindow           time.Duration
	SessionIdleTimeout      time.Duration
	MaxSessions             int
	MinPlaySeconds          int
	MinPlayPercent          int
	PromotionConcurrency    int
	QueueMax                int
	PromotionDelayAfterPlay time.Duration
	PromotionMaxRetries     int
	TouchInterval           time.Duration
	StatsEnabled            bool
	EventsMax               int
	StatsFlushInterval      time.Duration
	EventSampleRate         float64
}

type StatusSnapshot struct {
	Enabled                     bool          `json:"enabled"`
	Health                      string        `json:"health"`
	Paused                      bool          `json:"paused"`
	Path                        string        `json:"path"`
	UsedBytes                   int64         `json:"usedBytes"`
	MaxBytes                    int64         `json:"maxBytes"`
	UsagePercent                float64       `json:"usagePercent"`
	ReservedBytes               int64         `json:"reservedBytes"`
	PinnedBytes                 int64         `json:"pinnedBytes"`
	Entries                     int           `json:"entries"`
	QueueLength                 int           `json:"queueLength"`
	ActivePromotion             bool          `json:"activePromotion"`
	ActiveSessions              int           `json:"activeSessions"`
	RequestHits                 uint64        `json:"requestHits"`
	RequestMisses               uint64        `json:"requestMisses"`
	RequestHitRate              float64       `json:"requestHitRate"`
	CachedSessions              uint64        `json:"cachedSessions"`
	UncachedSessions            uint64        `json:"uncachedSessions"`
	SessionHitRate              float64       `json:"sessionHitRate"`
	Fallbacks                   uint64        `json:"fallbacks"`
	Evictions                   uint64        `json:"evictions"`
	ExpectedCancellations       uint64        `json:"expectedCancellations"`
	UnexpectedTransportErrors   uint64        `json:"unexpectedTransportErrors"`
	Errors24h                   uint64        `json:"errors24h"`
	ArtworkErrors24h            uint64        `json:"artworkErrors24h"`
	AverageHitTTFB              time.Duration `json:"averageHitTtfb"`
	AverageMissTTFB             time.Duration `json:"averageMissTtfb"`
	TotalHTTPRequests           uint64        `json:"totalHttpRequests"`
	RangeRequests               uint64        `json:"rangeRequests"`
	PlaySessions                uint64        `json:"playSessions"`
	ValidSessions               uint64        `json:"validSessions"`
	CompletedSessions           uint64        `json:"completedSessions"`
	SkippedSessions             uint64        `json:"skippedSessions"`
	AveragePlayDuration         time.Duration `json:"averagePlayDuration"`
	AverageRangeRequestsSession float64       `json:"averageRangeRequestsPerSession"`
	SeekOperations              uint64        `json:"seekOperations"`
	PromotionCompleted          uint64        `json:"promotionCompleted"`
	PromotionFailed             uint64        `json:"promotionFailed"`
	PromotionBytes              uint64        `json:"promotionBytes"`
	AveragePromotionDuration    time.Duration `json:"averagePromotionDuration"`
	AveragePromotionSpeed       float64       `json:"averagePromotionSpeed"`
	EvictionBytes               uint64        `json:"evictionBytes"`
	OrphanCleanup               uint64        `json:"orphanCleanup"`
	CorruptCleanup              uint64        `json:"corruptCleanup"`
	SourceInvalidations         uint64        `json:"sourceInvalidations"`
	BytesServedFromCache        uint64        `json:"bytesServedFromCache"`
	BytesServedFromSource       uint64        `json:"bytesServedFromSource"`
	SendfileRequests            uint64        `json:"sendfileRequests"`
	CacheInitializationDuration time.Duration `json:"cacheInitializationDuration"`
}

type SessionSnapshot struct {
	ID              string        `json:"id"`
	MediaID         string        `json:"mediaId"`
	Title           string        `json:"title"`
	Artist          string        `json:"artist"`
	Album           string        `json:"album"`
	User            string        `json:"user"`
	Player          string        `json:"player"`
	Codec           string        `json:"codec"`
	Container       string        `json:"container"`
	Cached          bool          `json:"cached"`
	PlayedDuration  time.Duration `json:"playedDuration"`
	TotalDuration   time.Duration `json:"totalDuration"`
	PlayedPercent   float64       `json:"playedPercent"`
	RequestCount    uint64        `json:"requestCount"`
	RangeCount      uint64        `json:"rangeCount"`
	SeekCount       uint64        `json:"seekCount"`
	ThresholdState  string        `json:"thresholdState"`
	Pinned          bool          `json:"pinned"`
	LastActivity    time.Time     `json:"lastActivity"`
	TransferPath    string        `json:"transferPath"`
	Sendfile        bool          `json:"sendfile"`
	ClientCancelled uint64        `json:"clientCancelled"`
}

type EntrySnapshot struct {
	MediaID              string        `json:"mediaId"`
	Title                string        `json:"title"`
	Artist               string        `json:"artist"`
	Album                string        `json:"album"`
	Codec                string        `json:"codec"`
	Container            string        `json:"container"`
	Extension            string        `json:"extension"`
	BitRate              int           `json:"bitRate"`
	FileSize             int64         `json:"fileSize"`
	CreatedAt            time.Time     `json:"createdAt"`
	LastSessionHit       time.Time     `json:"lastSessionHit"`
	LastRequestHit       time.Time     `json:"lastRequestHit"`
	SessionHits          uint64        `json:"sessionHits"`
	RequestHits          uint64        `json:"requestHits"`
	SourceModTime        time.Time     `json:"sourceModTime"`
	CacheModTime         time.Time     `json:"cacheModTime"`
	SHA256State          string        `json:"sha256State"`
	IntegrityState       string        `json:"integrityState"`
	Pinned               bool          `json:"pinned"`
	Playing              bool          `json:"playing"`
	LRURank              int           `json:"lruRank"`
	ExpectedEvictionRank int           `json:"expectedEvictionRank"`
	SendfileCapable      bool          `json:"sendfileCapable"`
	LastTransferPath     string        `json:"lastTransferPath"`
	LatestTTFB           time.Duration `json:"latestTtfb"`
	LatestRangeCount     uint64        `json:"latestRangeCount"`
}

type EntryQuery struct {
	Search string
	Format string
	State  string
	Sort   string
	Order  string
	Offset int
	Limit  int
}

type EntryPage struct {
	Items []EntrySnapshot `json:"items"`
	Total int             `json:"total"`
}

type QueueItemSnapshot struct {
	Position        int           `json:"position"`
	MediaID         string        `json:"mediaId"`
	Title           string        `json:"title"`
	Artist          string        `json:"artist"`
	Album           string        `json:"album"`
	Codec           string        `json:"codec"`
	Container       string        `json:"container"`
	SourceSize      int64         `json:"sourceSize"`
	PlayedDuration  time.Duration `json:"playedDuration"`
	PlayedPercent   float64       `json:"playedPercent"`
	Threshold       string        `json:"threshold"`
	ThresholdReason string        `json:"thresholdReason"`
	QueuedAt        time.Time     `json:"queuedAt"`
	State           string        `json:"state"`
	RetryCount      int           `json:"retryCount"`
	LastError       string        `json:"lastError,omitempty"`
}

type PromotionSnapshot struct {
	MediaID     string        `json:"mediaId"`
	Title       string        `json:"title"`
	SourcePath  string        `json:"sourcePath"`
	CachePath   string        `json:"cachePath"`
	BytesCopied int64         `json:"bytesCopied"`
	TotalBytes  int64         `json:"totalBytes"`
	Progress    float64       `json:"progress"`
	Speed       float64       `json:"speed"`
	Elapsed     time.Duration `json:"elapsed"`
	Phase       string        `json:"phase"`
}

type Event struct {
	ID              uint64    `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	Type            string    `json:"type"`
	Category        string    `json:"category"`
	Code            string    `json:"code,omitempty"`
	MediaID         string    `json:"mediaId,omitempty"`
	Title           string    `json:"title,omitempty"`
	Artist          string    `json:"artist,omitempty"`
	SessionID       string    `json:"sessionId,omitempty"`
	SourcePath      string    `json:"sourcePath,omitempty"`
	CachePath       string    `json:"cachePath,omitempty"`
	FallbackSuccess bool      `json:"fallbackSuccess"`
	PlaybackSuccess bool      `json:"playbackSuccess"`
	RetryCount      int       `json:"retryCount"`
	Message         string    `json:"message,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	Resolved        bool      `json:"resolved"`
}

type FormatSnapshot struct {
	Format             string  `json:"format"`
	Entries            int     `json:"entries"`
	Bytes              int64   `json:"bytes"`
	AverageFileSize    int64   `json:"averageFileSize"`
	RequestHits        uint64  `json:"requestHits"`
	RequestMisses      uint64  `json:"requestMisses"`
	RequestHitRate     float64 `json:"requestHitRate"`
	CachedSessions     uint64  `json:"cachedSessions"`
	UncachedSessions   uint64  `json:"uncachedSessions"`
	SessionHitRate     float64 `json:"sessionHitRate"`
	TTFBP50Micros      int64   `json:"ttfbP50Micros"`
	TTFBP95Micros      int64   `json:"ttfbP95Micros"`
	TTFBP99Micros      int64   `json:"ttfbP99Micros"`
	ThroughputBytesSec float64 `json:"throughputBytesPerSecond"`
	SendfileRate       float64 `json:"sendfileRate"`
	RangeRequests      uint64  `json:"rangeRequests"`
	Cancellations      uint64  `json:"cancellations"`
	Fallbacks          uint64  `json:"fallbacks"`
	PromotionCompleted uint64  `json:"promotionCompleted"`
	PromotionFailed    uint64  `json:"promotionFailed"`
}

type VerificationResult struct {
	Checked int `json:"checked"`
	Valid   int `json:"valid"`
	Removed int `json:"removed"`
	Failed  int `json:"failed"`
}

type CleanupResult struct {
	TemporaryRemoved int `json:"temporaryRemoved"`
	OrphansRemoved   int `json:"orphansRemoved"`
	CorruptRemoved   int `json:"corruptRemoved"`
}

type PurgeResult struct {
	Removed int   `json:"removed"`
	Skipped int   `json:"skipped"`
	Bytes   int64 `json:"bytes"`
}
