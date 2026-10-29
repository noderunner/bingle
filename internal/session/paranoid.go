package session

import (
	"context"
	"log/slog"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/store"
)

// Paranoid mode: the engine checked against itself after every move.
//
// BINGLE_PARANOID=1 is for development and CI, never production — it runs
// Legal a second time on every move, which the plan says plainly and the
// configuration repeats. What it catches is the class of bug that is
// otherwise found by a player: Apply accepting a move Legal did not list, a
// position that no longer validates, notation that does not survive a round
// trip, and a ruleset leaking a secret into the view everyone sees.
//
// Each failure is one ERROR line naming what broke. Writing a repro bundle
// beside it is Stage 7 of docs/history/observability-plan.md; until then the
// line is the record.

// checkParanoid runs the assertions after a move has been applied and
// recorded. before is the position the move was applied to.
func (m *Manager) checkParanoid(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, before *state.BoardState, seat int, result *state.MoveResult) {
	lg := m.logger()
	fail := func(what string, extra ...slog.Attr) {
		attrs := append([]slog.Attr{
			slog.String("check", what),
			slog.String("game", gameFamily(compiled)),
			logging.Room(game.RoomCode),
			slog.Int("seat", seat),
			slog.String("move", result.Notation),
		}, extra...)
		lg.LogAttrs(ctx, slog.LevelError, "paranoid mode found an inconsistency", attrs...)
	}

	// 1. Apply accepted a move Legal did not list. This is the one that makes
	//    a game unplayable in a way nobody can reproduce.
	listed := false
	want := result.Move.Key()
	for _, mv := range state.LegalMoves(compiled, before, seat) {
		if mv.Key() == want {
			listed = true
			break
		}
	}
	if !listed {
		fail("the applied move was not in Legal's list")
	}

	// 2. The position after the move still validates.
	if err := state.Validate(compiled, result.NewState); err != nil {
		fail("the position does not validate after the move", slog.String("err", err.Error()))
	}

	// 3. The notation survives Format -> Parse -> Format, or a replay of this
	//    game will not reproduce it.
	if result.Notation != "" {
		parsed, err := state.ParseMove(compiled, result.Notation)
		switch {
		case err != nil:
			fail("the notation does not parse back", slog.String("notation", result.Notation), slog.String("err", err.Error()))
		default:
			if again := state.FormatMove(compiled, parsed); again != result.Notation {
				fail("the notation does not round-trip",
					slog.String("notation", result.Notation), slog.String("again", again))
			}
		}
	}

	// 4. The public view really is public. A ruleset that forgets to redact
	//    hands everyone the committed dice, the hidden hand or the RNG state,
	//    and nothing else in the server would notice.
	pub := state.Public(compiled, result.NewState, 0)
	if pub == result.NewState && state.IsRuleset(compiled) && hasSecrets(compiled, result.NewState) {
		fail("the ruleset returned the full position as its public view")
	}
}

// hasSecrets reports whether the position has anything a ruleset ought to be
// redacting. It is deliberately conservative: it asks the ruleset itself, by
// comparing the view a seat gets with the view everyone gets, and only
// complains when a seat can see something the public view shows as well.
func hasSecrets(compiled *catalog.CompiledGame, st *state.BoardState) bool {
	public, err := state.Public(compiled, st, 0).MarshalJSON()
	if err != nil {
		return false
	}
	// Seat 1's view: if it differs from the public one, the ruleset is
	// redacting something and the public view is not the full position.
	seat, err := state.Public(compiled, st, 1).MarshalJSON()
	if err != nil {
		return false
	}
	return string(public) != string(seat)
}
