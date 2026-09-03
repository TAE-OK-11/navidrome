package rustworker

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	json "github.com/goccy/go-json"
)

const (
	// DefaultReadBuf is the default stdout read buffer for NDJSON worker headers.
	DefaultReadBuf = 64 * 1024
	// DefaultWriteBuf is the default stdin write buffer for worker requests.
	DefaultWriteBuf = 64 * 1024
)

// WriteJSONLine marshals v and writes it as a single NDJSON line.
func WriteJSONLine(w *bufio.Writer, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding worker request: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("writing worker request: %w", err)
	}
	if err := w.WriteByte('\n'); err != nil {
		return fmt.Errorf("framing worker request: %w", err)
	}
	return w.Flush()
}

// ReadJSONLine reads one NDJSON line and unmarshals it into dest.
func ReadJSONLine(r *bufio.Reader, dest any) error {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("reading worker response header: %w", err)
	}
	if err := json.Unmarshal(bytes.TrimSuffix(line, []byte{'\n'}), dest); err != nil {
		return fmt.Errorf("decoding worker response header: %w", err)
	}
	return nil
}

// WriteHeaderAndBodies marshals header as one NDJSON line, appends binary bodies, then flushes.
func WriteHeaderAndBodies(w *bufio.Writer, header any, bodies ...[]byte) error {
	payload, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("encoding worker request: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("writing worker request: %w", err)
	}
	if err := w.WriteByte('\n'); err != nil {
		return fmt.Errorf("framing worker request: %w", err)
	}
	for i, body := range bodies {
		if _, err := w.Write(body); err != nil {
			return fmt.Errorf("writing worker payload %d: %w", i, err)
		}
	}
	return w.Flush()
}

// ReadSizedBody reads exactly size bytes from r after a header line.
func ReadSizedBody(r io.Reader, size int64, maxBytes int64) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("worker returned invalid body size %d", size)
	}
	if maxBytes > 0 && size > maxBytes {
		return nil, fmt.Errorf("worker response exceeds maximum size of %d bytes", maxBytes)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("reading worker response body: %w", err)
	}
	return data, nil
}
