package batch

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// A Study is what a game family adds to the practice batches of its games,
// and to the tuning searches built on them: a pick for each seat (Cthulhu
// Wars' cults), a closer look at every game than who won, a report of its
// own, and knobs on its bots' guide. A game without one is batched all the
// same, and its report holds what every game has: the outcome per seat and per
// bot, the length, and how the games ended. The package names no game: a
// family registers its study with RegisterStudy, from the same file of
// internal/games/all that registers the game, so a build that leaves the game
// out leaves its study out too.
type Study interface {
	// Table is the game a table is played as: base with picks, one per seat
	// in seat order, and the options asked for (Cthulhu Wars' "hp"). picks is
	// all empty for a table that picks nothing, which plays base as it is
	// unless an option says otherwise. A pick or an option the game does not
	// know is an error.
	Table(base string, picks []string, options []string) (string, error)
	// Observer watches one game of a batch.
	Observer(game *catalog.CompiledGame) Observer
	// Report is the game's own part of a batch's report, as top-level fields
	// beside the common ones (Cthulhu Wars' "factions" and "area_names").
	Report(game *catalog.CompiledGame, recs []Record) (map[string]json.RawMessage, error)

	// Knobs are the guide's numbers a tuning may set.
	Knobs() []Knob
	// Shipped is the tuning a pick's bots play with when nothing says
	// otherwise.
	Shipped(pick string) Tuning
	// Guide is the moves of legal the guide would have seat consider, under
	// tuning t: the bot then chooses among them.
	Guide(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Selection, t Tuning) []state.Selection
	// Measures are what a tuning search may count besides wins, with the
	// weight each has unless the search says otherwise.
	Measures() []Measure
	// Score is how the seat that played pick fared in one game: whether it
	// won and each measure's value. ok is false when pick did not play.
	Score(rec Record, pick string) (won bool, values map[string]float64, ok bool)
}

// Observer watches one game: every applied selection, then the end. Finish
// returns the measurements to keep with the game's record, and the plies and
// rounds as the game counts them (0 rounds for a game without rounds).
type Observer interface {
	Observe(before, after *state.BoardState, seat int, mv state.Selection)
	Finish(final *state.BoardState, winner int, reason string) (metrics json.RawMessage, plies, rounds int)
}

// Tuning sets some of a game's knobs, by name; a knob it leaves out keeps the
// guide's own number.
type Tuning map[string]float64

// Knob is one number of a game's guide a tuning may set, and the range worth
// searching.
type Knob struct {
	Name     string
	Min, Max float64
}

// Measure is something a tuning search may count besides wins: its Weight in
// the fitness, against a win's 1, and the Scale its value is divided by first
// (Cthulhu Wars' Doom out of 30).
type Measure struct {
	Name   string
	Scale  float64
	Weight float64
}

var (
	studiesMu sync.Mutex
	studies   = map[string]Study{}
)

// RegisterStudy gives family's games their study. Registering twice is a
// build mistake, and panics.
func RegisterStudy(family string, s Study) {
	studiesMu.Lock()
	defer studiesMu.Unlock()
	if _, dup := studies[family]; dup {
		panic(fmt.Sprintf("batch: study of %q registered twice", family))
	}
	studies[family] = s
}

// StudyOf is the study of game's family, or nil.
func StudyOf(game *catalog.CompiledGame) Study {
	if game == nil {
		return nil
	}
	fam := game.Definition.Family
	if fam == "" {
		fam = game.Definition.ID
	}
	studiesMu.Lock()
	defer studiesMu.Unlock()
	return studies[fam]
}
