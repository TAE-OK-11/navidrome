package scannerworker

import (
	"bufio"
	"context"
	"errors"
	"runtime"

	"github.com/navidrome/navidrome/core/rustworker"
)

const maxFolderHashWorkers = 8

type FolderHashFile struct {
	Name      string `json:"name,omitempty"`
	Size      uint64 `json:"size"`
	ModTimeNS int64  `json:"mod_time_ns"`
}

// FolderHashRequest mirrors the Rust FolderHashInput payload.
type FolderHashRequest struct {
	Path              string                    `json:"path"`
	ModTimeNS         int64                     `json:"mod_time_ns"`
	ImagesUpdatedAtNS int64                     `json:"images_updated_at_ns"`
	NumPlaylists      int                         `json:"num_playlists"`
	NumSubfolders     int                         `json:"num_subfolders"`
	AudioFiles        map[string]FolderHashFile `json:"audio_files"`
	ImageFiles        map[string]FolderHashFile `json:"image_files"`
}

type folderHashResponse struct {
	OK    bool   `json:"ok"`
	Hash  string `json:"hash,omitempty"`
	Error string `json:"error,omitempty"`
}

type folderHashWorker struct {
	binary string
	pipes  *rustworker.Pipes
	writer *bufio.Writer
	reader *bufio.Reader
}

type folderHashWorkerSlot struct {
	worker *folderHashWorker
}

type folderHashWorkerPool struct {
	limit chan struct{}
	idle  chan *folderHashWorkerSlot
}

var persistentFolderHashWorkers = newFolderHashWorkerPool()

func newFolderHashWorkerPool() *folderHashWorkerPool {
	size := min(max(runtime.GOMAXPROCS(0), 1), maxFolderHashWorkers)
	return &folderHashWorkerPool{
		limit: make(chan struct{}, size),
		idle:  make(chan *folderHashWorkerSlot, size),
	}
}

// PersistentFolderHashWorkers returns the shared Rust folder-hash pool.
func PersistentFolderHashWorkers() *folderHashWorkerPool {
	return persistentFolderHashWorkers
}

func (p *folderHashWorkerPool) Hash(ctx context.Context, request FolderHashRequest) (string, error) {
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

	var slot *folderHashWorkerSlot
	select {
	case slot = <-p.idle:
	default:
		slot = &folderHashWorkerSlot{}
	}
	defer func() { p.idle <- slot }()

	var hash string
	err = rustworker.Run(ctx, rustworker.DefaultRestartAttempts, func() { slot.stop() }, func() error {
		worker, ensureErr := slot.ensure(binary)
		if ensureErr != nil {
			return ensureErr
		}
		var roundErr error
		hash, roundErr = worker.roundTrip(request)
		return roundErr
	})
	if err != nil {
		return "", rustworker.FailAfterRestarts("folder-hash", err)
	}
	return hash, nil
}

func (s *folderHashWorkerSlot) ensure(binary string) (*folderHashWorker, error) {
	if s.worker != nil && s.worker.binary == binary {
		return s.worker, nil
	}
	s.stop()
	worker, err := startFolderHashWorker(binary)
	if err != nil {
		return nil, err
	}
	s.worker = worker
	return worker, nil
}

func (s *folderHashWorkerSlot) stop() {
	if s.worker == nil {
		return
	}
	s.worker.close()
	s.worker = nil
}

func startFolderHashWorker(binary string) (*folderHashWorker, error) {
	pipes, err := rustworker.Start(binary, "--folder-hash-worker")
	if err != nil {
		return nil, err
	}
	return &folderHashWorker{
		binary: binary,
		pipes:  pipes,
		writer: bufio.NewWriterSize(pipes.Stdin, rustworker.DefaultWriteBuf),
		reader: bufio.NewReaderSize(pipes.Stdout, rustworker.DefaultReadBuf),
	}, nil
}

func (w *folderHashWorker) roundTrip(request FolderHashRequest) (string, error) {
	if err := rustworker.WriteJSONLine(w.writer, request); err != nil {
		return "", err
	}
	var response folderHashResponse
	if err := rustworker.ReadJSONLine(w.reader, &response); err != nil {
		return "", err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Rust folder hash worker failed"
		}
		return "", errors.New(response.Error)
	}
	return response.Hash, nil
}

func (w *folderHashWorker) close() {
	rustworker.Close(w.pipes)
}
