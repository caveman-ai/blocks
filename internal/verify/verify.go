// Package verify runs a block's example, checks the answer against its contract and writes the
// [stamp] table (docs/ARCHITECTURE.md#data-flow, docs/CI.md).
package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/repo"
	"github.com/JuliusBrussee/caveman-blocks/internal/runner"
)

// Timeout bounds one example run.
const Timeout = 300 * time.Second

// Outcome statuses.
const (
	Pass = "pass"
	Fail = "fail"
	Skip = "skip"
)

// Outcome is the verdict for one block.
type Outcome struct {
	Name   string
	Status string // pass | fail | skip
	Reason string // why it failed or was skipped; "" on pass
	Hash   string // current content hash
}

// Options controls one Verify call.
type Options struct {
	// Check recomputes and reruns but never writes; it also fails on a stale stamp or a
	// committed quarantined block (CI mode).
	Check bool
	// FixturesRoot overrides <root>/.blocks/fixtures, for first-party blocks.
	FixturesRoot string
	// Hash and Stamp default to blockfile.ContentHash and blockfile.WithStamp.
	Hash  func(*blockfile.Block, map[string][]byte) string
	Stamp func(*blockfile.Block, blockfile.Stamp) []byte
}

// Verify runs b's example from root with $FIXTURES expanded to the block's absolute fixtures dir.
// It skips when b's effect is not in allow or a `requires` executable is missing; in Check mode a
// stale or quarantined stamp fails first. It passes on exit
// 0 with stdout exactly one JSON object of at most 2 KB holding every returns.keys key. Outside
// Check mode a pass writes [stamp] verified = hash with no state, and a fail writes
// state = "quarantined" keeping verified; unchanged bytes are not rewritten. b itself is not updated.
func Verify(root string, b *blockfile.Block, fixtures map[string][]byte, allow []string, opts Options) Outcome {
	hashFn, stampFn := opts.Hash, opts.Stamp
	if hashFn == nil {
		hashFn = blockfile.ContentHash
	}
	if stampFn == nil {
		stampFn = blockfile.WithStamp
	}
	o := Outcome{Name: b.Header.Name, Hash: hashFn(b, fixtures)}

	old := blockfile.Stamp{}
	if b.Header.Stamp != nil {
		old = *b.Header.Stamp
	}
	// In Check mode the committed stamp is judged before any skip, so a pull request cannot land a
	// quarantined or hand-stamped block as a skip (docs/CI.md).
	if opts.Check {
		switch {
		case old.State == "quarantined":
			o.Status, o.Reason = Fail, "committed quarantined block: fix and re-verify, or retire"
			return o
		case old.Verified != o.Hash:
			o.Status, o.Reason = Fail, "stamp stale: run caveman-blocks verify locally and commit"
			return o
		}
	}
	if err := runner.CheckEffect(b, allow); err != nil {
		var ee *runner.EffectError
		if errors.As(err, &ee) {
			o.Status, o.Reason = Skip, "effect not allowed; grant with: "+ee.Line
		} else {
			o.Status, o.Reason = Fail, err.Error()
		}
		return o
	}
	if err := runner.CheckRequires(b); err != nil {
		o.Status, o.Reason = Skip, err.Error()
		return o
	}

	o.Reason = runExample(root, b, opts.FixturesRoot)
	o.Status = Pass
	next := blockfile.Stamp{Verified: o.Hash}
	if o.Reason != "" {
		o.Status = Fail
		next = blockfile.Stamp{Verified: old.Verified, State: "quarantined"}
	}
	if !opts.Check && next != old {
		if err := writeBlock(b.Path, stampFn(b, next)); err != nil {
			o.Status, o.Reason = Fail, "write stamp: "+err.Error()
		}
	}
	return o
}

