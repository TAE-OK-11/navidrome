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
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/navidrome/navidrome/log"
)

type promotionTask struct {
	key             string
	identity        streamIdentity
	sessionID       string
	threshold       string
	thresholdReason string
	playedDuration  time.Duration
	playedPercent   float64
	queuedAt        time.Time
	notBefore       time.Time
	startedAt       time.Time
	state           string
	phase           string
	retryCount      int
	lastError       string
	cachePath       string
	bytesCopied     atomic.Int64
	cancelled       atomic.Bool
}

func (r *resolver) startWorkers(concurrency int) {
	for range concurrency {
		r.workers.Add(1)
		go r.promotionWorker()
	}
	r.workers.Add(1)
	go r.maintenanceWorker()
	if r.runtime.enabled {
		r.workers.Add(1)
		go r.statsWorker()
	}
}

func (r *resolver) statsWorker() {
	defer r.workers.Done()
	ticker := time.NewTicker(r.statsFlush)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			if err := r.persistRuntimeStats(); err != nil {
				log.Debug("Could not persist final Hot Cache statistics", "path", r.statsPath, err)
			}
			return
		case <-ticker.C:
			if err := r.persistRuntimeStats(); err != nil {
				log.Debug("Could not persist Hot Cache statistics", "path", r.statsPath, err)
			}
		}
	}
}

func (r *resolver) promotionWorker() {
	defer r.workers.Done()
	for {
		select {
		case <-r.stop:
			return
		case task := <-r.queue:
			if task == nil {
				continue
			}
			if !r.waitForTask(task) {
				r.finishCancelledTask(task, "shutdown-or-cancelled")
				continue
			}
			r.runPromotionTask(task)
		}
	}
}

func (r *resolver) maintenanceWorker() {
	defer r.workers.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case key := <-r.touchQueue:
			r.persistTouch(key)
		case now := <-ticker.C:
			r.sessions.cleanup(now, false)
		}
	}
}

