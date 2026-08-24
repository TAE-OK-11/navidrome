package stream

import "testing"

func TestStreamCopyBufferMatchesH3BridgeFrame(t *testing.T) {
	const h3BridgeFrameSize = 64 * 1024
	if streamCopyBufferSize != h3BridgeFrameSize {
		t.Fatalf("stream copy buffer=%d, want H3 bridge frame=%d", streamCopyBufferSize, h3BridgeFrameSize)
	}
}
