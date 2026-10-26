package nyet

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is Nyet!'s state.Ruleset. Every decision is one click: cross a box
// of the board, pick the team, pick the card to discard or pass, play a card.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

// Seeded reports that the start position carries a shuffling seed to record.
func (*Ruleset) Seeded() bool { return true }

const (
	actCross   = "cross"   // Piece "line:option"
	actTeam    = "team"    // Piece "alone" or the partners' seats "2,3"
	actBonus   = "bonus"   // Piece: the seat given the bonus card
	actDiscard = "discard" // Piece: a card type id
	actGive    = "give"    // Piece: a card type id to pass left
	actPlay    = "play"    // Piece: a card type id
)

// box is one option of a line of the board.
type box struct {
	Line, Option string
}

func (b box) key() string { return b.Line + ":" + b.Option }

var colours = []string{"red", "blue", "yellow", "green"}

// lineOrder is the order of the lines on the board.
var lineOrder = []string{lineFirst, lineDiscard, lineTrump, lineSuper, linePoints}

// options lists the options of a line for n players (all five characters for
// the first-player line: the absent ones are covered for the whole game).
func options(line string, n int) []string {
	switch line {
	case lineFirst:
		var o []string
		for s := 1; s <= 5; s++ {
			o = append(o, strconv.Itoa(s))
		}
		return o
	case lineDiscard:
		return []string{"none", "1", "2", "except1", "passleft"}
	case lineTrump:
		return colours
	case lineSuper:
		return append(append([]string(nil), colours...), "none")
	case linePoints:
		return []string{"1", "2", "3", "4", "-2"}
	}
	return nil
}

// covered reports whether a box is covered for the whole game: a character
// that is not at the table.
func covered(b box, n int) bool {
	if b.Line != lineFirst {
		return false
	}
	s, _ := strconv.Atoi(b.Option)
	return s > n
}

// eng is a decoded position.
type eng struct {
	game *catalog.CompiledGame
	cc   *catalog.CompiledCards
	d    *Data
	n    int
	ev   []string
}

func newEng(game *catalog.CompiledGame, d *Data) *eng {
	return &eng{game: game, cc: game.Cards, d: d, n: len(game.Definition.Seats)}
}

func (e *eng) say(format string, args ...any) { e.ev = append(e.ev, fmt.Sprintf(format, args...)) }

func (e *eng) label(seat int) string {
	for _, s := range e.game.Definition.Seats {
		if s.Seat == seat {
			if s.Label != "" {
				return s.Label
			}
			return s.Color
		}
	}
	return fmt.Sprintf("Seat %d", seat)
}

// next is the seat after seat, clockwise.
func (e *eng) next(seat int) int { return seat%e.n + 1 }

func (e *eng) rounds() int   { return e.game.ParamInt("rounds", 8) }
func (e *eng) handSize() int { return e.game.ParamInt("hand", 15) }

// empties lists the boxes of a line that are still open.
func (e *eng) empties(line string) []box {
	var out []box
	for _, o := range options(line, e.n) {
		b := box{line, o}
		if covered(b, e.n) {
			continue
		}
		if _, crossed := e.d.Crossed[b.key()]; !crossed {
			out = append(out, b)
		}
	}
	return out
}

func (e *eng) typeOf(id string) *catalog.CardType { return e.cc.Type(id) }

// effective is the colour a card counts as in the trick phase: the super
// trump 1s turn into the trump colour whatever their own colour.
func (e *eng) effective(id string) string {
	t := e.typeOf(id)
	if e.isSuper(id) {
		return e.d.Trump
	}
	return t.Suit
}

func (e *eng) isSuper(id string) bool {
	t := e.typeOf(id)
	return e.d.Super != "" && e.d.Super != "none" && t.Suit == e.d.Super && t.Value == 1
}

func (e *eng) onTeam(seat int) bool {
	for _, s := range e.d.Team {
		if s == seat {
			return true
		}
	}
	return false
}

