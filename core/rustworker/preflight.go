package rustworker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	PreflightTimeout    = 5 * time.Second
	MinScannerBytes     = 800_000
	MinMetadataBytes    = 800_000
	MinSearchBytes      = 800_000
	MinApikeysBytes     = 800_000
	MinIntegrationBytes = 800_000
)

// WorkerCheck describes a Rust companion binary to validate at startup.
type WorkerCheck struct {
	Name string
	Path string
	Args []string
	// MinBytes rejects cache-stub binaries that are executable but non-functional.
	MinBytes int64
	// SmokeRequest is written to stdin when set. SmokeExpect must appear in the first response line.
	SmokeRequest string
	SmokeExpect  string
}

// Preflight validates Rust worker binaries and logs actionable errors. It never aborts startup.
func Preflight(ctx context.Context, checks []WorkerCheck) {
	for _, check := range checks {
		if err := check.run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "[navidrome] Rust worker preflight failed for %s (%s): %v\n",
				check.Name, check.Path, err)
		}
	}
}

func (c WorkerCheck) run(ctx context.Context) error {
	if strings.TrimSpace(c.Path) == "" {
		return fmt.Errorf("worker path is empty")
	}
	info, err := os.Stat(c.Path)
	if err != nil {
		return fmt.Errorf("worker binary not found: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("worker path is a directory")
	}
	if c.MinBytes > 0 && info.Size() < c.MinBytes {
		return fmt.Errorf("worker binary is too small (%d bytes); expected a production build", info.Size())
	}
	if c.SmokeRequest == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, PreflightTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.Path, c.Args...) //nolint:gosec // resolved worker path
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("opening worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("opening worker stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("starting worker: %w", err)
	}

	reader := bufio.NewReader(stdout)
	if _, err := stdin.Write([]byte(c.SmokeRequest)); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("writing smoke request: %w", err)
	}
	_ = stdin.Close()

	line, err := reader.ReadString('\n')
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("reading smoke response: %w", err)
	}
	if c.SmokeExpect != "" && !strings.Contains(line, c.SmokeExpect) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("unexpected smoke response: %s", strings.TrimSpace(line))
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return nil
}

// MetadataNormalizeSmokeRequest returns a valid normalize worker probe request.
func MetadataNormalizeSmokeRequest() string {
	payload, _ := json.Marshal(map[string][]string{"values": {"R.E.M."}})
	return string(payload) + "\n"
}
