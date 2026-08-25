// Package lofty provides the local metadata extractor backed by the Rust Lofty worker.
//
// The worker is kept alive for the lifetime of the extractor and receives scanner
// batches over a buffered NDJSON stream. This keeps the Go/Rust boundary off the
// per-file hot path while avoiding CGO/FFI lifetime and panic hazards.
package lofty

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/core/storage/local"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model/metadata"
)

const (
	protocolVersion       = 1
	loftyVersion          = "0.25.1"
	maxWorkerPool         = 16
	minFilesPerWorkerTask = 32
)

type request struct {
	Files []inputFile `json:"files"`
}

type inputFile struct {
	Key  string `json:"key"`
	Path string `json:"path"`
}

type response struct {
	Protocol int                  `json:"protocol"`
	Lofty    string               `json:"lofty"`
	Results  map[string]rawResult `json:"results"`
	Errors   map[string]string    `json:"errors"`
}

type rawResult struct {
	Tags       map[string][]string `json:"tags"`
	DurationNS uint64              `json:"duration_ns"`
	BitRate    uint32              `json:"bit_rate"`
	BitDepth   uint8               `json:"bit_depth"`
	SampleRate uint32              `json:"sample_rate"`
	Channels   uint8               `json:"channels"`
	Codec      string              `json:"codec"`
	HasPicture bool                `json:"has_picture"`
}

type worker struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writer  *bufio.Writer
	decoder *json.Decoder
}

type workerSlot struct {
	worker *worker
}

type extractor struct {
	baseDir  string
	poolOnce sync.Once
	pool     chan *workerSlot
}

func (e *extractor) Parse(files ...string) (map[string]metadata.Info, error) {
	return e.ParseContext(context.Background(), files...)
}

func (e *extractor) ParseContext(ctx context.Context, files ...string) (map[string]metadata.Info, error) {
	if len(files) == 0 {
		return map[string]metadata.Info{}, nil
	}

	req, err := e.buildRequest(files)
	if err != nil {
		return nil, err
	}

	pool := e.workerPool()
	taskCount := metadataTaskCount(len(req.Files), cap(pool))
	if taskCount == 1 {
		resp, err := e.roundTrip(ctx, pool, req)
		if err != nil {
			return nil, err
		}
		return convertResponse(resp)
	}

	// Folder-level scanner concurrency normally keeps the Rust workers busy, but
	// a library with many files in one folder previously occupied only one slot.
	// Split large batches across the same bounded persistent pool. This improves
	// that worst case without nesting Rust thread pools or creating more worker
	// processes than DevScannerThreads allows.
	type taskResult struct {
		response response
		err      error
	}
	results := make(chan taskResult, taskCount)
	chunkSize := (len(req.Files) + taskCount - 1) / taskCount
	for start := 0; start < len(req.Files); start += chunkSize {
		end := min(start+chunkSize, len(req.Files))
		task := request{Files: req.Files[start:end]}
		go func() {
			resp, err := e.roundTrip(ctx, pool, task)
			results <- taskResult{response: resp, err: err}
		}()
	}

	merged := response{
		Protocol: protocolVersion,
		Lofty:    loftyVersion,
		Results:  make(map[string]rawResult, len(req.Files)),
		Errors:   make(map[string]string),
	}
	var taskErrors []error
	for range taskCount {
		result := <-results
		if result.err != nil {
			taskErrors = append(taskErrors, result.err)
			continue
		}
		for key, value := range result.response.Results {
			merged.Results[key] = value
		}
		for key, value := range result.response.Errors {
			merged.Errors[key] = value
		}
	}
	if len(taskErrors) > 0 {
		return nil, fmt.Errorf("Lofty metadata tasks failed: %w", errors.Join(taskErrors...))
	}
	return convertResponse(merged)
}

func metadataTaskCount(fileCount, poolSize int) int {
	if fileCount <= 0 || poolSize <= 1 {
		return 1
	}
	return min(poolSize, max(1, (fileCount+minFilesPerWorkerTask-1)/minFilesPerWorkerTask))
}

// roundTrip reserves one persistent worker for one ordered request/response.
// A failed worker is replaced once before the request is returned to the scanner.
func (e *extractor) roundTrip(ctx context.Context, pool chan *workerSlot, req request) (response, error) {
	var slot *workerSlot
	select {
	case slot = <-pool:
	case <-ctx.Done():
		return response{}, ctx.Err()
	}
	defer func() { pool <- slot }()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		w, err := slot.ensureWorker()
		if err != nil {
			return response{}, err
		}
		cancelDone := make(chan struct{})
		stopCancel := context.AfterFunc(ctx, func() {
			w.kill()
			close(cancelDone)
		})
		resp, err := w.roundTrip(req)
		if !stopCancel() {
			<-cancelDone
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			slot.stopWorker()
			return response{}, ctxErr
		}
		if err == nil {
			return resp, nil
		}
		lastErr = err
		log.Warn("Lofty metadata worker request failed; restarting", "attempt", attempt+1, "error", err)
		slot.stopWorker()
	}
	return response{}, fmt.Errorf("Lofty metadata worker failed after restart: %w", lastErr)
}

