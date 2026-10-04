package runner

import "os/exec"

// killGroup is a no-op on Windows: cancellation kills the block's process only.
func killGroup(*exec.Cmd) {}

// reapGroup is a no-op on Windows: there is no process group to kill.
func reapGroup(*exec.Cmd) {}