func (r *resolver) waitForTask(task *promotionTask) bool {
	for {
		if task.cancelled.Load() || r.shuttingDown.Load() {
			return false
		}
		now := time.Now()
		replacementActive := false
		r.mu.Lock()
		if cached := r.entries[task.key]; cached != nil {
			replacementActive = cached.active > 0
		}
		r.mu.Unlock()
		if !r.paused.Load() && !now.Before(task.notBefore) && r.sourceStreams.Load() == 0 && !replacementActive {
			return true
		}
		wait := 250 * time.Millisecond
		if !r.paused.Load() && now.Before(task.notBefore) {
			wait = min(wait, time.Until(task.notBefore))
		}
		timer := time.NewTimer(wait)
		select {
		case <-r.stop:
			timer.Stop()
			return false
		case <-r.control:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (r *resolver) queuePromotion(identity streamIdentity, playedDuration time.Duration, playedPercent float64, reason, sessionID string) error {
	if !r.enabled || r.shuttingDown.Load() || (!r.promoteOnPlay && reason != "manual") {
		return errors.New("hot-cache promotion is unavailable")
	}
	if identity.sourceSize <= 0 || identity.sourceSize > r.maxSize {
		r.runtime.promotionCancelled.Add(1)
		r.addEvent(Event{Type: "promotion_cancelled", Category: "promotion", MediaID: identity.mediaID,
			Title: identity.title, Artist: identity.artist, SessionID: sessionID, Reason: "oversized-file",
			Message: "source is empty or larger than cache capacity", PlaybackSuccess: true, Resolved: true})
		return errors.New("source is empty or larger than hot-cache capacity")
	}
	key := keyFor(identity.mediaID, identity.sourcePath)
	now := time.Now()
	task := &promotionTask{
		key: key, identity: identity, sessionID: sessionID, threshold: thresholdLabel(r.sessions),
		thresholdReason: reason, playedDuration: playedDuration, playedPercent: playedPercent,
		queuedAt: now, notBefore: now.Add(r.promotionDelay), state: "queued", phase: "waiting",
		cachePath: r.path,
	}

	r.mu.Lock()
	if cached := r.entries[key]; cached != nil && !cached.stale {
		if entryMatchesIdentity(cached, identity) {
			r.mu.Unlock()
			return nil
		}
		r.runtime.sourceInvalidations.Add(1)
		r.markStaleLocked(cached)
	}
	if existing := r.promoting[key]; existing != nil {
		r.mu.Unlock()
		if existing.cancelled.Load() {
			return errors.New("hot-cache promotion cancellation is pending")
		}
		return nil
	}
	if len(r.promoting) >= r.queueMax {
		r.mu.Unlock()
		r.runtime.promotionCancelled.Add(1)
		r.addEvent(Event{Type: "promotion_cancelled", Category: "promotion", MediaID: identity.mediaID,
			Title: identity.title, Artist: identity.artist, SessionID: sessionID, Reason: "queue-full",
			PlaybackSuccess: true, Resolved: true})
		return errors.New("hot-cache promotion queue is full")
	}
	r.promoting[key] = task
	r.mu.Unlock()

	select {
	case r.queue <- task:
		r.runtime.promotionQueued.Add(1)
		r.addEvent(Event{Type: "promotion_queued", Category: "promotion", MediaID: identity.mediaID,
			Title: identity.title, Artist: identity.artist, SessionID: sessionID, PlaybackSuccess: true})
		log.Info("Hot cache promotion queued", "mediaID", identity.mediaID, "title", identity.title,
			"sessionID", sessionID, "sourceSize", identity.sourceSize, "reason", reason)
		return nil
	default:
		r.mu.Lock()
		delete(r.promoting, key)
		r.mu.Unlock()
		r.runtime.promotionCancelled.Add(1)
		return errors.New("hot-cache promotion queue is full")
	}
}

func entryMatchesIdentity(cached *entry, identity streamIdentity) bool {
	return cached.meta.SourceID == identity.mediaID &&
		cached.meta.SourcePath == identity.sourcePath &&
		cached.meta.SourceSize == identity.sourceSize &&
		cached.meta.SourceModTime == identity.sourceModTime
}

func thresholdLabel(tracker *sessionTracker) string {
	if tracker == nil {
		return "manual"
	}
	return fmt.Sprintf("%s or %.0f%%", tracker.minPlay, tracker.minPercent)
}

func (r *resolver) runPromotionTask(task *promotionTask) {
	r.mu.Lock()
	if task.cancelled.Load() || r.shuttingDown.Load() {
		r.mu.Unlock()
		r.finishCancelledTask(task, "shutdown-or-cancelled")
		return
	}
	task.state = "copying"
	task.phase = "copying"
	task.startedAt = time.Now()
	r.current = task
	r.mu.Unlock()
	r.runtime.promotionStarted.Add(1)
	r.addEvent(Event{Type: "promotion_started", Category: "promotion", MediaID: task.identity.mediaID,
		Title: task.identity.title, Artist: task.identity.artist, SessionID: task.sessionID, PlaybackSuccess: true})
	log.Info("Hot cache promotion started", "mediaID", task.identity.mediaID, "title", task.identity.title,
		"sourceSize", task.identity.sourceSize, "retry", task.retryCount)

	var err error
	for {
		err = r.promote(r.workerCtx, task)
		if err == nil || task.cancelled.Load() || r.shuttingDown.Load() || task.retryCount >= r.maxRetries || !isRetryablePromotionError(err) {
			break
		}
		r.mu.Lock()
		task.retryCount++
		task.lastError = err.Error()
		retryCount := task.retryCount
		r.mu.Unlock()
		task.bytesCopied.Store(0)
		r.runtime.promotionRetries.Add(1)
		backoff := time.Duration(1<<(retryCount-1)) * 250 * time.Millisecond
		if !r.waitRetry(task, backoff) {
			break
		}
	}

	elapsed := time.Since(task.startedAt)
	r.mu.Lock()
	if r.current == task {
		r.current = nil
	}
	delete(r.promoting, task.key)
	if err == nil {
		task.state = "completed"
		task.phase = "completed"
	} else if task.cancelled.Load() || r.shuttingDown.Load() {
		task.state = "cancelled"
		task.phase = "cancelled"
	} else {
		task.state = "failed"
		task.phase = "failed"
		task.lastError = err.Error()
	}
	r.mu.Unlock()

	if err == nil {
		r.promotions.Add(1)
		r.runtime.promotionCompleted.Add(1)
		r.runtime.promotionBytes.Add(uint64(task.identity.sourceSize))
		r.runtime.promotionElapsedNanos.Add(uint64(elapsed))
		r.runtime.format[validFormatIndex(task.identity.format)].promotionCompleted.Add(1)
		r.addEvent(Event{Type: "promotion_completed", Category: "promotion", MediaID: task.identity.mediaID,
			Title: task.identity.title, Artist: task.identity.artist, SessionID: task.sessionID,
			CachePath: task.cachePath, PlaybackSuccess: true})
		log.Info("Hot cache promotion completed", "mediaID", task.identity.mediaID, "title", task.identity.title,
			"size", task.identity.sourceSize, "elapsed", elapsed, "speed", promotionSpeed(task.identity.sourceSize, elapsed))
		return
	}

	if task.cancelled.Load() || r.shuttingDown.Load() {
		r.runtime.promotionCancelled.Add(1)
		r.addEvent(Event{Type: "promotion_cancelled", Category: "promotion", MediaID: task.identity.mediaID,
			Title: task.identity.title, SessionID: task.sessionID, Reason: "cancelled", PlaybackSuccess: true, Resolved: true})
		return
	}
	r.failures.Add(1)
	r.runtime.promotionFailed.Add(1)
	r.runtime.format[validFormatIndex(task.identity.format)].promotionFailed.Add(1)
	category, code := classifyPromotionError(err)
	r.addEvent(Event{Type: "promotion_failed", Category: category, Code: code, MediaID: task.identity.mediaID,
		Title: task.identity.title, Artist: task.identity.artist, SessionID: task.sessionID,
		SourcePath: task.identity.sourcePath, CachePath: task.cachePath, RetryCount: task.retryCount,
		Message: err.Error(), PlaybackSuccess: true})
	log.Warn("Hot cache promotion failed; source streaming was unaffected", "mediaID", task.identity.mediaID,
		"title", task.identity.title, "retry", task.retryCount, err)
}

func (r *resolver) finishCancelledTask(task *promotionTask, reason string) {
	r.mu.Lock()
	delete(r.promoting, task.key)
	if r.current == task {
		r.current = nil
	}
	task.state = "cancelled"
	task.phase = "cancelled"
	r.mu.Unlock()
	r.runtime.promotionCancelled.Add(1)
	r.addEvent(Event{Type: "promotion_cancelled", Category: "promotion", MediaID: task.identity.mediaID,
		Title: task.identity.title, SessionID: task.sessionID, Reason: reason, PlaybackSuccess: true, Resolved: true})
}

func (r *resolver) waitRetry(task *promotionTask, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-r.stop:
		return false
	case <-timer.C:
		return !task.cancelled.Load()
	}
}

func (r *resolver) promote(ctx context.Context, task *promotionTask) error {
	source := task.identity
	if source.sourceSize <= 0 || source.sourceSize > r.maxSize {
		return errors.New("file is larger than hot-cache capacity")
	}
	sourceFile, err := os.Open(source.sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	openedInfo, err := sourceFile.Stat()
	if err != nil || openedInfo.Size() != source.sourceSize || openedInfo.ModTime().UnixNano() != source.sourceModTime {
		return errors.New("source changed before promotion")
	}

	meta := metadata{
		Version: metadataVersion, Key: task.key, SourceID: source.mediaID, SourcePath: source.sourcePath,
		SourceSize: source.sourceSize, SourceModTime: source.sourceModTime, DataSize: source.sourceSize,
		DataModTime: time.Now().UnixNano(), SHA256: strings.Repeat("0", sha256.Size*2), Title: source.title,
		Artist: source.artist, Album: source.album, Codec: source.codec, Container: source.container,
		Extension: source.extension, BitRate: source.bitRate,
	}
	estimatedMetadata, _ := json.Marshal(meta)
	required := source.sourceSize + int64(len(estimatedMetadata)) + 512
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
		if r.reserved < 0 {
			r.reserved = 0
		}
		r.mu.Unlock()
	}()

	tempData, err := os.CreateTemp(r.path, task.key+".*.tmp")
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
	written, copyErr := r.copyFile(ctx, io.MultiWriter(tempData, hasher), io.LimitReader(sourceFile, source.sourceSize+1), *buffer, task)
	copyBufferPool.Put(buffer)
	if copyErr != nil {
		return copyErr
	}
	if written != source.sourceSize {
		return fmt.Errorf("source size changed during promotion: copied %d, expected %d", written, source.sourceSize)
	}
	r.setTaskPhase(task, "fsync")
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

	r.setTaskPhase(task, "checksum")
	currentInfo, err := os.Stat(source.sourcePath)
	if err != nil || currentInfo.Size() != source.sourceSize || currentInfo.ModTime().UnixNano() != source.sourceModTime {
		return errors.New("source changed during promotion")
	}
	meta.DataModTime = dataInfo.ModTime().UnixNano()
	meta.SHA256 = hex.EncodeToString(hasher.Sum(nil))
	metadataBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if int64(len(metadataBytes)) > required-source.sourceSize {
		return errors.New("cache metadata exceeded reserved capacity")
	}
	r.setTaskPhase(task, "sidecar")
	tempMetadata, err := os.CreateTemp(r.path, task.key+".*.tmp")
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
	if task.cancelled.Load() || r.shuttingDown.Load() {
		return context.Canceled
	}

	dataPath := r.path + string(os.PathSeparator) + task.key + ".data"
	metadataPath := r.path + string(os.PathSeparator) + task.key + ".json"
	r.mu.Lock()
	task.cachePath = dataPath
	r.mu.Unlock()
	r.setTaskPhase(task, "atomic-rename")
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing := r.entries[task.key]; existing != nil {
		if existing.active > 0 || r.playing[existing.meta.SourceID] > 0 {
			return errors.New("cache entry became active during replacement")
		}
		if err := r.removeEntryLocked(task.key, existing); err != nil {
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
	now := time.Now()
	r.entries[task.key] = &entry{
		meta: meta, dataPath: dataPath, metadataPath: metadataPath, metadataSize: metadataInfo.Size(),
		createdAt: now, lastUsed: now, lastPersisted: metadataInfo.ModTime(),
	}
	r.used += meta.DataSize + metadataInfo.Size()
	_ = syncDirectory(r.path)
	return nil
}

func (r *resolver) copyWithYield(ctx context.Context, dst io.Writer, src io.Reader, buffer []byte, task *promotionTask) (int64, error) {
	var written int64
	for {
		for r.sourceStreams.Load() > 0 || r.paused.Load() {
			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return written, ctx.Err()
			case <-timer.C:
			}
		}
		if err := ctx.Err(); err != nil {
			return written, err
		}
		read, readErr := src.Read(buffer)
		if read > 0 {
			count, writeErr := dst.Write(buffer[:read])
			written += int64(count)
			task.bytesCopied.Store(written)
			if writeErr != nil {
				return written, writeErr
			}
			if count != read {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func (r *resolver) setTaskPhase(task *promotionTask, phase string) {
	r.mu.Lock()
	task.phase = phase
	r.mu.Unlock()
	if log.IsGreaterOrEqualTo(log.LevelTrace) {
		log.Trace("Hot cache promotion phase", "mediaID", task.identity.mediaID, "phase", phase,
			"bytesCopied", task.bytesCopied.Load(), "totalBytes", task.identity.sourceSize)
	}
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
		if cached.active == 0 && r.playing[cached.meta.SourceID] == 0 {
			candidates = append(candidates, cached)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].lastUsed.Before(candidates[j].lastUsed) })
	for _, cached := range candidates {
		mediaID, size := cached.meta.SourceID, cached.meta.DataSize+cached.metadataSize
		if err := r.removeEntryLocked(cached.meta.Key, cached); err != nil {
			continue
		}
		r.evictions.Add(1)
		r.runtime.evictionBytes.Add(uint64(size))
		r.addEvent(Event{Type: "eviction_completed", Category: "storage", MediaID: mediaID,
			Reason: "lru-capacity", Message: fmt.Sprintf("evicted %d bytes", size), PlaybackSuccess: true, Resolved: true})
		log.Info("Hot cache entry evicted", "mediaID", mediaID, "size", size, "reason", "lru-capacity")
		if r.used+r.reserved+required <= r.maxSize {
			return nil
		}
	}
	return errors.New("not enough evictable hot-cache space")
}

func (r *resolver) removeEntryLocked(key string, cached *entry) error {
	if cached.active > 0 || r.playing[cached.meta.SourceID] > 0 {
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

func isRetryablePromotionError(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, syscall.ENOSPC) &&
		!errors.Is(err, syscall.EROFS) && !errors.Is(err, syscall.EACCES) &&
		!strings.Contains(err.Error(), "source changed") && !strings.Contains(err.Error(), "larger than hot-cache")
}

func classifyPromotionError(err error) (category, code string) {
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return "storage", "enospc"
	case errors.Is(err, syscall.EROFS), errors.Is(err, syscall.EACCES):
		return "storage", "readonly_cache"
	case strings.Contains(err.Error(), "source changed"):
		return "integrity", "source_changed"
	case strings.Contains(err.Error(), "checksum"):
		return "integrity", "checksum_mismatch"
	default:
		return "promotion", "promotion_failed"
	}
}

func promotionSpeed(size int64, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	return float64(size) / elapsed.Seconds()
}

func (r *resolver) Shutdown(ctx context.Context) error {
	if !r.enabled {
		return nil
	}
	r.shuttingDown.Store(true)
	r.sessions.cleanup(time.Now(), true)
	r.workerCancel()
	r.stopOnce.Do(func() { close(r.stop) })
	r.mu.Lock()
	for _, task := range r.promoting {
		task.cancelled.Store(true)
	}
	r.mu.Unlock()
	done := make(chan struct{})
	go func() {
		r.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		if err := r.persistRuntimeStats(); err != nil {
			log.Debug("Could not persist shutdown Hot Cache statistics", "path", r.statsPath, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func Shutdown(ctx context.Context) error {
	if resolverInstance == nil {
		return nil
	}
	manager, ok := resolverInstance.(Manager)
	if !ok {
		return nil
	}
	return manager.Shutdown(ctx)
}
