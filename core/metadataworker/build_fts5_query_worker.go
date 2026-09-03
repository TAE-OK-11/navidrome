package metadataworker

import (
	"bufio"
	"context"
	"errors"
	"runtime"

	"github.com/navidrome/navidrome/core/rustworker"
)

const maxBuildFTS5QueryWorkers = 8

type buildFTS5QueryRequest struct {
	Query string `json:"query"`
}

type buildFTS5QueryResponse struct {
	OK       bool   `json:"ok"`
	Query    string `json:"query,omitempty"`
	Degraded bool   `json:"degraded,omitempty"`
	Error    string `json:"error,omitempty"`
}

type buildFTS5QueryResult struct {
	Query    string
	Degraded bool
}

type buildFTS5QueryWorker struct {
	binary string
	pipes  *rustworker.Pipes
	writer *bufio.Writer
	reader *bufio.Reader
}

type buildFTS5QueryWorkerSlot struct {
	worker *buildFTS5QueryWorker
}

type buildFTS5QueryWorkerPool struct {
	limit chan struct{}
	idle  chan *buildFTS5QueryWorkerSlot
}

var persistentBuildFTS5QueryWorkers = newBuildFTS5QueryWorkerPool()

func newBuildFTS5QueryWorkerPool() *buildFTS5QueryWorkerPool {
	size := min(max(runtime.GOMAXPROCS(0), 1), maxBuildFTS5QueryWorkers)
	return &buildFTS5QueryWorkerPool{
		limit: make(chan struct{}, size),
		idle:  make(chan *buildFTS5QueryWorkerSlot, size),
	}
}

// PersistentBuildFTS5QueryWorkers returns the shared Rust FTS5 query builder pool.
func PersistentBuildFTS5QueryWorkers() *buildFTS5QueryWorkerPool {
	return persistentBuildFTS5QueryWorkers
}

func (p *buildFTS5QueryWorkerPool) Build(ctx context.Context, query string) (buildFTS5QueryResult, error) {
	if result, err := buildFTS5QueryGRPC(ctx, query); !errors.Is(err, errNoGRPC) {
		return result, err
	}
	binary, err := Resolve()
	if err != nil {
		return buildFTS5QueryResult{}, err
	}

	select {
	case p.limit <- struct{}{}:
	case <-ctx.Done():
		return buildFTS5QueryResult{}, ctx.Err()
	}
	defer func() { <-p.limit }()

	var slot *buildFTS5QueryWorkerSlot
	select {
	case slot = <-p.idle:
	default:
		slot = &buildFTS5QueryWorkerSlot{}
	}
	defer func() { p.idle <- slot }()

	var result buildFTS5QueryResult
	err = rustworker.Run(ctx, rustworker.DefaultRestartAttempts, func() { slot.stop() }, func() error {
		worker, ensureErr := slot.ensure(binary)
		if ensureErr != nil {
			return ensureErr
		}
		var roundErr error
		result, roundErr = worker.roundTrip(query)
		return roundErr
	})
	if err != nil {
		return buildFTS5QueryResult{}, rustworker.FailAfterRestarts("build-fts5-query", err)
	}
	return result, nil
}

func (s *buildFTS5QueryWorkerSlot) ensure(binary string) (*buildFTS5QueryWorker, error) {
	if s.worker != nil && s.worker.binary == binary {
		return s.worker, nil
	}
	s.stop()
	worker, err := startBuildFTS5QueryWorker(binary)
	if err != nil {
		return nil, err
	}
	s.worker = worker
	return worker, nil
}

func (s *buildFTS5QueryWorkerSlot) stop() {
	if s.worker == nil {
		return
	}
	s.worker.close()
	s.worker = nil
}

func startBuildFTS5QueryWorker(binary string) (*buildFTS5QueryWorker, error) {
	pipes, err := rustworker.Start(binary, "--build-fts5-query-worker")
	if err != nil {
		return nil, err
	}
	return &buildFTS5QueryWorker{
		binary: binary,
		pipes:  pipes,
		writer: bufio.NewWriterSize(pipes.Stdin, rustworker.DefaultWriteBuf),
		reader: bufio.NewReaderSize(pipes.Stdout, rustworker.DefaultReadBuf),
	}, nil
}

func (w *buildFTS5QueryWorker) roundTrip(query string) (buildFTS5QueryResult, error) {
	if err := rustworker.WriteJSONLine(w.writer, buildFTS5QueryRequest{Query: query}); err != nil {
		return buildFTS5QueryResult{}, err
	}
	var response buildFTS5QueryResponse
	if err := rustworker.ReadJSONLine(w.reader, &response); err != nil {
		return buildFTS5QueryResult{}, err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Rust build FTS5 query worker failed"
		}
		return buildFTS5QueryResult{}, errors.New(response.Error)
	}
	return buildFTS5QueryResult{Query: response.Query, Degraded: response.Degraded}, nil
}

func (w *buildFTS5QueryWorker) close() {
	rustworker.Close(w.pipes)
}
