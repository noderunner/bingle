// Package catalog compiles a DSL game definition, together with the
// primitive library and an optional Go-callback escape hatch, into a
// playable CompiledGame, per docs/history/plan.md section 6.3a and docs/history/backlog.md
// item 07.
//
// Compilation resolves everything that can be resolved statically — the
// board graph, movement primitives, action parameters, win rules — and
// cross-checks every name a definition uses (piece types, regions, action
// IDs, ...), so a definition that registers successfully cannot fail later
// for a static reason.
package catalog

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/callback"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

// semver is the form of a game's version: major.minor.patch.
var semver = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// Module bundles a game module's registration inputs — its catalog ID, raw
// definition JSON (typically embedded via go:embed by the module's own
// package), and optional Go-callback escape hatch — so a caller can register
// it with a single Catalog.Register(mod.ID, mod.Definition, mod.Callbacks)
// call. Callbacks may be nil; Register substitutes callback.NoopCallbacks in
// that case. See docs/history/plan.md section 6.5 and docs/history/backlog.md item 09.
type Module struct {
	ID         string
	Definition []byte
	Callbacks  callback.GameCallbacks
	// Rules is the game's Go ruleset (see state.Ruleset), required exactly
	// when the definition says engine: "ruleset".
	Rules any
	// Assets is the module's embedded files: the Markdown pages and images
	// of its rules guide.
	Assets fs.FS
	// Tutorials holds the game's tutorials, one YAML document each at its
	// root (see package tutorial); nil for a game without any.
	Tutorials fs.FS
}

// DerivedSep separates the id of a base game from the suffix naming the
// setup options of a derived game ("cthulhu-wars-5p~gc-cc-bg-ys-ww").
const DerivedSep = "~"

// Deriver is implemented by the ruleset of a game that takes setup options
// (see dsl.GameDefinition.Options). Every combination of options is a game
// definition of its own, derived from the base document by a merge patch and
// registered on demand under base~suffix: the sessions, the API, the bots and
// the clients see an ordinary game.
type Deriver interface {
	// Derive checks options against the base definition and returns the
	// canonical suffix that names the combination (stable; letters, digits
	// and dashes only) and the merge patch that turns the base document into
	// the derived game.
	Derive(base *dsl.GameDefinition, options map[string]any) (suffix string, patch map[string]any, err error)
	// FromSuffix rebuilds the patch of a suffix Derive returned: a game stored
	// under a derived id is loaded through it.
	FromSuffix(base *dsl.GameDefinition, suffix string) (patch map[string]any, err error)
}

// deriverFor returns what builds the derived games of a base game: the
// ruleset's own Deriver, or the built-in one of the pie-rule option for a game
// whose only choice that is.
func deriverFor(src source, base *CompiledGame) (Deriver, bool) {
	if d, ok := src.mod.Rules.(Deriver); ok {
		return d, true
	}
	if base != nil && pieChoice(base.Definition.PieRule) {
		return pieDeriver{}, true
	}
	return nil, false
}

// source remembers the document and module a registered game came from, so a
// game derived from it can be built later.
type source struct {
	doc []byte
	mod Module
}

// Catalog holds every CompiledGame registered under a game-definition ID.
type Catalog struct {
	mu    sync.RWMutex
	games map[string]*CompiledGame
	src   map[string]source
	// dmu serialises the building of derived games.
	dmu sync.Mutex
	// retired names the games this server no longer offers, by id.
	retired map[string]string
}

// NewCatalog returns an empty Catalog ready for Register/Get calls.
func NewCatalog() *Catalog {
	return &Catalog{
		games: make(map[string]*CompiledGame),
		src:   make(map[string]source),
	}
}

