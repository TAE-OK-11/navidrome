package metadataworker

import (
	"bufio"
	"context"
	"errors"
	"runtime"

	"github.com/navidrome/navidrome/core/rustworker"
)

const maxNormalizeWorkers = 8

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
	pipes  *rustworker.Pipes
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
	size := min(max(runtime.GOMAXPROCS(0), 1), maxNormalizeWorkers)
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

	var normalized string
	err = rustworker.Run(ctx, rustworker.DefaultRestartAttempts, func() { slot.stop() }, func() error {
		worker, ensureErr := slot.ensure(binary)
		if ensureErr != nil {
			return ensureErr
		}
		var roundErr error
		normalized, roundErr = worker.roundTrip(values)
		return roundErr
	})
	if err != nil {
		return "", rustworker.FailAfterRestarts("normalize", err)
	}
	return normalized, nil
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
	pipes, err := rustworker.Start(binary, "--normalize-fts-worker")
	if err != nil {
		return nil, err
	}
	return &normalizeWorker{
		binary: binary,
		pipes:  pipes,
		writer: bufio.NewWriterSize(pipes.Stdin, rustworker.DefaultWriteBuf),
		reader: bufio.NewReaderSize(pipes.Stdout, rustworker.DefaultReadBuf),
	}, nil
}

func (w *normalizeWorker) roundTrip(values []string) (string, error) {
	if err := rustworker.WriteJSONLine(w.writer, normalizeWorkerRequest{Values: values}); err != nil {
		return "", err
	}
	var response normalizeWorkerResponse
	if err := rustworker.ReadJSONLine(w.reader, &response); err != nil {
		return "", err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Rust normalize worker failed"
		}
		return "", errors.New(response.Error)
	}
	return response.Normalized, nil
}

func (w *normalizeWorker) close() {
	rustworker.Close(w.pipes)
}
