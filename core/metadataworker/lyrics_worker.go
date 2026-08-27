package metadataworker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/navidrome/navidrome/core/rustworker"
)

const (
	maxLyricsWorkers       = 2
	maxLyricsInputBytes    = 16 * 1024 * 1024
	maxLyricsResponseBytes = 16 * 1024 * 1024
)

type lyricsWorkerRequest struct {
	Suffix    string `json:"suffix"`
	Lang      string `json:"lang"`
	InputSize int    `json:"input_size"`
}

type lyricsWorkerResponse struct {
	OK         bool   `json:"ok"`
	LyricsJSON string `json:"lyrics_json,omitempty"`
	Error      string `json:"error,omitempty"`
}

type lyricsWorker struct {
	binary string
	pipes  *rustworker.Pipes
	writer *bufio.Writer
	reader *bufio.Reader
}

type lyricsWorkerSlot struct {
	worker *lyricsWorker
}

type lyricsWorkerPool struct {
	limit chan struct{}
	idle  chan *lyricsWorkerSlot
}

var persistentLyricsWorkers = newLyricsWorkerPool()

// PersistentLyricsWorkers returns the shared Rust lyrics parser pool.
func PersistentLyricsWorkers() *lyricsWorkerPool {
	return persistentLyricsWorkers
}

func newLyricsWorkerPool() *lyricsWorkerPool {
	size := min(max(runtime.GOMAXPROCS(0)/2, 1), maxLyricsWorkers)
	return &lyricsWorkerPool{
		limit: make(chan struct{}, size),
		idle:  make(chan *lyricsWorkerSlot, size),
	}
}

func (p *lyricsWorkerPool) Parse(ctx context.Context, suffix, lang string, contents []byte) (string, error) {
	return p.parse(ctx, suffix, lang, contents)
}

func (p *lyricsWorkerPool) parse(ctx context.Context, suffix, lang string, contents []byte) (string, error) {
	if len(contents) == 0 {
		return "[]", nil
	}
	if len(contents) > maxLyricsInputBytes {
		return "", fmt.Errorf("lyrics payload exceeds maximum size of %d bytes", maxLyricsInputBytes)
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

	var slot *lyricsWorkerSlot
	select {
	case slot = <-p.idle:
	default:
		slot = &lyricsWorkerSlot{}
	}
	defer func() { p.idle <- slot }()

	var lyricsJSON string
	err = rustworker.Run(ctx, rustworker.DefaultRestartAttempts, func() { slot.stop() }, func() error {
		worker, ensureErr := slot.ensure(binary)
		if ensureErr != nil {
			return ensureErr
		}
		var roundErr error
		lyricsJSON, roundErr = worker.roundTrip(suffix, lang, contents)
		return roundErr
	})
	if err != nil {
		return "", rustworker.FailAfterRestarts("lyrics", err)
	}
	return lyricsJSON, nil
}

func (s *lyricsWorkerSlot) ensure(binary string) (*lyricsWorker, error) {
	if s.worker != nil && s.worker.binary == binary {
		return s.worker, nil
	}
	s.stop()
	worker, err := startLyricsWorker(binary)
	if err != nil {
		return nil, err
	}
	s.worker = worker
	return worker, nil
}

func (s *lyricsWorkerSlot) stop() {
	if s.worker == nil {
		return
	}
	s.worker.close()
	s.worker = nil
}

func startLyricsWorker(binary string) (*lyricsWorker, error) {
	pipes, err := rustworker.Start(binary, "--parse-lyrics-worker")
	if err != nil {
		return nil, err
	}
	return &lyricsWorker{
		binary: binary,
		pipes:  pipes,
		writer: bufio.NewWriterSize(pipes.Stdin, rustworker.DefaultWriteBuf),
		reader: bufio.NewReaderSize(pipes.Stdout, rustworker.DefaultReadBuf),
	}, nil
}

func (w *lyricsWorker) roundTrip(suffix, lang string, contents []byte) (string, error) {
	request := lyricsWorkerRequest{
		Suffix:    suffix,
		Lang:      lang,
		InputSize: len(contents),
	}
	if err := rustworker.WriteHeaderAndBodies(w.writer, request, contents); err != nil {
		return "", err
	}

	var response lyricsWorkerResponse
	if err := rustworker.ReadJSONLine(w.reader, &response); err != nil {
		return "", err
	}
	if len(response.LyricsJSON) > maxLyricsResponseBytes {
		return "", fmt.Errorf("lyrics response exceeds maximum size of %d bytes", maxLyricsResponseBytes)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Rust lyrics worker could not parse lyrics"
		}
		return "", errors.New(response.Error)
	}
	if response.LyricsJSON == "" {
		return "[]", nil
	}
	return response.LyricsJSON, nil
}

func (w *lyricsWorker) close() {
	rustworker.Close(w.pipes)
}

var testBinaryOnce sync.Once

// EnsureTestBinary builds or locates navidrome-metadata for Go tests when
// ND_METADATAWORKERPATH is unset.
func EnsureTestBinary() error {
	var setupErr error
	testBinaryOnce.Do(func() {
		if configured := strings.TrimSpace(os.Getenv(EnvPath)); configured != "" {
			if _, err := rustworker.ResolveBinary(configured, BinaryName()); err != nil {
				setupErr = err
			}
			return
		}
		root, err := repoRoot()
		if err != nil {
			setupErr = err
			return
		}
		candidate := filepath.Join(root, "rust", "metadata", "target", "release", BinaryName())
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			_ = os.Setenv(EnvPath, candidate)
			return
		}
		cmd := exec.Command("cargo", "+1.97.0", "build", "--release", "--locked")
		cmd.Dir = filepath.Join(root, "rust", "metadata")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			setupErr = fmt.Errorf("building navidrome-metadata for tests: %w", err)
			return
		}
		_ = os.Setenv(EnvPath, candidate)
	})
	return setupErr
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not locate repository root")
		}
		dir = parent
	}
}
