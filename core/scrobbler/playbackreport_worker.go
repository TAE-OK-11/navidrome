package scrobbler

import (
	"context"

	"github.com/navidrome/navidrome/core/eventbus"
	"github.com/navidrome/navidrome/log"
)

func (p *playTracker) enqueuePlaybackReport(ctx context.Context, info PlaybackSession) {
	p.prMu.Lock()
	defer p.prMu.Unlock()
	ctx = context.WithoutCancel(ctx)
	p.prQueue = append(p.prQueue, playbackReportEntry{
		ctx:  ctx,
		info: info,
	})
	p.sendPlaybackReportSignal()
}

func (p *playTracker) sendPlaybackReportSignal() {
	select {
	case p.prSignal <- struct{}{}:
	default:
	}
}

func (p *playTracker) playbackReportWorker() {
	defer close(p.prWorkerDone)
	for {
		select {
		case <-p.shutdown:
			return
		case <-p.prSignal:
		}

		p.prMu.Lock()
		if len(p.prQueue) == 0 {
			p.prMu.Unlock()
			continue
		}
		entries := p.prQueue
		p.prQueue = nil
		p.prMu.Unlock()

		for _, entry := range entries {
			p.dispatchPlaybackReport(entry.ctx, entry.info)
		}
	}
}

func (p *playTracker) dispatchPlaybackReport(ctx context.Context, info PlaybackSession) {
	p.bus.PublishSync(ctx, eventbus.Event{
		Topic: eventbus.TopicPlaybackReport,
		Report: &eventbus.PlaybackReport{
			UserID:      info.UserId,
			PlayerID:    info.PlayerId,
			MediaFileID: info.MediaFile.ID,
			State:       info.State,
			PositionMs:  info.PositionMs,
			Data:        info,
		},
	})
}

func (p *playTracker) onPlaybackReport(ctx context.Context, evt eventbus.Event) {
	if evt.Report == nil {
		return
	}
	info, ok := evt.Report.Data.(PlaybackSession)
	if !ok {
		return
	}
	for name, s := range p.getActiveScrobblers() {
		if !s.IsAuthorized(ctx, info.UserId) {
			continue
		}
		log.Debug(ctx, "Sending PlaybackReport", "scrobbler", name, "track", info.MediaFile.Title, "state", info.State, "positionMs", info.PositionMs)
		if err := s.PlaybackReport(ctx, info); err != nil {
			log.Error(ctx, "Error sending PlaybackReport", "scrobbler", name, "track", info.MediaFile.Title, "state", info.State, err)
		}
	}
}
