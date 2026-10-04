// Package runner implements `caveman-blocks run`: the effects gate, path confinement, requires
// check, the python3 exec from the repo root, the 2 KB stdout cap with spill to the state dir, and
// state dir pruning (docs/FORMAT.md#calling-convention, docs/ARCHITECTURE.md).
package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/repo"
)

const (
	// Cap is the most stdout bytes an agent receives.
	Cap = 2048
	// HeadBytes is how much of an overflowing answer is returned inline.
	HeadBytes = 1024

	// Timeout bounds one `run`; verify sets its own.
	Timeout = 10 * time.Minute

	outTTL       = 7 * 24 * time.Hour
	cacheTTL     = time.Hour
	candidateTTL = 14 * 24 * time.Hour // hook rule 9's window
	candidateMax = 8 << 20             // newest bytes of a candidates file read when pruning
	pruneEvery   = 24 * time.Hour
)

// Sentinel errors, matched with errors.Is.
var (
	ErrEffectNotAllowed = errors.New("effect not allowed")
	ErrPathOutsideRepo  = errors.New("path outside the repo")
	ErrMissingRequires  = errors.New("required executable not on PATH")
	ErrNoPython         = errors.New("neither python3 nor python on PATH")
)

// EffectError is returned when the block's effect is not granted by allow_effects.
type EffectError struct {
	Block  string
	Effect blockfile.Effect
	// Line is the exact config.toml line that grants it, keeping existing grants.
	Line string
}

func (e *EffectError) Error() string {
	return fmt.Sprintf("block %s declares effects = %q, which committed .blocks/config.toml does not allow; add this line and commit it: %s",
		e.Block, e.Effect, e.Line)
}

// Is makes errors.Is(err, ErrEffectNotAllowed) true.
func (e *EffectError) Is(target error) bool { return target == ErrEffectNotAllowed }

// PathError is returned when a path param resolves outside the repo root.
type PathError struct{ Param, Value string }

func (e *PathError) Error() string {
	return fmt.Sprintf("--%s %s: path is outside the repo root", e.Param, e.Value)
}

// Is makes errors.Is(err, ErrPathOutsideRepo) true.
func (e *PathError) Is(target error) bool { return target == ErrPathOutsideRepo }

// MissingError is returned when a `requires` executable is not on PATH.
type MissingError struct{ Block, Bin string }

func (e *MissingError) Error() string {
	return fmt.Sprintf("block %s requires %s, which is not on PATH", e.Block, e.Bin)
}

// Is makes errors.Is(err, ErrMissingRequires) true.
func (e *MissingError) Is(target error) bool { return target == ErrMissingRequires }

// Result is the outcome of one run.
type Result struct {
	Stdout        []byte // what the agent sees
	Exit          int
	BytesFull     int    // stdout bytes the block printed
	BytesReturned int    // len(Stdout)
	FullPath      string // spill file when the cap triggered, else ""
	Unverified    bool
}

