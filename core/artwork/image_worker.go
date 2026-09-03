package artwork

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/core/metadataworker/gen"
	"github.com/navidrome/navidrome/core/rustworker"
)

const (
	maxImageWorkers      = 2
	maxWorkerOutputBytes = 64 * 1024 * 1024
)

type imageWorkerRequest struct {
	InputSize    int    `json:"input_size,omitempty"`
	InputSizes   []int  `json:"input_sizes,omitempty"`
	Mosaic       bool   `json:"mosaic,omitempty"`
	Sniff        bool   `json:"sniff,omitempty"`
	Size         int    `json:"size"`
	Square       bool   `json:"square"`
	Fill         bool   `json:"fill,omitempty"` // center-crop fill mode for playlist tiles
	AnimatedGIF  bool   `json:"animated_gif,omitempty"`
	AnimatedWebP bool   `json:"animated_webp,omitempty"`
	AnimatedPNG  bool   `json:"animated_png,omitempty"`
	Quality      int    `json:"quality"`
	Format       string `json:"format,omitempty"`
	Path         string `json:"path,omitempty"`
}

type imageAnimationFlags struct {
	AnimatedGIF  bool
	AnimatedWebP bool
	AnimatedPNG  bool
}

type imageWorkerResponse struct {
	OK           bool   `json:"ok"`
	Size         int64  `json:"size"`
	Error        string `json:"error"`
	AnimatedGIF  *bool  `json:"animated_gif,omitempty"`
	AnimatedWebP *bool  `json:"animated_webp,omitempty"`
	AnimatedPNG  *bool  `json:"animated_png,omitempty"`
}

