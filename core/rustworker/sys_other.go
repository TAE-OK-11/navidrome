//go:build !linux

package rustworker

import "os/exec"

func prepareCmd(cmd *exec.Cmd) {}
