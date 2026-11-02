package solitaire

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/cards"
)

// Klondike, Spider and FreeCell: columns of cards, built down, on their way home.
//
// A move is written from-to, and the piles are t1…t10 (the columns), w (the waste),
// c1…c4 (free cells), f1…f4 (the foundations, one per suit in the game's order of
// suits). A run of several cards names how many: t3.2-t5. A card going home is -f, a
// card going to a free cell -c: which one does not matter, so there is one move.

const (
	moveDraw    = "draw"
	moveRecycle = "recycle"
	moveDeal    = "deal"
	moveFinish  = "finish"
)

func (e *eng) columns() int { return e.game.ParamInt("columns", 7) }
func (e *eng) freeCells() int {
	if e.d.Mode == modeFreeCell {
		return e.game.ParamInt("cells", 4)
	}
	return 0
}
func (e *eng) hasFoundations() bool { return e.d.Mode != modeSpider }

func col(i int) string  { return "t" + strconv.Itoa(i) }
func cell(i int) string { return "c" + strconv.Itoa(i) }
func fnd(i int) string  { return "f" + strconv.Itoa(i) }

// foundationOf is the foundation of a card's suit.
func (e *eng) foundationOf(c cards.Card) string {
	s := e.suit(c)
	for i, id := range e.game.Cards.Suits {
		if id == s {
			return fnd(i + 1)
		}
	}
	return fnd(1)
}

// differentColour: the suits alternate in the game's order, so red and black are the
// odd and even places. That is what a themed set keeps too, since it wears the suits
// by place.
func (e *eng) differentColour(a, b cards.Card) bool {
	place := func(c cards.Card) int {
		s := e.suit(c)
		for i, id := range e.game.Cards.Suits {
			if id == s {
				return i
			}
		}
		return 0
	}
	return place(a)%2 != place(b)%2
}

// patienceDeal lays out the opening position.
func (e *eng) patienceDeal() {
	deck := e.shuffled()
	take := func(n int, up bool) []cards.Card {
		out := append([]cards.Card(nil), deck[:n]...)
		deck = deck[n:]
		for i := range out {
			out[i].Up = up
		}
		return out
	}
	n := e.columns()
	for i := 1; i <= n; i++ {
		z := e.zone(col(i))
		switch e.d.Mode {
		case modeKlondike:
			z.Cards = take(i, false)
		case modeSpider:
			size := 5
			if i <= 4 {
				size = 6
			}
			z.Cards = take(size, false)
		case modeFreeCell:
			size := 6
			if i <= 4 {
				size = 7
			}
			z.Cards = take(size, true)
		}
		if l := len(z.Cards); l > 0 {
			z.Cards[l-1].Up = true
		}
	}
	if e.hasFoundations() {
		for i := range e.game.Cards.Suits {
			e.zone(fnd(i + 1))
		}
	}
	for i := 1; i <= e.freeCells(); i++ {
		e.zone(cell(i))
	}
	// Every zone the rules read exists from here on: reading one must not write the table.
	e.zone(zoneWaste)
	e.zone(zoneStock).Cards = take(len(deck), false)
	if e.d.Mode == modeSpider {
		e.zone(zoneDone)
	}
}

// home is how many cards have come home.
func (e *eng) home() int {
	if e.d.Mode == modeSpider {
		return e.zone(zoneDone).Len()
	}
	n := 0
	for i := range e.game.Cards.Suits {
		n += e.zone(fnd(i + 1)).Len()
	}
	return n
}

// run is how many cards can be lifted off the end of a column together.
func (e *eng) run(z *cards.Zone) int {
	l := len(z.Cards)
	if l == 0 || !z.Cards[l-1].Up {
		return 0
	}
	n := 1
	for i := l - 1; i > 0; i-- {
		upper, lower := z.Cards[i-1], z.Cards[i]
		if !upper.Up || e.value(upper) != e.value(lower)+1 {
			break
		}
		if e.d.Mode == modeSpider {
			if e.suit(upper) != e.suit(lower) {
				break
			}
		} else if !e.differentColour(upper, lower) {
			break
		}
		n++
	}
	return n
}

