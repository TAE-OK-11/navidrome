package scrobbler

import (
	"context"
	"time"

	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/eventbus"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

func (p *playTracker) enqueueNowPlaying(ctx context.Context, playerId string, userId string, track *model.MediaFile, position int) {
	p.npMu.Lock()
	defer p.npMu.Unlock()
	ctx = context.WithoutCancel(ctx) // Prevent cancellation from affecting background processing
	p.npQueue[playerId] = nowPlayingEntry{
		ctx:      ctx,
		userId:   userId,
		track:    track,
		position: position,
	}
	p.sendNowPlayingSignal()
}

func (p *playTracker) sendNowPlayingSignal() {
	// Don't block if the previous signal was not read yet
	select {
	case p.npSignal <- struct{}{}:
	default:
	}
}

func (p *playTracker) nowPlayingWorker() {
	defer close(p.workerDone)
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-p.shutdown:
			return
		case <-timer.C:
		case <-p.npSignal:
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		p.npMu.Lock()
		if len(p.npQueue) == 0 {
			p.npMu.Unlock()
			timer.Reset(time.Second)
			continue
		}

		// Keep a copy of the entries to process and clear the queue
		entries := p.npQueue
		p.npQueue = make(map[string]nowPlayingEntry)
		p.npMu.Unlock()

		// Process entries without holding lock
		for _, entry := range entries {
			p.dispatchNowPlaying(entry.ctx, entry.userId, entry.track, entry.position)
		}
		timer.Reset(time.Second)
	}
}

func (p *playTracker) dispatchNowPlaying(ctx context.Context, userId string, t *model.MediaFile, position int) {
	if t.Artist == consts.UnknownArtist {
		log.Debug(ctx, "Ignoring external NowPlaying update for track with unknown artist", "track", t.Title, "artist", t.Artist)
		return
	}
	p.publish(ctx, eventbus.Event{
		Topic: eventbus.TopicNowPlaying,
		NowPlaying: &eventbus.NowPlaying{
			UserID:      userId,
			MediaFileID: t.ID,
			Title:       t.Title,
			Artist:      t.Artist,
			PositionSec: position,
			Track:       *t,
		},
	})
}

func (p *playTracker) onNowPlaying(ctx context.Context, evt eventbus.Event) {
	if evt.NowPlaying == nil {
		return
	}
	userId := evt.NowPlaying.UserID
	t := evt.NowPlaying.Track
	position := evt.NowPlaying.PositionSec
	for name, s := range p.getActiveScrobblers() {
		if !s.IsAuthorized(ctx, userId) {
			continue
		}
		log.Debug(ctx, "Sending NowPlaying update", "scrobbler", name, "track", t.Title, "artist", t.Artist, "position", position)
		if err := s.NowPlaying(ctx, userId, &t, position); err != nil {
			log.Error(ctx, "Error sending PlaybackSession", "scrobbler", name, "track", t.Title, "artist", t.Artist, err)
		}
	}
}
