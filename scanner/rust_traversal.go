package scanner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/rustworker"
	"github.com/navidrome/navidrome/core/scannerworker"
	"github.com/navidrome/navidrome/log"
)

const (
	maxRustScanEntries = 10_000_000
	maxScannerWorkers  = 16
)

type rustScanRequest struct {
	Root             string            `json:"root"`
	Targets          []string          `json:"targets"`
	FollowSymlinks   bool              `json:"follow_symlinks"`
	IgnoreDotFolders bool              `json:"ignore_dot_folders"`
	KnownHashes      map[string]string `json:"known_hashes,omitempty"`
	WalkThreads      int               `json:"walk_threads,omitempty"`
}

type rustScanEvent struct {
	Kind    string          `json:"kind"`
	Folder  *rustScanFolder `json:"folder"`
	Message string          `json:"message"`
}

type rustScanFolder struct {
	Path              string                  `json:"path"`
	ModTimeNS         int64                   `json:"mod_time_ns"`
	ImagesUpdatedAtNS int64                   `json:"images_updated_at_ns"`
	NumPlaylists      int                     `json:"num_playlists"`
	NumSubfolders     int                     `json:"num_subfolders"`
	AudioFiles        map[string]rustScanFile `json:"audio_files"`
	ImageFiles        map[string]rustScanFile `json:"image_files"`
	Hash              string                  `json:"hash,omitempty"`
}

type rustScanFile struct {
	Name      string `json:"name"`
	Size      uint64 `json:"size"`
	ModTimeNS int64  `json:"mod_time_ns"`
}

type scannerWorker struct {
	binary  string
	pipes   *rustworker.Pipes
	writer  *bufio.Writer
	encoder *json.Encoder
	decoder *json.Decoder
}

type scannerWorkerSlot struct {
	worker *scannerWorker
}

type scannerWorkerPool struct {
	once  sync.Once
	slots chan *scannerWorkerSlot
}

var persistentScannerWorkers scannerWorkerPool // shared across library walks

func (p *scannerWorkerPool) ensure() {
	p.once.Do(func() {
		size := scannerWorkerPoolSize()
		p.slots = make(chan *scannerWorkerSlot, size)
		for range size {
			p.slots <- &scannerWorkerSlot{}
		}
	})
}

func scannerWorkerPoolSize() int {
	threads := conf.Server.DevScannerThreads
	if threads < 1 {
		threads = uint(runtime.GOMAXPROCS(0))
	}
	return int(min(threads, maxScannerWorkers))
}

func streamRustFolders(ctx context.Context, job *scanJob, targets []string) (<-chan *rustScanFolder, <-chan error) {
	folders := make(chan *rustScanFolder, 64)
	errs := make(chan error, 1)

	go func() {
		defer close(folders)
		defer close(errs)

		binary, err := scannerworker.Resolve()
		if err != nil {
			log.Error(ctx, "Rust scanner worker binary not found",
				"lib", job.lib.Name, "root", job.localRoot, err)
			errs <- err
			return
		}
		request := rustScanRequest{
			Root:             job.localRoot,
			Targets:          append([]string{}, targets...),
			FollowSymlinks:   conf.Server.Scanner.FollowSymlinks,
			IgnoreDotFolders: conf.Server.Scanner.IgnoreDotFolders,
			KnownHashes:      job.knownHashesSnapshot(),
			WalkThreads:      rustWalkThreads(),
		}

		persistentScannerWorkers.ensure()
		select {
		case <-ctx.Done():
			errs <- ctx.Err()
			return
		case slot := <-persistentScannerWorkers.slots:
			defer func() { persistentScannerWorkers.slots <- slot }()
			var lastErr error
			for attempt := 0; attempt < 2; attempt++ {
				warn, err := slot.stream(ctx, binary, request, folders)
				for _, warning := range warn {
					log.Warn(ctx, "Rust scanner traversal warning", "warning", warning)
				}
				if err == nil {
					errs <- nil
					return
				}
				if ctx.Err() != nil {
					errs <- ctx.Err()
					return
				}
				lastErr = err
				log.Warn(ctx, "Rust scanner worker request failed; restarting",
					"attempt", attempt+1, "binary", binary, "lib", job.lib.Name, "root", request.Root, err)
				slot.stop()
			}
			log.Error(ctx, "Rust scanner worker failed after restart",
				"binary", binary, "lib", job.lib.Name, "root", request.Root, "targets", targets, lastErr)
			errs <- fmt.Errorf("persistent Rust scanner failed after restart: %w", lastErr)
		}
	}()

	return folders, errs
}

