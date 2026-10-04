// Package blockfile parses, validates, lints and stamps block files (docs/FORMAT.md).
// This file fixes the public API other packages compile against; bodies land with the package.
package blockfile

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
func Parse(path string, src []byte) (*Block, error) { panic("blockfile: not implemented") }

// Load reads and parses the file at path.
func Load(path string) (*Block, error) { panic("blockfile: not implemented") }

// ContentHash returns the 12-hex content hash: SHA-256 over the LF-normalized source with the
// [stamp] table removed, followed by the sorted (relpath, sha256) pairs of fixtures.
func ContentHash(b *Block, fixtures map[string][]byte) string { panic("blockfile: not implemented") }

// WithStamp returns a copy of the source with the [stamp] table replaced, added last in the header,
// or removed when s is the zero value. No other byte changes.
func WithStamp(b *Block, s Stamp) []byte { panic("blockfile: not implemented") }

// Lint applies the rules in docs/FORMAT.md. An empty result means clean.
func Lint(b *Block, opts LintOptions) []Finding { panic("blockfile: not implemented") }

// Indexed reports whether b belongs in the index: active and stamp equals hash.
func Indexed(b *Block, hash string) bool { panic("blockfile: not implemented") }

// CallHint renders `caveman-blocks run <name> --p <type> ...` with required params only.
func CallHint(b *Block) string { panic("blockfile: not implemented") }

// ParamsColumn renders the index params column, e.g. `--path <path> [--depth 2]`.
func ParamsColumn(b *Block) string { panic("blockfile: not implemented") }
