// Package registry is the set of game families a binary is built with. Each
// family is registered once, by name, from a file of package all that a build
// tag can leave out (no_<package>, such as no_tak): which games a
// binary has is chosen when it is built, and nothing outside package all names
// a game to include it (game plugins plan, step 2).
//
// A family is registered as a function, called only when the modules are
// wanted, so registering costs nothing at start-up.
package registry

import (
	"fmt"
	"sort"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Family returns the catalog modules of one game family: its game, or each
// of its versions (a prototype), or the several games one package ships.
type Family func() []catalog.Module

var (
	mu       sync.Mutex
	families = map[string]Family{}
)

// Register adds a family under name. Two families of one name is a build
// mistake, and panics.
func Register(name string, f Family) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := families[name]; dup {
		panic(fmt.Sprintf("registry: family %q registered twice", name))
	}
	families[name] = f
}

// Names lists the registered families, sorted.
func Names() []string {
	mu.Lock()
	defer mu.Unlock()
	out := make([]string, 0, len(families))
	for name := range families {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Modules returns every registered family's modules, family by family in the
// order of Names.
func Modules() []catalog.Module {
	var out []catalog.Module
	for _, name := range Names() {
		mu.Lock()
		f := families[name]
		mu.Unlock()
		out = append(out, f()...)
	}
	return out
}
