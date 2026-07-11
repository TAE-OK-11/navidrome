package hotcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

const (
	sessionShardCount = 16
	maxSessionRanges  = 32
)

type streamIdentity struct {
	mediaID       string
	sourcePath    string
	sourceSize    int64
	sourceModTime int64
	title         string
	artist        string
	album         string
	codec         string
	container     string
	extension     string
	bitRate       int
	duration      time.Duration
	format        int
	userID        string
	userName      string
	playerID      string
	player        string
	clientID      string
}

func newStreamIdentity(ctx context.Context, mf *model.MediaFile, sourcePath string, sourceSize, sourceModTime int64) streamIdentity {
	identity := streamIdentity{
		mediaID:       mf.ID,
		sourcePath:    sourcePath,
		sourceSize:    sourceSize,
		sourceModTime: sourceModTime,
		title:         mf.Title,
		artist:        mf.Artist,
		album:         mf.Album,
		codec:         strings.ToLower(mf.Codec),
		container:     strings.ToLower(mf.Suffix),
		extension:     strings.ToLower(strings.TrimPrefix(mf.Suffix, ".")),
		bitRate:       mf.BitRate,
		duration:      mediaDuration(mf),
		format:        formatIndexFor(mf),
	}
	if user, ok := request.UserFrom(ctx); ok {
		identity.userID = user.ID
		identity.userName = user.UserName
	}
	if player, ok := request.PlayerFrom(ctx); ok {
		identity.playerID = player.ID
		identity.player = player.Name
		if identity.player == "" {
			identity.player = player.Client
		}
	}
	if clientID, ok := request.ClientUniqueIdFrom(ctx); ok {
		identity.clientID = clientID
	}
	return identity
}

