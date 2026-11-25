package state

import (
	"context"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Searcher is implemented by rulesets that run a built-in bot's search
// themselves: a pack's module (ADR 0147), where a search through Legal and
// Apply would cross into the module at every position it looks at, and
// searching inside it crosses once a decision. The bots hand it the decision
// when the game's ruleset is one, and search as usual when it is not or when
// it fails.
type Searcher interface {
	// Search chooses a selection for seat. It stops when ctx ends, and keeps
	// to the request's Budget. The same request gives the same answer unless
	// the time ran out first.
	Search(ctx context.Context, game *catalog.CompiledGame, st *BoardState, seat int, req SearchRequest) (SearchResult, error)
}

// SearchRequest is a bot's decision as it is handed to a Searcher: its
// level and its limits. A zero limit takes the bot's default.
type SearchRequest struct {
	// Level is the bot that searches: tactical, minimax or mcts.
	Level string
	// Budget is the time the search may take; 0 is no limit but the others.
	Budget time.Duration
	// Iterations caps the playouts of mcts, Nodes the positions minimax looks
	// at and Depth how deep it goes, Plies a playout of mcts.
	Iterations, Nodes, Depth, Plies int
	// Exploration is the UCT constant of mcts.
	Exploration float64
	Seed        uint64
}

// SearchResult is a Searcher's answer: the selection and its estimate, as a
// bot's evaluation has them.
type SearchResult struct {
	Move Selection
	// WinRate is the move's chance of winning for seat, in [0, 1]: a squashed
	// score where Heuristic is set.
	WinRate   float64
	Heuristic bool
	// Visits is the positions or playouts behind it.
	Visits       int
	Alternatives []SearchCandidate
}

// SearchCandidate is one selection a search weighed.
type SearchCandidate struct {
	Move    Selection
	WinRate float64
	Visits  int
}

// SearcherOf returns game's ruleset as a Searcher, if it is one.
func SearcherOf(game *catalog.CompiledGame) (Searcher, bool) {
	rs, ok := RulesetOf(game)
	if !ok {
		return nil, false
	}
	s, ok := rs.(Searcher)
	return s, ok
}
