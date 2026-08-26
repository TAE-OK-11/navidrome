package artwork

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

	"github.com/navidrome/navidrome/core/metadataworker"
)

const (
	maxImageWorkers      = 2
	imageWorkerHeaderLen = 16 * 1024
	maxWorkerOutputBytes = 64 * 1024 * 1024
)

type imageWorkerRequest struct {
	InputSize   int    `json:"input_size,omitempty"`
	InputSizes  []int  `json:"input_sizes,omitempty"`
	Mosaic      bool   `json:"mosaic,omitempty"`
	Sniff       bool   `json:"sniff,omitempty"`
	Size        int    `json:"size"`
	Square      bool   `json:"square"`
	Fill        bool   `json:"fill,omitempty"` // center-crop fill mode for playlist tiles
	AnimatedGIF bool   `json:"animated_gif,omitempty"`
	Quality     int    `json:"quality"`
	Format      string `json:"format"`
}

type imageAnimationFlags struct {
	AnimatedGIF bool
	AnimatedWebP bool
	AnimatedPNG bool
}

type imageWorkerResponse struct {
	OK            bool   `json:"ok"`
	Size          int64  `json:"size"`
	Error         string `json:"error"`
	AnimatedGIF   *bool  `json:"animated_gif,omitempty"`
	AnimatedWebP  *bool  `json:"animated_webp,omitempty"`
	AnimatedPNG   *bool  `json:"animated_png,omitempty"`
}

type imageWorker struct {
	binary string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	writer *bufio.Writer
	reader *bufio.Reader
}

type imageWorkerSlot struct {
	worker *imageWorker
}

type imageWorkerPool struct {
	limit chan struct{}
	idle  chan *imageWorkerSlot
}

var persistentImageWorkers = newImageWorkerPool()

func newImageWorkerPool() *imageWorkerPool {
	size := min(max(runtime.GOMAXPROCS(0)/2, 1), maxImageWorkers)
	return &imageWorkerPool{
		limit: make(chan struct{}, size),
		idle:  make(chan *imageWorkerSlot, size),
	}
}

func (p *imageWorkerPool) resize(ctx context.Context, data []byte, size, quality int, square bool, format string) ([]byte, error) {
	return p.resizeRequest(ctx, [][]byte{data}, imageWorkerRequest{
		InputSize: len(data),
		Size:      size,
		Square:    square,
		Quality:   quality,
		Format:    format,
	})
}

func (p *imageWorkerPool) resizeAnimatedGIF(ctx context.Context, data []byte, size, quality int) ([]byte, error) {
	return p.resizeRequest(ctx, [][]byte{data}, imageWorkerRequest{
		InputSize:   len(data),
		Size:        size,
		AnimatedGIF: true,
		Quality:     quality,
		Format:      "gif",
	})
}

// mosaic fills and stitches 1 or 4 album covers into a playlist mosaic in one Rust round-trip.
func (p *imageWorkerPool) mosaic(ctx context.Context, tiles [][]byte, size, quality int, format string) ([]byte, error) {
	if len(tiles) == 0 || len(tiles) > 4 {
		return nil, fmt.Errorf("mosaic requires 1..=4 tiles, got %d", len(tiles))
	}
	sizes := make([]int, len(tiles))
	for i, tile := range tiles {
		sizes[i] = len(tile)
	}
	return p.resizeRequest(ctx, tiles, imageWorkerRequest{
		InputSizes: sizes,
		Mosaic:     true,
		Size:       size,
		Quality:    quality,
		Format:     format,
	})
}

func (p *imageWorkerPool) sniffAnimation(ctx context.Context, data []byte) (imageAnimationFlags, error) {
	var flags imageAnimationFlags
	binary, err := metadataworker.Resolve()
	if err != nil {
		return flags, err
	}

	select {
	case p.limit <- struct{}{}:
	case <-ctx.Done():
		return flags, ctx.Err()
	}
	defer func() { <-p.limit }()

	var slot *imageWorkerSlot
	select {
	case slot = <-p.idle:
	default:
		slot = &imageWorkerSlot{}
	}
	defer func() { p.idle <- slot }()

	worker, err := slot.ensure(binary)
	if err != nil {
		return flags, err
	}
	response, err := worker.roundTripHeader(imageWorkerRequest{
		Sniff:     true,
		InputSize: len(data),
	}, [][]byte{data})
	if err != nil {
		return flags, err
	}
	if response.AnimatedGIF != nil {
		flags.AnimatedGIF = *response.AnimatedGIF
	}
	if response.AnimatedWebP != nil {
		flags.AnimatedWebP = *response.AnimatedWebP
	}
	if response.AnimatedPNG != nil {
		flags.AnimatedPNG = *response.AnimatedPNG
	}
	return flags, nil
}

