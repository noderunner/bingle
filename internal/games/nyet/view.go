package nyet

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

var (
	_ state.HiddenInfo = (*Ruleset)(nil)
	_ state.Heuristic  = (*Ruleset)(nil)
	_ state.Describer  = (*Ruleset)(nil)
	_ state.BotGuide   = (*Ruleset)(nil)
)

// View is what clients draw beyond the (empty) board: the cards as this
// viewer may see them, the board of terms, and the scoreboard.
type View struct {
	Phase  string `json:"phase"`
	ToMove int    `json:"to_move,omitempty"`
	Prompt string `json:"prompt"`
	Round  int    `json:"round"`
	Rounds int    `json:"rounds"`
	Dealer int    `json:"dealer"`
	// Table is every zone of cards the viewer may see.
	Table []cards.ZoneView `json:"table"`
	// Grid is the board the players cross boxes off.
	Grid *Grid `json:"grid"`
	// Team and Bonus describe the round once teams are made.
	Team  []int `json:"team,omitempty"`
	Bonus int   `json:"bonus,omitempty"`
	// Seats is each seat's scoreboard.
	Seats map[string]*SeatView `json:"seats"`
	// Labels name the actions that have no label of their own (a team to pick).
	Labels map[string]string `json:"labels,omitempty"`
	// PanelActions are action id prefixes drawn by the panels, not as buttons.
	PanelActions []string `json:"panel_actions"`
}

// Grid is a board of choices, line by line.
type Grid struct {
	Lines []GridLine `json:"lines"`
}

// GridLine is one line of the board.
type GridLine struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
	// Icon names the picture of the line ("line:trump"), as GridBox.Icon does.
	Icon  string    `json:"icon"`
	Boxes []GridBox `json:"boxes"`
}

// GridBox is one box: open, crossed (by a seat), covered for the whole game, or
// the one left empty on its line (chosen).
type GridBox struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Icon names the picture of the box: "kind:argument", such as "suit:red",
	// "animal:wolf:red", "discard:passleft" or "points:-2". The page draws it.
	Icon  string `json:"icon"`
	State string `json:"state"`
	By    int    `json:"by,omitempty"`
	// ByIcon is the picture of the player who crossed the box, a seat's animal.
	ByIcon string `json:"by_icon,omitempty"`
}

// SeatView is one player's scoreboard.
type SeatView struct {
	Counters []CounterView `json:"counters"`
}

// CounterView is a labelled number.
type CounterView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value int    `json:"value"`
	Max   int    `json:"max,omitempty"`
}

// Redact attaches the viewer's view and strips what they must not see: the
// cards (they are in the view only as far as the viewer may see them), the
// shuffling generator and other players' pending choices.
func (rs *Ruleset) Redact(game *catalog.CompiledGame, st *state.BoardState, viewer int) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	pub := d.clone()
	pub.Table = nil
	pub.RNG = 0
	pub.Pending = map[int]string{}
	if own, ok := d.Pending[viewer]; ok && viewer != 0 {
		pub.Pending[viewer] = own
	}
	out := st.Clone()
	out.SetExt(&publicExt{Data: pub, View: newEng(game, d).buildView(viewer)})
	return out
}

// RedactMove: a card played is public; discards and passes name the card only
// to the player who made them.
func (rs *Ruleset) RedactMove(game *catalog.CompiledGame, st *state.BoardState, mv state.Move, notation string) (state.Move, string) {
	if mv.Action == actDiscard || mv.Action == actGive {
		mv.Piece = ""
		if mv.Action == actDiscard {
			return mv, "discard"
		}
		return mv, "pass"
	}
	return mv, notation
}

// Determinize deals the cards the viewer cannot see (the other hands and the
// unused cards) out again at random.
func (rs *Ruleset) Determinize(game *catalog.CompiledGame, st *state.BoardState, viewer int, rng *rand.Rand) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	nd := d.clone()
	n := len(game.Definition.Seats)
	var pool []cards.Card
	for s := 1; s <= n; s++ {
		if s == viewer {
			continue
		}
		pool = append(pool, nd.Table.Zone("hand", s).Cards...)
	}
	deck := nd.Table.Zone("deck", 0)
	pool = append(pool, deck.Cards...)
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	for s := 1; s <= n; s++ {
		if s == viewer {
			continue
		}
		z := nd.Table.Zone("hand", s)
		k := z.Len()
		z.Cards, pool = append([]cards.Card(nil), pool[:k]...), pool[k:]
	}
	deck.Cards = append([]cards.Card(nil), pool...)
	out := st.Clone()
	out.SetExt(nd)
	return out
}

