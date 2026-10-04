package repo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SafePath joins the slash- or OS-separated relative path rel onto root and refuses when rel leaves
// root or any existing component below root, the leaf included, is a symlink. A cloned repository
// can therefore never point one of our writes or deletes outside itself.
func SafePath(root, rel string) (string, error) {
	rel = filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(rel) || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: not a path inside the repo", rel)
	}
	p := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		p = filepath.Join(p, part)
		fi, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			break // nothing below a missing component exists either
		}
		if err != nil {
			return "", err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("%s is a symlink; refusing to write or delete through it", p)
		}
	}
	return filepath.Join(root, rel), nil
}

// WriteFileSafe writes data to root/rel through SafePath: parents are created 0755, the data goes
// to a temp file in the same directory (created O_EXCL, which never follows a symlink) that is then
// renamed over the leaf, so neither a symlinked leaf nor a symlinked parent is ever followed.
func WriteFileSafe(root, rel string, data []byte, perm fs.FileMode) error {
	p, err := SafePath(root, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// RemoveAllSafe removes root/rel and everything below it through SafePath. A missing path is not an
// error. os.RemoveAll never follows a symlink below the path it is given.
func RemoveAllSafe(root, rel string) error {
	p, err := SafePath(root, rel)
	if err != nil {
		return err
	}
	return os.RemoveAll(p)
}
