package hook

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/JuliusBrussee/caveman-blocks/internal/capture"
)

// cacheTTL is how long cache entries live; older ones are ignored. The runner prunes them daily.
const cacheTTL = time.Hour

// DefaultDeps wires the real capture package and a file-backed cache under <stateDir>/cache/.
func DefaultDeps(stateDir string) Deps {
	st := capture.Store{Dir: filepath.Join(stateDir, "candidates")}
	return Deps{
		Extract:        capture.Extract,
		IsBlockCall:    capture.IsBlockCall,
		StructuredDump: capture.StructuredDump,
		Scrub:          capture.Scrub,
		Append:         st.Append,
		Shapes:         st.Shapes,
		Cache:          &FileCache{Dir: filepath.Join(stateDir, "cache"), FlagDir: filepath.Join(stateDir, "hinted")},
	}
}

// FileCache is a Cache with one small file per key; an entry's age is its mtime.
//
// ponytail: rule 8's once-per-session add flags live in Dir too, so they expire with the 1 h TTL; a
// session longer than an hour can see the add hint again. Rule 9's per-day flags live in FlagDir,
// which the runner does not prune: one file per shape, holding the day it was last hinted.
type FileCache struct {
	Dir     string
	FlagDir string
	Now     func() time.Time // nil means time.Now
}

func (c *FileCache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *FileCache) path(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(c.Dir, hex.EncodeToString(h[:16])+".json")
}

// age returns how old the entry for key is, and false when it is missing, a symlink, or expired.
func (c *FileCache) age(key string) (time.Duration, bool) {
	fi, err := os.Lstat(c.path(key))
	if err != nil || !fi.Mode().IsRegular() {
		return 0, false
	}
	a := c.now().Sub(fi.ModTime())
	return a, a <= cacheTTL
}

// Get returns the decision stored for key within the TTL.
func (c *FileCache) Get(key string) (Decision, bool) {
	if _, ok := c.age(key); !ok {
		return Decision{}, false
	}
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return Decision{}, false
	}
	var d Decision
	if json.Unmarshal(b, &d) != nil {
		return Decision{}, false
	}
	return d, true
}

// Put stores d under key by writing a temp file and renaming it, so a reader never sees half a file
// and a planted symlink is replaced rather than followed.
func (c *FileCache) Put(key string, d Decision) {
	b, err := json.Marshal(d)
	if err != nil {
		return
	}
	c.write(key, b)
}

func (c *FileCache) write(key string, b []byte) {
	p := c.path(key)
	if replace(p, b) {
		t := c.now()
		os.Chtimes(p, t, t)
	}
}

// replace writes b to p through a temp file and a rename in p's directory.
func replace(p string, b []byte) bool {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return false
	}
	_, werr := f.Write(b)
	if cerr := f.Close(); werr != nil || cerr != nil || os.Rename(f.Name(), p) != nil {
		os.Remove(f.Name())
		return false
	}
	return true
}

// SeenRecently reports whether key was written within the window.
func (c *FileCache) SeenRecently(key string, within time.Duration) bool {
	a, ok := c.age(key)
	return ok && a <= within
}

func (c *FileCache) flag(fp string) string {
	h := sha256.Sum256([]byte("promote:" + fp))
	return filepath.Join(c.FlagDir, hex.EncodeToString(h[:16]))
}

// PromoteHinted reports whether rule 9 already hinted shape fp on day. A symlink is never read.
func (c *FileCache) PromoteHinted(fp, day string) bool {
	p := c.flag(fp)
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		return false
	}
	b, err := os.ReadFile(p)
	return err == nil && string(b) == day
}

// MarkPromoteHinted records rule 9's hint for shape fp on day.
func (c *FileCache) MarkPromoteHinted(fp, day string) { replace(c.flag(fp), []byte(day)) }

// AddHinted reports whether rule 8 already suggested adding block in session.
func (c *FileCache) AddHinted(session, block string) bool {
	_, ok := c.age("add:" + session + "\x00" + block)
	return ok
}

// MarkAddHinted records rule 8's add suggestion for block in session.
func (c *FileCache) MarkAddHinted(session, block string) {
	c.write("add:"+session+"\x00"+block, []byte("{}"))
}
