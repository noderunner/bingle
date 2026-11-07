package state

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// A game of patience (ADR 0122): the columns, foundations, free cells, stock
// and finished runs of a patience block, played with Solitaire's notation. A
// selection of the "patience" action is a move between piles (From the pile,
// Count the cards, To the column, or "f" home, or "c" a free cell) or one of
// the words draw, recycle, deal and finish (in Piece).

// The words of a game of patience.
const (
	patienceDraw    = "draw"
	patienceRecycle = "recycle"
	patienceRow     = "deal"
	patienceFinish  = "finish"
	// patienceRedeals counts the times the waste went back to the stock.
	patienceRedeals = "redeals"
	// Solitaire's end reasons.
	patienceCleared = "cleared"
	patienceStuck   = "stuck"
)

// pat reads and plays a game of patience on one table.
type pat struct {
	game *catalog.CompiledGame
	p    *catalog.Patience
	st   *BoardState
}

func patienceOf(game *catalog.CompiledGame, st *BoardState) *pat {
	return &pat{game: game, p: game.Patience, st: st}
}

func (e *pat) zone(id string) *cards.Zone {
	if e.st.table == nil {
		e.st.table = cards.NewTable()
	}
	return e.st.table.Zone(id, 0)
}

// count is a pile's cards, without making it.
func (e *pat) count(id string) int {
	if e.st.table == nil {
		return 0
	}
	if z, ok := e.st.table.Zones[id]; ok {
		return z.Len()
	}
	return 0
}

func (e *pat) top(id string) (cards.Card, bool) {
	if e.st.table == nil {
		return cards.Card{}, false
	}
	if z, ok := e.st.table.Zones[id]; ok {
		return z.Top()
	}
	return cards.Card{}, false
}

func (e *pat) pile(id string) []cards.Card {
	if e.st.table == nil {
		return nil
	}
	if z, ok := e.st.table.Zones[id]; ok {
		return z.Cards
	}
	return nil
}

func (e *pat) value(c cards.Card) int {
	if t := e.game.Cards.Type(c.ID); t != nil {
		return t.Value
	}
	return 0
}

func (e *pat) suit(c cards.Card) string {
	if t := e.game.Cards.Type(c.ID); t != nil {
		return t.Suit
	}
	return ""
}

// place is a suit's place in suit_order: its colour is the place's parity.
func (e *pat) place(c cards.Card) int {
	s := e.suit(c)
	for i, id := range e.game.Cards.Suits {
		if id == s {
			return i
		}
	}
	return 0
}

// builds says whether card c may lie on upper in a column.
func (e *pat) builds(upper, c cards.Card, suit string) bool {
	if e.p.RankUp {
		if e.value(c) != e.value(upper)+1 {
			return false
		}
	} else if e.value(upper) != e.value(c)+1 {
		return false
	}
	switch suit {
	case "alternate":
		return e.place(upper)%2 != e.place(c)%2
	case "same":
		return e.suit(upper) == e.suit(c)
	}
	return true
}

// run is how many cards may be lifted off the end of a column together.
func (e *pat) run(cs []cards.Card) int {
	l := len(cs)
	if l == 0 || !cs[l-1].Up {
		return 0
	}
	if e.p.Lift == "top" {
		return 1
	}
	n := 1
	for i := l - 1; i > 0; i-- {
		upper, lower := cs[i-1], cs[i]
		if !upper.Up {
			break
		}
		switch e.p.Lift {
		case "ordered":
			if !e.builds(upper, lower, e.p.Suit) {
				return n
			}
		case "same-suit":
			if !e.builds(upper, lower, "same") {
				return n
			}
		}
		n++
	}
	return n
}

// fits says whether c, the bottom card of what is moved, may go on column id.
func (e *pat) fits(c cards.Card, id string) bool {
	top, ok := e.top(id)
	if !ok {
		switch e.p.Empty {
		case "king":
			return e.value(c) == 13
		case "none":
			return false
		}
		return true
	}
	return top.Up && e.builds(top, c, e.p.Suit)
}

