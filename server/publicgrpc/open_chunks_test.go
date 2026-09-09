package publicgrpc

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/navidrome/navidrome/server/publicgrpc/gen"
	"google.golang.org/protobuf/proto"
)

type recordingOpenStream struct {
	gen.Public_OpenServer
	chunks []*gen.OpenChunk
	failAt int
	err    error
}

func (s *recordingOpenStream) Send(chunk *gen.OpenChunk) error {
	if s.err != nil && len(s.chunks) == s.failAt {
		return s.err
	}
	s.chunks = append(s.chunks, proto.Clone(chunk).(*gen.OpenChunk))
	return nil
}

func TestOpenWriteBoundsLargeHandlerWrites(t *testing.T) {
	sink := &recordingOpenStream{}
	w := &streamWriter{stream: sink}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.WriteHeader(http.StatusPartialContent)
	data := bytes.Repeat([]byte("audio"), openChunkSize)
	n, err := w.Write(data)
	if err != nil || n != len(data) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	var got []byte
	for i, chunk := range sink.chunks {
		if len(chunk.Data) > openChunkSize {
			t.Fatalf("oversized chunk: %d", len(chunk.Data))
		}
		if i == 0 {
			if chunk.Status != http.StatusPartialContent || chunk.ContentType != "audio/mpeg" {
				t.Fatal("missing response metadata")
			}
		} else if chunk.Status != 0 || chunk.ContentType != "" || len(chunk.Headers) != 0 {
			t.Fatal("response metadata repeated after first chunk")
		}
		got = append(got, chunk.Data...)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("streamed bytes changed")
	}
}

func TestOpenWriteReportsPartialTransportFailure(t *testing.T) {
	want := errors.New("transport closed")
	sink := &recordingOpenStream{failAt: 1, err: want}
	w := &streamWriter{stream: sink}
	n, err := w.Write(make([]byte, openChunkSize*3))
	if n != openChunkSize || !errors.Is(err, want) {
		t.Fatalf("Write = %d, %v; want one delivered chunk and transport failure", n, err)
	}
}
