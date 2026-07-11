package hotcache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

func (r *resolver) Status() StatusSnapshot {
	r.mu.Lock()
	used, reserved, entries := r.used, r.reserved, len(r.entries)
	queueLength := len(r.promoting)
	activePromotion := r.current != nil
	if activePromotion && queueLength > 0 {
		queueLength--
	}
	var pinnedBytes int64
	for _, cached := range r.entries {
		if cached.active > 0 || r.playing[cached.meta.SourceID] > 0 {
			pinnedBytes += cached.meta.DataSize
		}
	}
	r.mu.Unlock()

	hits, misses := r.runtime.requestHits.Load(), r.runtime.requestMisses.Load()
	cachedSessions, uncachedSessions := r.runtime.cachedSessions.Load(), r.runtime.uncachedSessions.Load()
	cutoff := time.Now().Add(-24 * time.Hour)
	errors24 := r.events.countSince(cutoff, func(event Event) bool { return isFailureEvent(event) })
	artworkErrors24 := r.events.countSince(cutoff, func(event Event) bool { return event.Category == "artwork" })
	health := "healthy"
	switch {
	case !r.enabled:
		health = "disabled"
	case r.paused.Load():
		health = "paused"
	case errors24 > 0:
		health = "degraded"
	}
	usage := float64(0)
	if r.maxSize > 0 {
		usage = float64(used+reserved) * 100 / float64(r.maxSize)
	}
	activeSessions := 0
	if r.sessions != nil {
		activeSessions = int(r.sessions.count.Load())
	}
	playSessions := r.runtime.playSessions.Load()
	promotionCompleted := r.runtime.promotionCompleted.Load()
	promotionBytes := r.runtime.promotionBytes.Load()
	promotionElapsed := r.runtime.promotionElapsedNanos.Load()
	averagePromotionSpeed := float64(0)
	if promotionElapsed > 0 {
		averagePromotionSpeed = float64(promotionBytes) * float64(time.Second) / float64(promotionElapsed)
	}
	return StatusSnapshot{
		Enabled: r.enabled, Health: health, Paused: r.paused.Load(), Path: r.path,
		UsedBytes: used, MaxBytes: r.maxSize, UsagePercent: usage, ReservedBytes: reserved,
		PinnedBytes: pinnedBytes, Entries: entries, QueueLength: queueLength,
		ActivePromotion: activePromotion, ActiveSessions: activeSessions,
		RequestHits: hits, RequestMisses: misses, RequestHitRate: ratio(hits, hits+misses),
		CachedSessions: cachedSessions, UncachedSessions: uncachedSessions,
		SessionHitRate: ratio(cachedSessions, cachedSessions+uncachedSessions),
		Fallbacks:      r.runtime.fallbacks.Load(), Evictions: r.evictions.Load(),
		ExpectedCancellations:     r.runtime.expectedCancellations.Load(),
		UnexpectedTransportErrors: r.runtime.unexpectedTransportErrors.Load(),
		Errors24h:                 errors24, ArtworkErrors24h: artworkErrors24,
		AverageHitTTFB:    averageDuration(r.runtime.hitTTFBTotalNanos.Load(), r.runtime.hitTTFBCount.Load()),
		AverageMissTTFB:   averageDuration(r.runtime.missTTFBTotalNanos.Load(), r.runtime.missTTFBCount.Load()),
		TotalHTTPRequests: r.runtime.totalHTTPRequests.Load(), RangeRequests: r.runtime.rangeRequests.Load(),
		PlaySessions: playSessions, ValidSessions: r.runtime.validSessions.Load(),
		CompletedSessions: r.runtime.completedSessions.Load(), SkippedSessions: r.runtime.skippedSessions.Load(),
		AveragePlayDuration:         averageDuration(r.runtime.sessionDurationNanos.Load(), playSessions),
		AverageRangeRequestsSession: float64(r.runtime.sessionRangeRequests.Load()) / float64(max(playSessions, 1)),
		SeekOperations:              r.runtime.sessionSeekOperations.Load(),
		PromotionCompleted:          promotionCompleted, PromotionFailed: r.runtime.promotionFailed.Load(),
		PromotionBytes: promotionBytes, AveragePromotionDuration: averageDuration(promotionElapsed, promotionCompleted),
		AveragePromotionSpeed: averagePromotionSpeed, EvictionBytes: r.runtime.evictionBytes.Load(),
		OrphanCleanup: r.runtime.orphanCleanup.Load(), CorruptCleanup: r.runtime.corruptCleanup.Load(),
		SourceInvalidations:   r.runtime.sourceInvalidations.Load(),
		BytesServedFromCache:  r.runtime.bytesServedFromCache.Load(),
		BytesServedFromSource: r.runtime.bytesServedFromSource.Load(),
		SendfileRequests:      r.runtime.sendfileRequests.Load(), CacheInitializationDuration: r.initializedIn,
	}
}

