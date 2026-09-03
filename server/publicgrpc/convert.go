package publicgrpc

import (
	"github.com/navidrome/navidrome/core/eventbus"
	"github.com/navidrome/navidrome/server/publicgrpc/gen"
)

func toPublicEvent(evt eventbus.Event) *gen.Event {
	out := &gen.Event{
		Id:             evt.ID,
		Topic:          string(evt.Topic),
		OccurredAtUnix: evt.OccurredAt.Unix(),
		Attributes:     evt.Attrs,
	}
	switch {
	case evt.UIScan != nil:
		out.Payload = &gen.Event_ScanStatus{ScanStatus: &gen.ScanStatus{
			Scanning:    evt.UIScan.Scanning,
			Count:       evt.UIScan.Count,
			FolderCount: evt.UIScan.FolderCount,
			Error:       evt.UIScan.Error,
			ScanType:    evt.UIScan.ScanType,
			ElapsedMs:   evt.UIScan.ElapsedTime.Milliseconds(),
		}}
	case evt.Refresh != nil:
		resources := make(map[string]*gen.StringList, len(evt.Refresh.Resources))
		for k, v := range evt.Refresh.Resources {
			resources[k] = &gen.StringList{Values: append([]string(nil), v...)}
		}
		out.Payload = &gen.Event_Refresh{Refresh: &gen.RefreshResource{Resources: resources}}
	case evt.NowPlayingCount != nil:
		out.Payload = &gen.Event_NowPlayingCount{NowPlayingCount: &gen.NowPlayingCount{Count: int32(evt.NowPlayingCount.Count)}}
	case evt.Scan != nil:
		out.Payload = &gen.Event_ScanCompleted{ScanCompleted: &gen.ScanCompleted{
			FullScan:        evt.Scan.FullScan,
			ChangesDetected: evt.Scan.ChangesDetected,
			Error:           evt.Scan.Error,
			FileCount:       evt.Scan.FileCount,
			FolderCount:     evt.Scan.FolderCount,
		}}
	}
	return out
}
