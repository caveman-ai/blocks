// Package hook is the decision engine behind every harness hook: a shell command in, one hint line and
// counted events out. The rule table it implements is docs/HOOK.md; golden cases live in
// testdata/decisions. Decide never denies and never rewrites a command (decisions 0003, 0010).
package hook

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/caveman-ai/blocks/internal/blockfile"
	"github.com/caveman-ai/blocks/internal/capture"
)

// Event kinds the hook writes to stats.jsonl.
const (
	KindScript      = "script"
	KindHint        = "hint"
	KindPromoteHint = "promote-hint"
	KindCall        = "call"
)

// Phases of a harness call.
const (
	PhasePre  = "pre"
	PhasePost = "post"
)

// PromoteHint is rule 9's sentence, appended to a hint or emitted alone.
const PromoteHint = "Run caveman-blocks promote when the task is done."

// Durations from docs/HOOK.md.
const (
	dedupeWindow  = 2 * time.Second     // rule 2
	promoteWindow = 14 * 24 * time.Hour // rule 9
	commandHead   = 200                 // Sighting.CommandHead, in runes
	scriptCap     = 16 << 10            // Sighting.Script, in bytes
)

// Event is one line of stats.jsonl.
type Event struct {
	TS      time.Time `json:"ts"`
	Session string    `json:"session"`
	Kind    string    `json:"kind"`
	Block   string    `json:"block,omitempty"`
	FP      string    `json:"fp,omitempty"`
	Lines   int       `json:"lines,omitempty"` // script events only
	OK      *bool     `json:"ok,omitempty"`
}

// BlockInfo is what the engine needs from one indexed block. The caller builds it from a
// blockfile.Block that passed blockfile.Validate; the engine checks the rendered parts again.
type BlockInfo struct {
	Name    string
	Effects string           // blockfile.Effect value
	Matches []*regexp.Regexp // compiled `matches` patterns
	Hint    string           // blockfile.CallHint: `caveman-blocks run <name> --p <type> ...`
}

// Config is the per-call configuration resolved by the caller.
type Config struct {
	RepoRoot    string // nearest parent of Cwd holding .blocks/; "" when there is none (rule 1)
	StateDir    string
	Blocks      []BlockInfo     // indexed blocks
	HintEnabled bool            // config.toml `hint`
	CoveredFPs  map[string]bool // fps named by any block's provenance.source (`candidate:<fp>`)
	Now         func() time.Time
}

// Input is one normalized hook call.
type Input struct {
	Phase   string // pre | post
	Command string
	Cwd     string
	Session string
	CallID  string
}

// Decision is the engine's answer: at most one hint line and the events to count.
type Decision struct {
	Hint   string  `json:"hint"`
	Events []Event `json:"events,omitempty"`
}

// Cache holds per-call decisions for dedupe and post-run replay, rule 9's once-per-shape-per-day
// flags and rule 8's once-per-session flags.
type Cache interface {
	Get(key string) (Decision, bool)
	Put(key string, d Decision)
	SeenRecently(key string, within time.Duration) bool
	PromoteHinted(fp, day string) bool
	MarkPromoteHinted(fp, day string)
	AddHinted(session, block string) bool
	MarkAddHinted(session, block string)
}

// Deps are the engine's side effects and classifiers, as function fields so tests can stub them.
type Deps struct {
	Extract        func(command string) (capture.Script, bool)
	IsBlockCall    func(command string) (string, bool)
	StructuredDump func(command string) (string, bool)
	Scrub          func(string) string
	Append         func(fp string, sg capture.Sighting) error
	Shapes         func(since time.Duration) ([]capture.Shape, error)
	Cache          Cache
	Log            func(error) // internal errors, for hook.log; nil drops them
}

// dumpBlocks lists, per structured-file extension, the first-party blocks that fit, best first (rule 8).
// A name not among cfg.Blocks is suggested with `caveman-blocks add`.
var dumpBlocks = map[string][]string{
	"json":   {"json-peek", "jsonl-stats"},
	"jsonl":  {"jsonl-stats", "json-peek"},
	"ndjson": {"jsonl-stats", "json-peek"},
	"log":    {"first-error"},
}

