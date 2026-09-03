package apikeyworker

import (
	"bufio"
	"context"
	"errors"
	"runtime"

	"github.com/navidrome/navidrome/core/rustworker"
)

const maxWorkers = 4

type workerRequest struct {
	Op     string `json:"op"`
	Pepper string `json:"pepper,omitempty"`
	Token  string `json:"token,omitempty"`
	Hash   string `json:"hash,omitempty"`
}

type workerResponse struct {
	OK           bool   `json:"ok"`
	Token        string `json:"token,omitempty"`
	LookupPrefix string `json:"lookup_prefix,omitempty"`
	Hash         string `json:"hash,omitempty"`
	Valid        *bool  `json:"valid,omitempty"`
	Error        string `json:"error,omitempty"`
}

type generateResult struct {
	Token        string
	LookupPrefix string
	Hash         string
}

type worker struct {
	binary string
	pipes  *rustworker.Pipes
	writer *bufio.Writer
	reader *bufio.Reader
}

type workerSlot struct {
	worker *worker
}

type workerPool struct {
	limit chan struct{}
	idle  chan *workerSlot
}

var persistentWorkers = newWorkerPool()

func newWorkerPool() *workerPool {
	size := min(max(runtime.GOMAXPROCS(0), 1), maxWorkers)
	return &workerPool{
		limit: make(chan struct{}, size),
		idle:  make(chan *workerSlot, size),
	}
}

func (p *workerPool) roundTrip(ctx context.Context, request workerRequest) (workerResponse, error) {
	binary, err := Resolve()
	if err != nil {
		return workerResponse{}, err
	}

	select {
	case p.limit <- struct{}{}:
	case <-ctx.Done():
		return workerResponse{}, ctx.Err()
	}
	defer func() { <-p.limit }()

	var slot *workerSlot
	select {
	case slot = <-p.idle:
	default:
		slot = &workerSlot{}
	}
	defer func() { p.idle <- slot }()

	var response workerResponse
	err = rustworker.Run(ctx, rustworker.DefaultRestartAttempts, func() { slot.stop() }, func() error {
		w, ensureErr := slot.ensure(binary)
		if ensureErr != nil {
			return ensureErr
		}
		if writeErr := rustworker.WriteJSONLine(w.writer, request); writeErr != nil {
			return writeErr
		}
		var roundErr error
		roundErr = rustworker.ReadJSONLine(w.reader, &response)
		return roundErr
	})
	if err != nil {
		return workerResponse{}, rustworker.FailAfterRestarts("apikeys", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Rust apikeys worker failed"
		}
		return workerResponse{}, errors.New(response.Error)
	}
	return response, nil
}

func (s *workerSlot) ensure(binary string) (*worker, error) {
	if s.worker != nil && s.worker.binary == binary {
		return s.worker, nil
	}
	s.stop()
	w, err := startWorker(binary)
	if err != nil {
		return nil, err
	}
	s.worker = w
	return w, nil
}

func (s *workerSlot) stop() {
	if s.worker == nil {
		return
	}
	s.worker.close()
	s.worker = nil
}

func startWorker(binary string) (*worker, error) {
	pipes, err := rustworker.Start(binary, "--apikeys-worker")
	if err != nil {
		return nil, err
	}
	return &worker{
		binary: binary,
		pipes:  pipes,
		writer: bufio.NewWriterSize(pipes.Stdin, rustworker.DefaultWriteBuf),
		reader: bufio.NewReaderSize(pipes.Stdout, rustworker.DefaultReadBuf),
	}, nil
}

func (w *worker) close() {
	rustworker.Close(w.pipes)
}

func Generate(ctx context.Context, pepper string) (generateResult, error) {
	if result, err := generateGRPC(ctx, pepper); !errors.Is(err, errNoGRPC) {
		return result, err
	}
	response, err := persistentWorkers.roundTrip(ctx, workerRequest{Op: "generate", Pepper: pepper})
	if err != nil {
		return generateResult{}, err
	}
	return generateResult{
		Token:        response.Token,
		LookupPrefix: response.LookupPrefix,
		Hash:         response.Hash,
	}, nil
}

func Hash(ctx context.Context, token, pepper string) (string, string, error) {
	if prefix, hash, err := hashGRPC(ctx, token, pepper); !errors.Is(err, errNoGRPC) {
		return prefix, hash, err
	}
	response, err := persistentWorkers.roundTrip(ctx, workerRequest{Op: "hash", Token: token, Pepper: pepper})
	if err != nil {
		return "", "", err
	}
	return response.LookupPrefix, response.Hash, nil
}

func Verify(ctx context.Context, token, hash, pepper string) (bool, error) {
	if valid, err := verifyGRPC(ctx, token, hash, pepper); !errors.Is(err, errNoGRPC) {
		return valid, err
	}
	response, err := persistentWorkers.roundTrip(ctx, workerRequest{Op: "verify", Token: token, Hash: hash, Pepper: pepper})
	if err != nil {
		return false, err
	}
	if response.Valid == nil {
		return false, errors.New("Rust apikeys worker returned no validity flag")
	}
	return *response.Valid, nil
}
