package repro

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Result is what replaying a bundle found.
type Result struct {
	// Reproduced says the same impossible thing happened again. That is the
	// answer the bundle exists to give: if it is false, either the bug is
	// fixed or the bundle does not carry enough to show it, and Notes says
	// which.
	Reproduced bool
	// Error is what happened this time, as text.
	Error string
	// Notes are everything else worth saying, in the order it was found.
	Notes []string
}

func (r *Result) note(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
}

// String is the report a person reads.
func (r Result) String() string {
	var b strings.Builder
	if r.Reproduced {
		b.WriteString("reproduced")
	} else {
		b.WriteString("NOT reproduced")
	}
	if r.Error != "" {
		b.WriteString(": ")
		b.WriteString(r.Error)
	}
	for _, n := range r.Notes {
		b.WriteString("\n  · ")
		b.WriteString(n)
	}
	return b.String()
}

// Replay reproduces a bundle in process: no server, no browser, no database.
//
// cat must hold the game the bundle names — scripts/replay builds one from
// app.BuiltinModules(), and so does TestRepro.
func Replay(cat *catalog.Catalog, b Bundle) (Result, error) {
	var res Result

	compiled, ok := cat.Get(b.GameDefID)
	if !ok {
		return res, fmt.Errorf("repro: this build has no game %q (the bundle was written by bingle %s)", b.GameDefID, b.Bingle)
	}

	st := state.NewBoardState()
	if err := st.UnmarshalJSON(b.Position); err != nil {
		return res, fmt.Errorf("repro: the bundle's position will not load: %w", err)
	}

	// How the position was reached is often the actual bug, so when the
	// bundle says, check it still leads there. A divergence is a finding of
	// its own and does not stop the replay.
	if len(b.Moves) > 0 {
		checkPath(cat, compiled, b, st, &res)
	}

	mv, haveMove := bundleMove(compiled, b, &res)

	switch b.Kind {
	case ApplyRejectedALegalMove, BatchIllegalMove:
		if !haveMove {
			res.note("the bundle carries no move, so there is nothing to apply")
			return res, nil
		}
		listed := legalLists(compiled, st, b.Seat, mv)
		_, err := applySafely(compiled, st, b.Seat, mv)
		switch {
		case err == nil:
			res.note("Apply accepted the move this time: either the bug is fixed, or the rules have changed")
		case !listed:
			res.Error = err.Error()
			res.note("Apply still refuses the move, but Legal no longer lists it either — which is consistent, so this is no longer the same fault")
		default:
			res.Reproduced = true
			res.Error = err.Error()
			res.note("Legal lists the move and Apply refuses it")
		}

	case LegalDidNotListAnAppliedMove:
		if !haveMove {
			res.note("the bundle carries no move, so there is nothing to apply")
			return res, nil
		}
		listed := legalLists(compiled, st, b.Seat, mv)
		_, err := applySafely(compiled, st, b.Seat, mv)
		switch {
		case err != nil:
			res.Error = err.Error()
			res.note("Apply refuses the move now, so it and Legal agree: this is no longer the same fault")
		case listed:
			res.note("Legal lists the move now: either the bug is fixed, or the rules have changed")
		default:
			res.Reproduced = true
			res.note("Apply accepts a move Legal does not list")
		}

	case ValidateFailed:
		after := st
		if haveMove {
			r, err := applySafely(compiled, st, b.Seat, mv)
			if err != nil {
				res.Error = err.Error()
				res.note("the move will not apply at all now, so the position after it cannot be checked")
				return res, nil
			}
			after = r.NewState
		}
		if err := state.Validate(compiled, after); err != nil {
			res.Reproduced = true
			res.Error = err.Error()
			res.note("the position still does not validate")
		} else {
			res.note("the position validates now")
		}

	case NotationDidNotRoundTrip:
		if b.Notation == "" {
			res.note("the bundle carries no notation")
			return res, nil
		}
		parsed, err := state.ParseMove(compiled, b.Notation)
		if err != nil {
			res.Reproduced = true
			res.Error = err.Error()
			res.note("the notation %q still will not parse", b.Notation)
			return res, nil
		}
		if again := state.FormatMove(compiled, parsed); again != b.Notation {
			res.Reproduced = true
			res.note("%q parses and formats back as %q", b.Notation, again)
		} else {
			res.note("the notation round-trips now")
		}

	case PublicViewLeaked:
		if state.Public(compiled, st, 0) == st {
			res.Reproduced = true
			res.note("the public view is still the whole position")
		} else {
			res.note("the public view is redacted now")
		}

	case RulesetPanicked:
		_, err := applySafely(compiled, st, b.Seat, mv)
		if err != nil && strings.Contains(err.Error(), "panicked") {
			res.Reproduced = true
			res.Error = err.Error()
			res.note("the ruleset still panics")
		} else if err != nil {
			res.Error = err.Error()
			res.note("the ruleset returns an error now rather than panicking, which is an improvement but not the same fault")
		} else {
			res.note("the move applies cleanly now")
		}

	case BatchNoLegalMoves:
		legal := state.LegalMoves(compiled, st, b.Seat)
		switch {
		case len(legal) > 0:
			res.note("seat %d has %d legal moves now", b.Seat, len(legal))
		case gameOver(compiled, st):
			res.note("the position has no moves, but a win rule now ends the game, which is the hole closed")
		default:
			res.Reproduced = true
			res.note("seat %d still has no legal moves and no win rule ends the game", b.Seat)
		}

	default:
		return res, fmt.Errorf("repro: this build does not know how to replay a %q bundle", b.Kind)
	}

	return res, nil
}