func rustWalkThreads() int {
	// Rust streams folders in post-order when walk_threads <= 1. Folder processing
	// stays parallel on the Go side via DevScannerThreads; a parallel Rust walk
	// would buffer the entire tree before the first folder reaches Go.
	return 1
}

func (j *scanJob) knownHashesSnapshot() map[string]string {
	j.lock.Lock()
	defer j.lock.Unlock()
	if len(j.knownHashes) == 0 {
		return nil
	}
	out := make(map[string]string, len(j.knownHashes))
	for path, hash := range j.knownHashes {
		if hash != "" {
			out[path] = hash
		}
	}
	return out
}

func (s *scannerWorkerSlot) stream(
	ctx context.Context,
	binary string,
	request rustScanRequest,
	folders chan<- *rustScanFolder,
) ([]string, error) {
	worker, err := s.ensure(binary)
	if err != nil {
		log.Error(ctx, "Rust scanner worker failed to start", "binary", binary, err)
		return nil, err
	}
	cancelDone := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		worker.kill()
		close(cancelDone)
	})
	pendingWarnings, err := worker.stream(request, folders)
	if !stopCancel() {
		<-cancelDone
	}
	if ctx.Err() != nil {
		s.stop()
		return nil, ctx.Err()
	}
	if err != nil {
		s.stop()
		return pendingWarnings, err
	}
	return pendingWarnings, nil
}

func (s *scannerWorkerSlot) ensure(binary string) (*scannerWorker, error) {
	if s.worker != nil && s.worker.binary == binary {
		return s.worker, nil
	}
	s.stop()
	worker, err := startScannerWorker(binary)
	if err != nil {
		return nil, err
	}
	s.worker = worker
	return worker, nil
}

func (s *scannerWorkerSlot) stop() {
	if s.worker == nil {
		return
	}
	s.worker.close()
	s.worker = nil
}

func startScannerWorker(binary string) (*scannerWorker, error) {
	pipes, err := rustworker.Start(binary)
	if err != nil {
		return nil, err
	}
	writer := bufio.NewWriterSize(pipes.Stdin, rustworker.DefaultWriteBuf)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return &scannerWorker{
		binary:  binary,
		pipes:   pipes,
		writer:  writer,
		encoder: encoder,
		decoder: json.NewDecoder(bufio.NewReaderSize(pipes.Stdout, 256*1024)),
	}, nil
}

func (w *scannerWorker) stream(request rustScanRequest, folders chan<- *rustScanFolder) ([]string, error) {
	if err := w.encoder.Encode(request); err != nil {
		return nil, fmt.Errorf("writing Rust scanner request: %w", err)
	}
	if err := w.writer.Flush(); err != nil {
		return nil, fmt.Errorf("flushing Rust scanner request: %w", err)
	}

	var warnings []string
	var seen int
	for {
		var event rustScanEvent
		if err := w.decoder.Decode(&event); errors.Is(err, io.EOF) {
			return warnings, errors.New("Rust scanner ended without completion marker")
		} else if err != nil {
			return warnings, fmt.Errorf("decoding Rust scanner response: %w", err)
		}
		switch event.Kind {
		case "folder":
			if event.Folder == nil || event.Folder.Path == "" {
				return warnings, errors.New("Rust scanner returned an invalid folder event")
			}
			seen++
			if seen > maxRustScanEntries {
				return warnings, errors.New("Rust scanner exceeded folder safety limit")
			}
			folders <- event.Folder
		case "folder_summary":
			if event.Folder == nil || event.Folder.Path == "" || event.Folder.Hash == "" {
				return warnings, errors.New("Rust scanner returned an invalid folder summary event")
			}
			seen++
			if seen > maxRustScanEntries {
				return warnings, errors.New("Rust scanner exceeded folder safety limit")
			}
			folders <- &rustScanFolder{
				Path: event.Folder.Path,
				Hash: event.Folder.Hash,
			}
		case "warning":
			if event.Message != "" {
				warnings = append(warnings, event.Message)
			}
		case "error":
			if event.Message == "" {
				event.Message = "Rust scanner request failed"
			}
			return warnings, errors.New(event.Message)
		case "done":
			return warnings, nil
		default:
			return warnings, fmt.Errorf("Rust scanner returned unknown event %q", event.Kind)
		}
	}
}

