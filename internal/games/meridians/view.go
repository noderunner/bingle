package meridians

import (
	"math/rand"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

var (
	_ state.HiddenInfo = (*Ruleset)(nil)
	_ state.Heuristic  = (*Ruleset)(nil)
	_ state.Describer  = (*Ruleset)(nil)
)

// View is what clients draw beyond the stones: which groups can see no friend
// (and so die when their owner's turn ends), where the last stone went, and how
// many stones each player has.
type View struct {
	Phase  string `json:"phase"`
	ToMove int    `json:"to_move,omitempty"`
	Prompt string `json:"prompt"`
	// Tints marks points by legend key, Badges the stones of a dead group.
	Tints  map[string]string   `json:"tints,omitempty"`
	Badges map[string][]string `json:"badges,omitempty"`
	// Seats is each player's scoreboard.
	Seats map[string]*SeatView `json:"seats"`
}

// SeatView is one player's numbers.
type SeatView struct {
	Counters []CounterView `json:"counters"`
}

// CounterView is a labelled number.
type CounterView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value int    `json:"value"`
}

// Redact attaches the derived view; Meridians has nothing to hide.
func (rs *Ruleset) Redact(game *catalog.CompiledGame, st *state.BoardState, viewer int) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	out := st.Clone()
	out.SetExt(&publicExt{Data: d.clone(), View: newEng(game, st, d).buildView()})
	return out
}

// RedactMove: a move reveals nothing.
func (rs *Ruleset) RedactMove(game *catalog.CompiledGame, st *state.BoardState, mv state.Move, notation string) (state.Move, string) {
	return mv, notation
}

// Determinize: there is nothing unknown to sample.
func (rs *Ruleset) Determinize(game *catalog.CompiledGame, st *state.BoardState, viewer int, rng *rand.Rand) *state.BoardState {
	return st
}

func (e *eng) buildView() *View {
	d := e.d
	v := &View{Phase: "play", ToMove: d.Turn, Tints: map[string]string{}, Badges: map[string][]string{}, Seats: map[string]*SeatView{}}
	if d.Last != "" {
		v.Tints[d.Last] = "last"
	}
	// Every group that can see no friend, whichever side it belongs to: the
	// owner's own turn takes it off, so both players want to see it.
	var dead []string
	for seat := 1; seat <= 2; seat++ {
		if len(e.groups(seat)) < 2 && d.Turns[seat-1] < 2 {
			continue // the opening stones are exempt
		}
		for _, group := range e.deadGroups(seat) {
			for _, i := range group {
				dead = append(dead, string(e.g.ids[i]))
			}
		}
	}
	if len(dead) > 0 {
		v.Badges["dead"] = dead
	}
	for seat := 1; seat <= 2; seat++ {
		v.Seats[itoa(seat)] = &SeatView{Counters: []CounterView{
			{ID: "stones", Label: "Stones", Value: e.count(seat)},
			{ID: "groups", Label: "Groups", Value: len(e.groups(seat))},
		}}
	}
	switch {
	case d.Turn == 0 && d.Winner != 0:
		v.Prompt = seatLabel(d.Winner) + " wins: " + seatLabel(other(d.Winner)) + " has no stones left"
	case d.Turn == 0:
		v.Prompt = "The game is a draw"
	case d.Turns[d.Turn-1] == 0:
		v.Prompt = seatLabel(d.Turn) + ": lay your first stone anywhere"
	case d.Turns[d.Turn-1] == 1:
		v.Prompt = seatLabel(d.Turn) + ": lay a stone your first can see, not touching it"
	default:
		v.Prompt = seatLabel(d.Turn) + ": lay a stone one of yours can see"
	}
	return v
}

func itoa(n int) string { return string(rune('0' + n)) }

// Heuristic scores a position for the minimax bot: stones, and the groups that
// are about to die.
func (rs *Ruleset) Heuristic(game *catalog.CompiledGame, st *state.BoardState, seat int) float64 {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	e := newEng(game, st, d)
	mine, theirs := e.count(seat), e.count(other(seat))
	score := float64(mine - theirs)
	// A group with no path is worth nothing: it comes off next turn.
	for _, group := range e.deadGroups(seat) {
		score -= float64(len(group))
	}
	for _, group := range e.deadGroups(other(seat)) {
		score += float64(len(group))
	}
	return score / 20
}
