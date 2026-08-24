package ffmpeg

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

const (
	maxPictureWorkers  = 4
	pictureHeaderBytes = 16 * 1024
)

type pictureRequest struct {
	Path     string `json:"path"`
	MaxBytes int64  `json:"max_bytes"`
}

type pictureResponse struct {
	OK    bool   `json:"ok"`
	Size  int64  `json:"size"`
	Error string `json:"error"`
}

type pictureWorker struct {
	binary string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	writer *bufio.Writer
	reader *bufio.Reader
}

type pictureWorkerSlot struct {
	worker *pictureWorker
}

type pictureWorkerPool struct {
	limit chan struct{}
	idle  chan *pictureWorkerSlot
}

var persistentPictureWorkers = newPictureWorkerPool()

func newPictureWorkerPool() *pictureWorkerPool {
	size := min(max(runtime.GOMAXPROCS(0), 1), maxPictureWorkers)
	return &pictureWorkerPool{
		limit: make(chan struct{}, size),
		idle:  make(chan *pictureWorkerSlot, size),
	}
}

func (p *pictureWorkerPool) extract(ctx context.Context, binary, path string, maxBytes int64) ([]byte, error) {
	select {
	case p.limit <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-p.limit }()

	var slot *pictureWorkerSlot
	select {
	case slot = <-p.idle:
	default:
		slot = &pictureWorkerSlot{}
	}
	// Return the worker before releasing capacity, so the next admitted request
	// observes the reusable process instead of spawning another one.
	defer func() { p.idle <- slot }()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		worker, err := slot.ensure(binary)
		if err != nil {
			return nil, err
		}
		stopCancel := context.AfterFunc(ctx, worker.kill)
		data, err := worker.roundTrip(path, maxBytes)
		stopCancel()
		if ctxErr := ctx.Err(); ctxErr != nil {
			slot.stop()
			return nil, ctxErr
		}
		if err == nil {
			return data, nil
		}
		var extractionErr *pictureExtractionError
		if errors.As(err, &extractionErr) {
			return nil, err
		}
		lastErr = err
		slot.stop()
	}
	return nil, fmt.Errorf("persistent metadata picture worker failed after restart: %w", lastErr)
}

func (s *pictureWorkerSlot) ensure(binary string) (*pictureWorker, error) {
	if s.worker != nil && s.worker.binary == binary {
		return s.worker, nil
	}
	s.stop()
	worker, err := startPictureWorker(binary)
	if err != nil {
		return nil, err
	}
	s.worker = worker
	return worker, nil
}

func (s *pictureWorkerSlot) stop() {
	if s.worker == nil {
		return
	}
	s.worker.close()
	s.worker = nil
}

func startPictureWorker(binary string) (*pictureWorker, error) {
	cmd := exec.Command(binary, "--picture-worker") //nolint:gosec // resolved administrator-controlled binary
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening picture worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("opening picture worker stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("starting picture worker %q: %w", binary, err)
	}
	return &pictureWorker{
		binary: binary,
		cmd:    cmd,
		stdin:  stdin,
		writer: bufio.NewWriterSize(stdin, pictureHeaderBytes),
		reader: bufio.NewReaderSize(stdout, pictureHeaderBytes),
	}, nil
}

func (w *pictureWorker) roundTrip(path string, maxBytes int64) ([]byte, error) {
	payload, err := json.Marshal(pictureRequest{Path: path, MaxBytes: maxBytes})
	if err != nil {
		return nil, fmt.Errorf("encoding picture request: %w", err)
	}
	if _, err := w.writer.Write(payload); err != nil {
		return nil, fmt.Errorf("writing picture request: %w", err)
	}
	if err := w.writer.WriteByte('\n'); err != nil {
		return nil, fmt.Errorf("framing picture request: %w", err)
	}
	if err := w.writer.Flush(); err != nil {
		return nil, fmt.Errorf("flushing picture request: %w", err)
	}

	header, err := w.reader.ReadSlice('\n')
	if err != nil {
		return nil, fmt.Errorf("reading picture response header: %w", err)
	}
	var response pictureResponse
	if err := json.Unmarshal(bytes.TrimSuffix(header, []byte{'\n'}), &response); err != nil {
		return nil, fmt.Errorf("decoding picture response header: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "metadata worker could not extract artwork"
		}
		return nil, &pictureExtractionError{message: response.Error}
	}
	if response.Size <= 0 {
		return nil, errors.New("metadata picture worker returned an invalid size")
	}
	if response.Size > maxBytes {
		return nil, fmt.Errorf("metadata picture worker response exceeds maximum size of %d bytes", maxBytes)
	}
	data := make([]byte, response.Size)
	if _, err := io.ReadFull(w.reader, data); err != nil {
		return nil, fmt.Errorf("reading metadata picture: %w", err)
	}
	return data, nil
}

func (w *pictureWorker) kill() {
	if w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
}

func (w *pictureWorker) close() {
	_ = w.stdin.Close()
	w.kill()
	_ = w.cmd.Wait()
}

type pictureExtractionError struct {
	message string
}

func (e *pictureExtractionError) Error() string {
	return e.message
}

func (p *pictureWorkerPool) closeIdle() {
	for {
		select {
		case slot := <-p.idle:
			slot.stop()
		default:
			return
		}
	}
}
