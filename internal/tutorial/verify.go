package tutorial

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Verify replays t against the engine, as the tutorial page will play it:
// the setup moves by whoever is to move, then each step's moves by the
// learner and its replies by the other seats. Every move must be legal for
// the seat that makes it, and every square a step points at must be on the
// board. family is the game whose files hold t; the game t is played on must
// belong to it.
func Verify(cat *catalog.Catalog, family string, t Tutorial) error {
	if err := t.check(); err != nil {
		return err
	}
	game, ok := cat.Get(t.Game)
	if !ok {
		return fmt.Errorf("%s: no game %q", t.ID, t.Game)
	}
	if got := familyOf(game); got != family {
		return fmt.Errorf("%s: %q is a game of %q, not of %q", t.ID, t.Game, got, family)
	}
	learner := t.learnerSeat()
	if _, ok := game.Seat(learner); !ok {
		return fmt.Errorf("%s: %q has no seat %d", t.ID, t.Game, learner)
	}
	if state.NeedsStartRecord(game) && t.Seed == 0 {
		return fmt.Errorf("%s: %q has chance: name a seed, so the scripted line is the same every time", t.ID, t.Game)
	}
	p, err := newPlayer(game, t.Seed)
	if err != nil {
		return fmt.Errorf("%s: %w", t.ID, err)
	}
	for i, mv := range t.Setup {
		if err := p.play(mv, 0); err != nil {
			return fmt.Errorf("%s: setup move %d: %w", t.ID, i+1, err)
		}
	}
	for i, s := range t.Steps {
		at := fmt.Sprintf("%s: step %d", t.ID, i+1)
		for _, sq := range append(append([]string{}, s.Highlight...), s.Arrow...) {
			if !game.Board.HasPosition(board.PositionID(sq)) {
				return fmt.Errorf("%s: %q is not a square of %q", at, sq, t.Game)
			}
		}
		for _, mv := range s.Play {
			if err := p.play(mv, learner); err != nil {
				return fmt.Errorf("%s: the learner's %w", at, err)
			}
		}
		for _, mv := range s.Reply {
			if err := p.play(mv, -learner); err != nil {
				return fmt.Errorf("%s: the reply %w", at, err)
			}
		}
	}
	return nil
}

// Start returns the position a tutorial's room begins from — the game's
// start, its chance seeded by t.Seed, with t.Setup played — and the seat to
// move from it.
func Start(game *catalog.CompiledGame, t Tutorial) (*state.BoardState, int, error) {
	p, err := newPlayer(game, t.Seed)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", t.ID, err)
	}
	for i, mv := range t.Setup {
		if err := p.play(mv, 0); err != nil {
			return nil, 0, fmt.Errorf("%s: setup move %d: %w", t.ID, i+1, err)
		}
	}
	if p.seat == 0 {
		return nil, 0, fmt.Errorf("%s: the setup ends the game", t.ID)
	}
	return p.st, p.seat, nil
}

// LearnerSeat is the seat the learner holds (Seat, default 1).
func (t Tutorial) LearnerSeat() int { return t.learnerSeat() }

func familyOf(game *catalog.CompiledGame) string {
	if f := game.Definition.Family; f != "" {
		return f
	}
	return game.Definition.ID
}

// player plays a game from its start, keeping track of whose move it is.
type player struct {
	game *catalog.CompiledGame
	st   *state.BoardState
	seat int // the seat to move, 0 once the game is over
}

func newPlayer(game *catalog.CompiledGame, seed uint64) (*player, error) {
	st, err := state.InitialStateSeeded(game, seed)
	if err != nil {
		return nil, err
	}
	p := &player{game: game, st: st, seat: state.FirstSeat(game)}
	if rs, ok := state.RulesetOf(game); ok {
		p.seat = rs.ToMove(game, st)
	}
	return p, nil
}

// play plays one move in the game's notation. who is the seat that must be
// to move: a seat, its negative for "any seat but this one", or 0 for any.
func (p *player) play(text string, who int) error {
	if p.seat == 0 {
		return fmt.Errorf("move %q: the game is already over", text)
	}
	switch {
	case who > 0 && p.seat != who:
		return fmt.Errorf("move %q: it is seat %d's move, not the learner's (seat %d)", text, p.seat, who)
	case who < 0 && p.seat == -who:
		return fmt.Errorf("move %q: it is the learner's own move (seat %d), not a reply", text, p.seat)
	}
	mv, err := state.ParseMove(p.game, text)
	if err != nil {
		return fmt.Errorf("move %q: %w", text, err)
	}
	res, err := state.Apply(p.game, p.st, p.seat, mv)
	if err != nil {
		return fmt.Errorf("move %q is not legal for seat %d: %w", text, p.seat, err)
	}
	// The page compares the learner's move with the script by its
	// notation, so the script must spell each move as the engine does.
	if res.Notation != "" && !strings.EqualFold(res.Notation, strings.TrimSpace(text)) {
		return fmt.Errorf("move %q: write it as the game does, %q", text, res.Notation)
	}
	p.st = res.NewState
	switch {
	case res.GameEnded:
		p.seat = 0
	default:
		p.seat = res.NextTurnSeat
		if rs, ok := state.RulesetOf(p.game); ok {
			p.seat = rs.ToMove(p.game, p.st)
		}
	}
	return nil
}
