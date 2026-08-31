package ioutils

import (
	"bytes"
	"io"
	"testing"
)

func BenchmarkCopy(b *testing.B) {
	payload := make([]byte, DefaultCopyBufferSize*4)
	for i := range payload {
		payload[i] = byte(i)
	}

	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for b.Loop() {
		r := bytes.NewReader(payload)
		if _, err := Copy(io.Discard, r); err != nil {
			b.Fatal(err)
		}
	}
}
