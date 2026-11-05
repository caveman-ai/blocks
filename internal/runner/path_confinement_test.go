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