// dealRound shuffles all the cards, deals every hand and opens the Nyet! phase.
func (e *eng) dealRound() {
	d := e.d
	t := d.Table
	deck := t.Zone("deck", 0)
	// Gather every card back, in a fixed order so a position is reproducible.
	keys := make([]string, 0, len(t.Zones))
	for k := range t.Zones {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k != "deck" {
			deck.Cards = append(deck.Cards, t.Zones[k].Clear()...)
		}
	}
	if deck.Len() == 0 {
		deck.Fill(e.cc.Deck("main"), false)
	}
	deck.FlipAll(false)
	rng := &cards.RNG{State: d.RNG}
	deck.Shuffle(rng)
	d.RNG = rng.State
	var hands []*cards.Zone
	for s := 1; s <= e.n; s++ {
		hands = append(hands, t.Zone("hand", s))
	}
	cards.Deal(deck, hands, e.handSize())
	d.Phase, d.Turn = phaseNyet, d.Dealer
	d.Crossed = map[string]int{}
	d.First, d.Disc, d.Trump, d.Super, d.Points = 0, "", "", "", 0
	d.Team, d.Bonus, d.DiscardDone, d.Pending = nil, 0, 0, map[int]string{}
	d.Plays, d.Tricks, d.Loot = nil, [6]int{}, [6]int{}
	e.say("Round %d of %d: %s deals.", d.Round, e.rounds(), e.label(d.Dealer))
}

// handTypes lists the card types in a seat's hand (one entry per type), in the
// order of the hand.
func (e *eng) handTypes(seat int, keep func(id string) bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range e.d.Table.Zone("hand", seat).Cards {
		tid := catalog.TypeID(c.ID)
		if seen[tid] || (keep != nil && !keep(c.ID)) {
			continue
		}
		seen[tid] = true
		out = append(out, tid)
	}
	sort.SliceStable(out, func(i, j int) bool { return cardLess(e.cc, out[i], out[j]) })
	return out
}

func cardLess(cc *catalog.CompiledCards, a, b string) bool {
	ta, tb := cc.Types[a], cc.Types[b]
	if ta.Suit != tb.Suit {
		return suitIndex(cc, ta.Suit) < suitIndex(cc, tb.Suit)
	}
	return ta.Value < tb.Value
}

func suitIndex(cc *catalog.CompiledCards, suit string) int {
	for i, s := range cc.Suits {
		if s == suit {
			return i
		}
	}
	return len(cc.Suits)
}

// instanceOf finds a card of the given type in the seat's hand.
func (e *eng) instanceOf(seat int, typeID string) (string, bool) {
	for _, c := range e.d.Table.Zone("hand", seat).Cards {
		if catalog.TypeID(c.ID) == typeID {
			return c.ID, true
		}
	}
	return "", false
}

// legal lists the decisions of the seat to move.
func (e *eng) legal() []state.Move {
	d := e.d
	if d.Turn == 0 {
		return nil
	}
	var out []state.Move
	switch d.Phase {
	case phaseNyet:
		for _, line := range lineOrder {
			open := e.empties(line)
			if len(open) < 2 {
				continue
			}
			for _, b := range open {
				out = append(out, state.Move{Action: actCross, Piece: b.key()})
			}
		}
	case phaseTeam:
		out = e.teamMoves()
	case phaseBonus:
		for _, s := range e.smallTeam() {
			out = append(out, state.Move{Action: actBonus, Piece: strconv.Itoa(s)})
		}
	case phaseDiscard:
		seat := d.Turn
		keep := func(id string) bool { return true }
		if d.Disc == "except1" {
			keep = func(id string) bool { return e.typeOf(id).Value != 1 }
		}
		act := actDiscard
		if d.Disc == "passleft" {
			act = actGive
		}
		for _, tid := range e.handTypes(seat, keep) {
			out = append(out, state.Move{Action: act, Piece: tid})
		}
	case phaseTrick:
		for _, tid := range e.playable(d.Turn) {
			out = append(out, state.Move{Action: actPlay, Piece: tid})
		}
	}
	return out
}

