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
	case evt.ScanProgress != nil:
		out.Payload = &gen.Event_ScanProgress{ScanProgress: &gen.ScanProgress{
			LibId:           int32(evt.ScanProgress.LibID),
			FileCount:       evt.ScanProgress.FileCount,
			Path:            evt.ScanProgress.Path,
			Phase:           evt.ScanProgress.Phase,
			ChangesDetected: evt.ScanProgress.ChangesDetected,
			Warning:         evt.ScanProgress.Warning,
			Error:           evt.ScanProgress.Error,
			ForceUpdate:     evt.ScanProgress.ForceUpdate,
		}}
	case evt.NowPlaying != nil:
		out.Payload = &gen.Event_NowPlaying{NowPlaying: &gen.NowPlaying{
			UserId:      evt.NowPlaying.UserID,
			MediaFileId: evt.NowPlaying.MediaFileID,
			Title:       evt.NowPlaying.Title,
			Artist:      evt.NowPlaying.Artist,
			PositionSec: int32(evt.NowPlaying.PositionSec),
		}}
	case evt.Scrobble != nil:
		out.Payload = &gen.Event_Scrobble{Scrobble: &gen.Scrobble{
			UserId:       evt.Scrobble.UserID,
			Username:     evt.Scrobble.Username,
			MediaFileId:  evt.Scrobble.MediaFileID,
			Title:        evt.Scrobble.Title,
			Artist:       evt.Scrobble.Artist,
			Album:        evt.Scrobble.Album,
			PlayedAtUnix: evt.Scrobble.PlayedAt.Unix(),
		}}
	case evt.Report != nil:
		out.Payload = &gen.Event_PlaybackReport{PlaybackReport: &gen.PlaybackReport{
			UserId:      evt.Report.UserID,
			PlayerId:    evt.Report.PlayerID,
			MediaFileId: evt.Report.MediaFileID,
			State:       evt.Report.State,
			PositionMs:  evt.Report.PositionMs,
		}}
	}
	return out
}