// gameOver reports whether the position is already finished.
//
// A Go ruleset answers this itself. A game written in the DSL has no such
// question to ask: its win rules are evaluated when a move is applied, and a
// position with no moves and no rule to end it is precisely the hole a
// batch-no-legal-moves bundle is about — so for those this says "not over",
// which is the honest answer.
func gameOver(compiled *catalog.CompiledGame, st *state.BoardState) bool {
	if rs, ok := state.RulesetOf(compiled); ok {
		return rs.Outcome(compiled, st).Ended
	}
	return false
}

// checkPath replays the recorded notations from the recorded start and reports
// whether they still lead to the recorded position.
func checkPath(cat *catalog.Catalog, compiled *catalog.CompiledGame, b Bundle, want *state.BoardState, res *Result) {
	st := state.NewBoardState()
	switch {
	case len(b.StartState) > 0:
		if err := st.UnmarshalJSON(b.StartState); err != nil {
			res.note("the bundle's start position will not load (%v), so the path was not checked", err)
			return
		}
	case b.Seed != 0:
		seeded, err := state.InitialStateSeeded(compiled, b.Seed)
		if err != nil {
			res.note("the opening position for seed %d will not build (%v), so the path was not checked", b.Seed, err)
			return
		}
		st = seeded
	default:
		st = state.InitialState(compiled)
	}

	seat := state.FirstSeat(compiled)
	for i, notation := range b.Moves {
		mv, err := state.ParseMove(compiled, notation)
		if err != nil {
			res.note("move %d of the path, %q, no longer parses (%v)", i+1, notation, err)
			return
		}
		r, err := applySafely(compiled, st, seat, mv)
		if err != nil {
			res.note("move %d of the path, %q, no longer applies (%v) — the way the position was reached has itself changed", i+1, notation, err)
			return
		}
		st, seat = r.NewState, r.NextTurnSeat
	}

	got, err1 := st.MarshalJSON()
	expected, err2 := want.MarshalJSON()
	switch {
	case err1 != nil || err2 != nil:
		res.note("the replayed position could not be compared with the recorded one")
	case string(got) == string(expected):
		res.note("replaying the %d recorded moves reaches the recorded position exactly", len(b.Moves))
	default:
		res.note("replaying the %d recorded moves reaches a DIFFERENT position from the recorded one — that difference is itself a finding", len(b.Moves))
	}
}

// bundleMove is the bundle's move, from its structured form or its notation.
func bundleMove(compiled *catalog.CompiledGame, b Bundle, res *Result) (state.Move, bool) {
	if len(b.Move) > 0 {
		var mv state.Move
		if err := json.Unmarshal(b.Move, &mv); err == nil {
			return mv, true
		}
		res.note("the bundle's move will not load; falling back to its notation")
	}
	if b.Notation != "" {
		mv, err := state.ParseMove(compiled, b.Notation)
		if err != nil {
			res.note("the bundle's notation %q will not parse: %v", b.Notation, err)
			return state.Move{}, false
		}
		return mv, true
	}
	return state.Move{}, false
}

// legalLists reports whether Legal offers mv, by the same key the engine
// compares moves with.
func legalLists(compiled *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) bool {
	want := mv.Key()
	for _, candidate := range state.LegalMoves(compiled, st, seat) {
		if candidate.Key() == want {
			return true
		}
	}
	return false
}

// applySafely applies a move and turns a panic into an error, so replaying a
// bundle whose whole point is a panicking ruleset reports it rather than
// taking the command down with it.
func applySafely(compiled *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (res *state.MoveResult, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("the ruleset panicked: %v", v)
		}
	}()
	return state.Apply(compiled, st, seat, mv)
}
