// Package tutorial reads a game's tutorials and proves them against the
// engine (docs/history/game-tutorials-plan.md on the plans branch).
//
// A tutorial is a hand-written YAML document in the game's own files
// (catalog.Module.Tutorials). It is played in a real room: the learner holds
// one seat, the page plays the scripted replies for the others, and the game
// is reached by playing moves. That is what lets Verify replay every tutorial
// in the tests: a tutorial whose moves stop being legal after a rule change
// fails, rather than teaching the wrong rules. A step may also draw a picture
// that is no part of the game, and such a picture need not be a position the
// rules can reach (ADR 0084).
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
	// VariantOnly says the tutorial teaches the game it names and no other of its
	// family, so it is offered only with that game. Without it a tutorial is
	// offered for the whole family, and Game is merely the board it is played on.
	VariantOnly bool `json:"variant_only,omitempty"`
	// Seat is the learner's seat (default 1); the page plays every other.
	Seat int `json:"seat,omitempty"`
	// Seed fixes the chance of a game that has any (its dice), so every
	// roll of the scripted line comes out the same. Required for such a
	// game.
	Seed uint64 `json:"seed,omitempty"`
	// Setup are moves played, by whichever seat is to move, before the
	// first step: the way to reach a position worth teaching from.
	Setup []string `json:"setup,omitempty"`
	// Start is a position document the tutorial begins from instead of the
	// game's start — a game well into play, which would take hundreds of
	// moves to reach — usually kept in a file of its own with !include. The
	// game checks it as it checks any custom start; its own state of chance
	// is used, so no seed is needed. StartSeat is who moves first from it in
	// a game whose position does not say (a game without a Go ruleset).
	Start     json.RawMessage `json:"start,omitempty"`
	StartSeat int             `json:"start_seat,omitempty"`
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
	// Hint is shown when the learner asks for one ("Need a hint?"). On a step
	// with no move to make, it can answer a question the text asks.
	Hint string `json:"hint,omitempty"`
	// Reply are the moves played for the other seats after the learner's,
	// in order.
	Reply []string `json:"reply,omitempty"`
	// Highlight are squares (or areas) to pick out on the board.
	Highlight []string `json:"highlight,omitempty"`
	// Arrow draws an arrow from its first square to its second.
	Arrow []string `json:"arrow,omitempty"`
	// Spotlight are the parts of the game's page to pick out, by name: the
	// player's Power, the round's phases, the History. See PageParts. A part of a
	// player's panel means the learner's, or another seat's as doom:2.
	Spotlight []string `json:"spotlight,omitempty"`
	// Illegal marks the step's arrow and highlighted squares as what must not
	// be done: drawn red, with a cross. It needs one of them, and such a step
	// asks for no moves.
	Illegal bool `json:"illegal,omitempty"`
	// Show depicts another position for this step only — the end of a
	// game, say — reached by these moves from the game's start (seeded as
	// the tutorial is). The room's game is not touched. A step that shows a
	// position asks for no moves.
	Show []string `json:"show,omitempty"`
	// Position depicts a position written out as a document instead, for
	// this step only; like Show it asks for no moves.
	Position json.RawMessage `json:"position,omitempty"`
	// Pieces depicts the board with just these pieces on it, for this step only:
	// piece@square, or piece:color@square where the piece is not named after its
	// colour, bottom first where several share a square. It need not be a position
	// play can reach, and like Show it asks for no moves.
	Pieces []string `json:"pieces,omitempty"`
}

// pictured says the step depicts a position other than the room's.
func (s Step) pictured() bool {
	return len(s.Show) > 0 || len(s.Position) > 0 || len(s.Pieces) > 0
}

// PageParts are the names a step's Spotlight may use, and whether each belongs
// to a seat. They name what a part is rather than how a page draws it, so that
// a tutorial says nothing about the web page's markup and another client can
// show them its own way (ADR 0111). internal/web/static/js/tutorial.js says
// where the web page has each one; a test keeps the two lists the same.
var PageParts = map[string]bool{
	"actions":       false, // the buttons of the moves on offer
	"battle":        false, // the dice and results of the battle
	"elder-bag":     false, // the Elder Signs left in the bag
	"first-player":  false, // who plays first, and which way round
	"history":       false, // the record of the game's moves
	"history-since": false, // the player's last move in that record
	"impulse":       false, // who is to act, and what they are asked
	"options":       false, // the button that opens the game's options
	"phases":        false, // the steps of a round
	"ritual-track":  false, // the Ritual of Annihilation track
	"round-bar":     false, // the round, its phases and tracks
	"rules":         false, // the button that opens the rules
	"books":         true,  // how many spellbooks a seat holds
	"doom":          true,  // a seat's Doom
	"elder-signs":   true,  // a seat's Elder Signs, one by one
	"gates":         true,  // how many Gates a seat controls
	"power":         true,  // a seat's Power
	"signs":         true,  // how many Elder Signs a seat holds
	"spellbooks":    true,  // a seat's spellbooks and requirements
	"units":         true,  // a seat's units: those in its pool, and those not
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
		if len(s.Arrow) != 0 && len(s.Arrow) != 2 {
			return fmt.Errorf("%s: step %d: an arrow is two squares, from and to", t.ID, i+1)
		}
		if s.pictured() && (len(s.Play) > 0 || len(s.Reply) > 0) {
			return fmt.Errorf("%s: step %d shows a position, so it cannot ask for moves too", t.ID, i+1)
		}
		pictures := 0
		for _, set := range []bool{len(s.Show) > 0, len(s.Position) > 0, len(s.Pieces) > 0} {
			if set {
				pictures++
			}
		}
		if pictures > 1 {
			return fmt.Errorf("%s: step %d: show, position or pieces, only one of them", t.ID, i+1)
		}
		for _, part := range s.Spotlight {
			if err := checkPart(part); err != nil {
				return fmt.Errorf("%s: step %d: %w", t.ID, i+1, err)
			}
		}
		if s.Illegal && len(s.Arrow) == 0 && len(s.Highlight) == 0 {
			return fmt.Errorf("%s: step %d is marked illegal, but has no arrow or highlight to mark", t.ID, i+1)
		}
		if s.Illegal && (len(s.Play) > 0 || len(s.Reply) > 0) {
			return fmt.Errorf("%s: step %d is marked illegal, so it cannot ask for moves too", t.ID, i+1)
		}
	}
	return nil
}

// checkPart says whether a step's Spotlight may name part: name, or name:seat.
func checkPart(part string) error {
	name, seat, seated := strings.Cut(part, ":")
	ofSeat, known := PageParts[name]
	switch {
	case !known:
		return fmt.Errorf("%q is not a part of the page", name)
	case seated && !ofSeat:
		return fmt.Errorf("%q does not belong to a seat, as %q says", name, part)
	case seated && (seat == "" || strings.Trim(seat, "0123456789") != "" || seat[0] == '0'):
		return fmt.Errorf("%q: %q is not a seat", part, seat)
	}
	return nil
}

// Schema returns docs/tutorial.schema.json, reading doc comments from the
// package's source in dir.
func Schema(dir string) ([]byte, error) {
	return schemagen.GenerateFor(reflect.TypeFor[Tutorial](), dir, SchemaID, "Bingle tutorial",
		"A tutorial of a bingle game, in YAML. Generated from internal/tutorial: do not edit.", Required)
}