// Decide applies the rules of docs/HOOK.md in order. Hints do not stack; the first rule that produces
// one wins, and rule 9 may append to it.
func Decide(cfg Config, in Input, deps Deps) Decision {
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	ts := now()

	// Rule 0: post replays the pre decision, events emptied.
	if in.Phase == PhasePost {
		d, _ := deps.Cache.Get(replayKey(in))
		d.Events = nil
		return d
	}

	// Rule 1: no .blocks/ above Cwd.
	if cfg.RepoRoot == "" {
		return Decision{}
	}

	// Rule 2: a second hook configuration for the same call.
	ck := cmdKey(in)
	if deps.Cache.SeenRecently(ck, dedupeWindow) {
		d, _ := deps.Cache.Get(ck)
		d.Events = nil
		if in.CallID != "" {
			deps.Cache.Put(callKey(in.CallID), d)
		}
		return d
	}

	d := decide(cfg, in, deps, ts)
	deps.Cache.Put(ck, d)
	if in.CallID != "" {
		deps.Cache.Put(callKey(in.CallID), d)
	}
	return d
}

// decide runs rules 3–9 for a pre call inside a repo.
func decide(cfg Config, in Input, deps Deps, ts time.Time) Decision {
	var d Decision
	event := func(kind, block, fp string, lines int) {
		d.Events = append(d.Events, Event{TS: ts, Session: in.Session, Kind: kind, Block: block, FP: fp, Lines: lines})
	}

	// Rule 3: a block call.
	if name, ok := deps.IsBlockCall(in.Command); ok {
		event(KindCall, name, "", 0)
		return d
	}

	// Rule 4: inline Python script; Extract unwraps the harness's shell wrapper.
	if s, ok := deps.Extract(in.Command); ok {
		// Rule 5: s.Edit. Rule 6: a block's matches; with hints off it falls through to capture.
		if b := bestMatch(cfg.Blocks, s.Body, s.Edit); b != nil && cfg.HintEnabled {
			d.Hint = runHint(*b)
			event(KindHint, b.Name, s.FP, 0)
		} else if s.Lines >= 10 && !s.Edit && s.FP != "" {
			// Rule 7: capture one scrubbed sighting.
			if err := deps.Append(s.FP, sighting(in, s, deps.Scrub, ts)); err != nil {
				logErr(deps, err)
			} else {
				event(KindScript, "", s.FP, s.Lines)
			}
		}
	}

	// Rule 8: a structured file dumped whole.
	if d.Hint == "" && cfg.HintEnabled {
		if ext, ok := deps.StructuredDump(in.Command); ok {
			if h, block := dumpHint(cfg.Blocks, ext, in.Session, deps.Cache); h != "" {
				d.Hint = h
				event(KindHint, block, "", 0)
			}
		}
	}

	// Rule 9: repeated shapes no block covers, each hinted at most once a day.
	if cfg.HintEnabled {
		day := ts.UTC().Format(time.DateOnly)
		var fresh []string
		for _, fp := range promoteDue(cfg.CoveredFPs, deps) {
			if !deps.Cache.PromoteHinted(fp, day) {
				fresh = append(fresh, fp)
			}
		}
		if len(fresh) > 0 {
			if d.Hint == "" {
				d.Hint = PromoteHint
			} else {
				d.Hint += " " + PromoteHint
			}
			event(KindPromoteHint, "", "", 0)
			for _, fp := range fresh {
				deps.Cache.MarkPromoteHinted(fp, day)
			}
		}
	}
	return d
}

// bestMatch returns the block whose patterns match body longest, ties broken by name. When edit, only
// write-workspace blocks are considered.
func bestMatch(blocks []BlockInfo, body string, edit bool) *BlockInfo {
	var best *BlockInfo
	bestLen := -1
	for i := range blocks {
		b := &blocks[i]
		if (edit && b.Effects != "write-workspace") || !renderable(*b) {
			continue
		}
		for _, re := range b.Matches {
			loc := re.FindStringIndex(body)
			if loc == nil {
				continue
			}
			n := loc[1] - loc[0]
			if n > bestLen || (n == bestLen && b.Name < best.Name) {
				best, bestLen = b, n
			}
		}
	}
	return best
}

