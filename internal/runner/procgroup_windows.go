package runner

import "os/exec"

// killGroup is a no-op on Windows: cancellation kills the block's process only.
func killGroup(*exec.Cmd) {}
