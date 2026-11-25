package bots

import (
	"context"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// searchSafety is the most time a search inside the rules may take where
// neither the bot nor its context sets a limit: tactical and minimax, which
// are bounded by positions, in a practice batch.
const searchSafety = time.Minute

// inRules hands a decision to the game's ruleset when it searches for itself
// (a pack's module, ADR 0147), so that the search crosses into the module
// once rather than at every position. ok is false when the ruleset does not
// search, or failed, or answered with a selection that is not one of legal:
// the bot then searches as usual. err is set only when ctx has ended.
func inRules(ctx context.Context, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Selection, bot string, req state.SearchRequest) (ev Evaluation, ok bool, err error) {
	s, ok := state.SearcherOf(game)
	if !ok {
		return Evaluation{}, false, nil
	}
	res, err := s.Search(ctx, game, st, seat, req)
	if err != nil {
		if ctx.Err() != nil {
			return Evaluation{}, false, ctx.Err()
		}
		return Evaluation{}, false, nil
	}
	if !listed(legal, res.Move) {
		return Evaluation{}, false, nil
	}
	addNodes(bot, res.Visits)
	ev = Evaluation{Move: res.Move, Heuristic: res.Heuristic, WinRate: res.WinRate, Visits: res.Visits}
	for _, c := range res.Alternatives {
		ev.Alternatives = append(ev.Alternatives, Candidate{Move: c.Move, WinRate: c.WinRate, Visits: c.Visits})
	}
	if len(ev.Alternatives) == 0 {
		ev.Alternatives = []Candidate{{Move: res.Move, WinRate: res.WinRate, Visits: res.Visits}}
	}
	return ev, true, nil
}

// listed reports whether mv is one of legal.
func listed(legal []state.Selection, mv state.Selection) bool {
	key := mv.Key()
	for _, l := range legal {
		if l.Key() == key {
			return true
		}
	}
	return false
}
