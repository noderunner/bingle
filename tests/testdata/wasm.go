package testdata

import (
	"os"
	"os/exec"
	"path/filepath"
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

var wasmBuilds sync.Map // main package → func() ([]byte, error)

// BuildWasm builds a rules module (ADR 0147) from its main package with the
// Go that runs the test, as a pack's is built: for wasip1, as a module whose
// exports are called rather than its main. No binary is committed. A package
// is built once for a test binary, and go's own cache keeps the rest.
func BuildWasm(t testing.TB, pkg string) []byte {
	t.Helper()
	build, _ := wasmBuilds.LoadOrStore(pkg, sync.OnceValues(func() ([]byte, error) {
		dir, err := os.MkdirTemp("", "bingle-wasm-")
		if err != nil {
			return nil, err
		}
		defer func() { _ = os.RemoveAll(dir) }()
		out := filepath.Join(dir, "rules.wasm")
		cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-buildvcs=false", "-o", out, pkg)
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