// RegisterModule registers mod, expanding any variants its definition
// declares into sibling games (each registered under its own ID with the
// same callbacks). It stops at, and returns, the first error.
func (c *Catalog) RegisterModule(mod Module) error {
	docs, err := dsl.ExpandVariants(mod.Definition)
	if err != nil {
		return fmt.Errorf("catalog: %w", err)
	}
	for i, doc := range docs {
		id := mod.ID
		if i > 0 {
			parsed, err := dsl.Parse(doc)
			if err != nil {
				return fmt.Errorf("catalog: %w", err)
			}
			id = parsed.ID
		}
		if err := c.RegisterWith(id, doc, mod.Callbacks, mod.Rules); err != nil {
			return err
		}
		c.mu.Lock()
		cg := c.games[id]
		cg.Assets = mod.Assets
		c.src[id] = source{doc: doc, mod: mod}
		c.mu.Unlock()
		if g := cg.Definition.Guide; g != nil {
			if mod.Assets == nil {
				return fmt.Errorf("catalog: game %q has a guide but its module brings no assets", id)
			}
			seen := map[string]bool{}
			for _, tab := range g.Tabs {
				if tab.ID == "" || tab.Title == "" || tab.File == "" {
					return fmt.Errorf("catalog: game %q has a guide tab without id, title or file", id)
				}
				if seen[tab.ID] {
					return fmt.Errorf("catalog: game %q has two guide tabs named %q", id, tab.ID)
				}
				seen[tab.ID] = true
				if _, err := fs.Stat(mod.Assets, tab.File); err != nil {
					return fmt.Errorf("catalog: game %q guide tab %q: %w", id, tab.ID, err)
				}
			}
		}
	}
	return nil
}

// Register parses defJSON, resolves its board and pieces against the board
// and primitives packages, and stores the resulting CompiledGame under id,
// replacing any prior entry for id.
//
// Register is atomic: if any step fails, it returns a non-nil error and
// leaves the catalog's existing entry for id (if any) completely untouched.
func (c *Catalog) Register(id string, defJSON []byte, cb callback.GameCallbacks) error {
	return c.RegisterWith(id, defJSON, cb, nil)
}

// RegisterWith is Register for a game that also brings a Go ruleset (see
// Module.Rules).
func (c *Catalog) RegisterWith(id string, defJSON []byte, cb callback.GameCallbacks, rules any) error {
	def, err := dsl.Parse(defJSON)
	if err != nil {
		return fmt.Errorf("catalog: %w", err)
	}

	if def.Version != "" && !semver.MatchString(def.Version) {
		return fmt.Errorf("catalog: game %q has version %q, which is not major.minor.patch", id, def.Version)
	}

	// The pie rule brings an action and a setup option with it (ADR 0056).
	if err := applyPieRule(id, def); err != nil {
		return err
	}

	g, err := resolveBoard(def)
	if err != nil {
		return err
	}

	pieces, err := resolvePieces(def)
	if err != nil {
		return err
	}

	callbacks := cb
	if callbacks == nil {
		callbacks = callback.NoopCallbacks{}
	}

	compiled := &CompiledGame{
		Definition: def,
		Board:      g,
		Pieces:     pieces,
		Callbacks:  callbacks,
		Rules:      rules,
	}
	if def.Engine == "ruleset" && rules == nil {
		return fmt.Errorf("catalog: game %q declares engine \"ruleset\" but was registered without one", id)
	}
	if def.Engine != "" && def.Engine != "ruleset" {
		return fmt.Errorf("catalog: game %q has unknown engine %q", id, def.Engine)
	}
	if def.Engine == "" && rules != nil {
		return fmt.Errorf("catalog: game %q was registered with a ruleset but does not declare engine \"ruleset\"", id)
	}
	for name := range pieces {
		compiled.PieceTypes = append(compiled.PieceTypes, name)
	}
	sort.Strings(compiled.PieceTypes)

	if g.IsGrid() {
		mode := def.Board.Generator.Adjacency
		compiled.Directions, _ = board.GridDirections(mode)
	}

	if err := compileRules(compiled); err != nil {
		return err
	}
	if err := checkPreview(compiled); err != nil {
		return err
	}

	c.mu.Lock()
	c.games[id] = compiled
	c.mu.Unlock()

	return nil
}

// Get returns the CompiledGame registered under id, and whether it was
// found. A derived id (base~suffix) is built from its base game the first
// time it is asked for.
func (c *Catalog) Get(id string) (*CompiledGame, bool) {
	c.mu.RLock()
	cg, ok := c.games[id]
	c.mu.RUnlock()
	if ok {
		return cg, true
	}
	if base, suffix, found := strings.Cut(id, DerivedSep); found && base != "" && suffix != "" {
		if cg, err := c.derive(base, suffix, nil); err == nil {
			return cg, true
		}
	}
	return nil, false
}

