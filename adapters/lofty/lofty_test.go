package lofty

import (
	"testing"
	"time"
)

func TestWireMetadataInfo(t *testing.T) {
	item := wireMetadata{
		Tags: map[string][]string{"title": {"Song"}},
		AudioProperties: wireAudioProperties{
			DurationMS: 1020,
			BitRate:    256,
			BitDepth:   24,
			SampleRate: 48000,
			Channels:   2,
			Codec:      "alac",
		},
		HasPicture: true,
	}

	info := item.info()
	if got := info.AudioProperties.Duration; got != 1020*time.Millisecond {
		t.Fatalf("duration = %v", got)
	}
	if info.AudioProperties.Codec != "alac" || info.AudioProperties.BitRate != 256 {
		t.Fatalf("unexpected audio properties: %+v", info.AudioProperties)
	}
	if !info.HasPicture || info.Tags["title"][0] != "Song" {
		t.Fatalf("unexpected metadata: %+v", info)
	}
}

func TestVersion(t *testing.T) {
	if got := (&extractor{}).Version(); got != "lofty/0.25.0" {
		t.Fatalf("Version() = %q", got)
	}
}