// Run executes block b with args from root after the effects gate, path confinement and requires
// checks. allow is allow_effects from committed config. unverified is the caller's verdict from
// blockfile.Indexed (stamp missing, stale or quarantined); it adds "_unverified": true to a JSON
// object answer. stdout over Cap bytes is spilled to <stateDir>/out/<id>.log and replaced by a
// truncation object. The block's exit code is passed through in Result.Exit. The run is killed after
// Timeout.
func Run(root, stateDir string, b *blockfile.Block, unverified bool, allow []string, args []string, stdin io.Reader, stderr io.Writer) (Result, error) {
	if err := CheckEffect(b, allow); err != nil {
		return Result{}, err
	}
	if err := CheckPaths(root, b, args); err != nil {
		return Result{}, err
	}
	if err := CheckRequires(b); err != nil {
		return Result{}, err
	}
	if stateDir != "" {
		prune(stateDir, time.Now())
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	outDir := ""
	if stateDir != "" {
		outDir = filepath.Join(stateDir, "out")
	}
	out, exit, err := Exec(ctx, root, outDir, b, args, stdin, stderr)
	if err != nil {
		return Result{}, err
	}
	res := Result{Exit: exit, BytesFull: len(out), Unverified: unverified, Stdout: out}
	if len(out) > Cap {
		path, err := spill(stateDir, out)
		if err != nil {
			return Result{}, fmt.Errorf("spill output: %w", err)
		}
		cut := HeadBytes
		for cut > 0 && !utf8.RuneStart(out[cut]) {
			cut--
		}
		res.FullPath = path
		res.Stdout, _ = json.Marshal(struct {
			Truncated  bool   `json:"_truncated"`
			Full       string `json:"_full"`
			Head       string `json:"head"`
			Unverified bool   `json:"_unverified,omitempty"`
		}{true, path, string(out[:cut]), unverified})
		res.Stdout = append(res.Stdout, '\n')
	} else if unverified {
		res.Stdout = markUnverified(out)
	}
	res.BytesReturned = len(res.Stdout)
	return res, nil
}

// CheckEffect refuses network and external unless allow names the block's effect, and unknown effects.
func CheckEffect(b *blockfile.Block, allow []string) error {
	eff := b.Header.Effects
	rank := eff.Rank()
	if rank < 0 {
		return fmt.Errorf("block %s: unknown effects %q", b.Header.Name, eff)
	}
	if rank < blockfile.EffectNetwork.Rank() {
		return nil
	}
	quoted := make([]string, 0, len(allow)+1)
	for _, a := range allow {
		if a == string(eff) {
			return nil
		}
		quoted = append(quoted, strconv.Quote(a))
	}
	quoted = append(quoted, strconv.Quote(string(eff)))
	return &EffectError{Block: b.Header.Name, Effect: eff,
		Line: "allow_effects = [" + strings.Join(quoted, ", ") + "]"}
}

// CheckPaths rejects any value of a path-typed param (`--name value` or `--name=value`) that
// resolves, symlinks included, outside root. Relative values resolve against root.
func CheckPaths(root string, b *blockfile.Block, args []string) error {
	types := map[string]string{}
	var pathNames []string
	for _, p := range b.Header.Params {
		types[p.Name] = p.Type
		if p.Type == "path" {
			pathNames = append(pathNames, p.Name)
		}
	}
	if len(pathNames) == 0 {
		return nil
	}
	// argparse accepts unambiguous prefixes (--pa for --path), so a flag that names no param
	// exactly but prefixes a path param is checked as that param.
	isPath := func(flag string) bool {
		if t, ok := types[flag]; ok {
			return t == "path"
		}
		for _, n := range pathNames {
			if flag != "" && strings.HasPrefix(n, flag) {
				return true
			}
		}
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	realRoot := resolve(absRoot)
	for i := 0; i < len(args); i++ {
		name, value, inline := strings.Cut(strings.TrimPrefix(args[i], "--"), "=")
		if !strings.HasPrefix(args[i], "--") || !isPath(name) {
			continue
		}
		if !inline {
			if i+1 >= len(args) {
				continue // argparse reports the missing value
			}
			i++
			value = args[i]
		}
		p := value
		if !filepath.IsAbs(p) {
			p = filepath.Join(absRoot, p)
		}
		if !within(realRoot, resolve(filepath.Clean(p))) {
			return &PathError{Param: name, Value: value}
		}
	}
	return nil
}

// resolve evaluates symlinks in the longest existing prefix of the clean absolute path p, so a
// path that does not exist yet still resolves through a symlinked parent.
func resolve(p string) string {
	rest := ""
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest)
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// CheckRequires fails on the first `requires` executable missing from PATH.
func CheckRequires(b *blockfile.Block) error {
	for _, bin := range b.Header.Requires {
		if _, err := exec.LookPath(bin); err != nil {
			return &MissingError{Block: b.Header.Name, Bin: bin}
		}
	}
	return nil
}

// Python returns python3, else python, from PATH, and refuses one older than 3.10. The answer is
// cached for the process.
func Python() (string, error) { return python() }

var python = sync.OnceValues(func() (string, error) {
	for _, name := range []string{"python3", "python"} {
		p, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, _ := exec.CommandContext(ctx, p, "--version").CombinedOutput()
		if !python310(string(out)) {
			return "", fmt.Errorf("%s reports %q; blocks need Python 3.10 or later", p, strings.TrimSpace(string(out)))
		}
		return p, nil
	}
	return "", ErrNoPython
})

// python310 reports whether `python --version` output names Python 3.10 or later.
func python310(out string) bool {
	var major, minor int
	if _, err := fmt.Sscanf(out, "Python %d.%d", &major, &minor); err != nil {
		return false
	}
	return major > 3 || major == 3 && minor >= 10
}

// Exec runs `python3 <b.Path> args...` from root and returns the full stdout and exit code. The
// environment is inherited plus PYTHONSAFEPATH=1, so a .blocks/json.py cannot shadow the stdlib,
// and BLOCKS_OUT=outDir when set, where blocks write their logs. The block runs in its own process
// group, killed whole when ctx ends or this process is interrupted. A non-zero exit is not an
// error; ctx ending is.
//
// ponytail: stdout is buffered in memory; stream to the spill file if blocks ever print gigabytes.
func Exec(ctx context.Context, root, outDir string, b *blockfile.Block, args []string, stdin io.Reader, stderr io.Writer) ([]byte, int, error) {
	py, err := Python()
	if err != nil {
		return nil, 0, err
	}
	path, err := filepath.Abs(b.Path)
	if err != nil {
		return nil, 0, err
	}
	// Its own process group takes the block out of the terminal's, so Ctrl-C must reach it via ctx.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd := exec.CommandContext(ctx, py, append([]string{path}, args...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PYTHONSAFEPATH=1")
	if outDir != "" {
		cmd.Env = append(cmd.Env, "BLOCKS_OUT="+outDir)
	}
	cmd.WaitDelay = 5 * time.Second
	killGroup(cmd)
	cmd.Stdin = stdin
	cmd.Stderr = stderr
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if ctx.Err() != nil {
		return out.Bytes(), -1, fmt.Errorf("block %s: %w", b.Header.Name, ctx.Err())
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out.Bytes(), exitErr.ExitCode(), nil
	}
	if err != nil {
		return nil, 0, err
	}
	return out.Bytes(), 0, nil
}

// markUnverified inserts "_unverified":true as the first key when out is exactly one JSON
// object, leaving every other byte as the block printed it. Anything else is returned unchanged.
func markUnverified(out []byte) []byte {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return out
	}
	i := bytes.IndexByte(out, '{') + 1
	sep := ","
	if bytes.TrimSpace(out[i:])[0] == '}' {
		sep = ""
	}
	marked := make([]byte, 0, len(out)+20)
	marked = append(marked, out[:i]...)
	marked = append(marked, `"_unverified":true`+sep...)
	return append(marked, out[i:]...)
}

// spill writes the full output to <stateDir>/out/<unix-nanos hex>.log, 0600, never through a symlink.
func spill(stateDir string, out []byte) (string, error) {
	if stateDir == "" {
		return "", errors.New("no state dir")
	}
	dir := filepath.Join(stateDir, "out")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, strconv.FormatInt(time.Now().UnixNano(), 16)+".log")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|repo.NoFollow, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(out); err != nil {
		f.Close()
		return "", err
	}
	return path, f.Close()
}

