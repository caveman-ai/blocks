// Package capture extracts inline scripts from shell commands, classifies them, fingerprints them,
// scrubs secrets and stores sightings (docs/HOOK.md).
package capture

import "time"

// Script is an inline script extracted from a shell command.
type Script struct {
	Lang      string   // "py" in v0
	Body      string   // the script text, unscrubbed
	Lines     int
	Edit      bool     // reads a file, replaces, writes the same path
	ScriptSHA string   // sha256 hex of the literal-stripped body
	FP        string   // 12 hex shape fingerprint, "" when the body has no features
	Literals  []string // string/number/path literals in order of appearance
}

// Extract finds an inline Python script in command: a heredoc to python/python3, or python -c.
// A leading `bash -lc '...'` / `sh -c '...'` wrapper is unwrapped first. ok is false when none.
func Extract(command string) (s Script, ok bool) { panic("capture: not implemented") }

// IsBlockCall reports whether command invokes a block, and which one.
func IsBlockCall(command string) (name string, ok bool) { panic("capture: not implemented") }

// StructuredDump reports whether command dumps a structured file whole (cat/head/tail/less of
// .json/.jsonl/.ndjson/.log/.csv) and returns the extension without the dot.
func StructuredDump(command string) (ext string, ok bool) { panic("capture: not implemented") }

// Scrub replaces secrets with «scrubbed» per the golden list in docs/HOOK.md.
func Scrub(s string) string { panic("capture: not implemented") }

// Sighting is one line in <fp>.jsonl.
type Sighting struct {
	TS          time.Time `json:"ts"`
	Session     string    `json:"session"`
	ScriptSHA   string    `json:"script_sha"`
	Lines       int       `json:"lines"`
	Literals    []string  `json:"literals"`
	CommandHead string    `json:"command_head"` // first 200 chars, scrubbed
	Script      string    `json:"script"`       // scrubbed body
}

// Shape summarizes one fp's sightings.
type Shape struct {
	FP       string
	Count    int
	Sessions int
	Last     time.Time
	Latest   Sighting
	Literals [][]string // literal vectors across sightings, for the promote brief
}

// Store is the append-only sightings directory (<state>/candidates).
type Store struct{ Dir string }

// Append writes one sighting with O_APPEND|O_NOFOLLOW, creating the file 0600.
func (st Store) Append(fp string, sg Sighting) error { panic("capture: not implemented") }

// Shapes lists shapes with sightings since the window, most sightings first.
func (st Store) Shapes(since time.Duration) ([]Shape, error) { panic("capture: not implemented") }
