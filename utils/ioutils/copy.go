package ioutils

import (
	"io"
	"sync"
)

// DefaultCopyBufferSize matches the largest HTTP/2 bridge frame used by the Rust
// HTTP/3 companion so streaming hot paths can reuse one tuned buffer size.
const DefaultCopyBufferSize = 64 * 1024

var copyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, DefaultCopyBufferSize)
		return &buf
	},
}

// Copy copies from r to w using a pooled buffer sized for media streaming.
func Copy(w io.Writer, r io.Reader) (int64, error) {
	bufPtr := copyBufferPool.Get().(*[]byte)
	defer copyBufferPool.Put(bufPtr)
	return io.CopyBuffer(w, r, *bufPtr)
}
