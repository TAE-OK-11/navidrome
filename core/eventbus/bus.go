// Package eventbus is the in-process async event stream that decouples
// producers (playback, scanner) from consumers (scrobblers, insights).
// Payloads mirror proto/navidrome/integration/v1/events.proto so the same
// contract can ride gRPC later without another translation layer.
package eventbus

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/navidrome/navidrome/core/lifecycle"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/utils/singleton"
)

const (
	TopicScrobble        Topic = "scrobble.submitted"
	TopicNowPlaying      Topic = "playback.now_playing"
	TopicPlaybackReport  Topic = "playback.report"
	TopicScanProgress    Topic = "library.scan_progress"
	TopicScanCompleted   Topic = "library.scan_completed"
	TopicRefreshResource Topic = "ui.refresh_resource"
	TopicScanStatus      Topic = "ui.scan_status"
	TopicNowPlayingCount Topic = "ui.now_playing_count"
	AttrBroadcast              = "broadcast"
	AttrUsername               = "username"
	AttrClientUniqueID         = "client_unique_id"
	defaultQueueSize           = 1024
	defaultWorkers             = 4
	defaultPublishWait         = 50 * time.Millisecond
)

type Topic string

type Event struct {
	ID              string
	Topic           Topic
	OccurredAt      time.Time
	Attrs           map[string]string
	Scrobble        *Scrobble
	NowPlaying      *NowPlaying
	Report          *PlaybackReport
	ScanProgress    *ScanProgress
	Scan            *ScanCompleted
	Refresh         *RefreshResource
	UIScan          *UIScanStatus
	NowPlayingCount *UINowPlayingCount
}

type Scrobble struct {
	UserID      string
	Username    string
	MediaFileID string
	Title       string
	Artist      string
	Album       string
	PlayedAt    time.Time
	Track       model.MediaFile
}

type NowPlaying struct {
	UserID      string
	MediaFileID string
	Title       string
	Artist      string
	PositionSec int
	Track       model.MediaFile
}

type PlaybackReport struct {
	UserID      string
	PlayerID    string
	MediaFileID string
	State       string
	PositionMs  int64
	Data        any
}

type ScanProgress struct {
	LibID           int
	FileCount       uint32
	Path            string
	Phase           string
	ChangesDetected bool
	Warning         string
	Error           string
	ForceUpdate     bool
}

type ScanCompleted struct {
	FullScan        bool
	ChangesDetected bool
	Error           string
	FileCount       int64
	FolderCount     int64
}

// RefreshResource is a UI invalidation (song/album/library/plugin ids).
type RefreshResource struct {
	Resources map[string][]string
}

func (r *RefreshResource) Add(resource string, ids ...string) *RefreshResource {
	if r.Resources == nil {
		r.Resources = make(map[string][]string)
	}
	if len(ids) == 0 {
		r.Resources[resource] = append(r.Resources[resource], "*")
	} else {
		r.Resources[resource] = append(r.Resources[resource], ids...)
	}
	return r
}

// UIScanStatus is the aggregated scan panel payload for SSE clients.
type UIScanStatus struct {
	Scanning    bool
	Count       int64
	FolderCount int64
	Error       string
	ScanType    string
	ElapsedTime time.Duration
}

type UINowPlayingCount struct {
	Count int
}

type Handler func(ctx context.Context, evt Event)

type subscription struct {
	id      uint64
	handler Handler
}

// Bus is a bounded, non-blocking pub/sub. Publish never waits on handlers;
// overflow drops the event and logs so a stuck consumer cannot stall playback.
type Bus struct {
	mu       sync.RWMutex
	subs     map[Topic][]subscription
	nextID   uint64
	queue    chan Event
	workers  int
	stop     chan struct{}
	once     sync.Once
	workerWG sync.WaitGroup
}

func New() *Bus {
	return NewWithSize(defaultQueueSize, defaultWorkers)
}

// Get returns the process-wide bus. Producers and consumers share this
// instance so playback, scan, and search are not wired point-to-point.
func Get() *Bus {
	return singleton.GetInstance(func() *Bus {
		b := New()
		lifecycle.Register(b)
		return b
	})
}

func NewWithSize(queueSize, workers int) *Bus {
	if queueSize < 1 {
		queueSize = defaultQueueSize
	}
	if workers < 1 {
		workers = 1
	}
	b := &Bus{
		subs:    make(map[Topic][]subscription),
		queue:   make(chan Event, queueSize),
		workers: workers,
		stop:    make(chan struct{}),
	}
	for range workers {
		b.workerWG.Add(1)
		go func() {
			defer b.workerWG.Done()
			b.loop()
		}()
	}
	return b
}

func (b *Bus) Subscribe(topic Topic, handler Handler) func() {
	if handler == nil {
		return func() {}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	b.subs[topic] = append(b.subs[topic], subscription{id: id, handler: handler})
	return func() { b.unsubscribe(topic, id) }
}

func (b *Bus) unsubscribe(topic Topic, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.subs[topic]
	kept := list[:0]
	for _, sub := range list {
		if sub.id != id {
			kept = append(kept, sub)
		}
	}
	b.subs[topic] = kept
}

func (b *Bus) Publish(ctx context.Context, evt Event) {
	if evt.ID == "" {
		evt.ID = uuid.NewString()
	}
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now()
	}
	select {
	case b.queue <- evt:
	case <-ctx.Done():
		log.Trace(ctx, "eventbus publish cancelled", "topic", evt.Topic, "id", evt.ID)
	default:
		timer := time.NewTimer(defaultPublishWait)
		select {
		case b.queue <- evt:
		case <-ctx.Done():
		case <-timer.C:
			log.Warn(ctx, "eventbus queue full, dropping event", "topic", evt.Topic, "id", evt.ID)
		}
		timer.Stop()
	}
}

func (b *Bus) PublishSync(ctx context.Context, evt Event) {
	if evt.ID == "" {
		evt.ID = uuid.NewString()
	}
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now()
	}
	b.dispatch(ctx, evt)
}

func (b *Bus) Close() {
	b.once.Do(func() {
		close(b.stop)
		b.workerWG.Wait()
	})
}

func (b *Bus) loop() {
	for {
		select {
		case <-b.stop:
			for {
				select {
				case evt := <-b.queue:
					b.dispatch(context.Background(), evt)
				default:
					return
				}
			}
		case evt := <-b.queue:
			b.dispatch(context.Background(), evt)
		}
	}
}

func (b *Bus) dispatch(ctx context.Context, evt Event) {
	b.mu.RLock()
	list := append([]subscription(nil), b.subs[evt.Topic]...)
	b.mu.RUnlock()
	for _, sub := range list {
		func(h Handler) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error(ctx, "eventbus handler panic", "topic", evt.Topic, "panic", rec)
				}
			}()
			h(ctx, evt)
		}(sub.handler)
	}
}
