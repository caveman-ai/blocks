//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

// killGroup starts cmd in its own process group and makes cancellation kill the whole group, so a
// block's children cannot outlive a timeout.
func killGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

// reapGroup kills whatever is left of the block's process group after the block has exited.
func reapGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