func (p *imageWorkerPool) resizeRequest(ctx context.Context, payloads [][]byte, request imageWorkerRequest) ([]byte, error) {
	binary, err := metadataworker.Resolve()
	if err != nil {
		return nil, err
	}

	select {
	case p.limit <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-p.limit }()

	var slot *imageWorkerSlot
	select {
	case slot = <-p.idle:
	default:
		slot = &imageWorkerSlot{}
	}
	defer func() { p.idle <- slot }()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		worker, err := slot.ensure(binary)
		if err != nil {
			return nil, err
		}
		cancelDone := make(chan struct{})
		stopCancel := context.AfterFunc(ctx, func() {
			worker.kill()
			close(cancelDone)
		})
		resized, err := worker.roundTrip(request, payloads)
		if !stopCancel() {
			<-cancelDone
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			slot.stop()
			return nil, ctxErr
		}
		if err == nil {
			return resized, nil
		}
		var resizeErr *imageResizeError
		if errors.As(err, &resizeErr) {
			return nil, err
		}
		lastErr = err
		slot.stop()
	}
	return nil, fmt.Errorf("persistent Rust image worker failed after restart: %w", lastErr)
}

func (s *imageWorkerSlot) ensure(binary string) (*imageWorker, error) {
	if s.worker != nil && s.worker.binary == binary {
		return s.worker, nil
	}
	s.stop()
	worker, err := startImageWorker(binary)
	if err != nil {
		return nil, err
	}
	s.worker = worker
	return worker, nil
}

func (s *imageWorkerSlot) stop() {
	if s.worker == nil {
		return
	}
	s.worker.close()
	s.worker = nil
}

func startImageWorker(binary string) (*imageWorker, error) {
	cmd := exec.Command(binary, "--image-worker") //nolint:gosec // resolved administrator-controlled binary
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening image worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("opening image worker stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("starting image worker %q: %w", binary, err)
	}
	return &imageWorker{
		binary: binary,
		cmd:    cmd,
		stdin:  stdin,
		writer: bufio.NewWriterSize(stdin, 128*1024),
		reader: bufio.NewReaderSize(stdout, imageWorkerHeaderLen),
	}, nil
}

func (w *imageWorker) roundTrip(request imageWorkerRequest, payloads [][]byte) ([]byte, error) {
	response, err := w.writeRequest(request, payloads)
	if err != nil {
		return nil, err
	}
	if request.Sniff {
		return nil, fmt.Errorf("sniff request must use roundTripHeader")
	}
	if response.Size <= 0 || response.Size > maxWorkerOutputBytes {
		return nil, fmt.Errorf("Rust image worker returned invalid output size %d", response.Size)
	}
	resized := make([]byte, response.Size)
	if _, err := io.ReadFull(w.reader, resized); err != nil {
		return nil, fmt.Errorf("reading resized image: %w", err)
	}
	return resized, nil
}

func (w *imageWorker) roundTripHeader(request imageWorkerRequest, payloads [][]byte) (imageWorkerResponse, error) {
	return w.writeRequest(request, payloads)
}

func (w *imageWorker) writeRequest(request imageWorkerRequest, payloads [][]byte) (imageWorkerResponse, error) {
	header, err := json.Marshal(request)
	if err != nil {
		return imageWorkerResponse{}, fmt.Errorf("encoding image request: %w", err)
	}
	if _, err := w.writer.Write(header); err != nil {
		return imageWorkerResponse{}, fmt.Errorf("writing image request: %w", err)
	}
	if err := w.writer.WriteByte('\n'); err != nil {
		return imageWorkerResponse{}, fmt.Errorf("framing image request: %w", err)
	}
	for i, data := range payloads {
		if _, err := w.writer.Write(data); err != nil {
			return imageWorkerResponse{}, fmt.Errorf("writing image payload %d: %w", i, err)
		}
	}
	if err := w.writer.Flush(); err != nil {
		return imageWorkerResponse{}, fmt.Errorf("flushing image request: %w", err)
	}

	responseHeader, err := w.reader.ReadSlice('\n')
	if err != nil {
		return imageWorkerResponse{}, fmt.Errorf("reading image response header: %w", err)
	}
	var response imageWorkerResponse
	if err := json.Unmarshal(bytes.TrimSuffix(responseHeader, []byte{'\n'}), &response); err != nil {
		return imageWorkerResponse{}, fmt.Errorf("decoding image response header: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Rust image worker request failed"
		}
		return imageWorkerResponse{}, &imageResizeError{message: response.Error}
	}
	return response, nil
}

func (w *imageWorker) kill() {
	if w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
}

func (w *imageWorker) close() {
	_ = w.stdin.Close()
	w.kill()
	_ = w.cmd.Wait()
}

type imageResizeError struct {
	message string
}

func (e *imageResizeError) Error() string {
	return e.message
}
