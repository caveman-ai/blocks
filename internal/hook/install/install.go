// Package install inserts, removes and reports the caveman-blocks hook entries in each harness's
// user-level config file shape (profiles.toml `config_format`). Our entries are the ones whose
// command runs `caveman-blocks hook`; everything else in the file is kept, in order.
package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// timeout is the hook timeout written everywhere, in seconds (docs/HOOK.md, failure policy).
const timeout = 5

// marker matches our command and captures the binary part: `<bin> hook ...`, bin possibly quoted.
var marker = regexp.MustCompile(`^(.*caveman-blocks'?)\s+hook(\s|$)`)

// Format is one config file shape.
type Format struct {
	harness string // value of --harness in our command
	cursor  bool   // cursor-hooks shape; otherwise the Claude Code shape
}

// For returns the writer for a profile's config_format.
func For(format string) (Format, error) {
	switch format {
	case "claude-settings":
		return Format{harness: "claude"}, nil
	case "codex-hooks":
		return Format{harness: "codex"}, nil
	case "cursor-hooks":
		return Format{harness: "cursor", cursor: true}, nil
	}
	return Format{}, fmt.Errorf("install: unsupported config format %q", format)
}

type entry struct {
	event string
	val   json.RawMessage
}

type cmdHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// entries renders our entries for bin.
func (f Format) entries(bin string) []entry {
	base := quote(bin) + " hook --harness " + f.harness
	if f.cursor {
		type cursorHook struct {
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		}
		return []entry{
			{"beforeShellExecution", marshal(cursorHook{base + " --phase pre", timeout})},
			{"afterShellExecution", marshal(cursorHook{base + " --phase post", timeout})},
		}
	}
	return []entry{{"PreToolUse", marshal(struct {
		Matcher string    `json:"matcher"`
		Hooks   []cmdHook `json:"hooks"`
	}{"Bash", []cmdHook{{"command", base, timeout}}})}}
}

// Install writes our entries for bin into the file at path, replacing any of ours already there.
// changed is false when the file already held exactly these entries.
func (f Format) Install(path, bin string) (changed bool, err error) {
	if !filepath.IsAbs(bin) || filepath.Base(bin) != "caveman-blocks" {
		return false, fmt.Errorf("install: binary %q must be an absolute path ending in caveman-blocks", bin)
	}
	path, doc, before, err := load(path)
	if err != nil {
		return false, err
	}
	hooks, err := doc.child("hooks")
	if err != nil {
		return false, fmt.Errorf("install: %s: %w", path, err)
	}
	if _, err := removeOurs(hooks); err != nil {
		return false, fmt.Errorf("install: %s: %w", path, err)
	}
	for _, e := range f.entries(bin) {
		arr, err := hooks.array(e.event)
		if err != nil {
			return false, fmt.Errorf("install: %s: hooks.%w", path, err)
		}
		hooks.set(e.event, rawArray(append(arr, e.val)))
	}
	if _, ok := doc.get("version"); f.cursor && !ok {
		doc.set("version", json.RawMessage("1"))
	}
	doc.set("hooks", hooks.raw())
	out := render(doc)
	if bytes.Equal(out, before) {
		return false, nil
	}
	return true, write(path, out)
}

// Uninstall removes exactly our entries, and any event list or hooks table they leave empty.
func (f Format) Uninstall(path string) (changed bool, err error) {
	path, doc, _, err := load(path)
	if err != nil {
		return false, err
	}
	if _, ok := doc.get("hooks"); !ok {
		return false, nil
	}
	hooks, err := doc.child("hooks")
	if err != nil {
		return false, fmt.Errorf("uninstall: %s: %w", path, err)
	}
	emptied, err := removeOurs(hooks)
	if err != nil || emptied == nil {
		return false, err
	}
	for _, k := range emptied {
		if a, _ := hooks.array(k); len(a) == 0 {
			hooks.del(k)
		}
	}
	if len(hooks.keys) == 0 {
		doc.del("hooks")
	} else {
		doc.set("hooks", hooks.raw())
	}
	return true, write(path, render(doc))
}

