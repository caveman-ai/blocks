package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caveman-ai/blocks/internal/blockfile"
)

func TestReviewSymlinkParentConfinement(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(outside, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("synthetic-outside-marker"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "child"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	b := fakeBlock(t, root, "readpath", `import json, sys
p = sys.argv[1].split("=", 1)[1] if "=" in sys.argv[1] else sys.argv[2]
with open(p) as f: print(json.dumps({"content": f.read()}))`, blockfile.EffectRead, blockfile.Param{Name: "path", Type: "path"})
	for _, value := range []string{"link/../sentinel", root + "/link/../sentinel", "link/../missing/leaf", "link/../missing/", "link/../missing///"} {
		for _, args := range [][]string{{"--path", value}, {"--path=" + value}, {"--pa", value}} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				if err := CheckPaths(root, b, args); !errors.Is(err, ErrPathOutsideRepo) {
					t.Errorf("outside path accepted: %v", err)
				}
			})
		}
	}
	res, err := Run(root, t.TempDir(), b, false, nil, []string{"--path", "link/../sentinel"}, nil, nil)
	if !errors.Is(err, ErrPathOutsideRepo) {
		t.Fatalf("Run must reject before execution: err=%v exit=%d stdout=%q", err, res.Exit, res.Stdout)
	}
}

func TestReviewSafeParentPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data", "child"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "data", "child"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	b := &blockfile.Block{Header: blockfile.Header{Params: []blockfile.Param{{Name: "path", Type: "path"}}}}
	for _, path := range []string{"data/../safe", "link/../new", "link/../new/deeper", "data/child/../../safe", "new/deeper/leaf"} {
		if err := CheckPaths(root, b, []string{"--path", path}); err != nil {
			t.Errorf("safe %q: %v", path, err)
		}
	}
}

func TestReviewMissingParentCannotHideSymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("synthetic-outside-marker"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	b := fakeBlock(t, root, "create-read", `import json, os, sys
os.mkdir("new")
with open(sys.argv[2]) as f: print(json.dumps({"content": f.read()}))`, blockfile.EffectWriteWorkspace, blockfile.Param{Name: "path", Type: "path"})
	res, err := Run(root, t.TempDir(), b, false, nil, []string{"--path", "new/../escape/sentinel"}, nil, nil)
	if !errors.Is(err, ErrPathOutsideRepo) {
		t.Fatalf("Run accepted missing-parent escape: err=%v stdout=%q", err, res.Stdout)
	}
}

func TestReviewUnresolvableSymlinksFailClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("loop", filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	b := &blockfile.Block{Header: blockfile.Header{Params: []blockfile.Param{{Name: "path", Type: "path"}}}}
	for _, p := range []string{"loop/../target", "dangling/new"} {
		if err := CheckPaths(root, b, []string{"--path", p}); !errors.Is(err, ErrPathOutsideRepo) {
			t.Errorf("accepted %q: %v", p, err)
		}
	}
}

func TestReviewParentTraversalProperties(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "inside"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	b := &blockfile.Block{Header: blockfile.Header{Params: []blockfile.Param{{Name: "path", Type: "path"}}}}
	for n := 0; n < 64; n++ {
		prefix := strings.Repeat("new/../", n)
		for _, tail := range []string{"/leaf", "/nested/leaf/"} {
			for _, abs := range []string{"", root + "/"} {
				safe := abs + prefix + "inside" + tail
				if err := CheckPaths(root, b, []string{"--path", safe}); err != nil {
					t.Fatalf("safe path rejected: %q: %v", safe, err)
				}
				unsafe := abs + prefix + "escape" + tail
				if err := CheckPaths(root, b, []string{"--path=" + unsafe}); !errors.Is(err, ErrPathOutsideRepo) {
					t.Fatalf("outside path accepted: %q: %v", unsafe, err)
				}
			}
		}
	}
}
