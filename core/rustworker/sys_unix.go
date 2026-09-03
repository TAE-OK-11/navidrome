//go:build linux

package rustworker

import (
	"os/exec"
	"syscall"
)

func prepareCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