// playable lists the card types a seat may play to the current trick: it must
// follow the colour led if it has one of it.
func (e *eng) playable(seat int) []string {
	hand := e.handTypes(seat, nil)
	if len(e.d.Plays) == 0 {
		return hand
	}
	led := e.effective(e.d.Plays[0].Card)
	var follow []string
	for _, tid := range hand {
		id, _ := e.instanceOf(seat, tid)
		if e.effective(id) == led {
			follow = append(follow, tid)
		}
	}
	if len(follow) > 0 {
		return follow
	}
	return hand
}

// teamMoves lists the ways the first player may form the teams.
func (e *eng) teamMoves() []state.Move {
	first := e.d.First
	var others []int
	for s := 1; s <= e.n; s++ {
		if s != first {
			others = append(others, s)
		}
	}
	join := func(ss []int) string {
		parts := make([]string, len(ss))
		for i, s := range ss {
			parts[i] = strconv.Itoa(s)
		}
		return strings.Join(parts, ",")
	}
	var out []state.Move
	add := func(ss []int) {
		p := join(ss)
		if p == "" {
			p = "alone"
		}
		out = append(out, state.Move{Action: actTeam, Piece: p})
	}
	switch e.n {
	case 3:
		add(nil)
		for _, s := range others {
			add([]int{s})
		}
	case 4:
		for _, s := range others {
			add([]int{s})
		}
	case 5:
		for _, s := range others {
			add([]int{s})
		}
		for i := 0; i < len(others); i++ {
			for j := i + 1; j < len(others); j++ {
				add([]int{others[i], others[j]})
			}
		}
	}
	return out
}

// smallTeam is the team that holds the bonus card in a game with an odd number
// of players (nil otherwise).
func (e *eng) smallTeam() []int {
	if e.n%2 == 0 {
		return nil
	}
	var rest []int
	for s := 1; s <= e.n; s++ {
		if !e.onTeam(s) {
			rest = append(rest, s)
		}
	}
	if len(e.d.Team) < len(rest) {
		return e.d.Team
	}
	return rest
}

// execute plays a legal decision.
func (e *eng) execute(mv state.Move, seat int) {
	d := e.d
	t := d.Table
	switch mv.Action {
	case actCross:
		d.Crossed[mv.Piece] = seat
		line, option, _ := strings.Cut(mv.Piece, ":")
		e.say("%s crosses off %s on the %s line.", e.label(seat), optionName(e, line, option), lineName(line))
		for _, l := range lineOrder {
			if len(e.empties(l)) > 1 {
				d.Turn = e.next(seat)
				return
			}
		}
		e.settleTerms()
	case actTeam:
		d.Team = []int{d.First}
		if mv.Piece != "alone" {
			for _, p := range strings.Split(mv.Piece, ",") {
				s, _ := strconv.Atoi(p)
				d.Team = append(d.Team, s)
			}
		}
		sort.Ints(d.Team)
		e.say("%s plays with %s.", e.label(seat), e.teamNames())
		switch e.n {
		case 3:
			d.Bonus = d.First // alone: the bonus is the solo player's
			if len(d.Team) == 2 {
				for s := 1; s <= e.n; s++ {
					if !e.onTeam(s) {
						d.Bonus = s
					}
				}
			}
			e.say("%s takes the x2 bonus card.", e.label(d.Bonus))
		case 5:
			d.Phase = phaseBonus
			return
		}
		e.startDiscard()
	case actBonus:
		d.Bonus, _ = strconv.Atoi(mv.Piece)
		e.say("%s gives the x2 bonus card to %s.", e.label(seat), e.label(d.Bonus))
		e.startDiscard()
	case actDiscard:
		id, _ := e.instanceOf(seat, mv.Piece)
		t.Move(id, "hand", seat, "discard", seat, false)
		d.DiscardDone++
		e.say("%s discards a card.", e.label(seat))
		e.advanceDiscard()
	case actGive:
		id, _ := e.instanceOf(seat, mv.Piece)
		d.Pending[seat] = id
		d.DiscardDone++
		e.say("%s chooses a card to pass left.", e.label(seat))
		if len(d.Pending) < e.n {
			e.advanceDiscard()
			return
		}
		// Everyone has chosen: pass the cards left at once.
		for s := 1; s <= e.n; s++ {
			to := e.next(s)
			t.Move(d.Pending[s], "hand", s, "hand", to, false)
		}
		d.Pending = map[int]string{}
		e.say("Every player passes the chosen card to the left.")
		e.startTricks()
	case actPlay:
		id, _ := e.instanceOf(seat, mv.Piece)
		t.Move(id, "hand", seat, "trick", 0, true)
		d.Plays = append(d.Plays, Play{Seat: seat, Card: id})
		e.say("%s plays %s.", e.label(seat), cardName(e, id))
		if len(d.Plays) < e.n {
			d.Turn = e.next(seat)
			return
		}
		e.resolveTrick()
	}
}

