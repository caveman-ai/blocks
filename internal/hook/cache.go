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

// cacheTTL is how long cache entries live; older ones are ignored and pruned.
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
		Cache:          &FileCache{Dir: filepath.Join(stateDir, "cache")},
	}
}

// FileCache is a Cache with one small file per key; an entry's age is its mtime.
//
// ponytail: once-per-session flags live here too, so they expire with the 1 h TTL; a session longer
// than an hour can see the promote or add hint again. Move flags to their own dir if that annoys.
type FileCache struct {
	Dir    string
	Now    func() time.Time // nil means time.Now
	pruned bool
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
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return
	}
	c.prune()
	f, err := os.CreateTemp(c.Dir, ".tmp-*")
	if err != nil {
		return
	}
	_, werr := f.Write(b)
	if cerr := f.Close(); werr != nil || cerr != nil {
		os.Remove(f.Name())
		return
	}
	p := c.path(key)
	if os.Rename(f.Name(), p) != nil {
		os.Remove(f.Name())
		return
	}
	t := c.now()
	os.Chtimes(p, t, t)
}

// SeenRecently reports whether key was written within the window.
func (c *FileCache) SeenRecently(key string, within time.Duration) bool {
	a, ok := c.age(key)
	return ok && a <= within
}

// PromoteHinted reports whether rule 9 already hinted in session.
func (c *FileCache) PromoteHinted(session string) bool {
	_, ok := c.age("promote:" + session)
	return ok
}

// MarkPromoteHinted records rule 9's hint for session.
func (c *FileCache) MarkPromoteHinted(session string) { c.write("promote:"+session, []byte("{}")) }

// AddHinted reports whether rule 8 already suggested adding block in session.
func (c *FileCache) AddHinted(session, block string) bool {
	_, ok := c.age("add:" + session + "\x00" + block)
	return ok
}

// MarkAddHinted records rule 8's add suggestion for block in session.
func (c *FileCache) MarkAddHinted(session, block string) {
	c.write("add:"+session+"\x00"+block, []byte("{}"))
}

// prune removes entries older than the TTL, at most once per FileCache value (one hook run).
func (c *FileCache) prune() {
	if c.pruned {
		return
	}
	c.pruned = true
	ents, err := os.ReadDir(c.Dir)
	if err != nil {
		return
	}
	cutoff := c.now().Add(-cacheTTL)
	for _, e := range ents {
		if fi, err := e.Info(); err == nil && fi.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(c.Dir, e.Name()))
		}
	}
}