// Status reports whether one of our entries is in the file, and the binary it runs.
func (f Format) Status(path string) (installed bool, bin string, err error) {
	_, doc, _, err := load(path)
	if err != nil {
		return false, "", err
	}
	hooks, err := doc.child("hooks")
	if err != nil {
		return false, "", err
	}
	for _, k := range hooks.keys {
		arr, _ := hooks.array(k)
		for _, e := range arr {
			type cmd struct {
				Command string `json:"command"`
			}
			var h struct {
				cmd
				Hooks []cmd `json:"hooks"`
			}
			if json.Unmarshal(e, &h) != nil {
				continue
			}
			for _, c := range append(h.Hooks, h.cmd) {
				if m := marker.FindStringSubmatch(c.Command); m != nil {
					return true, unquote(m[1]), nil
				}
			}
		}
	}
	return false, "", nil
}

// removeOurs drops our entries from every event list in hooks: flat entries whose command is ours
// (Cursor) and our commands inside nested `hooks` lists (Claude Code, Codex), dropping a nested entry
// it empties. It returns the events it removed from, nil when it removed nothing; lists are left in
// place, possibly empty, so a reinstall keeps the file's order.
func removeOurs(hooks *object) ([]string, error) {
	var touched []string
	for _, k := range hooks.keys {
		arr, err := hooks.array(k)
		if err != nil {
			continue // not an event list; not ours
		}
		kept, removed := arr[:0:0], false
		for _, e := range arr {
			ne, drop, err := filterEntry(e)
			if err != nil {
				return nil, err
			}
			removed = removed || drop || ne != nil
			switch {
			case drop:
			case ne != nil:
				kept = append(kept, ne)
			default:
				kept = append(kept, e)
			}
		}
		if removed {
			touched = append(touched, k)
			hooks.set(k, rawArray(kept))
		}
	}
	return touched, nil
}

// filterEntry decides one event-list entry: drop it, replace it with ne, or keep it (nil, false).
func filterEntry(e json.RawMessage) (ne json.RawMessage, drop bool, err error) {
	o, err := parseObject(e)
	if err != nil {
		return nil, false, nil // not an object; not ours
	}
	if v, ok := o.get("command"); ok {
		var c string
		if json.Unmarshal(v, &c) == nil && marker.MatchString(c) {
			return nil, true, nil
		}
	}
	inner, err := o.array("hooks")
	if err != nil || inner == nil {
		return nil, false, nil
	}
	kept := inner[:0:0]
	for _, h := range inner {
		var c struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(h, &c) == nil && marker.MatchString(c.Command) {
			continue
		}
		kept = append(kept, h)
	}
	switch {
	case len(kept) == len(inner):
		return nil, false, nil
	case len(kept) == 0:
		return nil, true, nil
	}
	o.set("hooks", rawArray(kept))
	return o.raw(), false, nil
}

// load reads the JSON object at path, following a symlink so a dotfiles link stays a link. A missing
// or blank file is an empty object with no previous rendering.
func load(path string) (string, *object, []byte, error) {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(bytes.TrimSpace(b)) == 0) {
		return path, newObject(), nil, nil
	}
	if err != nil {
		return "", nil, nil, err
	}
	doc, err := parseObject(b)
	if err != nil {
		return "", nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return path, doc, render(doc), nil
}

func render(doc *object) []byte {
	var b bytes.Buffer
	json.Indent(&b, doc.raw(), "", "  ") // doc.raw is valid JSON by construction
	b.WriteByte('\n')
	return b.Bytes()
}

// write replaces path atomically: a temp file in the same directory, then rename. The file keeps its
// mode; a new one is 0600.
func write(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	mode := fs.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(b)
	if err == nil {
		err = tmp.Chmod(mode)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

var plain = regexp.MustCompile(`^[A-Za-z0-9_./+@%:,=-]+$`)

func quote(s string) string {
	if plain.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], `'\''`, "'")
	}
	return s
}