// fitsColumn says whether card c can go on column z.
func (e *eng) fitsColumn(c cards.Card, z *cards.Zone) bool {
	top, ok := z.Top()
	if !ok {
		return e.d.Mode != modeKlondike || e.value(c) == 13
	}
	if !top.Up || e.value(top) != e.value(c)+1 {
		return false
	}
	return e.d.Mode == modeSpider || e.differentColour(top, c)
}

// fitsHome says whether c is the next card of its foundation.
func (e *eng) fitsHome(c cards.Card) bool {
	return e.value(c) == e.zone(e.foundationOf(c)).Len()+1
}

// capacity is how long a run FreeCell can move in one go, moving through the free
// cells and the empty columns; toEmpty says the column it goes to is one of those.
func (e *eng) capacity(toEmpty bool) int {
	free, empty := 0, 0
	for i := 1; i <= e.freeCells(); i++ {
		if e.zone(cell(i)).Len() == 0 {
			free++
		}
	}
	for i := 1; i <= e.columns(); i++ {
		if e.zone(col(i)).Len() == 0 {
			empty++
		}
	}
	if toEmpty {
		empty--
	}
	return (free + 1) << max(empty, 0)
}

// patienceLegal lists the moves, in a fixed order.
func (e *eng) patienceLegal() []string {
	var out []string
	if e.canFinish() {
		out = append(out, moveFinish)
	}
	n := e.columns()
	// A single card, from any pile that gives one up, to the columns.
	toColumns := func(from string, c cards.Card) {
		for j := 1; j <= n; j++ {
			if col(j) != from && e.fitsColumn(c, e.zone(col(j))) {
				out = append(out, from+"-"+col(j))
			}
		}
	}
	firstFree := ""
	for i := 1; i <= e.freeCells(); i++ {
		if e.zone(cell(i)).Len() == 0 {
			firstFree = "c"
			break
		}
	}
	for i := 1; i <= n; i++ {
		from := col(i)
		z := e.zone(from)
		run := e.run(z)
		l := z.Len()
		for k := 1; k <= run; k++ {
			c := z.Cards[l-k]
			for j := 1; j <= n; j++ {
				if j == i {
					continue
				}
				dest := e.zone(col(j))
				if !e.fitsColumn(c, dest) {
					continue
				}
				// A whole column to an empty one changes nothing.
				if dest.Len() == 0 && k == l {
					continue
				}
				if e.d.Mode == modeFreeCell && k > e.capacity(dest.Len() == 0) {
					continue
				}
				out = append(out, spec(from, k)+"-"+col(j))
			}
		}
		if run > 0 {
			top := z.Cards[l-1]
			if e.hasFoundations() && e.fitsHome(top) {
				out = append(out, from+"-f")
			}
			if firstFree != "" {
				out = append(out, from+"-"+firstFree)
			}
		}
	}
	if e.d.Mode == modeKlondike {
		if top, ok := e.zone(zoneWaste).Top(); ok {
			if e.fitsHome(top) {
				out = append(out, "w-f")
			}
			toColumns("w", top)
		}
		for i := range e.game.Cards.Suits {
			if top, ok := e.zone(fnd(i + 1)).Top(); ok {
				toColumns(fnd(i+1), top)
			}
		}
		if e.zone(zoneStock).Len() > 0 {
			out = append(out, moveDraw)
		} else if e.zone(zoneWaste).Len() > 0 {
			out = append(out, moveRecycle)
		}
	}
	for i := 1; i <= e.freeCells(); i++ {
		if top, ok := e.zone(cell(i)).Top(); ok {
			if e.fitsHome(top) {
				out = append(out, cell(i)+"-f")
			}
			toColumns(cell(i), top)
		}
	}
	if e.d.Mode == modeSpider && e.zone(zoneStock).Len() > 0 && !e.anyEmptyColumn() {
		out = append(out, moveDeal)
	}
	return out
}

