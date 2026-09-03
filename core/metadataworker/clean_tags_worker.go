package metadataworker

import (
	"bufio"
	"context"
	"errors"
	"runtime"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/rustworker"
)

const maxCleanTagsWorkers = 8

type cleanTagsRequest struct {
	Path                  string                      `json:"path"`
	Tags                  map[string][]string         `json:"tags"`
	Mappings              map[string]TagMappingExport `json:"mappings"`
	ArtistSplitExceptions []string                    `json:"artist_split_exceptions,omitempty"`
}

type cleanTagsResponse struct {
	OK    bool                `json:"ok"`
	Tags  map[string][]string `json:"tags,omitempty"`
	Error string              `json:"error,omitempty"`
}

type cleanTagsWorker struct {
	binary string
	pipes  *rustworker.Pipes
	writer *bufio.Writer
	reader *bufio.Reader
}

type cleanTagsWorkerSlot struct {
	worker *cleanTagsWorker
}

type cleanTagsWorkerPool struct {
	limit chan struct{}
	idle  chan *cleanTagsWorkerSlot
}

var persistentCleanTagsWorkers = newCleanTagsWorkerPool()

func newCleanTagsWorkerPool() *cleanTagsWorkerPool {
	size := min(max(runtime.GOMAXPROCS(0), 1), maxCleanTagsWorkers)
	return &cleanTagsWorkerPool{
		limit: make(chan struct{}, size),
		idle:  make(chan *cleanTagsWorkerSlot, size),
	}
}

// PersistentCleanTagsWorkers returns the shared Rust tag-clean pool.
func PersistentCleanTagsWorkers() *cleanTagsWorkerPool {
	return persistentCleanTagsWorkers
}

func (p *cleanTagsWorkerPool) Clean(ctx context.Context, filePath string, raw map[string][]string, mappings map[string]TagMappingExport) (map[string][]string, error) {
	if len(raw) == 0 {
		return map[string][]string{}, nil
	}
	if cleaned, err := cleanTagsGRPC(ctx, filePath, raw, mappings, confArtistSplitExceptions()); rustworker.PreferGRPC(err, errNoGRPC) {
		return cleaned, err
	}
	binary, err := Resolve()
	if err != nil {
		return nil, err
	}

	select {
	case p.limit <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-p.limit }()

	var slot *cleanTagsWorkerSlot
	select {
	case slot = <-p.idle:
	default:
		slot = &cleanTagsWorkerSlot{}
	}
	defer func() { p.idle <- slot }()

	rawCopy := make(map[string][]string, len(raw))
	for key, values := range raw {
		rawCopy[key] = append([]string(nil), values...)
	}
	request := cleanTagsRequest{
		Path:                  filePath,
		Tags:                  rawCopy,
		Mappings:              mappings,
		ArtistSplitExceptions: confArtistSplitExceptions(),
	}

	var cleaned map[string][]string
	err = rustworker.Run(ctx, rustworker.DefaultRestartAttempts, func() { slot.stop() }, func() error {
		worker, ensureErr := slot.ensure(binary)
		if ensureErr != nil {
			return ensureErr
		}
		var roundErr error
		cleaned, roundErr = worker.roundTrip(request)
		return roundErr
	})
	if err != nil {
		return nil, rustworker.FailAfterRestarts("clean-tags", err)
	}
	return cleaned, nil
}

func confArtistSplitExceptions() []string {
	return append([]string(nil), conf.Server.Scanner.ArtistSplitExceptions...)
}

func (s *cleanTagsWorkerSlot) ensure(binary string) (*cleanTagsWorker, error) {
	if s.worker != nil && s.worker.binary == binary {
		return s.worker, nil
	}
	s.stop()
	worker, err := startCleanTagsWorker(binary)
	if err != nil {
		return nil, err
	}
	s.worker = worker
	return worker, nil
}

func (s *cleanTagsWorkerSlot) stop() {
	if s.worker == nil {
		return
	}
	s.worker.close()
	s.worker = nil
}

func startCleanTagsWorker(binary string) (*cleanTagsWorker, error) {
	pipes, err := rustworker.Start(binary, "--clean-tags-worker")
	if err != nil {
		return nil, err
	}
	return &cleanTagsWorker{
		binary: binary,
		pipes:  pipes,
		writer: bufio.NewWriterSize(pipes.Stdin, rustworker.DefaultWriteBuf),
		reader: bufio.NewReaderSize(pipes.Stdout, rustworker.DefaultReadBuf),
	}, nil
}

func (w *cleanTagsWorker) roundTrip(request cleanTagsRequest) (map[string][]string, error) {
	if err := rustworker.WriteJSONLine(w.writer, request); err != nil {
		return nil, err
	}
	var response cleanTagsResponse
	if err := rustworker.ReadJSONLine(w.reader, &response); err != nil {
		return nil, err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Rust clean tags worker failed"
		}
		return nil, errors.New(response.Error)
	}
	out := make(map[string][]string, len(response.Tags))
	for key, values := range response.Tags {
		out[key] = append([]string(nil), values...)
	}
	return out, nil
}

func (w *cleanTagsWorker) close() {
	rustworker.Close(w.pipes)
}