// foundationSuit is the suit of foundation k (1-based).
func (e *pat) foundationSuit(k int) string {
	suits := e.game.Cards.Suits
	return suits[(k-1)%len(suits)]
}

// home is the foundation c goes to: the first of its suit that takes it.
func (e *pat) home(c cards.Card) (string, bool) {
	for k := 1; k <= e.p.Foundations; k++ {
		f := catalog.Foundation(k)
		if e.foundationSuit(k) == e.suit(c) && e.value(c) == e.count(f)+1 {
			return f, true
		}
	}
	return "", false
}

// capacity is how long a run may be with FreeCell's limit; toEmpty says the
// column it goes to is empty.
func (e *pat) capacity(toEmpty bool) int {
	free, empty := 0, 0
	for i := 1; i <= e.p.Cells; i++ {
		if e.count(catalog.Cell(i)) == 0 {
			free++
		}
	}
	for i := 1; i <= e.p.Columns; i++ {
		if e.count(catalog.Column(i)) == 0 {
			empty++
		}
	}
	if toEmpty {
		empty--
	}
	return (free + 1) << max(empty, 0)
}

func (e *pat) anyEmptyColumn() bool {
	for i := 1; i <= e.p.Columns; i++ {
		if e.count(catalog.Column(i)) == 0 {
			return true
		}
	}
	return false
}

// homeCount is how many cards are home or off.
func (e *pat) homeCount() int {
	n := e.count(catalog.PileDone)
	for k := 1; k <= e.p.Foundations; k++ {
		n += e.count(catalog.Foundation(k))
	}
	return n
}

// canFinish says everything can go home from where it lies, with nothing
// hidden and nothing in the stock or the waste.
func (e *pat) canFinish() bool {
	if !e.p.Finish || e.count(catalog.PileStock) > 0 || e.count(catalog.PileWaste) > 0 {
		return false
	}
	tops := map[string]int{}
	for k := 1; k <= e.p.Foundations; k++ {
		tops[catalog.Foundation(k)] = e.count(catalog.Foundation(k))
	}
	var piles [][]cards.Card
	for i := 1; i <= e.p.Columns; i++ {
		cs := e.pile(catalog.Column(i))
		for _, c := range cs {
			if !c.Up {
				return false
			}
		}
		piles = append(piles, append([]cards.Card(nil), cs...))
	}
	for i := 1; i <= e.p.Cells; i++ {
		piles = append(piles, append([]cards.Card(nil), e.pile(catalog.Cell(i))...))
	}
	for moved := true; moved; {
		moved = false
		for i, p := range piles {
			if len(p) == 0 {
				continue
			}
			c := p[len(p)-1]
			for k := 1; k <= e.p.Foundations; k++ {
				f := catalog.Foundation(k)
				if e.foundationSuit(k) == e.suit(c) && e.value(c) == tops[f]+1 {
					tops[f]++
					piles[i] = p[:len(p)-1]
					moved = true
					break
				}
			}
		}
	}
	for _, p := range piles {
		if len(p) > 0 {
			return false
		}
	}
	return true
}

func (e *pat) mv(word string) Selection {
	return Selection{Action: catalog.ActionPatience, Piece: word}
}

func (e *pat) move(from string, n int, to string) Selection {
	return Selection{Action: catalog.ActionPatience, From: board.PositionID(from), Count: n, To: board.PositionID(to)}
}

