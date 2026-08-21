package lofty

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/storage/local"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model/metadata"
)

const loftyVersion = "0.25.0"

type extractor struct {
	baseDir string

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

type request struct {
	Files []string `json:"files"`
}

type response struct {
	Files  map[string]wireMetadata `json:"files"`
	Errors map[string]string       `json:"errors"`
}

type wireMetadata struct {
	Tags            map[string][]string `json:"tags"`
	AudioProperties wireAudioProperties `json:"audio_properties"`
	HasPicture      bool                `json:"has_picture"`
}

type wireAudioProperties struct {
	DurationMS uint64 `json:"duration_ms"`
	BitRate    uint32 `json:"bit_rate"`
	BitDepth   uint8  `json:"bit_depth"`
	SampleRate uint32 `json:"sample_rate"`
	Channels   uint8  `json:"channels"`
	Codec      string `json:"codec"`
}

func (e *extractor) Parse(files ...string) (map[string]metadata.Info, error) {
	if len(files) == 0 {
		return map[string]metadata.Info{}, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	res, err := e.requestLocked(files)
	if err != nil {
		// A malformed file must not be able to permanently poison metadata scanning.
		// Restart the isolated worker once for transport/process failures.
		e.stopLocked()
		res, err = e.requestLocked(files)
		if err != nil {
			return nil, fmt.Errorf("lofty metadata worker failed after restart: %w", err)
		}
	}

	out := make(map[string]metadata.Info, len(res.Files))
	for path, item := range res.Files {
		out[path] = item.info()
	}
	for path, message := range res.Errors {
		log.Warn("Lofty could not read metadata. Skipping file", "filePath", path, "error", message)
	}
	return out, nil
}

func (e *extractor) Version() string {
	return "lofty/" + loftyVersion
}

func (m wireMetadata) info() metadata.Info {
	return metadata.Info{
		Tags: m.Tags,
		AudioProperties: metadata.AudioProperties{
			Duration:   time.Duration(m.AudioProperties.DurationMS) * time.Millisecond,
			BitRate:    int(m.AudioProperties.BitRate),
			BitDepth:   int(m.AudioProperties.BitDepth),
			SampleRate: int(m.AudioProperties.SampleRate),
			Channels:   int(m.AudioProperties.Channels),
			Codec:      m.AudioProperties.Codec,
		},
		HasPicture: m.HasPicture,
	}
}

func (e *extractor) requestLocked(files []string) (response, error) {
	if e.cmd == nil {
		if err := e.startLocked(); err != nil {
			return response{}, err
		}
	}

	payload, err := json.Marshal(request{Files: files})
	if err != nil {
		return response{}, err
	}
	payload = append(payload, '\n')
	if _, err := e.stdin.Write(payload); err != nil {
		return response{}, err
	}

	line, err := e.stdout.ReadBytes('\n')
	if err != nil {
		return response{}, err
	}
	var res response
	if err := json.Unmarshal(line, &res); err != nil {
		return response{}, fmt.Errorf("invalid Lofty worker response: %w", err)
	}
	return res, nil
}

func (e *extractor) startLocked() error {
	helper, err := helperPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(helper, "--root", e.baseDir) //nolint:gosec // executable is fixed/configured by the server operator
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}

	e.cmd = cmd
	e.stdin = stdin
	e.stdout = bufio.NewReaderSize(stdout, 256*1024)
	return nil
}

func (e *extractor) stopLocked() {
	if e.stdin != nil {
		_ = e.stdin.Close()
	}
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
		_, _ = e.cmd.Process.Wait()
	}
	e.cmd = nil
	e.stdin = nil
	e.stdout = nil
}

func helperPath() (string, error) {
	if override := os.Getenv("ND_LOFTY_HELPER"); override != "" {
		return override, nil
	}
	if path, err := exec.LookPath("navidrome-metadata"); err == nil {
		return path, nil
	}
	executable, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(executable), "navidrome-metadata")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("navidrome-metadata helper not found (set ND_LOFTY_HELPER or install it next to navidrome)")
}

var _ local.Extractor = (*extractor)(nil)

func init() {
	local.RegisterExtractor("lofty", func(_ fs.FS, baseDir string) local.Extractor {
		return &extractor{baseDir: baseDir}
	})
	conf.AddHook(func() {
		log.Debug("Lofty metadata extractor", "version", "lofty/"+loftyVersion)
	})
}
