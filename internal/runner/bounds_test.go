//go:build !windows

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/caveman-ai/blocks/internal/blockfile"
)

func TestPython310(t *testing.T) {
	for out, want := range map[string]bool{
		"Python 3.13.2\n": true, "Python 3.10.0+": true, "Python 4.0.0": true,
		"Python 3.9.18": false, "Python 2.7.18": false, "": false, "garbage": false,
	} {
		if got := python310(out); got != want {
			t.Errorf("python310(%q) = %v", out, got)
		}
	}
}

func TestRunEnv(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	b := fakeBlock(t, root, "env", `import json, os, sys
print(json.dumps({"safe": os.environ.get("PYTHONSAFEPATH"), "out": os.environ.get("BLOCKS_OUT"),
                  "v311": sys.version_info >= (3, 11)}))`, blockfile.EffectRead)
	// A .blocks/json.py must not shadow the stdlib json the block imports.
	if err := os.WriteFile(filepath.Join(root, ".blocks", "json.py"), []byte("raise SystemExit('shadowed')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := run(t, root, state, b, false)
	var got struct {
		Safe, Out string
		V311      bool
	}
	if err := json.Unmarshal(res.Stdout, &got); err != nil {
		if !strings.Contains(string(res.Stdout), "v311") {
			t.Skipf("python 3.10 ignores PYTHONSAFEPATH: %q", res.Stdout)
		}
		t.Fatalf("stdout %q: %v", res.Stdout, err)
	}
	if got.Safe != "1" || got.Out != filepath.Join(state, "out") {
		t.Errorf("env = %+v", got)
	}
}

// TestExecKillsProcessGroup: on timeout the block and the children it spawned die, and Exec
// returns promptly even though a grandchild holds stdout open.
func TestExecKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	pidFile := filepath.Join(root, "pid")
	b := fakeBlock(t, root, "hang", fmt.Sprintf(`import subprocess, time
p = subprocess.Popen(["sleep", "60"])
open(%q, "w").write(str(p.pid))
time.sleep(60)`, pidFile), blockfile.EffectExec)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, _, err := Exec(ctx, root, "", b, nil, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("Exec took %s", d)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(data))
	for i := 0; i < 50 && syscall.Kill(pid, 0) == nil; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
		t.Errorf("grandchild %d survived the timeout", pid)
	}
}

// TestExecChildHoldsStdout: a block that prints its answer, exits 0 and leaves a child holding
// stdout keeps its answer, and the child is killed.
func TestExecChildHoldsStdout(t *testing.T) {
	defer func(d time.Duration) { waitDelay = d }(waitDelay)
	waitDelay = 300 * time.Millisecond
	root := t.TempDir()
	pidFile := filepath.Join(root, "pid")
	b := fakeBlock(t, root, "bg", fmt.Sprintf(`import json, subprocess
p = subprocess.Popen(["sleep", "60"])
open(%q, "w").write(str(p.pid))
print(json.dumps({"ok": True}), flush=True)`, pidFile), blockfile.EffectExec)
	out, exit, err := Exec(context.Background(), root, "", b, nil, nil, nil)
	if err != nil || exit != 0 || strings.TrimSpace(string(out)) != `{"ok": true}` {
		t.Fatalf("Exec = %q, %d, %v", out, exit, err)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(data))
	time.Sleep(100 * time.Millisecond) // SIGKILL delivery
	if err := syscall.Kill(pid, 0); err == nil {
		exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
		t.Errorf("child %d survived the run", pid)
	}
}
