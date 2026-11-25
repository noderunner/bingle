package testdata

import (
	"embed"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The fixtures' rules modules, as the main packages they are built from.
const (
	// DraughtsWasm is the Draughts fixture's rules.
	DraughtsWasm = "github.com/noderunner/bingle/tests/testdata/draughts/wasm"
	// FaultyWasm is the same rules, which misbehave when asked to.
	FaultyWasm = "github.com/noderunner/bingle/tests/testdata/draughts/faulty/wasm"
)

var wasmBuilds sync.Map // main package and tags → func() ([]byte, error)

// NoBot is the build tag that leaves the bot call out of a module, as a
// module built before the call was added lacks it.
const NoBot = "bingle_nobot"

// BuildWasm builds a rules module (ADR 0147) from its main package with the
// Go that runs the test, as a pack's is built: for wasip1, as a module whose
// exports are called rather than its main. No binary is committed. A package
// is built once for a test binary with each set of tags, and go's own cache
// keeps the rest.
func BuildWasm(t testing.TB, pkg string, tags ...string) []byte {
	t.Helper()
	key := pkg + " " + strings.Join(tags, ",")
	build, _ := wasmBuilds.LoadOrStore(key, sync.OnceValues(func() ([]byte, error) {
		dir, err := os.MkdirTemp("", "bingle-wasm-")
		if err != nil {
			return nil, err
		}
		defer func() { _ = os.RemoveAll(dir) }()
		out := filepath.Join(dir, "rules.wasm")
		cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-buildvcs=false", "-tags", strings.Join(tags, ","), "-o", out, pkg)
		// The test's own flags (-race, say) are not the module's.
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0", "GOFLAGS=")
		if msg, err := cmd.CombinedOutput(); err != nil {
			return nil, &buildError{pkg: pkg, err: err, output: string(msg)}
		}
		return os.ReadFile(out)
	}))
	wasm, err := build.(func() ([]byte, error))()
	if err != nil {
		t.Fatal(err)
	}
	return wasm
}

type buildError struct {
	pkg    string
	err    error
	output string
}

func (e *buildError) Error() string {
	return "building " + e.pkg + " for wasip1: " + e.err.Error() + "\n" + e.output
}

// The Draughts fixture as an install pack (ADR 0147): its games' ids.
const (
	DraughtsPackID      = "pack-draughts-0.1.0"
	DraughtsPackSmallID = "pack-draughts-0.1.0-6"
)

//go:embed draughts/pack
var draughtsPackFS embed.FS

// DraughtsPack writes the Draughts fixture as an install pack into a new
// directory of packs, with its rules built as a module, and returns that
// directory (what BINGLE_PACKS names) and the pack's own.
func DraughtsPack(t testing.TB) (packs, dir string) {
	t.Helper()
	packs = t.TempDir()
	dir = filepath.Join(packs, "draughts")
	WritePack(t, dir, DraughtsWasm)
	return packs, dir
}

// WritePack writes the Draughts fixture pack into dir, with the rules of the
// main package pkg (DraughtsWasm or FaultyWasm) as its rules.wasm.
func WritePack(t testing.TB, dir, pkg string) {
	t.Helper()
	sub, err := fs.Sub(draughtsPackFS, "draughts/pack")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(dir, sub); err != nil {
		t.Fatal(err)
	}
	WriteRules(t, dir, pkg)
}

// WriteRules builds the main package pkg as the rules.wasm of the pack in dir.
func WriteRules(t testing.TB, dir, pkg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "rules.wasm"), BuildWasm(t, pkg), 0o644); err != nil {
		t.Fatal(err)
	}
}