func mediaDuration(mf *model.MediaFile) time.Duration {
	seconds := float64(mf.Duration)
	if seconds <= 0 && mf.BitRate > 0 && mf.Size > 0 {
		seconds = float64(mf.Size*8) / float64(mf.BitRate*1000)
	}
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

type sessionKey struct {
	userID     string
	mediaID    string
	playerID   string
	remoteAddr string
	userAgent  string
}

func (i streamIdentity) sessionKey(remoteAddr, userAgent string) sessionKey {
	player := i.playerID
	if player == "" {
		player = i.clientID
	}
	key := sessionKey{userID: i.userID, mediaID: i.mediaID, playerID: player}
	if key.playerID == "" {
		key.remoteAddr = remoteAddr
		key.userAgent = userAgent
	}
	return key
}

type byteInterval struct {
	start int64
	end   int64
}

type playSession struct {
	identity         streamIdentity
	id               string
	key              sessionKey
	started          time.Time
	lastActivity     time.Time
	cached           bool
	requestCount     uint64
	rangeCount       uint64
	seekCount        uint64
	cancellations    uint64
	intervals        []byteInterval
	uniqueBytes      int64
	lastRangeEnd     int64
	playedDuration   time.Duration
	playedPercent    float64
	thresholdState   string
	thresholdReached bool
	announced        bool
	sendfile         bool
	transferPath     string
}

type sessionShard struct {
	mu       sync.Mutex
	sessions map[sessionKey]*playSession
}

type sessionTracker struct {
	owner       *resolver
	window      time.Duration
	idleTimeout time.Duration
	maxSessions int64
	minPlay     time.Duration
	minPercent  float64
	count       atomic.Int64
	sequence    atomic.Uint64
	shards      [sessionShardCount]sessionShard
}

func newSessionTracker(owner *resolver, options Options) *sessionTracker {
	tracker := &sessionTracker{
		owner:       owner,
		window:      options.SessionWindow,
		idleTimeout: options.SessionIdleTimeout,
		maxSessions: int64(options.MaxSessions),
		minPlay:     time.Duration(options.MinPlaySeconds) * time.Second,
		minPercent:  float64(options.MinPlayPercent),
	}
	for i := range tracker.shards {
		tracker.shards[i].sessions = make(map[sessionKey]*playSession)
	}
	return tracker
}

func (t *sessionTracker) observe(ctx context.Context, identity streamIdentity, cached bool, observation PlaybackObservation) {
	rangeInfo := normalizeRange(observation.RangeHeader, identity.sourceSize)
	expectedBytes := observation.BytesExpected
	if expectedBytes <= 0 {
		expectedBytes = rangeInfo.length()
	}
	t.owner.runtime.observeRequest(identity.format, cached, rangeInfo.requested, observation.Sendfile, observation.Cancelled,
		expectedBytes, observation.Elapsed, observation.TTFB)
	formatStats := &t.owner.runtime.format[validFormatIndex(identity.format)]
	if cached {
		formatStats.requestHits.Add(1)
	} else {
		formatStats.requestMisses.Add(1)
	}
	if !observation.Playback || observation.Method == http.MethodHead {
		return
	}
	if t.owner.shuttingDown.Load() {
		return
	}

	now := time.Now()
	key := identity.sessionKey(observation.RemoteAddr, observation.UserAgent)
	shard := &t.shards[sessionShardIndex(key)]
	var ended *playSession
	var started *playSession
	var threshold *playSession

	shard.mu.Lock()
	session := shard.sessions[key]
	if session != nil && now.Sub(session.lastActivity) > t.window {
		delete(shard.sessions, key)
		t.count.Add(-1)
		ended = session
		session = nil
	}
	if session == nil && t.count.Add(1) <= t.maxSessions {
		start := now.Add(-max(observation.Elapsed, 0))
		session = &playSession{
			identity:       identity,
			id:             t.newSessionID(key, start),
			key:            key,
			started:        start,
			lastActivity:   now,
			cached:         cached,
			lastRangeEnd:   -1,
			intervals:      make([]byteInterval, 0, maxSessionRanges),
			thresholdState: "probe",
			transferPath:   transferPath(cached),
		}
		shard.sessions[key] = session
	} else if session == nil {
		t.count.Add(-1)
	}
	if session != nil {
		wasAnnounced := session.announced
		seekAdded := false
		session.requestCount++
		session.lastActivity = now
		session.sendfile = session.sendfile || observation.Sendfile
		if observation.Cancelled {
			session.cancellations++
		}
		if rangeInfo.requested {
			session.rangeCount++
			if rangeInfo.valid {
				if session.lastRangeEnd >= 0 && rangeInfo.start != session.lastRangeEnd+1 && rangeInfo.start != 0 {
					session.seekCount++
					seekAdded = true
				}
				session.lastRangeEnd = rangeInfo.end
				session.uniqueBytes += addInterval(&session.intervals, byteInterval{start: rangeInfo.start, end: rangeInfo.end})
			}
		} else if identity.sourceSize > 0 {
			session.uniqueBytes += addInterval(&session.intervals, byteInterval{start: 0, end: identity.sourceSize - 1})
		}
		t.updateProgress(session)
		if !session.announced && meaningfulPlayback(session, rangeInfo, observation) {
			session.announced = true
			session.started = now.Add(-max(observation.Elapsed, 0))
			if cached {
				session.thresholdState = "cached"
			} else {
				session.thresholdState = "threshold-wait"
			}
			t.updateProgress(session)
			started = lifecycleCopy(session)
		}
		if session.announced && !cached && !session.thresholdReached && t.thresholdMet(session) {
			session.thresholdReached = true
			session.thresholdState = "threshold-reached"
			threshold = lifecycleCopy(session)
		}
		if wasAnnounced {
			if rangeInfo.requested {
				t.owner.runtime.sessionRangeRequests.Add(1)
			}
			if seekAdded {
				t.owner.runtime.sessionSeekOperations.Add(1)
			}
		}
	}
	shard.mu.Unlock()

	if ended != nil {
		t.finish(ended, false)
	}
	if started != nil {
		t.start(started)
	}
	if threshold != nil {
		t.reachThreshold(ctx, threshold)
	}
}

func meaningfulPlayback(session *playSession, rangeInfo normalizedRange, observation PlaybackObservation) bool {
	if !rangeInfo.requested {
		return observation.Method == http.MethodGet
	}
	if observation.Elapsed >= 2*time.Second || rangeInfo.length() >= 64*1024 {
		return true
	}
	return session.requestCount >= 2 && session.uniqueBytes >= 16*1024
}

func lifecycleCopy(session *playSession) *playSession {
	copy := *session
	copy.intervals = nil
	return &copy
}

func (t *sessionTracker) start(session *playSession) {
	t.owner.runtime.playSessions.Add(1)
	t.owner.runtime.sessionRangeRequests.Add(session.rangeCount)
	t.owner.runtime.sessionSeekOperations.Add(session.seekCount)
	fc := &t.owner.runtime.format[validFormatIndex(session.identity.format)]
	if session.cached {
		t.owner.runtime.cachedSessions.Add(1)
		fc.cachedSessions.Add(1)
		t.owner.noteSessionHit(session.identity.mediaID)
	} else {
		t.owner.runtime.uncachedSessions.Add(1)
		fc.uncachedSessions.Add(1)
	}
	t.owner.sessionStarted(session.identity.mediaID)
	typeName := "session_cache_miss"
	if session.cached {
		typeName = "session_cache_hit"
	}
	t.owner.addEvent(Event{Type: "play_session_started", Category: "playback", MediaID: session.identity.mediaID,
		Title: session.identity.title, Artist: session.identity.artist, SessionID: session.id, PlaybackSuccess: true})
	t.owner.addEvent(Event{Type: typeName, Category: "playback", MediaID: session.identity.mediaID,
		Title: session.identity.title, Artist: session.identity.artist, SessionID: session.id, PlaybackSuccess: true})
	log.Info("Hot cache play session started", "mediaID", session.identity.mediaID, "title", session.identity.title,
		"artist", session.identity.artist, "sessionID", session.id, "user", session.identity.userName,
		"player", session.identity.player, "cached", session.cached, "codec", session.identity.codec,
		"container", session.identity.container)
}

func (t *sessionTracker) reachThreshold(ctx context.Context, session *playSession) {
	t.owner.runtime.thresholdReached.Add(1)
	t.owner.runtime.validSessions.Add(1)
	t.owner.addEvent(Event{Type: "threshold_reached", Category: "promotion", MediaID: session.identity.mediaID,
		Title: session.identity.title, Artist: session.identity.artist, SessionID: session.id, PlaybackSuccess: true})
	log.Info(ctx, "Hot cache threshold reached", "mediaID", session.identity.mediaID, "title", session.identity.title,
		"sessionID", session.id, "playedDuration", session.playedDuration, "playedPercent", session.playedPercent)
	t.owner.queuePromotion(session.identity, session.playedDuration, session.playedPercent, "play-threshold", session.id)
}

func (t *sessionTracker) updateProgress(session *playSession) {
	wallProgress := max(session.lastActivity.Sub(session.started), 0)
	byteProgress := wallProgress
	if session.identity.duration > 0 && session.identity.sourceSize > 0 {
		byteProgress = time.Duration(float64(session.identity.duration) * float64(session.uniqueBytes) / float64(session.identity.sourceSize))
	}
	session.playedDuration = min(wallProgress, byteProgress)
	if session.identity.duration > 0 {
		session.playedPercent = min(100, float64(session.playedDuration)*100/float64(session.identity.duration))
	}
}

func (t *sessionTracker) thresholdMet(session *playSession) bool {
	if t.minPlay > 0 && session.playedDuration >= t.minPlay {
		return true
	}
	return session.identity.duration > 0 && t.minPercent > 0 && session.playedPercent >= t.minPercent
}

func (t *sessionTracker) cleanup(now time.Time, shutdown bool) {
	for i := range t.shards {
		shard := &t.shards[i]
		var ended []*playSession
		shard.mu.Lock()
		for key, session := range shard.sessions {
			if shutdown || now.Sub(session.lastActivity) > t.idleTimeout {
				delete(shard.sessions, key)
				t.count.Add(-1)
				ended = append(ended, session)
			}
		}
		shard.mu.Unlock()
		for _, session := range ended {
			t.finish(session, shutdown)
		}
	}
}

func (t *sessionTracker) finish(session *playSession, shutdown bool) {
	if !session.announced {
		return
	}
	t.owner.sessionEnded(session.identity.mediaID)
	t.owner.runtime.sessionDurationNanos.Add(uint64(max(session.playedDuration, 0)))
	completed := session.identity.duration > 0 && session.playedPercent >= 90
	if completed {
		t.owner.runtime.completedSessions.Add(1)
	} else if !session.thresholdReached && !session.cached {
		t.owner.runtime.skippedSessions.Add(1)
		t.owner.runtime.thresholdNotReached.Add(1)
		session.thresholdState = "skipped"
		t.owner.addEvent(Event{Type: "threshold_not_reached", Category: "promotion", MediaID: session.identity.mediaID,
			Title: session.identity.title, Artist: session.identity.artist, SessionID: session.id,
			Reason: "session-ended-before-threshold", PlaybackSuccess: true})
	}
	if shutdown && !session.thresholdReached && !session.cached {
		session.thresholdState = "cancelled"
	}
	t.owner.addEvent(Event{Type: "play_session_ended", Category: "playback", MediaID: session.identity.mediaID,
		Title: session.identity.title, Artist: session.identity.artist, SessionID: session.id,
		Reason: session.thresholdState, PlaybackSuccess: true})
	if log.IsGreaterOrEqualTo(log.LevelDebug) {
		log.Debug("Hot cache play session ended", "mediaID", session.identity.mediaID, "sessionID", session.id,
			"requests", session.requestCount, "ranges", session.rangeCount, "seeks", session.seekCount,
			"playedDuration", session.playedDuration, "playedPercent", session.playedPercent,
			"state", session.thresholdState)
	}
}

func (t *sessionTracker) snapshots() []SessionSnapshot {
	result := make([]SessionSnapshot, 0, max(0, int(t.count.Load())))
	for i := range t.shards {
		shard := &t.shards[i]
		shard.mu.Lock()
		for _, session := range shard.sessions {
			result = append(result, session.snapshot())
		}
		shard.mu.Unlock()
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastActivity.After(result[j].LastActivity) })
	return result
}

