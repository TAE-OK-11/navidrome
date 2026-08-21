// Package lofty provides the local metadata extractor backed by the Rust Lofty worker.
//
// The worker is kept alive for the lifetime of the extractor and receives scanner
// batches over a buffered NDJSON stream. This keeps the Go/Rust boundary off the
// per-file hot path while avoiding CGO/FFI lifetime and panic hazards.
package lofty

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/storage/local"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model/metadata"
)

const (
	protocolVersion = 1
	loftyVersion    = "0.25.0"
	workerEnv       = "ND_METADATAWORKERPATH"
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

type extractor struct {
	baseDir string
	mu      sync.Mutex
	worker  *worker
}

func (e *extractor) Parse(files ...string) (map[string]metadata.Info, error) {
	if len(files) == 0 {
		return map[string]metadata.Info{}, nil
	}

	req, err := e.buildRequest(files)
	if err != nil {
		return nil, err
	}

	// A worker carries a single ordered request/response stream. Serializing each
	// scanner batch keeps framing trivial and still amortizes process startup over
	// the complete scan. Scanner batching already keeps this boundary coarse.
	e.mu.Lock()
	defer e.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		w, err := e.ensureWorker()
		if err != nil {
			return nil, err
		}
		resp, err := w.roundTrip(req)
		if err == nil {
			return convertResponse(resp)
		}
		lastErr = err
		log.Warn("Lofty metadata worker request failed; restarting", "attempt", attempt+1, "error", err)
		e.stopWorker()
	}
	return nil, fmt.Errorf("Lofty metadata worker failed after restart: %w", lastErr)
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

func (e *extractor) ensureWorker() (*worker, error) {
	if e.worker != nil {
		return e.worker, nil
	}
	w, err := startWorker(resolveWorkerPath())
	if err != nil {
		return nil, err
	}
	e.worker = w
	return w, nil
}

func (e *extractor) stopWorker() {
	if e.worker == nil {
		return
	}
	e.worker.close()
	e.worker = nil
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
	if w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
	_ = w.cmd.Wait()
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
	if configured := strings.TrimSpace(os.Getenv(workerEnv)); configured != "" {
		return configured
	}
	name := "navidrome-metadata"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), name)
		if _, err := os.Stat(candidate); err == nil { //nolint:gosec // trusted installation directory
			return candidate
		}
	}
	return name
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