// Tutorials returns the family a game belongs to (its base game: the
// family of a variant, the base of a derived game) and the tutorials that
// family's module carries (nil when it has none). ok is false for an unknown
// game.
func (c *Catalog) Tutorials(id string) (family string, fsys fs.FS, ok bool) {
	cg, ok := c.Get(id)
	if !ok {
		return "", nil, false
	}
	family = cg.Definition.Family
	if family == "" {
		family, _, _ = strings.Cut(cg.Definition.ID, DerivedSep)
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return family, c.src[family].mod.Tutorials, true
}

// Retire records that the game id is no longer offered, under the name it
// had. Games of it already played stay readable as a record (see Retired);
// nothing can be played in them.
func (c *Catalog) Retire(id, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.retired == nil {
		c.retired = map[string]string{}
	}
	c.retired[id] = name
}

// Retired reports whether id is a retired game that is not registered, and
// the name it had. A derived id (base~suffix) is retired with its base.
func (c *Catalog) Retired(id string) (name string, ok bool) {
	base, _, _ := strings.Cut(id, DerivedSep)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, registered := c.games[id]; registered {
		return "", false
	}
	name, ok = c.retired[base]
	return name, ok
}

// RetiredIDs lists the retired games that are not registered.
func (c *Catalog) RetiredIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []string
	for id := range c.retired {
		if _, registered := c.games[id]; !registered {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Derive turns the setup options of a game into the id of the game they
// describe, building (and keeping) its definition. Without options it returns
// baseID itself. It fails when the game takes no options or rejects them.
func (c *Catalog) Derive(baseID string, options map[string]any) (string, error) {
	if len(options) == 0 {
		return baseID, nil
	}
	c.mu.RLock()
	src, ok := c.src[baseID]
	base := c.games[baseID]
	c.mu.RUnlock()
	if !ok || base == nil {
		return "", fmt.Errorf("catalog: game %q takes no setup options", baseID)
	}
	d, ok := deriverFor(src, base)
	if !ok {
		return "", fmt.Errorf("catalog: game %q takes no setup options", baseID)
	}
	suffix, _, err := d.Derive(base.Definition, options)
	if err != nil {
		return "", err
	}
	if suffix == "" {
		// The options describe the game's own layout: there is nothing to derive.
		return baseID, nil
	}
	id := baseID + DerivedSep + suffix
	if _, err := c.derive(baseID, suffix, options); err != nil {
		return "", err
	}
	return id, nil
}

// derive builds the game base~suffix from base's document. options, when
// non-nil, are the options the suffix came from (they are re-checked).
func (c *Catalog) derive(baseID, suffix string, options map[string]any) (*CompiledGame, error) {
	id := baseID + DerivedSep + suffix
	c.dmu.Lock()
	defer c.dmu.Unlock()
	c.mu.RLock()
	if cg, ok := c.games[id]; ok {
		c.mu.RUnlock()
		return cg, nil
	}
	src, ok := c.src[baseID]
	base := c.games[baseID]
	c.mu.RUnlock()
	if !ok || base == nil {
		return nil, fmt.Errorf("catalog: no game %q to derive from", baseID)
	}
	d, ok := deriverFor(src, base)
	if !ok {
		return nil, fmt.Errorf("catalog: game %q takes no setup options", baseID)
	}
	patch, err := d.FromSuffix(base.Definition, suffix)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(src.doc, &doc); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	merged, ok := dsl.MergePatch(doc, any(patch)).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("catalog: derived document of %q is not an object", id)
	}
	merged["id"] = id
	if _, set := merged["family"]; !set {
		merged["family"] = baseID
	}
	// A derived game is not itself a base: its options are settled.
	delete(merged, "options")
	out, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	if err := c.RegisterWith(id, out, src.mod.Callbacks, src.mod.Rules); err != nil {
		return nil, err
	}
	c.mu.Lock()
	cg := c.games[id]
	cg.Assets = src.mod.Assets
	c.mu.Unlock()
	return cg, nil
}

// List returns every registered game, sorted by Definition.ID for
// determinism. Derived games (see Derive) are not listed: they exist only for
// the combinations somebody chose.
func (c *Catalog) List() []*CompiledGame {
	c.mu.RLock()
	defer c.mu.RUnlock()

	games := make([]*CompiledGame, 0, len(c.games))
	for id, cg := range c.games {
		if strings.Contains(id, DerivedSep) {
			continue
		}
		games = append(games, cg)
	}
	sort.Slice(games, func(i, j int) bool {
		return games[i].Definition.ID < games[j].Definition.ID
	})
	return games
}