// legal lists the moves of the position.
func (e *pat) legal() []Selection {
	p := e.p
	var out []Selection
	if e.canFinish() {
		out = append(out, e.mv(patienceFinish))
	}
	toColumns := func(from string, c cards.Card) {
		for j := 1; j <= p.Columns; j++ {
			if catalog.Column(j) != from && e.fits(c, catalog.Column(j)) {
				out = append(out, e.move(from, 1, catalog.Column(j)))
			}
		}
	}
	freeCell := false
	for i := 1; i <= p.Cells; i++ {
		freeCell = freeCell || e.count(catalog.Cell(i)) == 0
	}
	for i := 1; i <= p.Columns; i++ {
		from := catalog.Column(i)
		cs := e.pile(from)
		l, run := len(cs), e.run(cs)
		for k := 1; k <= run; k++ {
			c := cs[l-k]
			for j := 1; j <= p.Columns; j++ {
				dest := catalog.Column(j)
				if j == i || !e.fits(c, dest) {
					continue
				}
				empty := e.count(dest) == 0
				// A whole column to an empty one changes nothing.
				if empty && k == l {
					continue
				}
				if p.LiftCells && k > e.capacity(empty) {
					continue
				}
				out = append(out, e.move(from, k, dest))
			}
		}
		if run > 0 {
			if _, ok := e.home(cs[l-1]); ok {
				out = append(out, e.move(from, 1, "f"))
			}
			if freeCell {
				out = append(out, e.move(from, 1, "c"))
			}
		}
	}
	if p.Draw > 0 {
		if top, ok := e.top(catalog.PileWaste); ok {
			if _, ok := e.home(top); ok {
				out = append(out, e.move("w", 1, "f"))
			}
			toColumns("w", top)
		}
	}
	if p.Back {
		for k := 1; k <= p.Foundations; k++ {
			if top, ok := e.top(catalog.Foundation(k)); ok {
				toColumns(catalog.Foundation(k), top)
			}
		}
	}
	if p.Draw > 0 {
		switch {
		case e.count(catalog.PileStock) > 0:
			out = append(out, e.mv(patienceDraw))
		case e.count(catalog.PileWaste) > 0 && (p.Redeals < 0 || e.st.Counter(patienceRedeals) < p.Redeals):
			out = append(out, e.mv(patienceRecycle))
		}
	}
	for i := 1; i <= p.Cells; i++ {
		if top, ok := e.top(catalog.Cell(i)); ok {
			if _, ok := e.home(top); ok {
				out = append(out, e.move(catalog.Cell(i), 1, "f"))
			}
			toColumns(catalog.Cell(i), top)
		}
	}
	if p.Row > 0 && e.count(catalog.PileStock) > 0 && (!p.RowUnlessEmpty || !e.anyEmptyColumn()) {
		out = append(out, e.mv(patienceRow))
	}
	return out
}

// sourceZone is the zone a move's source names.
func sourceZone(pile string) string {
	if pile == "w" {
		return catalog.PileWaste
	}
	return pile
}

// play plays a legal move on e's table, then takes finished runs off.
func (e *pat) play(m Selection) {
	switch m.Piece {
	case patienceDraw:
		waste := e.zone(catalog.PileWaste)
		for _, c := range e.zone(catalog.PileStock).Draw(e.p.Draw) {
			c.Up = true
			waste.Push(c)
		}
	case patienceRecycle:
		stock, waste := e.zone(catalog.PileStock), e.zone(catalog.PileWaste)
		for i := waste.Len() - 1; i >= 0; i-- {
			c := waste.Cards[i]
			c.Up = false
			stock.Push(c)
		}
		waste.Cards = nil
		if e.p.Redeals >= 0 {
			e.st.SetCounter(patienceRedeals, e.st.Counter(patienceRedeals)+1)
		}
	case patienceRow:
		stock := e.zone(catalog.PileStock)
		for i := 1; i <= e.p.Row; i++ {
			for _, c := range stock.Draw(1) {
				c.Up = true
				e.zone(catalog.Column(i)).Push(c)
			}
		}
	case patienceFinish:
		e.finish()
	default:
		pile, n, to := string(m.From), max(m.Count, 1), string(m.To)
		from := e.zone(sourceZone(pile))
		lift := append([]cards.Card(nil), from.Cards[from.Len()-n:]...)
		from.Cards = from.Cards[:from.Len()-n]
		dest := to
		switch to {
		case "f":
			dest, _ = e.home(lift[0])
		case "c":
			for i := 1; i <= e.p.Cells; i++ {
				if e.count(catalog.Cell(i)) == 0 {
					dest = catalog.Cell(i)
					break
				}
			}
		}
		d := e.zone(dest)
		d.Cards = append(d.Cards, lift...)
		if l := from.Len(); l > 0 && strings.HasPrefix(pile, "t") {
			from.Cards[l-1].Up = true
		}
	}
	if e.p.Off {
		e.takeOff()
	}
}

