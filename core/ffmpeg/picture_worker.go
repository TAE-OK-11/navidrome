package ffmpeg

import (
	"bufio"
	"context"
	"errors"
	"runtime"

	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/core/rustworker"
)

const maxPictureWorkers = 4

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
	pipes  *rustworker.Pipes
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
	if data, err := metadataworker.ExtractPicture(ctx, path, maxBytes); rustworker.PreferGRPC(err, metadataworker.ErrNoGRPC) {
		if err != nil {
			return nil, &pictureExtractionError{message: err.Error()}
		}
		return data, nil
	}
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
	defer func() { p.idle <- slot }()

	var data []byte
	err := rustworker.Run(ctx, rustworker.DefaultRestartAttempts, func() { slot.stop() }, func() error {
		worker, ensureErr := slot.ensure(binary)
		if ensureErr != nil {
			return ensureErr
		}
		var roundErr error
		data, roundErr = worker.roundTrip(path, maxBytes)
		if roundErr != nil {
			var extractionErr *pictureExtractionError
			if errors.As(roundErr, &extractionErr) {
				return roundErr
			}
		}
		return roundErr
	})
	if err != nil {
		var extractionErr *pictureExtractionError
		if errors.As(err, &extractionErr) {
			return nil, err
		}
		return nil, rustworker.FailAfterRestarts("picture", err)
	}
	return data, nil
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
	pipes, err := rustworker.Start(binary, "--picture-worker")
	if err != nil {
		return nil, err
	}
	return &pictureWorker{
		binary: binary,
		pipes:  pipes,
		writer: bufio.NewWriterSize(pipes.Stdin, rustworker.DefaultWriteBuf),
		reader: bufio.NewReaderSize(pipes.Stdout, rustworker.DefaultReadBuf),
	}, nil
}

func (w *pictureWorker) roundTrip(path string, maxBytes int64) ([]byte, error) {
	if err := rustworker.WriteJSONLine(w.writer, pictureRequest{Path: path, MaxBytes: maxBytes}); err != nil {
		return nil, err
	}
	var response pictureResponse
	if err := rustworker.ReadJSONLine(w.reader, &response); err != nil {
		return nil, err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "metadata worker could not extract artwork"
		}
		return nil, &pictureExtractionError{message: response.Error}
	}
	return rustworker.ReadSizedBody(w.reader, response.Size, maxBytes)
}

func (w *pictureWorker) close() {
	rustworker.Close(w.pipes)
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