func (r *resolver) Sessions() []SessionSnapshot {
	if r.sessions == nil {
		return []SessionSnapshot{}
	}
	result := r.sessions.snapshots()
	r.mu.Lock()
	for i := range result {
		result[i].Pinned = r.playing[result[i].MediaID] > 0
	}
	r.mu.Unlock()
	return result
}

func (r *resolver) Entries(query EntryQuery) EntryPage {
	r.mu.Lock()
	ordered := make([]*entry, 0, len(r.entries))
	for _, cached := range r.entries {
		ordered = append(ordered, cached)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].lastUsed.Before(ordered[j].lastUsed) })
	ranks := make(map[string]int, len(ordered))
	for i, cached := range ordered {
		ranks[cached.meta.Key] = i + 1
	}
	items := make([]EntrySnapshot, 0, len(ordered))
	for _, cached := range ordered {
		item := entrySnapshot(cached, ranks[cached.meta.Key], r.playing[cached.meta.SourceID] > 0)
		if entryMatches(item, query) {
			items = append(items, item)
		}
	}
	r.mu.Unlock()

	sortEntries(items, query.Sort, query.Order)
	total := len(items)
	offset := max(query.Offset, 0)
	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	if offset >= total {
		return EntryPage{Items: []EntrySnapshot{}, Total: total}
	}
	return EntryPage{Items: items[offset:min(total, offset+limit)], Total: total}
}

func entrySnapshot(cached *entry, rank int, playing bool) EntrySnapshot {
	return EntrySnapshot{
		MediaID: cached.meta.SourceID, Title: cached.meta.Title, Artist: cached.meta.Artist,
		Album: cached.meta.Album, Codec: cached.meta.Codec, Container: cached.meta.Container,
		Extension: cached.meta.Extension, BitRate: cached.meta.BitRate, FileSize: cached.meta.DataSize,
		CreatedAt: cached.createdAt, LastSessionHit: cached.lastSessionHit, LastRequestHit: cached.lastRequestHit,
		SessionHits: cached.sessionHits, RequestHits: cached.requestHits,
		SourceModTime: time.Unix(0, cached.meta.SourceModTime), CacheModTime: time.Unix(0, cached.meta.DataModTime),
		SHA256State: "verified", IntegrityState: integrityState(cached), Pinned: cached.active > 0,
		Playing: playing, LRURank: rank, ExpectedEvictionRank: rank, SendfileCapable: true,
		LastTransferPath: "nvme-cache", LatestTTFB: cached.latestTTFB, LatestRangeCount: cached.latestRangeCount,
	}
}

func integrityState(cached *entry) string {
	if cached.stale {
		return "stale"
	}
	return "verified"
}

func entryMatches(item EntrySnapshot, query EntryQuery) bool {
	search := strings.ToLower(strings.TrimSpace(query.Search))
	if search != "" && !strings.Contains(strings.ToLower(item.Title+"\x00"+item.Artist+"\x00"+item.Album+"\x00"+item.MediaID), search) {
		return false
	}
	format := strings.ToLower(strings.TrimSpace(query.Format))
	if format != "" && format != "all" && !strings.Contains(strings.ToLower(item.Codec+"/"+item.Container+"/"+item.Extension), format) {
		return false
	}
	switch strings.ToLower(query.State) {
	case "pinned":
		return item.Pinned
	case "playing":
		return item.Playing
	case "corrupted":
		return item.IntegrityState != "verified"
	case "recently-used":
		return time.Since(item.LastRequestHit) <= 24*time.Hour
	default:
		return true
	}
}

