package main

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestHookWatchdog runs the hook in a child process whose stdin never closes: the watchdog must answer
// the dialect's empty response and exit 0 at the deadline.
func TestHookWatchdog(t *testing.T) {
	if os.Getenv("CB_HOOK_CHILD") == "1" {
		root := newRoot()
		root.SetArgs([]string{"hook", "--harness", "claude"})
		root.Execute()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHookWatchdog$")
	cmd.Env = append(os.Environ(), "CB_HOOK_CHILD=1")
	w, err := cmd.StdinPipe() // held open until the test ends
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	start := time.Now()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hook exited with %v", err)
	}
	if string(out) != "{}\n" {
		t.Fatalf("stdout %q, want {}", out)
	}
	if d := time.Since(start); d < hookDeadline || d > hookDeadline+3*time.Second {
		t.Fatalf("answered after %v, want about %v", d, hookDeadline)
	}
}