func (e *eng) buildView(viewer int) *View {
	d := e.d
	v := &View{
		Phase: d.Phase, ToMove: d.Turn, Round: d.Round, Rounds: e.rounds(), Dealer: d.Dealer,
		Team: d.Team, Bonus: d.Bonus, Seats: map[string]*SeatView{}, Labels: map[string]string{},
		PanelActions: []string{"card-", "cross-"},
	}
	v.Table = d.Table.View(e.cc, viewer)
	for i := range v.Table {
		z := &v.Table[i]
		if z.ID == "trick" {
			for _, p := range d.Plays {
				z.Captions = append(z.Captions, e.label(p.Seat))
			}
		}
	}
	if len(d.LastTrick) > 0 {
		z := cards.ZoneView{ID: "last", Count: len(d.LastTrick)}
		for _, p := range d.LastTrick {
			z.Cards = append(z.Cards, cards.Card{ID: p.Card, Up: true})
			c := e.label(p.Seat)
			if p.Seat == d.LastWinner {
				c += " ★"
			}
			z.Captions = append(z.Captions, c)
		}
		v.Table = append(v.Table, z)
	}
	v.Grid = e.buildGrid()
	switch {
	case d.Turn == 0 && d.Winner != 0:
		v.Prompt = fmt.Sprintf("%s wins with %d points", e.label(d.Winner), d.Scores[d.Winner])
	case d.Turn == 0:
		v.Prompt = "The game is a tie"
	case d.Phase == phaseNyet:
		v.Prompt = fmt.Sprintf("%s: cross off a box you do NOT want (round %d of %d)", e.label(d.Turn), d.Round, e.rounds())
	case d.Phase == phaseTeam:
		v.Prompt = fmt.Sprintf("%s leads: choose the team", e.label(d.Turn))
	case d.Phase == phaseBonus:
		v.Prompt = fmt.Sprintf("%s: give the x2 bonus card to a player of the smaller team", e.label(d.Turn))
	case d.Phase == phaseDiscard && d.Disc == "passleft":
		v.Prompt = fmt.Sprintf("%s: choose a card to pass to the left", e.label(d.Turn))
	case d.Phase == phaseDiscard:
		v.Prompt = fmt.Sprintf("%s: choose a card to discard", e.label(d.Turn))
	default:
		v.Prompt = fmt.Sprintf("%s: play a card", e.label(d.Turn))
	}
	for _, mv := range e.legalForView() {
		switch mv.Action {
		case actTeam:
			if mv.Piece == "alone" {
				v.Labels["team-alone"] = "Play alone"
				continue
			}
			var names []string
			id := "team"
			for _, p := range splitSeats(mv.Piece) {
				names = append(names, e.label(p))
				id += "-" + strconv.Itoa(p)
			}
			label := "Team with " + names[0]
			if len(names) == 2 {
				label += " and " + names[1]
			}
			v.Labels[id] = label
		case actBonus:
			s, _ := strconv.Atoi(mv.Piece)
			v.Labels["bonus-"+mv.Piece] = "Bonus card to " + e.label(s)
		}
	}
	for s := 1; s <= e.n; s++ {
		v.Seats[strconv.Itoa(s)] = &SeatView{Counters: []CounterView{
			{ID: "score", Label: "Points", Value: d.Scores[s]},
			{ID: "tricks", Label: "Tricks", Value: d.Tricks[s]},
			{ID: "loot", Label: "Loot", Value: d.Loot[s]},
			{ID: "cards", Label: "Cards in hand", Value: d.Table.Zone("hand", s).Len()},
		}}
	}
	return v
}

func (e *eng) legalForView() []state.Move {
	if e.d.Phase == phaseTeam || e.d.Phase == phaseBonus {
		return e.legal()
	}
	return nil
}