// finish sends everything home.
func (e *pat) finish() {
	var piles []string
	for i := 1; i <= e.p.Columns; i++ {
		piles = append(piles, catalog.Column(i))
	}
	for i := 1; i <= e.p.Cells; i++ {
		piles = append(piles, catalog.Cell(i))
	}
	for moved := true; moved; {
		moved = false
		for _, id := range piles {
			if top, ok := e.top(id); ok {
				if f, ok := e.home(top); ok {
					z := e.zone(id)
					z.Cards = z.Cards[:z.Len()-1]
					e.zone(f).Push(top)
					moved = true
				}
			}
		}
	}
}

// takeOff moves each built run from king to ace of one suit off its column.
func (e *pat) takeOff() {
	for i := 1; i <= e.p.Columns; i++ {
		id := catalog.Column(i)
		cs := e.pile(id)
		l := len(cs)
		if l < 13 {
			continue
		}
		run := cs[l-13:]
		whole := true
		for k, c := range run {
			if !c.Up || e.value(c) != 13-k || e.suit(c) != e.suit(run[0]) {
				whole = false
				break
			}
		}
		if !whole {
			continue
		}
		done := e.zone(catalog.PileDone)
		done.Cards = append(done.Cards, run...)
		z := e.zone(id)
		z.Cards = z.Cards[:l-13]
		if l > 13 {
			z.Cards[l-14].Up = true
		}
	}
}

// patienceDeal lays out the opening position: the deck shuffled with the
// seed, the columns dealt from it in order, and the rest left in the stock.
func patienceDeal(game *catalog.CompiledGame, seed uint64) *cards.Table {
	p := game.Patience
	t := cards.NewTable()
	z := &cards.Zone{}
	z.Fill(game.Cards.Deck(p.Deck), false)
	z.Shuffle(&cards.RNG{State: seed})
	deck := z.Cards
	for i := 1; i <= p.Columns; i++ {
		size := p.Sizes[i-1]
		cs := append([]cards.Card(nil), deck[:size]...)
		deck = deck[size:]
		if p.KingsUnder {
			var kings, rest []cards.Card
			for _, c := range cs {
				if t := game.Cards.Type(c.ID); t != nil && t.Value == 13 {
					kings = append(kings, c)
				} else {
					rest = append(rest, c)
				}
			}
			cs = append(kings, rest...)
		}
		for k := range cs {
			cs[k].Up = k >= p.Down[i-1]
		}
		t.Zone(catalog.Column(i), 0).Cards = cs
	}
	for k := 1; k <= p.Foundations; k++ {
		t.Zone(catalog.Foundation(k), 0)
	}
	for i := 1; i <= p.Cells; i++ {
		t.Zone(catalog.Cell(i), 0)
	}
	if p.Draw > 0 {
		t.Zone(catalog.PileWaste, 0)
	}
	if p.Off {
		t.Zone(catalog.PileDone, 0)
	}
	t.Zone(catalog.PileStock, 0).Cards = deck
	return t
}

// validatePatience checks a patience move against the legal ones.
func (c *rctx) validatePatience(act catalog.ResolvedAction, mv Selection) (executor, error) {
	e := patienceOf(c.game, c.st)
	want := mv.Key()
	if mv.Piece == "" && mv.Count == 0 {
		mv.Count = 1
		want = mv.Key()
	}
	for _, m := range e.legal() {
		if m.Key() == want {
			return func(ns *BoardState) []board.PositionID {
				patienceOf(c.game, ns).play(m)
				return nil
			}, nil
		}
	}
	return nil, fmt.Errorf("state: %q is not a legal move now", formatPatience(mv))
}

func (c *rctx) enumeratePatience(act catalog.ResolvedAction, keep func(catalog.ResolvedAction, Selection, bool)) {
	for _, m := range patienceOf(c.game, c.st).legal() {
		if c.enough() {
			return
		}
		keep(act, m, true)
	}
}