// settleTerms reads the terms off the board once every line has one box left.
func (e *eng) settleTerms() {
	d := e.d
	for _, l := range lineOrder {
		b := e.empties(l)[0]
		switch l {
		case lineFirst:
			d.First, _ = strconv.Atoi(b.Option)
		case lineDiscard:
			d.Disc = b.Option
		case lineTrump:
			d.Trump = b.Option
		case lineSuper:
			d.Super = b.Option
		case linePoints:
			d.Points, _ = strconv.Atoi(b.Option)
		}
	}
	e.say("The terms: %s leads; discard %s; trump %s; super trump %s; %s a trick.", e.label(d.First), discardName(d.Disc), suitName(d.Trump), superName(d.Super), pointsName(d.Points))
	if e.n == 2 {
		d.Team = []int{d.First}
		e.startDiscard()
		return
	}
	d.Phase, d.Turn = phaseTeam, d.First
}

// startDiscard opens the discard phase, or goes straight to the tricks.
func (e *eng) startDiscard() {
	d := e.d
	d.Pending = map[int]string{}
	d.DiscardDone = 0
	if d.Disc == "none" {
		e.startTricks()
		return
	}
	d.Phase, d.Turn = phaseDiscard, d.First
	e.advanceDiscard()
}

// discardsEach is how many cards each player discards in this round.
func (e *eng) discardsEach() int {
	if e.d.Disc == "2" {
		return 2
	}
	return 1
}

// advanceDiscard moves to the next discard decision, skipping players who
// cannot make one, and on to the tricks when all are done.
func (e *eng) advanceDiscard() {
	d := e.d
	per := e.discardsEach()
	for d.DiscardDone < e.n*per {
		seat := (d.First-1+d.DiscardDone/per)%e.n + 1
		d.Turn = seat
		if len(e.legalDiscard(seat)) > 0 {
			return
		}
		d.DiscardDone++ // nothing to discard: skip
	}
	if d.Disc == "passleft" && len(d.Pending) > 0 {
		return
	}
	e.startTricks()
}

func (e *eng) legalDiscard(seat int) []string {
	keep := func(id string) bool { return true }
	if e.d.Disc == "except1" {
		keep = func(id string) bool { return e.typeOf(id).Value != 1 }
	}
	return e.handTypes(seat, keep)
}

func (e *eng) startTricks() {
	d := e.d
	d.Phase, d.Turn = phaseTrick, d.First
	d.Plays, d.LastTrick, d.LastWinner = nil, nil, 0
	d.Pending = map[int]string{}
	e.say("The tricks begin: %s leads.", e.label(d.First))
}

// trickWinner is the seat that takes the trick the plays make.
func (e *eng) trickWinner(plays []Play) int {
	led := e.effective(plays[0].Card)
	best, bestRank, bestVal := 0, -1, -1
	for _, p := range plays {
		t := e.typeOf(p.Card)
		eff := e.effective(p.Card)
		rank := 0 // 0 off-colour (never wins), 1 led colour, 2 trump, 3 super trump
		switch {
		case e.isSuper(p.Card):
			rank = 3
		case eff == e.d.Trump:
			rank = 2
		case eff == led:
			rank = 1
		}
		if rank == 0 {
			continue
		}
		// A later card of the same rank and value wins ties.
		if rank > bestRank || (rank == bestRank && t.Value >= bestVal) {
			best, bestRank, bestVal = p.Seat, rank, t.Value
		}
	}
	return best
}

