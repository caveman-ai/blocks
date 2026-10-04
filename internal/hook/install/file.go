package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/caveman-ai/blocks/plugins"
)

// The opencode-plugin format is a whole file of ours, not an entry in someone else's: the embedded
// OpenCode plugin with bin baked in, its first line the marker command as a comment.

const installedLine = `const INSTALLED = "";`

func (f Format) installFile(path, bin string) (changed bool, err error) {
	lit, _ := json.Marshal(bin) // a JSON string is a JS string literal
	body := strings.Replace(plugins.OpenCode, installedLine, "const INSTALLED = "+string(lit)+";", 1)
	out := []byte("// " + quote(bin) + " hook --harness " + f.harness + "\n" + body)
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, out) {
		return false, nil
	}
	return true, write(path, out)
}

// uninstallFile removes the file only when its first line is our marker; a user's own plugin stays.
func (f Format) uninstallFile(path string) (changed bool, err error) {
	ok, _, err := f.statusFile(path)
	if !ok || err != nil {
		return false, err
	}
	return true, os.Remove(path)
}

func (f Format) statusFile(path string) (installed bool, bin string, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	first, _, _ := strings.Cut(string(b), "\n")
	if m := marker.FindStringSubmatch(strings.TrimPrefix(first, "// ")); m != nil {
		return true, unquote(m[1]), nil
	}
	return false, "", nil
}