// patienceOutcome is how a game of patience stands: won with every card home
// or off, lost when no move is left.
func patienceOutcome(game *catalog.CompiledGame, st *BoardState) Outcome {
	e := patienceOf(game, st)
	detail := fmt.Sprintf("%d of %d cards home", e.homeCount(), game.Patience.Total)
	if e.homeCount() == game.Patience.Total {
		w := game.Definition.Seats[0].Seat
		return Outcome{Ended: true, Winner: &w, Reason: patienceCleared, Detail: detail}
	}
	if len(e.legal()) == 0 {
		return Outcome{Ended: true, Reason: patienceStuck, Detail: detail}
	}
	return Outcome{}
}

// formatPatience writes a patience move in Solitaire's notation.
func formatPatience(mv Selection) string {
	if mv.Piece != "" {
		return mv.Piece
	}
	from := string(mv.From)
	if mv.Count > 1 {
		from += "." + strconv.Itoa(mv.Count)
	}
	return from + "-" + string(mv.To)
}

// parsePatience reads Solitaire's notation.
func parsePatience(text string) (Selection, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	switch t {
	case patienceDraw, patienceRecycle, patienceRow, patienceFinish:
		return Selection{Action: catalog.ActionPatience, Piece: t}, nil
	}
	fromSpec, to, ok := strings.Cut(t, "-")
	if !ok {
		return Selection{}, fmt.Errorf("state: cannot read %q as a move (from-to, draw, recycle, deal or finish)", text)
	}
	pile, n := fromSpec, 1
	if p, count, ok := strings.Cut(fromSpec, "."); ok {
		var err error
		if n, err = strconv.Atoi(count); err != nil || n < 1 {
			return Selection{}, fmt.Errorf("state: bad count in %q", text)
		}
		pile = p
	}
	if !patiencePile(pile) || (to != "f" && to != "c" && !patiencePile(to)) {
		return Selection{}, fmt.Errorf("state: unknown pile in %q", text)
	}
	return Selection{Action: catalog.ActionPatience, From: board.PositionID(pile), Count: n, To: board.PositionID(to)}, nil
}

func patiencePile(s string) bool {
	if s == "w" {
		return true
	}
	if len(s) < 2 || !strings.ContainsRune("tcf", rune(s[0])) {
		return false
	}
	_, err := strconv.Atoi(s[1:])
	return err == nil
}

// patienceGestures are a move's clicks, as the page's patience table makes
// them: the cards to lift and where they go, or the word.
func patienceGestures(mv Selection) []Gesture {
	if mv.Piece != "" {
		return []Gesture{{GestureAction, mv.Piece}}
	}
	return []Gesture{{GestureAction, fmt.Sprintf("from-%s-%d", mv.From, max(mv.Count, 1))}, {GestureAction, "to-" + string(mv.To)}}
}

// patienceWorth orders the moves for a hint, as Solitaire does: finish, then
// home, then a card turned over, an empty column, a card off a cell, a run
// built in suit; then the rest, the stock, and last a card back from a
// foundation or into a cell.
func (e *pat) worth(m Selection) int {
	switch m.Piece {
	case patienceFinish:
		return 100
	case patienceDraw, patienceRow:
		return 1
	case patienceRecycle:
		return -1
	}
	pile, n, to := string(m.From), max(m.Count, 1), string(m.To)
	switch to {
	case "f":
		return 50
	case "c":
		return -3
	}
	from := e.pile(sourceZone(pile))
	switch {
	case strings.HasPrefix(pile, "f"):
		return -2
	case !strings.HasPrefix(pile, "t"):
		return 20
	case len(from) == n:
		if e.count(to) == 0 {
			return -1
		}
		return 30
	case !from[len(from)-n-1].Up:
		return 40
	}
	if top, ok := e.top(to); ok && e.p.Lift == "same-suit" && e.suit(from[len(from)-n]) == e.suit(top) {
		return 25
	}
	return 0
}

// patienceGuide keeps the moves the hint order puts first.
func patienceGuide(game *catalog.CompiledGame, st *BoardState, legal []Selection) []Selection {
	e := patienceOf(game, st)
	best := -1 << 30
	var out []Selection
	for _, m := range legal {
		v := e.worth(m)
		if v > best {
			best, out = v, nil
		}
		if v == best {
			out = append(out, m)
		}
	}
	return out
}
