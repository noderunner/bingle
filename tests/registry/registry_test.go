// Package registry_test checks how a binary's games are chosen (game plugins
// plan, step 2): every family this repository ships registers itself from
// package all, and a build tag leaves one out of the build entirely.
package registry_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/games/registry"
	"github.com/noderunner/bingle/internal/pack"
)

var shipped = []string{"blooms", "classics", "courtfall", "fendo", "fist", "meridians", "mycelium", "solitaire", "tak"}

// The default build has every family the repository ships (a private build,
// ADR 0127, adds its own), and the server's modules are theirs.
func TestEveryFamilyIsRegistered(t *testing.T) {
	got := registry.Names()
	for _, name := range shipped {
		if !slices.Contains(got, name) {
			t.Errorf("family %s is not registered (families %v)", name, got)
		}
	}
	mods := app.BuiltinModules()
	if len(got) == len(shipped) && len(mods) != 21 {
		t.Errorf("%d modules, want 21 (12 classics, Solitaire and Bowling, and one for every other family)", len(mods))
	}
	seen := map[string]bool{}
	for _, m := range mods {
		if seen[m.ID] {
			t.Errorf("module %s twice", m.ID)
		}
		seen[m.ID] = true
		// The prefix is the packs', so an installed pack can never shadow a built-in game (ADR 0128).
		if strings.HasPrefix(m.ID, pack.Prefix) {
			t.Errorf("the built-in module %s has the prefix of a data pack", m.ID)
		}
	}
	for _, id := range []string{"tak-5x5", "bowling-solitaire", "go-9x9", "courtfall-1.0.0"} {
		if !seen[id] {
			t.Errorf("no module %s", id)
		}
	}
}

// A family is registered once: twice is a build mistake.
func TestRegisteringTwicePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a second blooms was registered")
		}
	}()
	registry.Register("blooms", nil)
}

// Built with no_<package> tags, the server leaves those games out: their
// packages are not even compiled into it.
func TestATagLeavesAGameOut(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	deps := func(tags string) []string {
		t.Helper()
		args := []string{"list", "-deps", "-f", "{{.ImportPath}}"}
		if tags != "" {
			args = append(args, "-tags", tags)
		}
		out, err := exec.Command("go", append(args, "github.com/noderunner/bingle/internal/games/all")...).CombinedOutput()
		if err != nil {
			t.Fatalf("go list: %v\n%s", err, out)
		}
		return strings.Fields(string(out))
	}
	const tak, classics = "github.com/noderunner/bingle/internal/games/tak", "github.com/noderunner/bingle/internal/games/classics"
	if all := deps(""); !slices.Contains(all, tak) || !slices.Contains(all, classics) {
		t.Fatal("the default build lacks Tak or the classics")
	}
	if some := deps("no_tak,no_classics"); slices.Contains(some, tak) || slices.Contains(some, classics) {
		t.Error("no_tak,no_classics still builds them in")
	}
	// Nothing else pulls a game in: practice batches and tuning name no game
	// (ADR 0126), and without the tag "private" the server compiles nothing
	// of the private overlay, checked out or not (ADR 0127).
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "github.com/noderunner/bingle").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "internal/games/private") {
		t.Error("the server built without the tag private compiles the private overlay")
	}
	// And the server itself builds without them.
	if out, err := exec.Command("go", "build", "-tags", "no_tak,no_classics", "-o", "/dev/null", "github.com/noderunner/bingle").CombinedOutput(); err != nil {
		t.Errorf("the server without Tak and the classics: %v\n%s", err, out)
	}
}
