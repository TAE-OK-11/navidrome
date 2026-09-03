package integration

import (
	"strings"
)

// Destination names a remote system. All outbound HTTP is routed through the
// integration gateway instead of each adapter owning a point-to-point client.
type Destination string

const (
	DestUnknown      Destination = "unknown"
	DestLastFM       Destination = "lastfm"
	DestLibreFM      Destination = "librefm"
	DestListenBrainz Destination = "listenbrainz"
	DestDeezer       Destination = "deezer"
	DestInsights     Destination = "insights"
	DestArtwork      Destination = "artwork"
)

func DestinationFromHost(host string) Destination {
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "audioscrobbler.com"), strings.Contains(h, "last.fm"):
		return DestLastFM
	case strings.Contains(h, "libre.fm"):
		return DestLibreFM
	case strings.Contains(h, "listenbrainz.org"):
		return DestListenBrainz
	case strings.Contains(h, "deezer.com"):
		return DestDeezer
	case strings.Contains(h, "navidrome.org"):
		return DestInsights
	default:
		return DestUnknown
	}
}
