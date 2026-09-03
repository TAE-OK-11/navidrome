package rustworker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Pipes holds the stdin/stdout handles for a started worker subprocess.
type Pipes struct {
	Cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
}

// Start starts a worker subprocess with stderr attached to os.Stderr.
func Start(binary string, args ...string) (*Pipes, error) {
	cmd := exec.Command(binary, args...) //nolint:gosec // resolved administrator-controlled binary
	prepareCmd(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("opening worker stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("starting worker %q: %w", binary, err)
	}
	return &Pipes{Cmd: cmd, Stdin: stdin, Stdout: stdout}, nil
}

// Kill terminates the worker process if it is still running.
func Kill(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// Close shuts down a worker subprocess.
func Close(p *Pipes) {
	if p == nil {
		return
	}
	_ = p.Stdin.Close()
	Kill(p.Cmd)
	_ = p.Cmd.Wait()
}
