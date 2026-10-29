// TestRepro replays a bundle from a file, which is the other half of
// scripts/replay: the same reproduction, inside the test suite, so a bundle
// can be committed as a regression test once the bug it holds is understood.
//
//	go test ./tests/repro/ -run TestRepro$ -bundle /path/to/bundle.json
//
// Without -bundle it looks in tests/repro/testdata for *.json and replays
// every one it finds, so a bundle dropped in there becomes a test.

package repro_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/repro"
)

var bundleFlag = flag.String("bundle", "",
	"a repro bundle to replay (see docs/observability.md); without it, tests/repro/testdata/*.json")

// TestRepro replays whatever bundle it is pointed at.
//
// A bundle in testdata is there because its fault is **fixed**: the test fails
// if it reproduces again. A bundle named with -bundle is being investigated,
// so the test reports what happened either way and only fails when the bundle
// cannot be replayed at all.
func TestRepro(t *testing.T) {
	cat := builtinCatalog(t)

	if *bundleFlag != "" {
		b, err := repro.Load(*bundleFlag)
		if err != nil {
			t.Fatalf("%v", err)
		}
		res, err := repro.Replay(cat, b)
		if err != nil {
			t.Fatalf("replaying %s: %v", *bundleFlag, err)
		}
		t.Logf("%s (%s on %s)\n%s", filepath.Base(*bundleFlag), b.Kind, b.GameDefID, res)
		if res.Reproduced {
			t.Logf("the fault is still there")
		}
		return
	}

	paths, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatalf("looking in testdata: %v", err)
	}
	if len(paths) == 0 {
		t.Skip("no bundles in tests/repro/testdata: drop one in to turn it into a regression test")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			b, err := repro.Load(path)
			if err != nil {
				t.Fatalf("%v", err)
			}
			res, err := repro.Replay(cat, b)
			if err != nil {
				t.Fatalf("replaying: %v", err)
			}
			if res.Reproduced {
				t.Errorf("a fault that was fixed has come back: %s on %s\n%s\n%s",
					b.Kind, b.GameDefID, res, b.Board)
			}
		})
	}
}

// The directory exists even when it is empty, so the test says what it is for
// rather than failing to glob.
func TestRepro_TestdataIsADocumentedPlace(t *testing.T) {
	readme := filepath.Join("testdata", "README.md")
	if _, err := os.Stat(readme); err != nil {
		t.Errorf("tests/repro/testdata has no README saying what belongs in it: %v", err)
	}
}
