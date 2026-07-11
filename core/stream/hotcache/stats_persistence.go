package hotcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"
)

const runtimeStatsVersion = 1

type persistedRuntimeStats struct {
	Version  int               `json:"version"`
	SavedAt  time.Time         `json:"savedAt"`
	Counters map[string]uint64 `json:"counters"`
}

func (s *runtimeStats) counterBindings() map[string]*atomic.Uint64 {
	bindings := map[string]*atomic.Uint64{
		"http.total": &s.totalHTTPRequests, "http.hits": &s.requestHits, "http.misses": &s.requestMisses,
		"http.ranges": &s.rangeRequests, "http.cacheBytes": &s.bytesServedFromCache,
		"http.sourceBytes": &s.bytesServedFromSource, "http.sendfile": &s.sendfileRequests,
		"http.fallbacks": &s.fallbacks, "session.total": &s.playSessions,
		"session.cached": &s.cachedSessions, "session.uncached": &s.uncachedSessions,
		"session.valid": &s.validSessions, "session.completed": &s.completedSessions,
		"session.skipped": &s.skippedSessions, "session.durationNanos": &s.sessionDurationNanos,
		"session.ranges": &s.sessionRangeRequests, "session.seeks": &s.sessionSeekOperations,
		"threshold.reached": &s.thresholdReached, "threshold.notReached": &s.thresholdNotReached,
		"promotion.queued": &s.promotionQueued, "promotion.started": &s.promotionStarted,
		"promotion.completed": &s.promotionCompleted, "promotion.failed": &s.promotionFailed,
		"promotion.cancelled": &s.promotionCancelled, "promotion.retries": &s.promotionRetries,
		"promotion.bytes": &s.promotionBytes, "promotion.elapsedNanos": &s.promotionElapsedNanos,
		"storage.evictionBytes": &s.evictionBytes, "storage.orphans": &s.orphanCleanup,
		"storage.corrupt": &s.corruptCleanup, "storage.invalidations": &s.sourceInvalidations,
		"transport.expectedCancel": &s.expectedCancellations,
		"transport.unexpected":     &s.unexpectedTransportErrors, "transport.sourceRead": &s.sourceReadErrors,
		"transport.cacheRead": &s.cacheReadErrors, "transport.fallbackSuccess": &s.fallbackSuccesses,
		"transport.fallbackFailure": &s.fallbackFailures, "ttfb.hitNanos": &s.hitTTFBTotalNanos,
		"ttfb.hitCount": &s.hitTTFBCount, "ttfb.missNanos": &s.missTTFBTotalNanos,
		"ttfb.missCount": &s.missTTFBCount,
	}
	for formatIndex := range s.format {
		prefix := "format." + strconv.Itoa(formatIndex) + "."
		counter := &s.format[formatIndex]
		bindings[prefix+"requestHits"] = &counter.requestHits
		bindings[prefix+"requestMisses"] = &counter.requestMisses
		bindings[prefix+"cachedSessions"] = &counter.cachedSessions
		bindings[prefix+"uncachedSessions"] = &counter.uncachedSessions
		bindings[prefix+"ranges"] = &counter.rangeRequests
		bindings[prefix+"cancellations"] = &counter.cancellations
		bindings[prefix+"fallbacks"] = &counter.fallbacks
		bindings[prefix+"sendfile"] = &counter.sendfileRequests
		bindings[prefix+"requests"] = &counter.requests
		bindings[prefix+"bytes"] = &counter.bytes
		bindings[prefix+"elapsedNanos"] = &counter.elapsedNanos
		bindings[prefix+"promotionCompleted"] = &counter.promotionCompleted
		bindings[prefix+"promotionFailed"] = &counter.promotionFailed
		for bucketIndex := range counter.ttfb.buckets {
			bindings[prefix+"ttfb."+strconv.Itoa(bucketIndex)] = &counter.ttfb.buckets[bucketIndex]
		}
	}
	return bindings
}

func (r *resolver) persistRuntimeStats() error {
	if !r.runtime.enabled || r.statsPath == "" {
		return nil
	}
	counters := make(map[string]uint64)
	for name, counter := range r.runtime.counterBindings() {
		if value := counter.Load(); value != 0 {
			counters[name] = value
		}
	}
	raw, err := json.Marshal(persistedRuntimeStats{Version: runtimeStatsVersion, SavedAt: time.Now(), Counters: counters})
	if err != nil {
		return fmt.Errorf("encoding statistics: %w", err)
	}
	directory := filepath.Dir(r.statsPath)
	temporary, err := os.CreateTemp(directory, ".hot-cache-stats-*.tmp")
	if err != nil {
		return fmt.Errorf("creating statistics snapshot: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(raw)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("writing statistics snapshot: %w", err)
	}
	if err := os.Rename(temporaryPath, r.statsPath); err != nil {
		return fmt.Errorf("publishing statistics snapshot: %w", err)
	}
	return nil
}

func (r *resolver) loadRuntimeStats() error {
	raw, err := os.ReadFile(r.statsPath)
	if err != nil {
		return err
	}
	var snapshot persistedRuntimeStats
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return fmt.Errorf("decoding statistics snapshot: %w", err)
	}
	if snapshot.Version != runtimeStatsVersion || snapshot.Counters == nil {
		return errors.New("unsupported statistics snapshot")
	}
	for name, counter := range r.runtime.counterBindings() {
		counter.Store(snapshot.Counters[name])
	}
	return nil
}
