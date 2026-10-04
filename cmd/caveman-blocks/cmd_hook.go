package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/JuliusBrussee/caveman-blocks/internal/blockfile"
	"github.com/JuliusBrussee/caveman-blocks/internal/hook"
	"github.com/JuliusBrussee/caveman-blocks/internal/hook/dialect/claude"
	"github.com/JuliusBrussee/caveman-blocks/internal/hook/dialect/cursor"
	"github.com/JuliusBrussee/caveman-blocks/internal/hook/dialect/generic"
	"github.com/JuliusBrussee/caveman-blocks/internal/hook/protocol"
	"github.com/JuliusBrussee/caveman-blocks/internal/promote"
	"github.com/JuliusBrussee/caveman-blocks/internal/repo"
	"github.com/JuliusBrussee/caveman-blocks/internal/stats"
	"github.com/spf13/cobra"
)

func hookCmd() *cobra.Command {
	var harness, phase string
	c := &cobra.Command{
		Use:   "hook --harness <claude|codex|cursor|generic> [--phase pre|post]",
		Short: "Entry point the harness calls on every shell command; always exits 0.",
		// The hook must never fail a session: stray arguments and unknown flags are ignored.
		Args:               cobra.ArbitraryArgs,
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		Run: func(c *cobra.Command, _ []string) {
			out := c.OutOrStdout()
			var once sync.Once
			answer := func(b []byte) { once.Do(func() { out.Write(b) }) }
			// The watchdog answers empty and exits 0 when stdin never closes or a decision runs long.
			go func() {
				time.Sleep(hookDeadline)
				answer(emptyAnswer(harness, phase))
				os.Exit(0)
			}()
			stdin, _ := io.ReadAll(io.LimitReader(os.Stdin, hookMaxInput))
			answer(hookAnswer(harness, phase, stdin, time.Now))
		},
	}
	c.Flags().StringVar(&harness, "harness", "generic", "dialect of the calling harness")
	c.Flags().StringVar(&phase, "phase", "", "pre or post, for harnesses whose payload does not say")
	return c
}

// Bounds on one hook process. A payload over hookMaxInput is cut and fails to parse: empty answer.
const (
	hookDeadline = 2 * time.Second
	hookMaxInput = 1 << 20
)

type dialect struct {
	parse  func([]byte) (protocol.Request, error)
	render func(protocol.Response, string) ([]byte, error)
}

var dialects = map[string]dialect{
	"claude":  {claude.Parse, claude.Render},
	"codex":   {claude.Parse, claude.Render},
	"cursor":  {cursor.Parse, cursor.Render},
	"generic": {generic.Parse, generic.Render},
}

// emptyAnswer is the dialect's allow-with-no-hint answer, "{}" for an unknown harness.
func emptyAnswer(harness, phase string) []byte {
	d, ok := dialects[harness]
	if !ok {
		return []byte("{}\n")
	}
	if phase == "" {
		phase = hook.PhasePre
	}
	return render(d, protocol.Allow(""), phase)
}

// hookAnswer turns one harness payload into the harness's answer. Every failure, panics included,
// becomes the dialect's empty answer; internal errors go to <stateDir>/hook.log.
func hookAnswer(harness, phase string, stdin []byte, now func() time.Time) (out []byte) {
	d, ok := dialects[harness]
	if !ok {
		return []byte("{}\n")
	}
	if phase == "" {
		phase = hook.PhasePre
	}
	stateDir := ""
	defer func() {
		if r := recover(); r != nil {
			hookLog(stateDir, fmt.Errorf("H001 panic: %v", r), now())
			out = render(d, protocol.Allow(""), phase)
		}
	}()

	req, err := d.parse(stdin)
	if err != nil {
		return render(d, protocol.Allow(""), phase)
	}
	if req.Phase == "" {
		req.Phase = phase
	}
	phase = req.Phase
	if req.Cwd == "" {
		req.Cwd, _ = os.Getwd()
	}
	// Rule 1 and its 5 ms budget: nothing else is loaded outside a repo.
	root, ok := repo.FindRoot(req.Cwd)
	if !ok {
		return render(d, protocol.Allow(""), phase)
	}
	if stateDir, err = repo.StateDir(root); err != nil {
		return render(d, protocol.Allow(""), phase)
	}
	cfg := hook.Config{RepoRoot: root, StateDir: stateDir}
	if req.Phase == hook.PhasePre { // post only replays the cache (rule 0)
		cfg = hookConfig(root, stateDir, now)
	}
	deps := hook.DefaultDeps(stateDir)
	deps.Log = func(err error) { hookLog(stateDir, err, now()) }
	dec := hook.Decide(cfg, hook.Input{Phase: req.Phase, Command: req.Command, Cwd: req.Cwd, Session: req.Session, CallID: req.CallID}, deps)
	for _, e := range dec.Events {
		if err := stats.Append(stateDir, stats.Event{TS: e.TS, Session: e.Session, Kind: e.Kind, Block: e.Block, FP: e.FP, OK: e.OK}); err != nil {
			hookLog(stateDir, err, now())
		}
	}
	return render(d, protocol.Allow(dec.Hint), phase)
}

// hookConfig builds the engine's view of the repo: indexed blocks with compiled patterns, the fps
// that blocks' provenance covers, and config.toml's hint switch. A bad config.toml means defaults.
//
// It reads block headers only, never fixtures, so a repo's fixture size cannot slow the hook. Without
// fixtures there is no content hash, so "indexed" is approximated as a non-empty [stamp].verified
// and an empty state: a block edited since its last verify still hints until sync or verify catches
// it. The runner checks the real hash before running anything.
func hookConfig(root, stateDir string, now func() time.Time) hook.Config {
	conf, err := repo.LoadConfig(root)
	if err != nil {
		hookLog(stateDir, err, now())
		conf = repo.DefaultConfig()
	}
	cfg := hook.Config{RepoRoot: root, StateDir: stateDir, HintEnabled: conf.Hint, Now: now}
	paths, _ := repo.ListBlocks(root)
	bs := make([]*blockfile.Block, 0, len(paths))
	for _, p := range paths {
		b, err := blockfile.Load(p)
		if err != nil {
			continue
		}
		bs = append(bs, b)
		if s := b.Header.Stamp; s != nil && s.Verified != "" && s.State == "" {
			cfg.Blocks = append(cfg.Blocks, hook.BlockInfo{Name: b.Header.Name, Effects: string(b.Header.Effects),
				Matches: compile(b), Hint: blockfile.CallHint(b)})
		}
	}
	cfg.CoveredFPs = promote.Covered(bs)
	return cfg
}

func render(d dialect, resp protocol.Response, phase string) []byte {
	b, err := d.render(resp, phase)
	if err != nil {
		return []byte("{}\n")
	}
	return append(b, '\n')
}

// hookLog appends one line to <stateDir>/hook.log, at most one per minute, never through a symlink.
func hookLog(stateDir string, err error, now time.Time) {
	if stateDir == "" {
		return
	}
	p := filepath.Join(stateDir, "hook.log")
	if fi, e := os.Lstat(p); e == nil && now.Sub(fi.ModTime()) < time.Minute {
		return
	}
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE|repo.NoFollow, 0o600)
	if e != nil {
		return
	}
	fmt.Fprintf(f, "%s %s\n", now.UTC().Format(time.RFC3339), oneLine(err.Error()))
	f.Close()
}
