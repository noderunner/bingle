// Package tutorial reads a game's tutorials and proves them against the
// engine (docs/history/game-tutorials-plan.md on the plans branch).
//
// A tutorial is a hand-written YAML document in the game's own files
// (catalog.Module.Tutorials). It is played in a real room: the learner holds
// one seat, the page plays the scripted replies for the others, and every
// position is reached by playing moves, never written out by hand. That is
// what lets Verify replay every tutorial in the tests: a tutorial whose moves
// stop being legal after a rule change fails, rather than teaching the wrong
// rules.
package tutorial

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/dsl/schemagen"
)

// The kinds of tutorial a game can have. A big game splits its rules into
// several rules tutorials by topic, so a player can come back to one.
const (
	KindRules    = "rules"    // how to play, or one part of it
	KindStrategy = "strategy" // why a move is good
	KindVariant  = "variant"  // what a variant changes
)

// Tutorial is one tutorial of a game.
type Tutorial struct {
	// ID names the tutorial; unique among the game's tutorials.
	ID string `json:"id"`
	// Kind is rules, strategy or variant.
	Kind string `json:"kind"`
	// Title is what the list of tutorials shows; Summary is one line under it.
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	// Order sorts the game's tutorials of one kind (lowest first).
	Order int `json:"order,omitempty"`
	// Game is the game (or variant) the tutorial is played on, a member of
	// the family whose files hold it.
	Game string `json:"game"`
	// Seat is the learner's seat (default 1); the page plays every other.
	Seat int `json:"seat,omitempty"`
	// Seed fixes the chance of a game that has any (its dice), so every
	// roll of the scripted line comes out the same. Required for such a
	// game.
	Seed uint64 `json:"seed,omitempty"`
	// Setup are moves played, by whichever seat is to move, before the
	// first step: the way to reach a position worth teaching from.
	Setup []string `json:"setup,omitempty"`
	// Steps are shown one after another.
	Steps []Step `json:"steps"`
}

// Step is one page of a tutorial: some prose, and what happens on the board.
type Step struct {
	// Text is the step's prose, in Markdown (the game's guide images may be
	// used, as in its rules pages).
	Text string `json:"text"`
	// Play are the moves the learner is asked to make, in order, in the
	// game's notation as the engine writes it. The page offers only these.
	Play []string `json:"play,omitempty"`
	// Hint is shown when the learner asks for one ("Need a hint?").
	Hint string `json:"hint,omitempty"`
	// Reply are the moves played for the other seats after the learner's,
	// in order.
	Reply []string `json:"reply,omitempty"`
	// Highlight are squares (or areas) to pick out on the board.
	Highlight []string `json:"highlight,omitempty"`
	// Arrow draws an arrow from its first square to its second.
	Arrow []string `json:"arrow,omitempty"`
}

// SchemaID is the $id of docs/tutorial.schema.json; Required are the fields
// every tutorial has.
const SchemaID = "https://github.com/noderunner/bingle/docs/tutorial.schema.json"

// Required are the fields every tutorial has.
var Required = []string{"id", "kind", "title", "game", "steps"}

// learnerSeat is the seat the learner holds.
func (t Tutorial) learnerSeat() int {
	if t.Seat == 0 {
		return 1
	}
	return t.Seat
}

// Load reads every tutorial at the root of fsys (*.yaml), sorted by kind
// and order. Field names are checked as a game definition's are, with the
// file and line of a mistake.
func Load(fsys fs.FS) ([]Tutorial, error) {
	if fsys == nil {
		return nil, nil
	}
	files, err := fs.Glob(fsys, "*.yaml")
	if err != nil {
		return nil, err
	}
	var out []Tutorial
	seen := map[string]string{}
	for _, f := range files {
		data, err := dsl.LoadAs(fsys, f, reflect.TypeFor[Tutorial]())
		if err != nil {
			return nil, fmt.Errorf("tutorial: %w", err)
		}
		var t Tutorial
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&t); err != nil {
			return nil, fmt.Errorf("tutorial: %s: %w", f, err)
		}
		if err := t.check(); err != nil {
			return nil, fmt.Errorf("tutorial: %s: %w", f, err)
		}
		if other, dup := seen[t.ID]; dup {
			return nil, fmt.Errorf("tutorial: %s and %s are both %q", other, f, t.ID)
		}
		seen[t.ID] = f
		if want := strings.TrimSuffix(path.Base(f), ".yaml"); t.ID != want {
			return nil, fmt.Errorf("tutorial: %s holds %q: name the file after the tutorial", f, t.ID)
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ki, kj := kindRank(out[i].Kind), kindRank(out[j].Kind); ki != kj {
			return ki < kj
		}
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func kindRank(k string) int {
	switch k {
	case KindRules:
		return 0
	case KindStrategy:
		return 1
	}
	return 2
}

// check is what a tutorial needs before any engine is asked.
func (t Tutorial) check() error {
	switch {
	case strings.TrimSpace(t.ID) == "":
		return fmt.Errorf("a tutorial needs an id")
	case t.Kind != KindRules && t.Kind != KindStrategy && t.Kind != KindVariant:
		return fmt.Errorf("%s: kind %q is not rules, strategy or variant", t.ID, t.Kind)
	case strings.TrimSpace(t.Title) == "":
		return fmt.Errorf("%s: a tutorial needs a title", t.ID)
	case strings.TrimSpace(t.Game) == "":
		return fmt.Errorf("%s: a tutorial names the game it is played on", t.ID)
	case len(t.Steps) == 0:
		return fmt.Errorf("%s: a tutorial needs steps", t.ID)
	case t.Seat < 0:
		return fmt.Errorf("%s: seat %d", t.ID, t.Seat)
	}
	for i, s := range t.Steps {
		if strings.TrimSpace(s.Text) == "" {
			return fmt.Errorf("%s: step %d has no text", t.ID, i+1)
		}
		if s.Hint != "" && len(s.Play) == 0 {
			return fmt.Errorf("%s: step %d has a hint but asks for no move", t.ID, i+1)
		}
		if len(s.Arrow) != 0 && len(s.Arrow) != 2 {
			return fmt.Errorf("%s: step %d: an arrow is two squares, from and to", t.ID, i+1)
		}
	}
	return nil
}

// Schema returns docs/tutorial.schema.json, reading doc comments from the
// package's source in dir.
func Schema(dir string) ([]byte, error) {
	return schemagen.GenerateFor(reflect.TypeFor[Tutorial](), dir, SchemaID, "Bingle tutorial",
		"A tutorial of a bingle game, in YAML. Generated from internal/tutorial: do not edit.", Required)
}
