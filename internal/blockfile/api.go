// Package blockfile parses, validates, lints and stamps block files (docs/FORMAT.md).
// This file holds the public API other packages compile against; helpers live beside it.
package blockfile

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Effect is a declared side-effect class, ordered read < write-workspace < exec < network < external.
type Effect string

const (
	EffectRead           Effect = "read"
	EffectWriteWorkspace Effect = "write-workspace"
	EffectExec           Effect = "exec"
	EffectNetwork        Effect = "network"
	EffectExternal       Effect = "external"
)

// Rank returns the ladder position of e, or -1 for an unknown value.
func (e Effect) Rank() int {
	switch e {
	case EffectRead:
		return 0
	case EffectWriteWorkspace:
		return 1
	case EffectExec:
		return 2
	case EffectNetwork:
		return 3
	case EffectExternal:
		return 4
	}
	return -1
}

// Param is one entry of the [params] table, in header order.
type Param struct {
	Name     string
	Type     string // str | int | float | bool | path | enum
	Required bool
	Default  any
	Help     string
	Min, Max *float64
	Values   []string
}

// Returns is the [returns] table.
type Returns struct {
	Keys []string
	Doc  string
}

// Provenance is the optional [provenance] table.
type Provenance struct {
	Created  string
	Source   string
	Sessions int
}

// Stamp is the tool-owned [stamp] table.
type Stamp struct {
	Verified string // 12 hex of ContentHash, or ""
	State    string // "" (active) or "quarantined"
}

// Header is the parsed TOML inside the `# /// block` fence.
type Header struct {
	Name       string
	Summary    string
	Effects    Effect
	Example    []string
	Matches    []string
	Returns    Returns
	Params     []Param
	Requires   []string
	Provenance *Provenance
	Stamp      *Stamp
}

// Block is a parsed block file.
type Block struct {
	Path   string // as given to Load/Parse; may be ""
	Header Header
	Source []byte // the full file as read
	// Line indexes (0-based) of the fence lines in Source, and of the `# [stamp]` line or -1.
	FenceStart, FenceEnd, StampLine int
}

// Finding is one lint result. Codes: F001..F012, W001 (docs/FORMAT.md).
type Finding struct {
	Code    string
	Message string
	Line    int // 1-based, 0 when not line-specific
}

// LintOptions selects optional rules.
type LintOptions struct {
	FirstParty      bool // F012: stdlib-only, one import per line
	OnDefaultBranch bool // emits W001 when true
}

// Parse parses src. path is used for F003 and error messages only.
// Parse errors are F001 Finding values wrapped with the path; recover them with errors.As.
// A path that does not end in .py, or a non-python shebang, is rejected: v0 blocks are Python only.
func Parse(path string, src []byte) (*Block, error) { return parse(path, src) }

// Load reads and parses the file at path.
func Load(path string) (*Block, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load block: %w", err)
	}
	return Parse(path, src)
}

// ContentHash returns the 12-hex content hash: SHA-256 over the LF-normalized source with the
// [stamp] table removed, followed by the sorted (relpath, sha256) pairs of fixtures.
// Bare `#` lines directly above the [stamp] table (or above the closing fence) are dropped as well,
// so stamping with WithStamp never changes the hash. Fixture lines are "relpath\x00sha256hex\n".
func ContentHash(b *Block, fixtures map[string][]byte) string { return contentHash(b, fixtures) }

// WithStamp returns a copy of the source with the [stamp] table replaced, added last in the header,
// or removed when s is the zero value. No other byte changes.
// A new table is preceded by a bare `#` line; CRLF files keep CRLF.
func WithStamp(b *Block, s Stamp) []byte { return withStamp(b, s) }

// Lint applies the rules in docs/FORMAT.md. An empty result means clean.
// Findings are sorted by code, then line. F001 comes from Parse, not Lint.
func Lint(b *Block, opts LintOptions) []Finding { return lint(b, opts) }

// Indexed reports whether b belongs in the index: active and stamp equals hash.
func Indexed(b *Block, hash string) bool {
	s := b.Header.Stamp
	return s != nil && s.State == "" && s.Verified == hash
}

// CallHint renders `caveman-blocks run <name> --p <type> ...` with required params only.
func CallHint(b *Block) string {
	out := "caveman-blocks run " + b.Header.Name
	for _, p := range b.Header.Params {
		if p.Required {
			out += " --" + p.Name + " <" + p.Type + ">"
		}
	}
	return out
}

// ParamsColumn renders the index params column, e.g. `--path <path> [--depth 2]`.
// Optional params show their default; booleans are bare flags; a param with neither shows its type.
func ParamsColumn(b *Block) string {
	var parts []string
	for _, p := range b.Header.Params {
		switch {
		case p.Required:
			parts = append(parts, "--"+p.Name+" <"+p.Type+">")
		case p.Type == "bool":
			parts = append(parts, "[--"+p.Name+"]")
		case p.Default == nil:
			parts = append(parts, "[--"+p.Name+" <"+p.Type+">]")
		default:
			parts = append(parts, "[--"+p.Name+" "+compact(p.Default)+"]")
		}
	}
	return strings.Join(parts, " ")
}

// compact renders a default for the params column: bare unless it needs quoting.
func compact(v any) string {
	s := fmt.Sprint(v)
	if f, ok := v.(float64); ok {
		s = strconv.FormatFloat(f, 'g', -1, 64)
	}
	if s == "" || strings.ContainsAny(s, " \t\"'[]") {
		return strconv.Quote(s)
	}
	return s
}