func (w *scannerWorker) kill() {
	rustworker.Kill(w.pipes.Cmd)
}

func (w *scannerWorker) close() {
	rustworker.Close(w.pipes)
}

func validateRustFolder(folder *rustScanFolder) error {
	for name, file := range folder.AudioFiles {
		if file.Size > math.MaxInt64 {
			return fmt.Errorf("audio file %q in %q exceeds supported size", name, folder.Path)
		}
	}
	for name, file := range folder.ImageFiles {
		if file.Size > math.MaxInt64 {
			return fmt.Errorf("image file %q in %q exceeds supported size", name, folder.Path)
		}
	}
	return nil
}

func folderEntryFromRust(job *scanJob, source *rustScanFolder) (*folderEntry, error) {
	if err := validateRustFolder(source); err != nil {
		return nil, err
	}
	entry := job.createFolderEntry(source.Path)
	entry.path = source.Path
	entry.modTime = unixNanoTime(source.ModTimeNS)
	entry.imagesUpdatedAt = unixNanoTime(source.ImagesUpdatedAtNS)
	entry.numPlaylists = source.NumPlaylists
	entry.numSubFolders = source.NumSubfolders
	entry.rustHash = source.Hash
	for name, file := range source.AudioFiles {
		dirEntry, err := rustDirEntryFromFile(name, file)
		if err != nil {
			return nil, err
		}
		entry.audioFiles[name] = dirEntry
		entry.fileInfos[name] = dirEntry.info
	}
	for name, file := range source.ImageFiles {
		dirEntry, err := rustDirEntryFromFile(name, file)
		if err != nil {
			return nil, err
		}
		entry.imageFiles[name] = dirEntry
		entry.fileInfos[name] = dirEntry.info
	}
	entry.elapsed.Start()
	return entry, nil
}

func rustDirEntryFromFile(name string, file rustScanFile) (*rustDirEntry, error) {
	if file.Size > math.MaxInt64 {
		return nil, fmt.Errorf("file %q exceeds supported size", name)
	}
	return &rustDirEntry{
		name: name,
		info: rustFileInfo{name: name, size: int64(file.Size), modTime: unixNanoTime(file.ModTimeNS)},
	}, nil
}

func unixNanoTime(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value)
}

type rustDirEntry struct {
	name string
	info rustFileInfo
}

func (e *rustDirEntry) Name() string               { return e.name }
func (e *rustDirEntry) IsDir() bool                { return false }
func (e *rustDirEntry) Type() fs.FileMode          { return 0 }
func (e *rustDirEntry) Info() (fs.FileInfo, error) { return e.info, nil }

type rustFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (i rustFileInfo) Name() string       { return i.name }
func (i rustFileInfo) Size() int64        { return i.size }
func (i rustFileInfo) Mode() fs.FileMode  { return 0 }
func (i rustFileInfo) ModTime() time.Time { return i.modTime }
func (i rustFileInfo) IsDir() bool        { return false }
func (i rustFileInfo) Sys() any           { return nil }