func (e *extractor) Version() string {
	return loftyVersion
}

func (e *extractor) buildRequest(files []string) (request, error) {
	base, err := filepath.Abs(e.baseDir)
	if err != nil {
		return request{}, fmt.Errorf("resolving music root: %w", err)
	}

	inputs := make([]inputFile, 0, len(files))
	for _, key := range files {
		if key == "" {
			continue
		}
		candidate := filepath.Clean(filepath.Join(base, filepath.FromSlash(key)))
		rel, err := filepath.Rel(base, candidate)
		if err != nil {
			return request{}, fmt.Errorf("resolving metadata path %q: %w", key, err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return request{}, fmt.Errorf("metadata path escapes music root: %q", key)
		}
		inputs = append(inputs, inputFile{Key: key, Path: candidate})
	}
	return request{Files: inputs}, nil
}

func (e *extractor) workerPool() chan *workerSlot {
	e.poolOnce.Do(func() {
		e.pool = make(chan *workerSlot, workerPoolSize(conf.Server.DevScannerThreads))
		for range cap(e.pool) {
			e.pool <- &workerSlot{}
		}
	})
	return e.pool
}

func workerPoolSize(scannerThreads uint) int {
	if scannerThreads < 1 {
		return 1
	}
	return int(min(scannerThreads, maxWorkerPool))
}

func (s *workerSlot) ensureWorker() (*worker, error) {
	if s.worker != nil {
		return s.worker, nil
	}
	w, err := startWorker(resolveWorkerPath())
	if err != nil {
		return nil, err
	}
	s.worker = w
	return w, nil
}

func (s *workerSlot) stopWorker() {
	if s.worker == nil {
		return
	}
	s.worker.close()
	s.worker = nil
}

func startWorker(binary string) (*worker, error) {
	cmd := exec.Command(binary) //nolint:gosec // binary path is administrator-controlled or colocated with Navidrome
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening Lofty worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("opening Lofty worker stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("starting Lofty metadata worker %q: %w", binary, err)
	}
	return &worker{
		cmd:     cmd,
		stdin:   stdin,
		writer:  bufio.NewWriterSize(stdin, 256*1024),
		decoder: json.NewDecoder(bufio.NewReaderSize(stdout, 256*1024)),
	}, nil
}

func (w *worker) roundTrip(req request) (response, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return response{}, fmt.Errorf("encoding Lofty request: %w", err)
	}
	if _, err := w.writer.Write(payload); err != nil {
		return response{}, fmt.Errorf("writing Lofty request: %w", err)
	}
	if err := w.writer.WriteByte('\n'); err != nil {
		return response{}, fmt.Errorf("framing Lofty request: %w", err)
	}
	if err := w.writer.Flush(); err != nil {
		return response{}, fmt.Errorf("flushing Lofty request: %w", err)
	}

	var resp response
	if err := w.decoder.Decode(&resp); err != nil {
		return response{}, fmt.Errorf("reading Lofty response: %w", err)
	}
	if resp.Protocol != protocolVersion {
		return response{}, fmt.Errorf("unsupported Lofty protocol %d", resp.Protocol)
	}
	return resp, nil
}

func (w *worker) close() {
	_ = w.stdin.Close()
	w.kill()
	_ = w.cmd.Wait()
}

func (w *worker) kill() {
	if w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
}

func convertResponse(resp response) (map[string]metadata.Info, error) {
	if requestErr := resp.Errors["$request"]; requestErr != "" {
		return nil, errors.New(requestErr)
	}
	results := make(map[string]metadata.Info, len(resp.Results))
	for key, value := range resp.Results {
		results[key] = metadata.Info{
			Tags: value.Tags,
			AudioProperties: metadata.AudioProperties{
				Duration:   time.Duration(value.DurationNS),
				BitRate:    int(value.BitRate),
				BitDepth:   int(value.BitDepth),
				SampleRate: int(value.SampleRate),
				Channels:   int(value.Channels),
				Codec:      value.Codec,
			},
			HasPicture: value.HasPicture,
		}
	}
	for key, workerErr := range resp.Errors {
		if key == "$request" {
			continue
		}
		log.Warn("Lofty could not read metadata; skipping file", "filePath", key, "error", workerErr)
	}
	return results, nil
}

func resolveWorkerPath() string {
	if workerPath, err := metadataworker.Resolve(); err == nil {
		return workerPath
	}
	return metadataworker.BinaryName()
}

var _ local.Extractor = (*extractor)(nil)

func init() {
	local.RegisterExtractor("lofty", func(_ fs.FS, baseDir string) local.Extractor {
		return &extractor{baseDir: baseDir}
	})
	conf.AddHook(func() {
		log.Debug("Lofty metadata extractor", "version", loftyVersion, "worker", resolveWorkerPath())
	})
}