// resolveTrick gives the trick to its winner, with the loot, and goes on.
func (e *eng) resolveTrick() {
	d := e.d
	t := d.Table
	winner := e.trickWinner(d.Plays)
	d.Tricks[winner]++
	loot := 0
	for _, p := range d.Plays {
		if e.typeOf(p.Card).Value == 1 && e.onTeam(p.Seat) != e.onTeam(winner) {
			t.Move(p.Card, "trick", 0, "loot", winner, true)
			d.Loot[winner]++
			loot++
		} else {
			t.Move(p.Card, "trick", 0, "won", winner, false)
		}
	}
	d.LastTrick, d.LastWinner = d.Plays, winner
	d.Plays = nil
	if loot > 0 {
		e.say("%s takes the trick and %d loot.", e.label(winner), loot)
	} else {
		e.say("%s takes the trick.", e.label(winner))
	}
	if t.Zone("hand", winner).Len() > 0 {
		d.Turn = winner
		return
	}
	e.endRound()
}

// endRound scores the round and deals the next, or ends the game.
func (e *eng) endRound() {
	d := e.d
	total := func(team bool) int {
		n := 0
		for s := 1; s <= e.n; s++ {
			if e.onTeam(s) == team {
				n += d.Tricks[s] + d.Loot[s]
			}
		}
		return n
	}
	for _, team := range []bool{true, false} {
		pts := total(team) * d.Points
		var names []string
		for s := 1; s <= e.n; s++ {
			if e.onTeam(s) != team {
				continue
			}
			got := pts
			if s == d.Bonus {
				got *= 2
			}
			d.Scores[s] += got
			names = append(names, fmt.Sprintf("%s %+d", e.label(s), got))
		}
		e.say("Team of %d took %d trick(s) and loot, worth %d each: %s.", len(names), total(team), d.Points, strings.Join(names, ", "))
	}
	// The cards of the last trick stay on view until the next deal.
	if d.Round >= e.rounds() {
		best, winners := -1<<31, []int{}
		for s := 1; s <= e.n; s++ {
			switch {
			case d.Scores[s] > best:
				best, winners = d.Scores[s], []int{s}
			case d.Scores[s] == best:
				winners = append(winners, s)
			}
		}
		d.Turn, d.Reason = 0, "rounds"
		if len(winners) == 1 {
			d.Winner = winners[0]
			e.say("The game is over: %s wins with %d points.", e.label(d.Winner), best)
		} else {
			e.say("The game is over: a tie at %d points.", best)
		}
		return
	}
	d.Round++
	d.Dealer = e.next(d.Dealer)
	e.dealRound()
}

func discardName(o string) string {
	switch o {
	case "none":
		return "nothing"
	case "1":
		return "1 card"
	case "2":
		return "2 cards"
	case "except1":
		return "1 card (not a 1)"
	case "passleft":
		return "pass 1 card left"
	}
	return o
}

func pointsName(p int) string {
	if p < 0 {
		return fmt.Sprintf("%d points each", p)
	}
	return fmt.Sprintf("%d point(s) each", p)
}

func lineName(line string) string {
	switch line {
	case lineFirst:
		return "first player"
	case lineDiscard:
		return "discard"
	case lineTrump:
		return "trump"
	case lineSuper:
		return "super trump"
	case linePoints:
		return "points"
	}
	return line
}

func optionName(e *eng, line, option string) string {
	switch line {
	case lineFirst:
		s, _ := strconv.Atoi(option)
		return e.label(s)
	case lineDiscard:
		return discardName(option)
	case linePoints:
		p, _ := strconv.Atoi(option)
		return pointsName(p)
	case lineTrump:
		return suitName(option)
	case lineSuper:
		return superName(option)
	}
	return option
}

// superName says what a super trump box means: the 1s of a suit, or none.
func superName(option string) string {
	if option == "none" {
		return "no super trump"
	}
	return "the 1s of " + suitName(option)
}