// runExample runs the example and returns "" when the answer meets the contract, else the reason.
func runExample(root string, b *blockfile.Block, fixturesRoot string) string {
	if len(b.Header.Example) == 0 {
		return "no example"
	}
	if fixturesRoot == "" {
		fixturesRoot = filepath.Join(root, ".blocks", "fixtures")
	}
	fixturesDir, err := filepath.Abs(filepath.Join(fixturesRoot, b.Header.Name))
	if err != nil {
		return err.Error()
	}
	args := make([]string, len(b.Header.Example))
	for i, a := range b.Header.Example {
		args[i] = strings.ReplaceAll(a, "$FIXTURES", fixturesDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	var stderr bytes.Buffer
	outDir, err := os.MkdirTemp("", "caveman-blocks-verify-")
	if err != nil {
		return err.Error()
	}
	defer os.RemoveAll(outDir)
	out, exit, err := runner.Exec(ctx, root, outDir, b, args, nil, &stderr)
	if err != nil {
		return err.Error()
	}
	trimmed := bytes.TrimSpace(out)
	if exit != 0 {
		return fmt.Sprintf("exit %d%s", exit, failureDetail(trimmed, stderr.Bytes()))
	}
	if len(out) > runner.Cap {
		return "output over 2 KB"
	}
	var answer map[string]json.RawMessage
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) || json.Unmarshal(trimmed, &answer) != nil {
		return "stdout is not exactly one JSON object"
	}
	var missing []string
	for _, k := range b.Header.Returns.Keys {
		if _, ok := answer[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return "answer lacks returns keys: " + strings.Join(missing, ", ")
	}
	return ""
}

// failureDetail is ": <error>" from the JSON answer, else ": <last stderr line>", at most 200 bytes.
func failureDetail(stdout, stderr []byte) string {
	var answer struct{ Error string }
	msg := ""
	if json.Unmarshal(stdout, &answer) == nil && answer.Error != "" {
		msg = answer.Error
	} else if lines := strings.Split(strings.TrimSpace(string(stderr)), "\n"); lines[len(lines)-1] != "" {
		msg = lines[len(lines)-1]
	}
	if msg == "" {
		return ""
	}
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	return ": " + msg
}

// writeBlock replaces the regular file at path with data via a temp file and rename, keeping its mode.
func writeBlock(path string, data []byte) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file", path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(fi.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Select returns the block paths to verify. mode "all" is every .blocks/*.py; "name" is the block
// named arg; "changed" is blocks whose file or fixtures dir differs from the merge base with
// defaultBranch, untracked files included, falling back to all when there is no merge base.
func Select(root, mode, arg, defaultBranch string) ([]string, error) {
	switch mode {
	case "all":
		return repo.ListBlocks(root)
	case "name":
		if arg == "" || strings.ContainsAny(arg, `/\`) || strings.HasPrefix(arg, ".") {
			return nil, fmt.Errorf("invalid block name %q", arg)
		}
		p := filepath.Join(root, ".blocks", arg+".py")
		if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("no block %s in .blocks/", arg)
		}
		return []string{p}, nil
	case "changed":
		all, err := repo.ListBlocks(root)
		if err != nil {
			return nil, err
		}
		mb, err := repo.MergeBase(root, defaultBranch)
		if err != nil {
			return all, nil
		}
		diff, err := repo.Git(root, "diff", "-z", "--name-only", "--relative", mb, "--", ".blocks")
		if err != nil {
			return nil, err
		}
		untracked, err := repo.Git(root, "ls-files", "-z", "--others", "--exclude-standard", "--", ".blocks")
		if err != nil {
			return nil, err
		}
		changed := strings.Split(diff+"\x00"+untracked, "\x00")
		var out []string
		for _, p := range all {
			name := strings.TrimSuffix(filepath.Base(p), ".py")
			for _, c := range changed {
				if c == ".blocks/"+name+".py" || strings.HasPrefix(c, ".blocks/fixtures/"+name+"/") {
					out = append(out, p)
					break
				}
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown selection mode %q", mode)
}