type imageWorker struct {
	binary string
	pipes  *rustworker.Pipes
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

func (p *imageWorkerPool) resizePath(ctx context.Context, path string, size, quality int, square bool, format string) ([]byte, error) {
	return p.resizeRequest(ctx, nil, imageWorkerRequest{
		Path:    path,
		Size:    size,
		Square:  square,
		Quality: quality,
		Format:  format,
	})
}

func (p *imageWorkerPool) resizeAnimatedGIF(ctx context.Context, data []byte, size, quality int) ([]byte, error) {
	return p.resizeRequest(ctx, [][]byte{data}, imageWorkerRequest{
		InputSize:   len(data),
		Size:        size,
		AnimatedGIF: true,
		Quality:     quality,
	})
}

func (p *imageWorkerPool) resizeAnimatedGIFPath(ctx context.Context, path string, size, quality int) ([]byte, error) {
	return p.resizeRequest(ctx, nil, imageWorkerRequest{
		Path:        path,
		Size:        size,
		AnimatedGIF: true,
		Quality:     quality,
	})
}

func (p *imageWorkerPool) resizeAnimatedWebP(ctx context.Context, data []byte, size, quality int) ([]byte, error) {
	return p.resizeRequest(ctx, [][]byte{data}, imageWorkerRequest{
		InputSize:    len(data),
		Size:         size,
		AnimatedWebP: true,
		Quality:      quality,
	})
}

func (p *imageWorkerPool) resizeAnimatedWebPPath(ctx context.Context, path string, size, quality int) ([]byte, error) {
	return p.resizeRequest(ctx, nil, imageWorkerRequest{
		Path:         path,
		Size:         size,
		AnimatedWebP: true,
		Quality:      quality,
	})
}

func (p *imageWorkerPool) resizeAnimatedPNG(ctx context.Context, data []byte, size, quality int) ([]byte, error) {
	return p.resizeRequest(ctx, [][]byte{data}, imageWorkerRequest{
		InputSize:   len(data),
		Size:        size,
		AnimatedPNG: true,
		Quality:     quality,
	})
}

func (p *imageWorkerPool) resizeAnimatedPNGPath(ctx context.Context, path string, size, quality int) ([]byte, error) {
	return p.resizeRequest(ctx, nil, imageWorkerRequest{
		Path:        path,
		Size:        size,
		AnimatedPNG: true,
		Quality:     quality,
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
	if flags, err := sniffViaGRPC(ctx, [][]byte{data}, imageWorkerRequest{Sniff: true, InputSize: len(data)}); !errors.Is(err, metadataworker.ErrNoGRPC) {
		return flags, err
	}
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

	var response imageWorkerResponse
	err = rustworker.Run(ctx, rustworker.DefaultRestartAttempts, func() { slot.stop() }, func() error {
		worker, ensureErr := slot.ensure(binary)
		if ensureErr != nil {
			return ensureErr
		}
		var roundErr error
		response, roundErr = worker.roundTripHeader(imageWorkerRequest{
			Sniff:     true,
			InputSize: len(data),
		}, [][]byte{data})
		if roundErr != nil {
			var resizeErr *imageResizeError
			if errors.As(roundErr, &resizeErr) {
				return roundErr
			}
		}
		return roundErr
	})
	if err != nil {
		var resizeErr *imageResizeError
		if errors.As(err, &resizeErr) {
			return flags, err
		}
		return flags, rustworker.FailAfterRestarts("image", err)
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

func (p *imageWorkerPool) sniffAnimationPath(ctx context.Context, path string) (imageAnimationFlags, error) {
	if flags, err := sniffViaGRPC(ctx, nil, imageWorkerRequest{Sniff: true, Path: path}); !errors.Is(err, metadataworker.ErrNoGRPC) {
		return flags, err
	}
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

	var response imageWorkerResponse
	err = rustworker.Run(ctx, rustworker.DefaultRestartAttempts, func() { slot.stop() }, func() error {
		worker, ensureErr := slot.ensure(binary)
		if ensureErr != nil {
			return ensureErr
		}
		var roundErr error
		response, roundErr = worker.roundTripHeader(imageWorkerRequest{
			Sniff: true,
			Path:  path,
		}, nil)
		if roundErr != nil {
			var resizeErr *imageResizeError
			if errors.As(roundErr, &resizeErr) {
				return roundErr
			}
		}
		return roundErr
	})
	if err != nil {
		var resizeErr *imageResizeError
		if errors.As(err, &resizeErr) {
			return flags, err
		}
		return flags, rustworker.FailAfterRestarts("image", err)
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
	if body, err := resizeViaGRPC(ctx, payloads, request); !errors.Is(err, metadataworker.ErrNoGRPC) {
		return body, err
	}
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

	var resized []byte
	err = rustworker.Run(ctx, rustworker.DefaultRestartAttempts, func() { slot.stop() }, func() error {
		worker, ensureErr := slot.ensure(binary)
		if ensureErr != nil {
			return ensureErr
		}
		var roundErr error
		resized, roundErr = worker.roundTrip(request, payloads)
		if roundErr != nil {
			var resizeErr *imageResizeError
			if errors.As(roundErr, &resizeErr) {
				return roundErr
			}
		}
		return roundErr
	})
	if err != nil {
		var resizeErr *imageResizeError
		if errors.As(err, &resizeErr) {
			return nil, err
		}
		return nil, rustworker.FailAfterRestarts("image", err)
	}
	return resized, nil
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
	pipes, err := rustworker.Start(binary, "--image-worker")
	if err != nil {
		return nil, err
	}
	return &imageWorker{
		binary: binary,
		pipes:  pipes,
		writer: bufio.NewWriterSize(pipes.Stdin, rustworker.DefaultWriteBuf),
		reader: bufio.NewReaderSize(pipes.Stdout, rustworker.DefaultReadBuf),
	}, nil
}

func (w *imageWorker) roundTrip(request imageWorkerRequest, payloads [][]byte) ([]byte, error) {
	response, err := w.roundTripHeader(request, payloads)
	if err != nil {
		return nil, err
	}
	if request.Sniff {
		return nil, fmt.Errorf("sniff request must use roundTripHeader")
	}
	return rustworker.ReadSizedBody(w.reader, response.Size, maxWorkerOutputBytes)
}

func (w *imageWorker) roundTripHeader(request imageWorkerRequest, payloads [][]byte) (imageWorkerResponse, error) {
	if err := rustworker.WriteHeaderAndBodies(w.writer, request, payloads...); err != nil {
		return imageWorkerResponse{}, err
	}
	var response imageWorkerResponse
	if err := rustworker.ReadJSONLine(w.reader, &response); err != nil {
		return imageWorkerResponse{}, err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Rust image worker request failed"
		}
		return imageWorkerResponse{}, &imageResizeError{message: response.Error}
	}
	return response, nil
}

func (w *imageWorker) close() {
	rustworker.Close(w.pipes)
}

func toProtoImageRequest(request imageWorkerRequest, payloads [][]byte) *gen.ImageRequest {
	return &gen.ImageRequest{
		Payloads:     payloads,
		Mosaic:       request.Mosaic,
		Sniff:        request.Sniff,
		Size:         uint32(max(request.Size, 0)),
		Square:       request.Square,
		Fill:         request.Fill,
		AnimatedGif:  request.AnimatedGIF,
		AnimatedWebp: request.AnimatedWebP,
		AnimatedPng:  request.AnimatedPNG,
		Quality:      uint32(max(request.Quality, 0)),
		Format:       request.Format,
		Path:         request.Path,
	}
}

func resizeViaGRPC(ctx context.Context, payloads [][]byte, request imageWorkerRequest) ([]byte, error) {
	resp, err := metadataworker.ProcessImage(ctx, toProtoImageRequest(request, payloads))
	if err != nil {
		return nil, err
	}
	return resp.GetBody(), nil
}

func sniffViaGRPC(ctx context.Context, payloads [][]byte, request imageWorkerRequest) (imageAnimationFlags, error) {
	var flags imageAnimationFlags
	resp, err := metadataworker.ProcessImage(ctx, toProtoImageRequest(request, payloads))
	if err != nil {
		return flags, err
	}
	return imageAnimationFlags{
		AnimatedGIF:  resp.GetAnimatedGif(),
		AnimatedWebP: resp.GetAnimatedWebp(),
		AnimatedPNG:  resp.GetAnimatedPng(),
	}, nil
}

type imageResizeError struct {
	message string
}

func (e *imageResizeError) Error() string {
	return e.message
}
