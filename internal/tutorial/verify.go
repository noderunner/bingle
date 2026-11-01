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
		if len(s.Show) > 0 {
			if _, err := Shown(game, t, i); err != nil {
				return fmt.Errorf("%s: %w", at, err)
			}
		}
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

// Shown returns the position step i of t depicts (Step.Show): its moves
// played from the game's start, seeded as the tutorial is. The moves may end
// the game — showing how a game ends is what it is for.
func Shown(game *catalog.CompiledGame, t Tutorial, i int) (*state.BoardState, error) {
	if i < 0 || i >= len(t.Steps) || len(t.Steps[i].Show) == 0 {
		return nil, fmt.Errorf("step %d shows no position", i+1)
	}
	p, err := newPlayer(game, t.Seed)
	if err != nil {
		return nil, err
	}
	for j, mv := range t.Steps[i].Show {
		if err := p.play(mv, 0); err != nil {
			return nil, fmt.Errorf("shown move %d: %w", j+1, err)
		}
	}
	return p.st, nil
}

// StepGuide is what a client needs, besides the tutorial itself, to guide a
// learner through one step: the selections each of the learner's moves is
// made of ("piece red", then "pos d4") to point them at in its own way, and
// the position a step that shows one depicts.
type StepGuide struct {
	PlaySteps [][]state.Step    `json:"play_steps,omitempty"`
	Shown     *state.BoardState `json:"show_state,omitempty"`
}

// Guide walks t as the page plays it and returns each step's guide. viewer
// is the seat a shown position is seen from (its secrets only).
func Guide(game *catalog.CompiledGame, t Tutorial) ([]StepGuide, error) {
	p, err := newPlayer(game, t.Seed)
	if err != nil {
		return nil, err
	}
	for i, mv := range t.Setup {
		if err := p.play(mv, 0); err != nil {
			return nil, fmt.Errorf("%s: setup move %d: %w", t.ID, i+1, err)
		}
	}
	out := make([]StepGuide, len(t.Steps))
	for i, s := range t.Steps {
		if len(s.Show) > 0 {
			st, err := Shown(game, t, i)
			if err != nil {
				return nil, err
			}
			out[i].Shown = state.Public(game, st, t.learnerSeat())
		}
		for _, text := range s.Play {
			steps, err := p.stepsOf(text)
			if err != nil {
				return nil, fmt.Errorf("%s: step %d: %w", t.ID, i+1, err)
			}
			out[i].PlaySteps = append(out[i].PlaySteps, steps)
			if err := p.play(text, 0); err != nil {
				return nil, err
			}
		}
		for _, text := range s.Reply {
			if err := p.play(text, 0); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// stepsOf is the selections of the legal move written text, as the engine
// lists it for the seat to move.
func (p *player) stepsOf(text string) ([]state.Step, error) {
	for _, mv := range state.LegalMoves(p.game, p.st, p.seat) {
		if strings.EqualFold(state.FormatMove(p.game, mv), strings.TrimSpace(text)) {
			return state.Steps(p.game, mv), nil
		}
	}
	return nil, fmt.Errorf("move %q is not among the legal moves", text)
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
