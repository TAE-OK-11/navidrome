package metadataworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

type normalizeWorkerRequest struct {
	Values []string `json:"values"`
}

type normalizeWorkerResponse struct {
	OK         bool   `json:"ok"`
	Normalized string `json:"normalized,omitempty"`
	Error      string `json:"error,omitempty"`
}

type normalizeWorker struct {
	binary string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	writer *bufio.Writer
	reader *bufio.Reader
}

type normalizeWorkerSlot struct {
	worker *normalizeWorker
}

type normalizeWorkerPool struct {
	limit chan struct{}
	idle  chan *normalizeWorkerSlot
}

var persistentNormalizeWorkers = newNormalizeWorkerPool()

func newNormalizeWorkerPool() *normalizeWorkerPool {
	size := min(max(runtime.GOMAXPROCS(0)/2, 1), maxLyricsWorkers)
	return &normalizeWorkerPool{
		limit: make(chan struct{}, size),
		idle:  make(chan *normalizeWorkerSlot, size),
	}
}

// PersistentNormalizeWorkers returns the shared Rust FTS normalize pool.
func PersistentNormalizeWorkers() *normalizeWorkerPool {
	return persistentNormalizeWorkers
}

func (p *normalizeWorkerPool) Normalize(ctx context.Context, values ...string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	binary, err := Resolve()
	if err != nil {
		return "", err
	}

	select {
	case p.limit <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-p.limit }()

	var slot *normalizeWorkerSlot
	select {
	case slot = <-p.idle:
	default:
		slot = &normalizeWorkerSlot{}
	}
	defer func() { p.idle <- slot }()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		worker, err := slot.ensure(binary)
		if err != nil {
			return "", err
		}
		cancelDone := make(chan struct{})
		stopCancel := context.AfterFunc(ctx, func() {
			worker.kill()
			close(cancelDone)
		})
		normalized, err := worker.roundTrip(values)
		if !stopCancel() {
			<-cancelDone
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			slot.stop()
			return "", ctxErr
		}
		if err == nil {
			return normalized, nil
		}
		lastErr = err
		slot.stop()
	}
	return "", fmt.Errorf("persistent Rust normalize worker failed after restart: %w", lastErr)
}

func (s *normalizeWorkerSlot) ensure(binary string) (*normalizeWorker, error) {
	if s.worker != nil && s.worker.binary == binary {
		return s.worker, nil
	}
	s.stop()
	worker, err := startNormalizeWorker(binary)
	if err != nil {
		return nil, err
	}
	s.worker = worker
	return worker, nil
}

func (s *normalizeWorkerSlot) stop() {
	if s.worker == nil {
		return
	}
	s.worker.close()
	s.worker = nil
}

func startNormalizeWorker(binary string) (*normalizeWorker, error) {
	cmd := exec.Command(binary, "--normalize-fts-worker") //nolint:gosec // resolved administrator-controlled binary
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening normalize worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("opening normalize worker stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("starting normalize worker %q: %w", binary, err)
	}
	return &normalizeWorker{
		binary: binary,
		cmd:    cmd,
		stdin:  stdin,
		writer: bufio.NewWriterSize(stdin, 16*1024),
		reader: bufio.NewReaderSize(stdout, lyricsWorkerReadBufBytes),
	}, nil
}

func (w *normalizeWorker) roundTrip(values []string) (string, error) {
	request := normalizeWorkerRequest{Values: values}
	header, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encoding normalize request: %w", err)
	}
	if _, err := w.writer.Write(header); err != nil {
		return "", fmt.Errorf("writing normalize request: %w", err)
	}
	if err := w.writer.WriteByte('\n'); err != nil {
		return "", fmt.Errorf("framing normalize request: %w", err)
	}
	if err := w.writer.Flush(); err != nil {
		return "", fmt.Errorf("flushing normalize request: %w", err)
	}

	responseHeader, err := w.reader.ReadSlice('\n')
	if err != nil {
		return "", fmt.Errorf("reading normalize response header: %w", err)
	}
	var response normalizeWorkerResponse
	if err := json.Unmarshal(bytes.TrimSuffix(responseHeader, []byte{'\n'}), &response); err != nil {
		return "", fmt.Errorf("decoding normalize response header: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Rust normalize worker failed"
		}
		return "", errors.New(response.Error)
	}
	return response.Normalized, nil
}

func (w *normalizeWorker) kill() {
	if w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
}

func (w *normalizeWorker) close() {
	_ = w.stdin.Close()
	w.kill()
	_ = w.cmd.Wait()
}