func (e *eng) anyEmptyColumn() bool {
	for i := 1; i <= e.columns(); i++ {
		if e.zone(col(i)).Len() == 0 {
			return true
		}
	}
	return false
}

func spec(from string, n int) string {
	if n == 1 {
		return from
	}
	return from + "." + strconv.Itoa(n)
}

// parseFrom splits "t3.2" into the pile and the number of cards.
func parseFrom(s string) (string, int) {
	pile, count, ok := strings.Cut(s, ".")
	if !ok {
		return pile, 1
	}
	n, _ := strconv.Atoi(count)
	return pile, n
}

// sourceZone is the zone a move's source names.
func sourceZone(pile string) string {
	if pile == "w" {
		return zoneWaste
	}
	return pile
}

// canFinish says the game is decided: everything can go home from where it lies.
// Nothing is hidden and nothing waits in the stock, so the last clicks are only clicks.
func (e *eng) canFinish() bool {
	if e.d.Mode == modeSpider || e.zone(zoneStock).Len() > 0 || e.zone(zoneWaste).Len() > 0 {
		return false
	}
	tops := map[string]int{}
	for i := range e.game.Cards.Suits {
		tops[fnd(i+1)] = e.zone(fnd(i + 1)).Len()
	}
	piles := [][]cards.Card{}
	for i := 1; i <= e.columns(); i++ {
		z := e.zone(col(i))
		for _, c := range z.Cards {
			if !c.Up {
				return false
			}
		}
		piles = append(piles, append([]cards.Card(nil), z.Cards...))
	}
	for i := 1; i <= e.freeCells(); i++ {
		piles = append(piles, append([]cards.Card(nil), e.zone(cell(i)).Cards...))
	}
	for moved := true; moved; {
		moved = false
		for i, p := range piles {
			if len(p) == 0 {
				continue
			}
			c := p[len(p)-1]
			f := e.foundationOf(c)
			if e.value(c) == tops[f]+1 {
				tops[f]++
				piles[i] = p[:len(p)-1]
				moved = true
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

// patiencePlay plays a legal move, and sees what it did.
func (e *eng) patiencePlay(move string) {
	switch move {
	case moveDraw:
		stock, waste := e.zone(zoneStock), e.zone(zoneWaste)
		for _, c := range stock.Draw(e.game.ParamInt("draw", 1)) {
			c.Up = true
			waste.Push(c)
		}
	case moveRecycle:
		stock, waste := e.zone(zoneStock), e.zone(zoneWaste)
		for i := waste.Len() - 1; i >= 0; i-- {
			c := waste.Cards[i]
			c.Up = false
			stock.Push(c)
		}
		waste.Cards = nil
		e.say("The waste goes back to the stock.")
	case moveDeal:
		stock := e.zone(zoneStock)
		for i := 1; i <= e.columns(); i++ {
			for _, c := range stock.Draw(1) {
				c.Up = true
				e.zone(col(i)).Push(c)
			}
		}
		e.say("A new row is dealt: %d left to deal.", stock.Len()/e.columns())
	case moveFinish:
		e.finish()
	default:
		fromSpec, to, _ := strings.Cut(move, "-")
		pile, n := parseFrom(fromSpec)
		from := e.zone(sourceZone(pile))
		lift := append([]cards.Card(nil), from.Cards[from.Len()-n:]...)
		from.Cards = from.Cards[:from.Len()-n]
		dest := to
		switch to {
		case "f":
			dest = e.foundationOf(lift[0])
		case "c":
			for i := 1; i <= e.freeCells(); i++ {
				if e.zone(cell(i)).Len() == 0 {
					dest = cell(i)
					break
				}
			}
		}
		d := e.zone(dest)
		d.Cards = append(d.Cards, lift...)
		if top := from.Len() - 1; top >= 0 && strings.HasPrefix(pile, "t") {
			from.Cards[top].Up = true
		}
	}
	e.settle()
}

// finish sends everything home.
func (e *eng) finish() {
	piles := []string{}
	for i := 1; i <= e.columns(); i++ {
		piles = append(piles, col(i))
	}
	for i := 1; i <= e.freeCells(); i++ {
		piles = append(piles, cell(i))
	}
	for moved := true; moved; {
		moved = false
		for _, p := range piles {
			z := e.zone(p)
			if top, ok := z.Top(); ok && e.fitsHome(top) {
				z.Cards = z.Cards[:z.Len()-1]
				e.zone(e.foundationOf(top)).Push(top)
				moved = true
			}
		}
	}
}

// settle takes Spider's finished suits off the columns and sees whether the game is over.
func (e *eng) settle() {
	if e.d.Mode == modeSpider {
		e.takeFinishedSuits()
	}
	total := len(e.game.Cards.Deck("main"))
	switch {
	case e.home() == total:
		e.d.Turn, e.d.Won, e.d.Reason = 0, true, reasonCleared
		e.say("Every card is home!")
	case len(e.patienceLegal()) == 0:
		e.d.Turn, e.d.Reason = 0, reasonStuck
		e.say("No move is left.")
	}
}

// takeFinishedSuits moves a run from king to ace of one suit off a column.
func (e *eng) takeFinishedSuits() {
	for i := 1; i <= e.columns(); i++ {
		z := e.zone(col(i))
		if e.run(z) < 13 {
			continue
		}
		l := z.Len()
		// The run is as long as it gets at the king, and starts at the ace.
		if e.value(z.Cards[l-13]) != 13 || e.value(z.Cards[l-1]) != 1 {
			continue
		}
		done := e.zone(zoneDone)
		done.Cards = append(done.Cards, z.Cards[l-13:]...)
		z.Cards = z.Cards[:l-13]
		if l > 13 {
			z.Cards[l-14].Up = true
		}
		e.say("A suit is finished: %d of %d.", done.Len()/13, len(e.game.Cards.Deck("main"))/13)
	}
}

// worth orders the moves for a hint: going home first, then anything that turns a
// card over or empties a column, then other moves; the stock when nothing else helps.
func (e *eng) worth(move string) int {
	if e.d.Mode == modeBowling {
		return e.bowlingWorth(move)
	}
	switch move {
	case moveFinish:
		return 100
	case moveDraw, moveDeal:
		return 1
	case moveRecycle:
		return -1
	}
	fromSpec, to, _ := strings.Cut(move, "-")
	pile, n := parseFrom(fromSpec)
	switch to {
	case "f":
		return 50
	case "c":
		return -3 // a free cell is spent, for nothing yet
	}
	from, dest := e.zone(sourceZone(pile)), e.zone(to)
	switch {
	case strings.HasPrefix(pile, "f"):
		return -2 // back from a foundation: only ever on purpose
	case !strings.HasPrefix(pile, "t"):
		return 20
	case from.Len() == n:
		if dest.Len() == 0 {
			return -1
		}
		return 30 // empties a column
	case !from.Cards[from.Len()-n-1].Up:
		return 40 // turns a card over
	}
	if top, ok := dest.Top(); ok && e.d.Mode == modeSpider && e.suit(from.Cards[from.Len()-n]) == e.suit(top) {
		return 25 // builds in suit
	}
	return 0
}

// validPatience checks that text reads as a move of this game's kind.
func validPatience(text string) error {
	switch text {
	case moveDraw, moveRecycle, moveDeal, moveFinish:
		return nil
	}
	from, to, ok := strings.Cut(text, "-")
	if !ok {
		return fmt.Errorf("expected from-to, draw, recycle, deal or finish")
	}
	pile, n := parseFrom(from)
	if n < 1 || !pileName(pile) || (to != "f" && to != "c" && !pileName(to)) {
		return fmt.Errorf("unknown pile in %q", text)
	}
	return nil
}

func pileName(s string) bool {
	if s == "w" {
		return true
	}
	if len(s) < 2 || !strings.ContainsRune("tcf", rune(s[0])) {
		return false
	}
	_, err := strconv.Atoi(s[1:])
	return err == nil
}