func sortEntries(items []EntrySnapshot, field, order string) {
	descending := strings.EqualFold(order, "desc")
	less := func(i, j int) bool {
		switch strings.ToLower(field) {
		case "recent", "lastrequesthit":
			return items[i].LastRequestHit.Before(items[j].LastRequestHit)
		case "sessionhits":
			return items[i].SessionHits < items[j].SessionHits
		case "requesthits":
			return items[i].RequestHits < items[j].RequestHits
		case "largest", "size":
			return items[i].FileSize < items[j].FileSize
		case "slowest", "ttfb":
			return items[i].LatestTTFB < items[j].LatestTTFB
		default:
			return items[i].LRURank < items[j].LRURank
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if descending {
			return less(j, i)
		}
		return less(i, j)
	})
}

func (r *resolver) Queue() []QueueItemSnapshot {
	r.mu.Lock()
	tasks := make([]*promotionTask, 0, len(r.promoting))
	for _, task := range r.promoting {
		if task != r.current {
			tasks = append(tasks, task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].queuedAt.Before(tasks[j].queuedAt) })
	result := make([]QueueItemSnapshot, len(tasks))
	for i, task := range tasks {
		result[i] = queueSnapshot(task, i+1)
	}
	r.mu.Unlock()
	return result
}

func queueSnapshot(task *promotionTask, position int) QueueItemSnapshot {
	return QueueItemSnapshot{
		Position: position, MediaID: task.identity.mediaID, Title: task.identity.title,
		Artist: task.identity.artist, Album: task.identity.album, Codec: task.identity.codec,
		Container: task.identity.container, SourceSize: task.identity.sourceSize,
		PlayedDuration: task.playedDuration, PlayedPercent: task.playedPercent, Threshold: task.threshold,
		ThresholdReason: task.thresholdReason, QueuedAt: task.queuedAt, State: task.state,
		RetryCount: task.retryCount, LastError: task.lastError,
	}
}

func (r *resolver) CurrentPromotion() *PromotionSnapshot {
	r.mu.Lock()
	task := r.current
	if task == nil {
		r.mu.Unlock()
		return nil
	}
	bytesCopied := task.bytesCopied.Load()
	elapsed := time.Since(task.startedAt)
	result := &PromotionSnapshot{
		MediaID: task.identity.mediaID, Title: task.identity.title, SourcePath: task.identity.sourcePath,
		CachePath: task.cachePath, BytesCopied: bytesCopied, TotalBytes: task.identity.sourceSize,
		Elapsed: elapsed, Phase: task.phase, Speed: promotionSpeed(bytesCopied, elapsed),
	}
	if task.identity.sourceSize > 0 {
		result.Progress = float64(bytesCopied) * 100 / float64(task.identity.sourceSize)
	}
	r.mu.Unlock()
	return result
}

func (r *resolver) Events(after uint64, limit int) []Event {
	return r.events.list(after, limit, false)
}

func (r *resolver) Errors(after uint64, limit int) []Event {
	return r.events.list(after, limit, true)
}

func (r *resolver) Formats() []FormatSnapshot {
	entries := [formatCount]int{}
	bytes := [formatCount]int64{}
	r.mu.Lock()
	for _, cached := range r.entries {
		index := formatIndexFromMetadata(cached.meta)
		entries[index]++
		bytes[index] += cached.meta.DataSize
	}
	r.mu.Unlock()
	result := make([]FormatSnapshot, formatCount)
	for i := range result {
		counter := &r.runtime.format[i]
		hits, misses := counter.requestHits.Load(), counter.requestMisses.Load()
		cachedSessions, uncachedSessions := counter.cachedSessions.Load(), counter.uncachedSessions.Load()
		requests := counter.requests.Load()
		item := FormatSnapshot{
			Format: formatNames[i], Entries: entries[i], Bytes: bytes[i], RequestHits: hits,
			RequestMisses: misses, RequestHitRate: ratio(hits, hits+misses), CachedSessions: cachedSessions,
			UncachedSessions: uncachedSessions, SessionHitRate: ratio(cachedSessions, cachedSessions+uncachedSessions),
			TTFBP50Micros: counter.ttfb.quantile(0.50), TTFBP95Micros: counter.ttfb.quantile(0.95),
			TTFBP99Micros: counter.ttfb.quantile(0.99), RangeRequests: counter.rangeRequests.Load(),
			Cancellations: counter.cancellations.Load(), Fallbacks: counter.fallbacks.Load(),
			PromotionCompleted: counter.promotionCompleted.Load(), PromotionFailed: counter.promotionFailed.Load(),
			SendfileRate: ratio(counter.sendfileRequests.Load(), requests),
		}
		if entries[i] > 0 {
			item.AverageFileSize = bytes[i] / int64(entries[i])
		}
		if elapsed := counter.elapsedNanos.Load(); elapsed > 0 {
			item.ThroughputBytesSec = float64(counter.bytes.Load()) * float64(time.Second) / float64(elapsed)
		}
		result[i] = item
	}
	return result
}