func (s *playSession) snapshot() SessionSnapshot {
	return SessionSnapshot{
		ID: s.id, MediaID: s.identity.mediaID, Title: s.identity.title, Artist: s.identity.artist,
		Album: s.identity.album, User: s.identity.userName, Player: s.identity.player,
		Codec: s.identity.codec, Container: s.identity.container, Cached: s.cached,
		PlayedDuration: s.playedDuration, TotalDuration: s.identity.duration, PlayedPercent: s.playedPercent,
		RequestCount: s.requestCount, RangeCount: s.rangeCount, SeekCount: s.seekCount,
		ThresholdState: s.thresholdState, LastActivity: s.lastActivity, TransferPath: s.transferPath,
		Sendfile: s.sendfile, ClientCancelled: s.cancellations,
	}
}

func (t *sessionTracker) newSessionID(key sessionKey, started time.Time) string {
	sequence := t.sequence.Add(1)
	hasher := sha256.New()
	for _, value := range [...]string{key.userID, key.mediaID, key.playerID, key.remoteAddr, key.userAgent} {
		_, _ = io.WriteString(hasher, value)
		_, _ = io.WriteString(hasher, "\x00")
	}
	_, _ = io.WriteString(hasher, strconv.FormatInt(started.UnixNano(), 10))
	_, _ = io.WriteString(hasher, strconv.FormatUint(sequence, 10))
	digest := hasher.Sum(nil)
	return hex.EncodeToString(digest[:8])
}

