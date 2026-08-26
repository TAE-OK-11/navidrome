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
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/scannerworker"
	"github.com/navidrome/navidrome/log"
)

const (
	maxRustScanEntries = 10_000_000
	maxScannerWorkers  = 4
)

type rustScanRequest struct {
	Root             string   `json:"root"`
	Targets          []string `json:"targets"`
	FollowSymlinks   bool     `json:"follow_symlinks"`
	IgnoreDotFolders bool     `json:"ignore_dot_folders"`
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
}

type rustScanFile struct {
	Name      string `json:"name"`
	Size      uint64 `json:"size"`
	ModTimeNS int64  `json:"mod_time_ns"`
}

type scannerWorker struct {
	binary  string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
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
		size := min(max(runtime.GOMAXPROCS(0)/2, 1), maxScannerWorkers)
		p.slots = make(chan *scannerWorkerSlot, size)
		for range size {
			p.slots <- &scannerWorkerSlot{}
		}
	})
}

func collectRustFolders(ctx context.Context, job *scanJob, targets []string) ([]rustScanFolder, []string, error) {
	binary, err := scannerworker.Resolve()
	if err != nil {
		return nil, nil, err
	}
	request := rustScanRequest{
		Root:             job.localRoot,
		Targets:          targets,
		FollowSymlinks:   conf.Server.Scanner.FollowSymlinks,
		IgnoreDotFolders: conf.Server.Scanner.IgnoreDotFolders,
	}

	persistentScannerWorkers.ensure()
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case slot := <-persistentScannerWorkers.slots:
		defer func() { persistentScannerWorkers.slots <- slot }()
		var lastErr error
		for attempt := 0; attempt < 2; attempt++ {
			folders, warnings, err := slot.roundTrip(ctx, binary, request)
			if err == nil {
				return folders, warnings, nil
			}
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			lastErr = err
			log.Debug(ctx, "Rust scanner worker request failed; restarting", "attempt", attempt+1, err)
			slot.stop()
		}
		return nil, nil, fmt.Errorf("persistent Rust scanner failed after restart: %w", lastErr)
	}
}

func (s *scannerWorkerSlot) roundTrip(ctx context.Context, binary string, request rustScanRequest) ([]rustScanFolder, []string, error) {
	worker, err := s.ensure(binary)
	if err != nil {
		return nil, nil, err
	}
	cancelDone := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		worker.kill()
		close(cancelDone)
	})
	folders, warnings, err := worker.roundTrip(request)
	if !stopCancel() {
		<-cancelDone
	}
	if ctx.Err() != nil {
		s.stop()
		return nil, nil, ctx.Err()
	}
	if err != nil {
		s.stop()
		return nil, nil, err
	}
	return folders, warnings, nil
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
	cmd := exec.Command(binary) //nolint:gosec // resolved administrator-controlled binary
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening Rust scanner stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("opening Rust scanner stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("starting Rust scanner %q: %w", binary, err)
	}
	writer := bufio.NewWriterSize(stdin, 64*1024)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return &scannerWorker{
		binary:  binary,
		cmd:     cmd,
		stdin:   stdin,
		writer:  writer,
		encoder: encoder,
		decoder: json.NewDecoder(bufio.NewReaderSize(stdout, 256*1024)),
	}, nil
}

func (w *scannerWorker) roundTrip(request rustScanRequest) ([]rustScanFolder, []string, error) {
	if err := w.encoder.Encode(request); err != nil {
		return nil, nil, fmt.Errorf("writing Rust scanner request: %w", err)
	}
	if err := w.writer.Flush(); err != nil {
		return nil, nil, fmt.Errorf("flushing Rust scanner request: %w", err)
	}

	folders := make([]rustScanFolder, 0, 1024)
	var warnings []string
	for {
		var event rustScanEvent
		if err := w.decoder.Decode(&event); errors.Is(err, io.EOF) {
			return nil, nil, errors.New("Rust scanner ended without completion marker")
		} else if err != nil {
			return nil, nil, fmt.Errorf("decoding Rust scanner response: %w", err)
		}
		switch event.Kind {
		case "folder":
			if event.Folder == nil || event.Folder.Path == "" {
				return nil, nil, errors.New("Rust scanner returned an invalid folder event")
			}
			folders = append(folders, *event.Folder)
			if len(folders) > maxRustScanEntries {
				return nil, nil, errors.New("Rust scanner exceeded folder safety limit")
			}
		case "warning":
			if event.Message != "" {
				warnings = append(warnings, event.Message)
			}
		case "error":
			if event.Message == "" {
				event.Message = "Rust scanner request failed"
			}
			return nil, nil, errors.New(event.Message)
		case "done":
			if err := validateRustFolders(folders); err != nil {
				return nil, nil, err
			}
			return folders, warnings, nil
		default:
			return nil, nil, fmt.Errorf("Rust scanner returned unknown event %q", event.Kind)
		}
	}
}

func (w *scannerWorker) kill() {
	if w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
}

func (w *scannerWorker) close() {
	_ = w.stdin.Close()
	w.kill()
	_ = w.cmd.Wait()
}

func validateRustFolders(folders []rustScanFolder) error {
	for _, folder := range folders {
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
	}
	return nil
}

func folderEntryFromRust(job *scanJob, source rustScanFolder) (*folderEntry, error) {
	entry := job.createFolderEntry(source.Path)
	entry.path = source.Path
	entry.modTime = unixNanoTime(source.ModTimeNS)
	entry.imagesUpdatedAt = unixNanoTime(source.ImagesUpdatedAtNS)
	entry.numPlaylists = source.NumPlaylists
	entry.numSubFolders = source.NumSubfolders
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