// callArgs is the shape of the params part of a blockfile.CallHint.
var callArgs = regexp.MustCompile(`^( --[a-z][a-z0-9_-]{0,31} <(str|int|float|bool|path|enum)>)*$`)

// renderable reports whether b's name and call hint have the shape blockfile renders. Defense in
// depth: a header that slipped past the caller's validation never reaches the agent as a command.
func renderable(b BlockInfo) bool {
	if !blockfile.ValidName(b.Name) {
		return false
	}
	if b.Hint == "" {
		return true
	}
	args, ok := strings.CutPrefix(b.Hint, "caveman-blocks run "+b.Name)
	return ok && callArgs.MatchString(args)
}

// runHint renders `Blocks: <name> covers this. Next time: <call hint>`.
func runHint(b BlockInfo) string {
	call := b.Hint
	if call == "" {
		call = "caveman-blocks run " + b.Name
	}
	return "Blocks: " + b.Name + " covers this. Next time: " + call
}

// dumpHint picks the indexed block fitting ext, or suggests adding the best fit once per session.
func dumpHint(blocks []BlockInfo, ext, session string, c Cache) (hint, block string) {
	fits := dumpBlocks[ext]
	for _, name := range fits {
		for _, b := range blocks {
			if b.Name == name && renderable(b) {
				return runHint(b), name
			}
		}
	}
	if len(fits) == 0 || c.AddHinted(session, fits[0]) {
		return "", ""
	}
	c.MarkAddHinted(session, fits[0])
	return "Blocks: " + fits[0] + " covers this. Add it with: caveman-blocks add " + fits[0], fits[0]
}

// promoteDue returns the uncovered shapes in the window that call for promotion: those seen in 2+
// sessions, or else all of them when there are 5+.
func promoteDue(covered map[string]bool, deps Deps) []string {
	shapes, err := deps.Shapes(promoteWindow)
	if err != nil {
		logErr(deps, err)
		return nil
	}
	var multi, all []string
	for _, s := range shapes {
		if covered[s.FP] {
			continue
		}
		all = append(all, s.FP)
		if s.Sessions >= 2 {
			multi = append(multi, s.FP)
		}
	}
	if len(multi) > 0 {
		return multi
	}
	if len(all) >= 5 {
		return all
	}
	return nil
}

// sighting builds the scrubbed record rule 7 stores. Scrub runs before truncation so a secret split
// at the cut is still caught. s.Literals already come from the scrubbed body; scrubbing each again
// is defense in depth.
func sighting(in Input, s capture.Script, scrub func(string) string, ts time.Time) capture.Sighting {
	lits := make([]string, len(s.Literals))
	for i, l := range s.Literals {
		lits[i] = scrub(l)
	}
	script := scrub(s.Body)
	if len(script) > scriptCap {
		script = strings.ToValidUTF8(script[:scriptCap], "")
	}
	return capture.Sighting{
		TS:          ts,
		Session:     in.Session,
		ScriptSHA:   s.ScriptSHA,
		Lines:       s.Lines,
		Literals:    lits,
		CommandHead: truncate(scrub(in.Command), commandHead),
		Script:      script,
	}
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func logErr(deps Deps, err error) {
	if deps.Log != nil {
		deps.Log(err)
	}
}

// cmdKey is hash(Session, Cwd, Command), the dedupe key and the replay key without a call id.
func cmdKey(in Input) string {
	h := sha256.Sum256([]byte(in.Session + "\x00" + in.Cwd + "\x00" + in.Command))
	return "cmd:" + hex.EncodeToString(h[:])
}

func callKey(id string) string { return "call:" + id }

func replayKey(in Input) string {
	if in.CallID != "" {
		return callKey(in.CallID)
	}
	return cmdKey(in)
}
