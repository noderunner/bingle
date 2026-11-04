package catalog

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

// Each version of a prototype's rules is a game of its own, named
// family-version, with its variants family-version-name and its derived games
// family-version~suffix (ADR 0098). The id is what pins a game in progress to
// its rules, so it has to say the version it claims.

// checkPrototype checks that the id of a prototype says its version.
func checkPrototype(id string, def *dsl.GameDefinition) error {
	if !def.Prototype {
		return nil
	}
	if def.Family == "" || def.Version == "" {
		return fmt.Errorf("catalog: prototype %q needs a family and a version", id)
	}
	home := def.Home()
	if id != home && !strings.HasPrefix(id, home+"-") && !strings.HasPrefix(id, home+DerivedSep) {
		return fmt.Errorf("catalog: prototype %q of version %s has to be named %q", id, def.Version, home)
	}
	return nil
}

// noteVersion records the base game of a module if it is the newest version of
// its prototype so far.
func (c *Catalog) noteVersion(id string, def *dsl.GameDefinition) error {
	if !def.Prototype {
		return nil
	}
	if id != def.Home() {
		return fmt.Errorf("catalog: prototype %q of version %s has to be named %q", id, def.Version, def.Home())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cur, ok := c.games[c.newest[def.Family]]; !ok || compareVersions(def.Version, cur.Definition.Version) > 0 {
		c.newest[def.Family] = id
	}
	return nil
}

// Pin returns the id a game is stored under: a prototype's name alone (or a
// game derived from it) becomes its newest version, and any other id is itself.
func (c *Catalog) Pin(id string) string {
	base, suffix, derived := strings.Cut(id, DerivedSep)
	c.mu.RLock()
	newest, ok := c.newest[base]
	c.mu.RUnlock()
	switch {
	case !ok:
		return id
	case derived:
		return newest + DerivedSep + suffix
	}
	return newest
}

// compareVersions orders two major.minor.patch versions: numerically, so that
// 1.10.0 comes after 1.9.0. RegisterWith has checked their form.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range min(len(as), len(bs)) {
		x, _ := strconv.Atoi(as[i])
		y, _ := strconv.Atoi(bs[i])
		if x != y {
			return x - y
		}
	}
	return 0
}
