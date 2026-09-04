package scanner

import (
	"context"

	"github.com/navidrome/navidrome/core/eventbus"
	"github.com/navidrome/navidrome/core/scannerworker/gen"
)

// SubscribeProgress fans scan progress from the process-wide event bus into a
// channel. Callers must unsubscribe (and then close is owned by the producer
// helper below) so a finished scan cannot leak handlers.
func SubscribeProgress(bus *eventbus.Bus) (chan *ProgressInfo, func()) {
	if bus == nil {
		bus = eventbus.Get()
	}
	progress := make(chan *ProgressInfo, 256)
	unsub := bus.Subscribe(eventbus.TopicScanProgress, func(_ context.Context, evt eventbus.Event) {
		if evt.ScanProgress == nil {
			return
		}
		select {
		case progress <- ProgressFromEvent(evt.ScanProgress):
		default:
			// Drop when the consumer is slow; scan work must not block on UI fan-in.
		}
	})
	return progress, unsub
}

// PublishProgress delivers one scan update on the bus. PublishSync is required
// so errors and folder counts are not dropped when the async queue is full.
func PublishProgress(ctx context.Context, info *ProgressInfo) {
	if info == nil {
		return
	}
	eventbus.Get().PublishSync(ctx, eventbus.Event{
		Topic:        eventbus.TopicScanProgress,
		ScanProgress: ProgressToEvent(info),
	})
}

func ProgressToEvent(info *ProgressInfo) *eventbus.ScanProgress {
	if info == nil {
		return nil
	}
	return &eventbus.ScanProgress{
		LibID:           info.LibID,
		FileCount:       info.FileCount,
		Path:            info.Path,
		Phase:           info.Phase,
		ChangesDetected: info.ChangesDetected,
		Warning:         info.Warning,
		Error:           info.Error,
		ForceUpdate:     info.ForceUpdate,
	}
}

func ProgressFromEvent(p *eventbus.ScanProgress) *ProgressInfo {
	if p == nil {
		return &ProgressInfo{}
	}
	return &ProgressInfo{
		LibID:           p.LibID,
		FileCount:       p.FileCount,
		Path:            p.Path,
		Phase:           p.Phase,
		ChangesDetected: p.ChangesDetected,
		Warning:         p.Warning,
		Error:           p.Error,
		ForceUpdate:     p.ForceUpdate,
	}
}

func progressToProto(info *ProgressInfo) *gen.ProgressEvent {
	if info == nil {
		return &gen.ProgressEvent{}
	}
	return &gen.ProgressEvent{
		LibId:           int32(info.LibID),
		FileCount:       info.FileCount,
		Path:            info.Path,
		Phase:           info.Phase,
		ChangesDetected: info.ChangesDetected,
		Warning:         info.Warning,
		Error:           info.Error,
		ForceUpdate:     info.ForceUpdate,
	}
}

func progressFromProto(evt *gen.ProgressEvent) *ProgressInfo {
	if evt == nil {
		return &ProgressInfo{}
	}
	return &ProgressInfo{
		LibID:           int(evt.GetLibId()),
		FileCount:       evt.GetFileCount(),
		Path:            evt.GetPath(),
		Phase:           evt.GetPhase(),
		ChangesDetected: evt.GetChangesDetected(),
		Warning:         evt.GetWarning(),
		Error:           evt.GetError(),
		ForceUpdate:     evt.GetForceUpdate(),
	}
}
