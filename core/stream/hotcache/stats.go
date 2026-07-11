package hotcache

import (
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/navidrome/navidrome/model"
)

const formatCount = 11

var formatNames = [formatCount]string{
	"ALAC/M4A", "AAC/M4A", "AAC/ADTS", "FLAC", "MP3", "Opus", "Vorbis", "WAV", "AIFF", "Ogg/Other", "Other",
}

var histogramBoundsMicros = [...]int64{
	100, 250, 500, 1_000, 2_000, 3_000, 5_000, 7_500, 10_000,
	20_000, 50_000, 100_000, 250_000, 500_000, 1_000_000, 2_000_000, 5_000_000,
}

type fixedHistogram struct {
	buckets [len(histogramBoundsMicros) + 1]atomic.Uint64
}

func (h *fixedHistogram) observe(duration time.Duration) {
	micros := duration.Microseconds()
	index := len(histogramBoundsMicros)
	for i, bound := range histogramBoundsMicros {
		if micros <= bound {
			index = i
			break
		}
	}
	h.buckets[index].Add(1)
}

func (h *fixedHistogram) quantile(q float64) int64 {
	var total uint64
	for i := range h.buckets {
		total += h.buckets[i].Load()
	}
	if total == 0 {
		return 0
	}
	target := uint64(math.Ceil(float64(total) * q))
	var accumulated uint64
	for i := range h.buckets {
		accumulated += h.buckets[i].Load()
		if accumulated >= target {
			if i == len(histogramBoundsMicros) {
				return histogramBoundsMicros[len(histogramBoundsMicros)-1]
			}
			return histogramBoundsMicros[i]
		}
	}
	return 0
}

func (h *fixedHistogram) reset() {
	for i := range h.buckets {
		h.buckets[i].Store(0)
	}
}

type formatCounter struct {
	requestHits        atomic.Uint64
	requestMisses      atomic.Uint64
	cachedSessions     atomic.Uint64
	uncachedSessions   atomic.Uint64
	rangeRequests      atomic.Uint64
	cancellations      atomic.Uint64
	fallbacks          atomic.Uint64
	sendfileRequests   atomic.Uint64
	requests           atomic.Uint64
	bytes              atomic.Uint64
	elapsedNanos       atomic.Uint64
	promotionCompleted atomic.Uint64
	promotionFailed    atomic.Uint64
	ttfb               fixedHistogram
}

type runtimeStats struct {
	enabled bool

	totalHTTPRequests         atomic.Uint64
	requestHits               atomic.Uint64
	requestMisses             atomic.Uint64
	rangeRequests             atomic.Uint64
	bytesServedFromCache      atomic.Uint64
	bytesServedFromSource     atomic.Uint64
	sendfileRequests          atomic.Uint64
	fallbacks                 atomic.Uint64
	playSessions              atomic.Uint64
	cachedSessions            atomic.Uint64
	uncachedSessions          atomic.Uint64
	validSessions             atomic.Uint64
	completedSessions         atomic.Uint64
	skippedSessions           atomic.Uint64
	sessionDurationNanos      atomic.Uint64
	sessionRangeRequests      atomic.Uint64
	sessionSeekOperations     atomic.Uint64
	thresholdReached          atomic.Uint64
	thresholdNotReached       atomic.Uint64
	promotionQueued           atomic.Uint64
	promotionStarted          atomic.Uint64
	promotionCompleted        atomic.Uint64
	promotionFailed           atomic.Uint64
	promotionCancelled        atomic.Uint64
	promotionRetries          atomic.Uint64
	promotionBytes            atomic.Uint64
	promotionElapsedNanos     atomic.Uint64
	evictionBytes             atomic.Uint64
	orphanCleanup             atomic.Uint64
	corruptCleanup            atomic.Uint64
	sourceInvalidations       atomic.Uint64
	expectedCancellations     atomic.Uint64
	unexpectedTransportErrors atomic.Uint64
	sourceReadErrors          atomic.Uint64
	cacheReadErrors           atomic.Uint64
	fallbackSuccesses         atomic.Uint64
	fallbackFailures          atomic.Uint64
	hitTTFBTotalNanos         atomic.Uint64
	hitTTFBCount              atomic.Uint64
	missTTFBTotalNanos        atomic.Uint64
	missTTFBCount             atomic.Uint64
	format                    [formatCount]formatCounter
}

func (s *runtimeStats) observeRequest(format int, cached, ranged, sendfile, cancelled bool, expectedBytes int64, elapsed, ttfb time.Duration) {
	if !s.enabled {
		return
	}
	s.totalHTTPRequests.Add(1)
	fc := &s.format[validFormatIndex(format)]
	fc.requests.Add(1)
	if cached {
		s.requestHits.Add(1)
	} else {
		s.requestMisses.Add(1)
	}
	if ranged {
		s.rangeRequests.Add(1)
		fc.rangeRequests.Add(1)
	}
	if sendfile {
		s.sendfileRequests.Add(1)
		fc.sendfileRequests.Add(1)
	}
	if cancelled {
		fc.cancellations.Add(1)
	}
	if expectedBytes > 0 {
		bytes := uint64(expectedBytes)
		fc.bytes.Add(bytes)
		if cached {
			s.bytesServedFromCache.Add(bytes)
		} else {
			s.bytesServedFromSource.Add(bytes)
		}
	}
	if elapsed > 0 {
		fc.elapsedNanos.Add(uint64(elapsed))
	}
	if ttfb > 0 {
		fc.ttfb.observe(ttfb)
		if cached {
			s.hitTTFBTotalNanos.Add(uint64(ttfb))
			s.hitTTFBCount.Add(1)
		} else {
			s.missTTFBTotalNanos.Add(uint64(ttfb))
			s.missTTFBCount.Add(1)
		}
	}
}