// MediaStates returns cache-control state for the requested media IDs in one
// bounded pass. It is used by the administrator search UI and never touches
// the streaming hot path.
func (r *resolver) MediaStates(mediaIDs []string) map[string]string {
	wanted := make(map[string]struct{}, len(mediaIDs))
	result := make(map[string]string, len(mediaIDs))
	for _, mediaID := range mediaIDs {
		if mediaID != "" {
			wanted[mediaID] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return result
	}

	r.mu.Lock()
	for _, cached := range r.entries {
		if _, ok := wanted[cached.meta.SourceID]; ok && !cached.stale {
			result[cached.meta.SourceID] = "cached"
		}
	}
	for _, task := range r.promoting {
		mediaID := task.identity.mediaID
		if _, ok := wanted[mediaID]; !ok || task.cancelled.Load() {
			continue
		}
		state := "queued"
		if task == r.current || task.state == "copying" {
			state = "copying"
		}
		result[mediaID] = state
	}
	r.mu.Unlock()
	return result
}

func formatIndexFromMetadata(meta metadata) int {
	mf := model.MediaFile{Codec: meta.Codec, Suffix: meta.Extension}
	if mf.Suffix == "" {
		mf.Suffix = meta.Container
	}
	return formatIndexFor(&mf)
}

func (r *resolver) Promote(ctx context.Context, mf *model.MediaFile) error {
	if !r.enabled {
		return errors.New("hot cache is disabled")
	}
	info, err := os.Stat(mf.AbsolutePath())
	if err != nil {
		return err
	}
	identity := newStreamIdentity(ctx, mf, mf.AbsolutePath(), info.Size(), info.ModTime().UnixNano())
	return r.queuePromotion(identity, 0, 0, "manual", "")
}

func (r *resolver) Retry(ctx context.Context, mf *model.MediaFile) error {
	return r.Promote(ctx, mf)
}

func (r *resolver) Remove(mediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.playing[mediaID] > 0 {
		return errors.New("cache entry is currently playing")
	}
	for _, task := range r.promoting {
		if task.identity.mediaID == mediaID && !task.cancelled.Load() {
			return errors.New("cache entry is being promoted")
		}
	}
	for key, cached := range r.entries {
		if cached.meta.SourceID == mediaID {
			if cached.active > 0 {
				return errors.New("cache entry is pinned")
			}
			if err := r.removeEntryLocked(key, cached); err != nil {
				return err
			}
			r.addEvent(Event{Type: "manual_remove", Category: "admin_action", MediaID: mediaID,
				Reason: "administrator", PlaybackSuccess: true, Resolved: true})
			return nil
		}
	}
	return model.ErrNotFound
}

func (r *resolver) Cancel(mediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.promoting {
		if task.identity.mediaID == mediaID {
			task.cancelled.Store(true)
			task.state = "cancelled"
			r.notifyControl()
			return nil
		}
	}
	return model.ErrNotFound
}

func (r *resolver) Pause() {
	if r.paused.CompareAndSwap(false, true) {
		r.addEvent(Event{Type: "cache_paused", Category: "admin_action", PlaybackSuccess: true, Resolved: true})
	}
	r.notifyControl()
}

func (r *resolver) Resume() {
	if r.paused.CompareAndSwap(true, false) {
		r.addEvent(Event{Type: "cache_resumed", Category: "admin_action", PlaybackSuccess: true, Resolved: true})
	}
	r.notifyControl()
}

func (r *resolver) notifyControl() {
	select {
	case r.control <- struct{}{}:
	default:
	}
}

func (r *resolver) Verify(ctx context.Context) VerificationResult {
	type target struct {
		key      string
		dataPath string
		meta     metadata
	}
	r.mu.Lock()
	targets := make([]target, 0, len(r.entries))
	for key, cached := range r.entries {
		if cached.active == 0 && r.playing[cached.meta.SourceID] == 0 {
			targets = append(targets, target{key: key, dataPath: cached.dataPath, meta: cached.meta})
		}
	}
	r.mu.Unlock()
	result := VerificationResult{}
	for _, target := range targets {
		if ctx.Err() != nil {
			break
		}
		result.Checked++
		digest, err := fileSHA256(target.dataPath)
		sourceInfo, sourceErr := os.Stat(target.meta.SourcePath)
		valid := err == nil && sourceErr == nil && digest == target.meta.SHA256 && sourceMatches(target.meta, sourceInfo)
		if valid {
			result.Valid++
			continue
		}
		r.mu.Lock()
		cached := r.entries[target.key]
		if cached != nil && cached.active == 0 && r.playing[cached.meta.SourceID] == 0 {
			if removeErr := r.removeEntryLocked(target.key, cached); removeErr == nil {
				result.Removed++
				r.runtime.corruptCleanup.Add(1)
			} else {
				result.Failed++
			}
		} else {
			result.Failed++
		}
		r.mu.Unlock()
		r.addEvent(Event{Type: "checksum_mismatch", Category: "integrity", MediaID: target.meta.SourceID,
			SourcePath: target.meta.SourcePath, CachePath: target.dataPath, Message: fmt.Sprint(errors.Join(err, sourceErr)),
			PlaybackSuccess: true})
	}
	return result
}

func (r *resolver) Cleanup(ctx context.Context) CleanupResult {
	var result CleanupResult
	items, err := os.ReadDir(r.path)
	if err != nil {
		return result
	}
	r.mu.Lock()
	known := make(map[string]struct{}, len(r.entries)*2)
	for _, cached := range r.entries {
		known[cached.dataPath] = struct{}{}
		known[cached.metadataPath] = struct{}{}
	}
	r.mu.Unlock()
	for _, item := range items {
		if ctx.Err() != nil || item.IsDir() {
			break
		}
		path := filepath.Join(r.path, item.Name())
		if _, ok := known[path]; ok {
			continue
		}
		if strings.HasSuffix(item.Name(), ".tmp") {
			if os.Remove(path) == nil {
				result.TemporaryRemoved++
			}
		} else if strings.HasSuffix(item.Name(), ".data") || strings.HasSuffix(item.Name(), ".json") {
			if os.Remove(path) == nil {
				result.OrphansRemoved++
			}
		}
	}
	if result.TemporaryRemoved+result.OrphansRemoved > 0 {
		r.runtime.orphanCleanup.Add(uint64(result.TemporaryRemoved + result.OrphansRemoved))
		r.addEvent(Event{Type: "orphan_removed", Category: "recovery",
			Message:         fmt.Sprintf("temporary=%d orphan=%d", result.TemporaryRemoved, result.OrphansRemoved),
			PlaybackSuccess: true, Resolved: true})
	}
	return result
}

func (r *resolver) Purge(ctx context.Context) PurgeResult {
	result := PurgeResult{}
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, cached := range r.entries {
		if ctx.Err() != nil {
			break
		}
		if cached.active > 0 || r.playing[cached.meta.SourceID] > 0 {
			result.Skipped++
			continue
		}
		size := cached.meta.DataSize + cached.metadataSize
		if err := r.removeEntryLocked(key, cached); err != nil {
			result.Skipped++
			continue
		}
		result.Removed++
		result.Bytes += size
	}
	r.addEvent(Event{Type: "manual_purge", Category: "admin_action", Reason: "administrator",
		Message:         fmt.Sprintf("removed=%d skipped=%d bytes=%d", result.Removed, result.Skipped, result.Bytes),
		PlaybackSuccess: true, Resolved: true})
	return result
}

func (r *resolver) ResetEvents() { r.events.reset() }

func (r *resolver) ResetStats() {
	r.runtime.reset()
	r.hits.Store(0)
	r.misses.Store(0)
	r.promotions.Store(0)
	r.failures.Store(0)
	r.evictions.Store(0)
	r.mu.Lock()
	for _, cached := range r.entries {
		cached.requestHits = 0
		cached.sessionHits = 0
		cached.latestRangeCount = 0
		cached.latestTTFB = 0
	}
	r.mu.Unlock()
	if err := r.persistRuntimeStats(); err != nil {
		log.Debug("Could not persist reset Hot Cache statistics", "path", r.statsPath, err)
	}
}

func (r *resolver) sessionStarted(mediaID string) {
	r.mu.Lock()
	r.playing[mediaID]++
	r.mu.Unlock()
}

func (r *resolver) sessionEnded(mediaID string) {
	r.mu.Lock()
	if r.playing[mediaID] <= 1 {
		delete(r.playing, mediaID)
	} else {
		r.playing[mediaID]--
	}
	r.mu.Unlock()
}

func (r *resolver) noteSessionHit(mediaID string) {
	now := time.Now()
	r.mu.Lock()
	for _, cached := range r.entries {
		if cached.meta.SourceID == mediaID {
			cached.sessionHits++
			cached.lastSessionHit = now
			break
		}
	}
	r.mu.Unlock()
}

func (r *resolver) addEvent(event Event) {
	if r.events != nil {
		r.events.add(event)
	}
}

func RecordTransportEvent(expected bool, eventType, code, mediaID, message string) {
	manager := GetManager()
	r, ok := manager.(*resolver)
	if !ok {
		return
	}
	if expected {
		r.runtime.expectedCancellations.Add(1)
	} else {
		r.runtime.unexpectedTransportErrors.Add(1)
	}
	r.addEvent(Event{Type: eventType, Category: "transport", Code: code, MediaID: mediaID,
		Message: message, PlaybackSuccess: expected, Resolved: expected})
}

type artworkFailure struct {
	count uint64
	last  time.Time
}

var artworkFailures = struct {
	sync.Mutex
	items map[string]artworkFailure
}{items: make(map[string]artworkFailure)}

// RecordArtworkError classifies the first two not-found requests in a 24-hour
// window as stale client references. Repeated failures are surfaced as a
// consistency warning. The map is bounded and never participates in audio
// streaming.
func RecordArtworkError(eventType, artworkID, message string, staleCandidate bool) bool {
	now := time.Now()
	artworkFailures.Lock()
	if len(artworkFailures.items) >= 1024 {
		for id, item := range artworkFailures.items {
			if now.Sub(item.last) > 24*time.Hour {
				delete(artworkFailures.items, id)
			}
		}
		if len(artworkFailures.items) >= 1024 {
			for id := range artworkFailures.items {
				delete(artworkFailures.items, id)
				break
			}
		}
	}
	failure := artworkFailures.items[artworkID]
	if now.Sub(failure.last) > 24*time.Hour {
		failure.count = 0
	}
	failure.count++
	failure.last = now
	artworkFailures.items[artworkID] = failure
	artworkFailures.Unlock()
	stale := staleCandidate && failure.count < 3

	manager := GetManager()
	r, ok := manager.(*resolver)
	if !ok {
		return stale
	}
	code := "artwork_consistency_error"
	if stale {
		code = "artwork_stale_request"
	}
	r.addEvent(Event{Type: eventType, Category: "artwork", Code: code, MediaID: artworkID,
		Message: fmt.Sprintf("%s (count=%d)", message, failure.count), PlaybackSuccess: true, Resolved: stale})
	return stale
}

func ResetArtworkFailures() {
	artworkFailures.Lock()
	clear(artworkFailures.items)
	artworkFailures.Unlock()
}

func isFailureEvent(event Event) bool {
	return strings.Contains(event.Type, "failed") || strings.Contains(event.Type, "error") ||
		strings.Contains(event.Type, "mismatch") || event.Type == "readonly_cache" || event.Type == "enospc" ||
		event.Type == "unexpected_broken_pipe"
}