// prune removes out/ entries older than 7 days, cache/ entries older than 1 hour and sightings older
// than 14 days, at most once
// per day, tracked by the mtime of <stateDir>/.pruned. Best effort: errors are ignored.
func prune(stateDir string, now time.Time) {
	marker := filepath.Join(stateDir, ".pruned")
	if fi, err := os.Lstat(marker); err == nil && now.Sub(fi.ModTime()) < pruneEvery {
		return
	}
	pruneCandidates(filepath.Join(stateDir, "candidates"), now)
	for sub, ttl := range map[string]time.Duration{"out": outTTL, "cache": cacheTTL} {
		dir := filepath.Join(stateDir, sub)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if fi, err := e.Info(); err == nil && now.Sub(fi.ModTime()) > ttl {
				os.RemoveAll(filepath.Join(dir, e.Name()))
			}
		}
	}
	if f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|repo.NoFollow, 0o600); err == nil {
		f.Close()
		os.Chtimes(marker, now, now)
	}
}

// pruneCandidates drops sightings older than candidateTTL from <stateDir>/candidates/*.jsonl.
// Sightings are appended in time order, so only the newest candidateMax bytes of a file are read
// and a bigger file is cut to them. A file is rewritten (temp + rename) only when a line goes.
//
// ponytail: a sighting the hook appends during the rewrite is lost; the hook would need a lock.
func pruneCandidates(dir string, now time.Time) {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	for _, p := range paths {
		pruneSightings(p, now.Add(-candidateTTL))
	}
}

func pruneSightings(p string, cutoff time.Time) error {
	f, err := os.OpenFile(p, os.O_RDONLY|repo.NoFollow, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return err
	}
	cut := fi.Size() > candidateMax
	if cut {
		if _, err := f.Seek(fi.Size()-candidateMax, io.SeekStart); err != nil {
			return err
		}
	}
	var keep bytes.Buffer
	r := bufio.NewReader(f)
	dropped, partial := cut, cut // after a seek the first line is partial
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && !partial {
			var sg struct {
				TS time.Time `json:"ts"`
			}
			if json.Unmarshal(line, &sg) == nil && !sg.TS.Before(cutoff) {
				keep.Write(line)
			} else {
				dropped = true
			}
		}
		partial = false
		if err != nil {
			break
		}
	}
	if !dropped {
		return nil
	}
	if keep.Len() == 0 {
		return os.Remove(p)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".prune-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(keep.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil { // CreateTemp makes it 0600
		return err
	}
	return os.Rename(tmp.Name(), p)
}