func cardName(e *eng, id string) string {
	t := e.typeOf(id)
	return "the " + t.Label + " of " + suitName(t.Suit)
}

func (e *eng) teamNames() string {
	var names []string
	for _, s := range e.d.Team {
		names = append(names, e.label(s))
	}
	if len(names) == 1 {
		return "no partner"
	}
	return strings.Join(names[1:], " and ")
}

// --- ruleset interface ----------------------------------------------------

// Start deals the first round: seat 1 is the first dealer.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	if game.Cards == nil {
		return nil, fmt.Errorf("nyet: the definition has no cards")
	}
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	d := &Data{Round: 1, Dealer: 1, RNG: seed, Table: cards.NewTable()}
	e := newEng(game, d)
	e.dealRound()
	d.Events = e.ev
	st := state.NewBoardState()
	st.SetExt(d)
	return st, nil
}

// ToMove is the seat that decides next, or 0 once the game is over.
func (rs *Ruleset) ToMove(game *catalog.CompiledGame, st *state.BoardState) int {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	return d.Turn
}

// Outcome reports a finished game.
func (rs *Ruleset) Outcome(game *catalog.CompiledGame, st *state.BoardState) state.Outcome {
	d, err := dataOf(st)
	if err != nil || d.Turn != 0 {
		return state.Outcome{}
	}
	out := state.Outcome{Ended: true, Reason: d.Reason, Detail: scoreLine(newEng(game, d))}
	if d.Winner != 0 {
		w := d.Winner
		out.Winner = &w
	}
	return out
}

func scoreLine(e *eng) string {
	var parts []string
	for s := 1; s <= e.n; s++ {
		parts = append(parts, fmt.Sprintf("%s %d", e.label(s), e.d.Scores[s]))
	}
	return strings.Join(parts, ", ")
}

// Validate checks a position document.
func (rs *Ruleset) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	d, err := dataOf(st)
	if err != nil {
		return err
	}
	n := len(game.Definition.Seats)
	if d.Turn < 0 || d.Turn > n || d.Dealer < 1 || d.Dealer > n {
		return fmt.Errorf("nyet: bad turn %d / dealer %d", d.Turn, d.Dealer)
	}
	switch d.Phase {
	case phaseNyet, phaseTeam, phaseBonus, phaseDiscard, phaseTrick:
	default:
		return fmt.Errorf("nyet: unknown phase %q", d.Phase)
	}
	seen := map[string]bool{}
	for k, z := range d.Table.Zones {
		for _, c := range z.Cards {
			if c.ID == "" || game.Cards.Type(c.ID) == nil {
				return fmt.Errorf("nyet: unknown card %q in %s", c.ID, k)
			}
			if seen[c.ID] {
				return fmt.Errorf("nyet: card %q is in two places", c.ID)
			}
			seen[c.ID] = true
		}
	}
	return nil
}

// Legal lists the decisions of seat.
func (rs *Ruleset) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Move {
	d, err := dataOf(st)
	if err != nil || d.Turn == 0 || seat != d.Turn {
		return nil
	}
	return newEng(game, d).legal()
}

// Apply plays one decision.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("nyet: the game is over")
	}
	e := newEng(game, d)
	if seat != d.Turn {
		return nil, fmt.Errorf("nyet: it is %s's decision", e.label(d.Turn))
	}
	want := rs.Format(game, mv)
	var legal state.Move
	found := false
	for _, cand := range e.legal() {
		if rs.Format(game, cand) == want {
			legal, found = cand, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("nyet: %q is not a legal move now", want)
	}
	nd := d.clone()
	e.d = nd
	e.execute(legal, seat)
	nd.Events = e.ev
	ns := st.Clone()
	ns.SetExt(nd)
	ns.SetPly(st.Ply() + 1)
	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal), NextTurnSeat: nd.Turn}
	if nd.Turn == 0 {
		res.GameEnded = true
		res.NextTurnSeat = seat
		res.EndReason = nd.Reason
		res.EndDetail = scoreLine(e)
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
	}
	return res, nil
}
