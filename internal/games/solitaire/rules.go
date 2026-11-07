package solitaire

import (
	"fmt"
	"math/rand"
	"sync"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset plays Bowling Solitaire. Klondike, Spider and FreeCell, which it
// played too until Solitaire 0.3.0, are declared with the patience vocabulary
// (ADR 0122); Bowling has no vocabulary and keeps its Go rules.
type Ruleset struct {
	// samples holds each game's sample view, worked out once: it never changes.
	samples sync.Map
}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

// Seeded: the deal comes from a secret seed, recorded with the start position.
func (*Ruleset) Seeded() bool { return true }

// eng applies the rules of one position.
type eng struct {
	game *catalog.CompiledGame
	d    *Data
	ev   []string
}

func newEng(game *catalog.CompiledGame, d *Data) *eng { return &eng{game: game, d: d} }

func (e *eng) say(format string, args ...any) { e.ev = append(e.ev, fmt.Sprintf(format, args...)) }

func (e *eng) zone(id string) *cards.Zone { return e.d.Table.Zone(id, 0) }

func (e *eng) value(c cards.Card) int {
	if t := e.game.Cards.Type(c.ID); t != nil {
		return t.Value
	}
	return 0
}

// shuffled returns the whole deck in a new order, face down.
func (e *eng) shuffled() []cards.Card {
	z := &cards.Zone{}
	z.Fill(e.game.Cards.Deck("main"), false)
	rng := cards.RNG{State: e.d.RNG}
	z.Shuffle(&rng)
	e.d.RNG = rng.State
	return z.Cards
}

// legal lists the moves of the position.
func (e *eng) legal() []string {
	if e.d.Turn == 0 {
		return nil
	}
	return e.bowlingLegal()
}

// execute plays a legal move.
func (e *eng) execute(move string) {
	e.bowlingPlay(move)
}

// --- the ruleset interface --------------------------------------------------

// Start deals the game.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	if game.Cards == nil {
		return nil, fmt.Errorf("solitaire: the definition has no cards")
	}
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	d := &Data{Mode: game.ParamString("mode", modeBowling), Turn: player, RNG: seed, Table: cards.NewTable()}
	if d.Mode != modeBowling {
		return nil, fmt.Errorf("solitaire: unknown mode %q", d.Mode)
	}
	e := newEng(game, d)
	e.bowlingStart()
	d.Events = e.ev
	st := state.NewBoardState()
	st.SetExt(d)
	return st, nil
}

// ImpulseSeat is the player, or 0 once the game is over.
func (rs *Ruleset) ImpulseSeat(game *catalog.CompiledGame, st *state.BoardState) int {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	return d.Turn
}

// Outcome reports a finished game. Clearing the table is a win; bowling has only a
// score, and a game that got stuck has nobody to lose to: neither names a winner.
func (rs *Ruleset) Outcome(game *catalog.CompiledGame, st *state.BoardState) state.Outcome {
	d, err := dataOf(st)
	if err != nil || d.Turn != 0 {
		return state.Outcome{}
	}
	out := state.Outcome{Ended: true, Reason: d.Reason, Detail: newEng(game, d).endDetail()}
	if d.Won {
		w := player
		out.Winner = &w
	}
	return out
}

func (e *eng) endDetail() string {
	return fmt.Sprintf("%d points", bowlingTotal(e.d.Rolls))
}

// Validate checks a position document.
func (rs *Ruleset) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	d, err := dataOf(st)
	if err != nil {
		return err
	}
	if d.Mode != game.ParamString("mode", modeBowling) {
		return fmt.Errorf("solitaire: a position of %q in a game of %q", d.Mode, game.ParamString("mode", modeBowling))
	}
	if d.Turn != 0 && d.Turn != player {
		return fmt.Errorf("solitaire: bad turn %d", d.Turn)
	}
	seen := map[string]bool{}
	for k, z := range d.Table.Zones {
		for _, c := range z.Cards {
			if c.ID == "" || game.Cards.Type(c.ID) == nil {
				return fmt.Errorf("solitaire: unknown card %q in %s", c.ID, k)
			}
			if seen[c.ID] {
				return fmt.Errorf("solitaire: card %q is in two places", c.ID)
			}
			seen[c.ID] = true
		}
	}
	return nil
}

// Legal lists the decisions of seat.
func (rs *Ruleset) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Selection {
	d, err := dataOf(st)
	if err != nil || d.Turn == 0 || seat != d.Turn {
		return nil
	}
	list := newEng(game, d).legal()
	moves := make([]state.Selection, len(list))
	for i, m := range list {
		moves[i] = state.Selection{Action: actMove, Piece: m}
	}
	return moves
}

// Apply plays one decision.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Selection) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("solitaire: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("solitaire: seat %d does not play here", seat)
	}
	found := false
	for _, m := range newEng(game, d).legal() {
		if m == mv.Piece {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("solitaire: %q is not a legal move now", mv.Piece)
	}
	nd := d.clone()
	e := newEng(game, nd)
	e.execute(mv.Piece)
	nd.Events = e.ev
	ns := st.Clone()
	ns.SetExt(nd)
	ns.SetPly(st.Ply() + 1)
	res := &state.MoveResult{NewState: ns, Move: state.Selection{Action: actMove, Piece: mv.Piece}, Notation: mv.Piece, NextImpulseSeat: seat}
	if nd.Turn == 0 {
		res.GameEnded = true
		res.EndReason = nd.Reason
		res.EndDetail = e.endDetail()
		if nd.Won {
			w := player
			res.WinnerSeat = &w
		}
	}
	return res, nil
}

