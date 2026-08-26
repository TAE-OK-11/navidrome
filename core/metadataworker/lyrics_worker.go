package metadataworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	maxLyricsWorkers        = 2
	maxLyricsInputBytes      = 16 * 1024 * 1024
	maxLyricsResponseBytes   = 16 * 1024 * 1024
	lyricsWorkerReadBufBytes = 64 * 1024
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
	cmd    *exec.Cmd
	stdin  io.WriteCloser
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

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		worker, err := slot.ensure(binary)
		if err != nil {
			return "", err
		}
		cancelDone := make(chan struct{})
		stopCancel := context.AfterFunc(ctx, func() {
			worker.kill()
			close(cancelDone)
		})
		jsonPayload, err := worker.roundTrip(suffix, lang, contents)
		if !stopCancel() {
			<-cancelDone
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			slot.stop()
			return "", ctxErr
		}
		if err == nil {
			return jsonPayload, nil
		}
		lastErr = err
		slot.stop()
	}
	return "", fmt.Errorf("persistent Rust lyrics worker failed after restart: %w", lastErr)
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
	cmd := exec.Command(binary, "--parse-lyrics-worker") //nolint:gosec // resolved administrator-controlled binary
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening lyrics worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("opening lyrics worker stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("starting lyrics worker %q: %w", binary, err)
	}
	return &lyricsWorker{
		binary: binary,
		cmd:    cmd,
		stdin:  stdin,
		writer: bufio.NewWriterSize(stdin, 64*1024),
		reader: bufio.NewReaderSize(stdout, lyricsWorkerReadBufBytes),
	}, nil
}

func (w *lyricsWorker) roundTrip(suffix, lang string, contents []byte) (string, error) {
	request := lyricsWorkerRequest{
		Suffix:    suffix,
		Lang:      lang,
		InputSize: len(contents),
	}
	header, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encoding lyrics request: %w", err)
	}
	if _, err := w.writer.Write(header); err != nil {
		return "", fmt.Errorf("writing lyrics request: %w", err)
	}
	if err := w.writer.WriteByte('\n'); err != nil {
		return "", fmt.Errorf("framing lyrics request: %w", err)
	}
	if _, err := w.writer.Write(contents); err != nil {
		return "", fmt.Errorf("writing lyrics payload: %w", err)
	}
	if err := w.writer.Flush(); err != nil {
		return "", fmt.Errorf("flushing lyrics request: %w", err)
	}

	responseHeader, err := w.reader.ReadBytes('\n')
	if err != nil {
		return "", fmt.Errorf("reading lyrics response header: %w", err)
	}
	if len(responseHeader) > maxLyricsResponseBytes {
		return "", fmt.Errorf("lyrics response exceeds maximum size of %d bytes", maxLyricsResponseBytes)
	}
	var response lyricsWorkerResponse
	if err := json.Unmarshal(bytes.TrimSuffix(responseHeader, []byte{'\n'}), &response); err != nil {
		return "", fmt.Errorf("decoding lyrics response header: %w", err)
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

func (w *lyricsWorker) kill() {
	if w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
}

func (w *lyricsWorker) close() {
	_ = w.stdin.Close()
	w.kill()
	_ = w.cmd.Wait()
}

var testBinaryOnce sync.Once

// EnsureTestBinary builds or locates navidrome-metadata for Go tests when
// ND_METADATAWORKERPATH is unset.
func EnsureTestBinary() error {
	var setupErr error
	testBinaryOnce.Do(func() {
		if configured := strings.TrimSpace(os.Getenv(EnvPath)); configured != "" {
			if _, err := resolveConfiguredBinary(configured); err != nil {
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
