//go:build e2e

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/caveman-ai/blocks/blocks"
	"github.com/caveman-ai/blocks/internal/blockfile"
	"github.com/caveman-ai/blocks/internal/repo"
)

// TestHookBudget enforces the hook latency budget of docs/ARCHITECTURE.md: median wall time of a
// whole `hook --harness claude` process, under 5 ms outside a repo (rule 1) and under 30 ms for a
// full decision in a repo with 20 indexed blocks. CAVEMAN_BLOCKS_BIN names the binary to time
// (make bench-hook passes the release build); otherwise the test binary's caveman-blocks copy runs.
func TestHookBudget(t *testing.T) {
	const runs = 200
	bin := os.Getenv("CAVEMAN_BLOCKS_BIN")
	if bin == "" {
		var err error
		if bin, err = exec.LookPath("caveman-blocks"); err != nil {
			t.Fatal(err)
		}
	}
	outside := t.TempDir()
	if _, ok := repo.FindRoot(outside); ok {
		t.Skipf("%s sits under a .blocks/ directory; rule 1 cannot be measured here", outside)
	}
	rule1 := median(runs, func(int) time.Duration {
		return hookRun(t, bin, outside, payload("bench", outside, "ls -la"))
	})

	root := benchRepo(t, 20)
	heredoc := "python3 - <<'EOF'\nimport json\nd=json.load(open('x.json'))\nprint(d.keys())\nEOF"
	if out := runHook(t, bin, root, payload("warm", root, heredoc)); !strings.Contains(string(out), "covers this") {
		t.Fatalf("fixture repo gives no hint, so the full path is not measured: %s", out)
	}
	full := median(runs, func(i int) time.Duration {
		// A new session per call so rule 2's dedupe never short-cuts the decision.
		return hookRun(t, bin, root, payload(fmt.Sprint("s", i), root, heredoc))
	})

	baseline := median(runs/4, func(int) time.Duration { return timeRun(t, exec.Command("/usr/bin/true")) })
	t.Logf("hook median over %d runs: rule 1 %v (budget 5ms), full decision with 20 blocks %v (budget 30ms); /usr/bin/true %v; binary %s",
		runs, rule1, full, baseline, bin)
	if baseline > 20*time.Millisecond {
		t.Skipf("machine under heavy load: /usr/bin/true takes %v at the median; the hook budget cannot be measured", baseline)
	}
	if full >= 30*time.Millisecond {
		t.Errorf("full decision median %v, budget 30ms", full)
	}
	// The budgets are wall time of a whole process. When spawning a process that does nothing
	// already takes half the rule-1 budget, rule 1 cannot be told apart from the machine's load.
	if baseline > 2500*time.Microsecond {
		t.Skipf("rule 1 not measurable: /usr/bin/true alone takes %v, over half its 5ms budget", baseline)
	}
	if rule1 >= 5*time.Millisecond {
		t.Errorf("rule 1 median %v, budget 5ms", rule1)
	}
}

func payload(session, cwd, command string) []byte {
	b, _ := json.Marshal(map[string]any{"session_id": session, "hook_event_name": "PreToolUse", "cwd": cwd,
		"tool_input": map[string]string{"command": command}})
	return b
}

func runHook(t *testing.T, bin, dir string, stdin []byte) []byte {
	cmd := exec.Command(bin, "hook", "--harness", "claude")
	cmd.Dir, cmd.Stdin = dir, bytes.NewReader(stdin)
	cmd.Env = append(os.Environ(), "XDG_STATE_HOME="+filepath.Join(filepath.Dir(dir), "state"))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	return out
}

func hookRun(t *testing.T, bin, dir string, stdin []byte) time.Duration {
	start := time.Now()
	runHook(t, bin, dir, stdin)
	return time.Since(start)
}

func timeRun(t *testing.T, cmd *exec.Cmd) time.Duration {
	start := time.Now()
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return time.Since(start)
}

func median(n int, f func(int) time.Duration) time.Duration {
	ds := make([]time.Duration, n)
	for i := range ds {
		ds[i] = f(i)
	}
	slices.Sort(ds)
	return ds[n/2]
}

// benchRepo is a git repository with n indexed copies of json-peek, stamped in-process.
func benchRepo(t *testing.T, n int) string {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(root, ".blocks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("git"); err == nil {
		if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v %s", err, out)
		}
	}
	src, err := blocks.FS.ReadFile("json-peek.py")
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		name := fmt.Sprintf("peek-%02d", i)
		path := filepath.Join(root, ".blocks", name+".py")
		b, err := blockfile.Parse(path, bytes.Replace(src, []byte(`name = "json-peek"`), []byte(`name = "`+name+`"`), 1))
		if err != nil {
			t.Fatal(err)
		}
		stamped := blockfile.WithStamp(b, blockfile.Stamp{Verified: blockfile.ContentHash(b, nil)})
		if err := os.WriteFile(path, stamped, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
