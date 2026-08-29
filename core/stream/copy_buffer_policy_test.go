package stream

import (
	"testing"

	"github.com/navidrome/navidrome/utils/ioutils"
)

func TestStreamCopyBufferMatchesH3BridgeFrame(t *testing.T) {
	const h3BridgeFrameSize = 64 * 1024
	if streamCopyBufferSize != h3BridgeFrameSize {
		t.Fatalf("stream copy buffer=%d, want H3 bridge frame=%d", streamCopyBufferSize, h3BridgeFrameSize)
	}
	if ioutils.DefaultCopyBufferSize != h3BridgeFrameSize {
		t.Fatalf("shared copy buffer=%d, want H3 bridge frame=%d", ioutils.DefaultCopyBufferSize, h3BridgeFrameSize)
	}
}