// Determinize deals the cards nobody has seen again at random: search looks at
// what might be under a card, not at what is. The cards keep their places and
// their faces; only the identities of the face-down ones change.
func (rs *Ruleset) Determinize(game *catalog.CompiledGame, st *state.BoardState, viewer int, rng *rand.Rand) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	nd := d.clone()
	var holes []*cards.Card
	for _, k := range nd.sortedZones() {
		z := nd.Table.Zones[k]
		for i := range z.Cards {
			if !z.Cards[i].Up {
				holes = append(holes, &z.Cards[i])
			}
		}
	}
	ids := make([]string, len(holes))
	for i, c := range holes {
		ids[i] = c.ID
	}
	rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	for i, c := range holes {
		c.ID = ids[i]
	}
	out := st.Clone()
	out.SetExt(nd)
	return out
}

// RedactMove: a move names piles, never a hidden card.
func (rs *Ruleset) RedactMove(game *catalog.CompiledGame, st *state.BoardState, mv state.Selection, notation string) (state.Selection, string) {
	return mv, notation
}

// Redact attaches the view and strips the cards and the generator: the view holds
// the cards as far as they can be seen.
func (rs *Ruleset) Redact(game *catalog.CompiledGame, st *state.BoardState, viewer int) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	pub := d.clone()
	pub.Events = d.Events
	pub.Table = nil
	pub.RNG = 0
	out := st.Clone()
	out.SetExt(&publicExt{Data: pub, View: newEng(game, d).buildView(viewer)})
	return out
}

// WorthSearching: a hint comes from GuideMoves; playing a game out at random
// says nothing about the position.
func (rs *Ruleset) WorthSearching(game *catalog.CompiledGame, st *state.BoardState) bool {
	return false
}

// Heuristic is how much of the game is done, from -1 to 1.
func (rs *Ruleset) Heuristic(game *catalog.CompiledGame, st *state.BoardState, seat int) float64 {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	return float64(bowlingTotal(d.Rolls))/150 - 1
}

// GuideMoves keeps the moves worth suggesting: a computer player and the Hint button
// choose among them.
func (rs *Ruleset) GuideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Selection) []state.Selection {
	d, err := dataOf(st)
	if err != nil {
		return legal
	}
	e := newEng(game, d)
	best := -1 << 30
	var out []state.Selection
	for _, m := range legal {
		v := e.bowlingWorth(m.Piece)
		if v > best {
			best, out = v, nil
		}
		if v == best {
			out = append(out, m)
		}
	}
	return out
}

// sampleSeed is the deal the sample of a game is played from.
const sampleSeed = 21

// samplePlies is how far a sample is played: a few frames in, far from the end.
const samplePlies = 45

// Sample plays the game's own hints from a fixed deal, and returns the view of where
// that gets to, as a spectator sees it.
func (rs *Ruleset) Sample(game *catalog.CompiledGame) any {
	if v, ok := rs.samples.Load(game.Definition.ID); ok {
		return v
	}
	st, err := rs.Start(game, sampleSeed)
	if err != nil {
		return nil
	}
	for ply := 0; ply < samplePlies; ply++ {
		d, err := dataOf(st)
		if err != nil || (d.Mode == modeBowling && d.Frame >= 4 && !d.Fresh) {
			break
		}
		moves := state.GuidedMoves(game, st, player, rs.Legal(game, st, player))
		if len(moves) == 0 {
			break
		}
		res, err := rs.Apply(game, st, player, moves[0])
		if err != nil || res.GameEnded {
			break
		}
		st = res.NewState
	}
	d, err := dataOf(st)
	if err != nil {
		return nil
	}
	v := newEng(game, d).buildView(0)
	v.ImpulseSeat = 0
	v.Prompt = ""
	v.Hint = ""
	rs.samples.Store(game.Definition.ID, v)
	return v
}

var (
	_ state.Sampler    = (*Ruleset)(nil)
	_ state.HiddenInfo = (*Ruleset)(nil)
	_ state.Heuristic  = (*Ruleset)(nil)
	_ state.Describer  = (*Ruleset)(nil)
	_ state.BotGuide   = (*Ruleset)(nil)
	_ state.Searchable = (*Ruleset)(nil)
	_ state.UndoGuard  = (*Ruleset)(nil)
)

// CanTakeBack: a play stands once it has shown the player a card they could not
// see before (a draw from the stock, a card uncovered in a column), or Back would
// let them peek and change their mind (ADR 0083, ADR 0114).
func (rs *Ruleset) CanTakeBack(game *catalog.CompiledGame, before, after *state.BoardState) bool {
	b, err := dataOf(before)
	if err != nil {
		return false
	}
	a, err := dataOf(after)
	if err != nil {
		return false
	}
	seen := visibleCards(game, b)
	for id := range visibleCards(game, a) {
		if !seen[id] {
			return false
		}
	}
	return true
}

// visibleCards is every card the player can see in d.
func visibleCards(game *catalog.CompiledGame, d *Data) map[string]bool {
	out := map[string]bool{}
	if d.Table == nil {
		return out
	}
	for _, z := range d.Table.Redact(game.Cards, player).Zones {
		for _, c := range z.Cards {
			if !c.Hidden() {
				out[c.ID] = true
			}
		}
	}
	return out
}