func sessionShardIndex(key sessionKey) int {
	var hash uint32 = 2166136261
	for _, value := range [...]string{key.userID, key.mediaID, key.playerID, key.remoteAddr, key.userAgent} {
		for index := range len(value) {
			hash ^= uint32(value[index])
			hash *= 16777619
		}
	}
	return int(hash % sessionShardCount)
}

type normalizedRange struct {
	requested bool
	valid     bool
	start     int64
	end       int64
}

func (r normalizedRange) length() int64 {
	if !r.valid || r.end < r.start {
		return 0
	}
	return r.end - r.start + 1
}

func normalizeRange(header string, size int64) normalizedRange {
	if header == "" {
		if size <= 0 {
			return normalizedRange{}
		}
		return normalizedRange{valid: true, start: 0, end: size - 1}
	}
	result := normalizedRange{requested: true}
	if size <= 0 || !strings.HasPrefix(header, "bytes=") || strings.Contains(header, ",") {
		return result
	}
	value := strings.TrimSpace(strings.TrimPrefix(header, "bytes="))
	startText, endText, ok := strings.Cut(value, "-")
	if !ok {
		return result
	}
	if startText == "" {
		suffix, err := strconv.ParseInt(endText, 10, 64)
		if err != nil || suffix <= 0 {
			return result
		}
		suffix = min(suffix, size)
		result.start, result.end, result.valid = size-suffix, size-1, true
		return result
	}
	start, err := strconv.ParseInt(startText, 10, 64)
	if err != nil || start < 0 || start >= size {
		return result
	}
	end := size - 1
	if endText != "" {
		end, err = strconv.ParseInt(endText, 10, 64)
		if err != nil || end < start {
			return result
		}
		end = min(end, size-1)
	}
	result.start, result.end, result.valid = start, end, true
	return result
}

func addInterval(intervals *[]byteInterval, candidate byteInterval) int64 {
	if candidate.end < candidate.start {
		return 0
	}
	existing := *intervals
	index := 0
	for index < len(existing) && existing[index].end+1 < candidate.start {
		index++
	}
	if index == len(existing) || candidate.end+1 < existing[index].start {
		if len(existing) >= maxSessionRanges {
			return 0
		}
		existing = append(existing, byteInterval{})
		copy(existing[index+1:], existing[index:])
		existing[index] = candidate
		*intervals = existing
		return candidate.end - candidate.start + 1
	}

	mergedStart := min(candidate.start, existing[index].start)
	mergedEnd := max(candidate.end, existing[index].end)
	removedBytes := int64(0)
	endIndex := index
	for endIndex < len(existing) && existing[endIndex].start <= mergedEnd+1 {
		removedBytes += existing[endIndex].end - existing[endIndex].start + 1
		mergedEnd = max(mergedEnd, existing[endIndex].end)
		endIndex++
	}
	existing[index] = byteInterval{start: mergedStart, end: mergedEnd}
	copy(existing[index+1:], existing[endIndex:])
	existing = existing[:len(existing)-(endIndex-index)+1]
	*intervals = existing
	return mergedEnd - mergedStart + 1 - removedBytes
}

func transferPath(cached bool) string {
	if cached {
		return "nvme-cache"
	}
	return "source"
}