func splitSeats(piece string) []int {
	var out []int
	cur := ""
	for _, r := range piece + "," {
		if r == ',' {
			if n, err := strconv.Atoi(cur); err == nil {
				out = append(out, n)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	return out
}

var lineHints = map[string]string{
	lineFirst:   "The player whose box stays empty leads, picks the team and decides who has the bonus card.",
	lineDiscard: "Nyet! = no discards. 1 or 2 = each player discards that many cards. “except 1” = each discards a card that is not a 1. “pass left” = each passes a card to the player on their left.",
	lineTrump:   "The suit whose box stays empty is trump.",
	lineSuper:   "The three 1s of the suit whose box stays empty are super trumps (they count as trump and beat every trump). Nyet! = no super trump.",
	linePoints:  "What each trick and each captured 1 (loot) is worth; the last box, −2, costs points.",
}

func (e *eng) buildGrid() *Grid {
	g := &Grid{}
	for _, line := range lineOrder {
		gl := GridLine{ID: line, Label: lineTitle(line), Hint: lineHints[line], Icon: "line:" + line}
		open := e.empties(line)
		for _, o := range options(line, e.n) {
			b := box{line, o}
			gb := GridBox{ID: o, Label: boxLabel(e, line, o), Icon: boxIcon(line, o), State: "open"}
			switch {
			case covered(b, e.n):
				gb.State = "covered"
			case e.d.Crossed[b.key()] != 0:
				gb.State, gb.By = "crossed", e.d.Crossed[b.key()]
				gb.ByIcon = characterIcon(gb.By)
			case len(open) == 1:
				gb.State = "chosen"
			}
			gl.Boxes = append(gl.Boxes, gb)
		}
		g.Lines = append(g.Lines, gl)
	}
	return g
}

func lineTitle(line string) string {
	switch line {
	case lineFirst:
		return "First player"
	case lineDiscard:
		return "Discard"
	case lineTrump:
		return "Trump"
	case lineSuper:
		return "Super trump"
	}
	return "Points"
}

func boxLabel(e *eng, line, o string) string {
	switch line {
	case lineFirst:
		s, _ := strconv.Atoi(o)
		return e.label(s)
	case lineDiscard:
		switch o {
		case "none":
			return "Nyet!"
		case "except1":
			return "except 1"
		case "passleft":
			return "pass left"
		}
		return o
	case lineSuper:
		if o == "none" {
			return "Nyet!"
		}
		return "1s of " + suitName(o)
	case lineTrump:
		return strings.ToUpper(suitName(o)[:1]) + suitName(o)[1:]
	}
	return o
}

// score values the position for seat by the points banked this game.
func (e *eng) score(seat int) float64 {
	best := 0
	for s := 1; s <= e.n; s++ {
		if s != seat && e.d.Scores[s] > best {
			best = e.d.Scores[s]
		}
	}
	return float64(e.d.Scores[seat] - best)
}

// Heuristic values a position for seat in [-1, 1] by the points banked.
func (rs *Ruleset) Heuristic(game *catalog.CompiledGame, st *state.BoardState, seat int) float64 {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	if d.Turn == 0 {
		switch {
		case d.Winner == seat:
			return 1
		case d.Winner != 0:
			return -1
		}
		return 0
	}
	x := newEng(game, d).score(seat) / 60
	if x > 0.99 {
		x = 0.99
	}
	if x < -0.99 {
		x = -0.99
	}
	return x
}

// GuideMoves makes a bot play the tricks sensibly: take a trick with the
// cheapest card that wins it, or throw the lowest card (not a 1, which an
// opponent could capture) when it cannot.
func (rs *Ruleset) GuideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	d, err := dataOf(st)
	if err != nil || d.Phase != phaseTrick || len(legal) < 2 {
		return legal
	}
	e := newEng(game, d)
	value := func(mv state.Move) int { return e.typeOf(mv.Piece).Value }
	type opt struct {
		mv   state.Move
		wins bool
	}
	var opts []opt
	for _, mv := range legal {
		id, _ := e.instanceOf(seat, mv.Piece)
		plays := append(append([]Play(nil), d.Plays...), Play{Seat: seat, Card: id})
		opts = append(opts, opt{mv, e.trickWinner(plays) == seat})
	}
	pick := func(wins bool) []state.Move {
		var best state.Move
		found := false
		for _, o := range opts {
			if o.wins != wins {
				continue
			}
			// Cheapest card; a 1 is worth keeping unless it wins.
			if !found || value(o.mv) < value(best) {
				best, found = o.mv, true
			}
		}
		if found {
			return []state.Move{best}
		}
		return nil
	}
	if len(d.Plays) == e.n-1 {
		// Last to play: win only if the trick is not already a teammate's.
		cur := e.trickWinner(d.Plays)
		if e.onTeam(cur) == e.onTeam(seat) {
			if m := pick(false); m != nil {
				return m
			}
		}
	}
	if m := pick(true); m != nil {
		return m
	}
	if m := pick(false); m != nil {
		return m
	}
	return legal
}