func (s *runtimeStats) reset() {
	if !s.enabled {
		return
	}
	values := []*atomic.Uint64{
		&s.totalHTTPRequests, &s.requestHits, &s.requestMisses, &s.rangeRequests, &s.bytesServedFromCache, &s.bytesServedFromSource,
		&s.sendfileRequests, &s.fallbacks, &s.playSessions, &s.cachedSessions, &s.uncachedSessions,
		&s.validSessions, &s.completedSessions, &s.skippedSessions, &s.thresholdReached,
		&s.sessionDurationNanos, &s.sessionRangeRequests, &s.sessionSeekOperations,
		&s.thresholdNotReached, &s.promotionQueued, &s.promotionStarted, &s.promotionCompleted,
		&s.promotionFailed, &s.promotionCancelled, &s.promotionRetries, &s.promotionBytes,
		&s.promotionElapsedNanos, &s.evictionBytes, &s.orphanCleanup, &s.corruptCleanup, &s.sourceInvalidations,
		&s.expectedCancellations, &s.unexpectedTransportErrors,
		&s.sourceReadErrors, &s.cacheReadErrors, &s.fallbackSuccesses, &s.fallbackFailures,
		&s.hitTTFBTotalNanos, &s.hitTTFBCount, &s.missTTFBTotalNanos, &s.missTTFBCount,
	}
	for _, value := range values {
		value.Store(0)
	}
	for i := range s.format {
		fc := &s.format[i]
		for _, value := range []*atomic.Uint64{
			&fc.requestHits, &fc.requestMisses, &fc.cachedSessions, &fc.uncachedSessions,
			&fc.rangeRequests, &fc.cancellations, &fc.fallbacks, &fc.sendfileRequests,
			&fc.requests, &fc.bytes, &fc.elapsedNanos, &fc.promotionCompleted, &fc.promotionFailed,
		} {
			value.Store(0)
		}
		fc.ttfb.reset()
	}
}

type eventRing struct {
	mu     sync.RWMutex
	items  []Event
	start  int
	count  int
	max    int
	nextID atomic.Uint64
}

func newEventRing(maximum int) *eventRing {
	if maximum < 0 {
		maximum = 0
	}
	return &eventRing{items: make([]Event, maximum), max: maximum}
}

func (r *eventRing) add(event Event) {
	if r == nil || r.max == 0 {
		return
	}
	event.ID = r.nextID.Add(1)
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	r.mu.Lock()
	if r.count < r.max {
		index := (r.start + r.count) % r.max
		r.items[index] = event
		r.count++
	} else {
		r.items[r.start] = event
		r.start = (r.start + 1) % r.max
	}
	r.mu.Unlock()
}

func (r *eventRing) list(after uint64, limit int, errorsOnly bool) []Event {
	if r == nil || r.max == 0 {
		return []Event{}
	}
	if limit <= 0 || limit > r.max {
		limit = min(200, r.max)
	}
	r.mu.RLock()
	result := make([]Event, 0, min(limit, r.count))
	for offset := r.count - 1; offset >= 0 && len(result) < limit; offset-- {
		event := r.items[(r.start+offset)%r.max]
		if event.ID <= after || (errorsOnly && !isErrorEvent(event)) {
			continue
		}
		result = append(result, event)
	}
	r.mu.RUnlock()
	return result
}

func (r *eventRing) reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	clear(r.items)
	r.start = 0
	r.count = 0
	r.mu.Unlock()
}

func (r *eventRing) countSince(cutoff time.Time, predicate func(Event) bool) uint64 {
	if r == nil {
		return 0
	}
	var count uint64
	r.mu.RLock()
	for offset := 0; offset < r.count; offset++ {
		event := r.items[(r.start+offset)%r.max]
		if !event.Timestamp.Before(cutoff) && predicate(event) {
			count++
		}
	}
	r.mu.RUnlock()
	return count
}

func isErrorEvent(event Event) bool {
	return isFailureEvent(event) || event.Category == "artwork" || event.Type == "lru_touch_failed" ||
		event.Type == "corrupt_entry_removed" || event.Type == "source_deleted"
}

func formatIndexFor(mf *model.MediaFile) int {
	codec := strings.ToLower(strings.TrimSpace(mf.Codec))
	extension := strings.ToLower(strings.TrimPrefix(mf.Suffix, "."))
	switch {
	case codec == "alac" && isM4A(extension):
		return 0
	case codec == "aac" && isM4A(extension):
		return 1
	case codec == "aac" || extension == "aac" || extension == "adts":
		return 2
	case codec == "flac" || extension == "flac":
		return 3
	case codec == "mp3" || extension == "mp3":
		return 4
	case codec == "opus" || extension == "opus":
		return 5
	case codec == "vorbis":
		return 6
	case codec == "pcm_s16le" || extension == "wav":
		return 7
	case extension == "aif" || extension == "aiff":
		return 8
	case extension == "ogg" || extension == "oga":
		return 9
	default:
		return 10
	}
}

func validFormatIndex(index int) int {
	if index < 0 || index >= formatCount {
		return formatCount - 1
	}
	return index
}

func isM4A(extension string) bool {
	return extension == "m4a" || extension == "m4b" || extension == "mp4" || extension == "alac"
}

func ratio(numerator, denominator uint64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func averageDuration(total, count uint64) time.Duration {
	if count == 0 {
		return 0
	}
	return time.Duration(total / count)
}
