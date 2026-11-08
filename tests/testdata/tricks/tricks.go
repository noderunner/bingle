// Package tricks is a card game only the tests play, never registered by the
// server: the public suite's vehicle for the parts of the platform that only
// a commercial card game used to exercise, now that it is built apart (game
// plugins plan, step 5). It has
//
//   - tables of three, four and five (variants), for lobbies, takeovers and
//     the clocks of a big table;
//   - hands only their owner sees (state.HiddenInfo: Redact, RedactMove and
//     Determinize), on the card table of the page;
//   - a board of choices crossed off in turn (the view's grid), whose
//     pictures and players' pictures its page script draws;
//   - tricks, and a plain deck of four suits that can wear a set of cards.
//
// It is not a game worth playing: every rule is there for a test.
package tricks

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/web/look"
)

// ID is the fixture's game id.
const ID = "test-tricks"

//go:embed tricks.yaml
var fsys embed.FS

//go:embed web
var webFS embed.FS

var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(fsys, "tricks.yaml") })

// Module returns the fixture's catalog.Module.
func Module() catalog.Module {
	scripts, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: ID, Definition: definition(), Rules: New(), Scripts: scripts, Web: look.MustLoadFile(webFS, "web/web.yaml")}
}

// Ruleset is the fixture's rules.
type Ruleset struct{}

var (
	_ state.SeededRuleset = (*Ruleset)(nil)
	_ state.HiddenInfo    = (*Ruleset)(nil)
	_ state.Describer     = (*Ruleset)(nil)
)

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

// Seeded: the deal comes from the game's seed.
func (*Ruleset) Seeded() bool { return true }

// The phases, the lines of the board and the decisions.
const (
	phaseTerms = "terms"
	phaseTrick = "trick"

	lineTrump  = "trump"
	linePoints = "points"

	actCross = "cross"
	actPlay  = "play"
)

var (
	lineOrder = []string{lineTrump, linePoints}
	lineBoxes = map[string][]string{lineTrump: {"red", "blue", "yellow", "green"}, linePoints: {"1", "2", "3"}}
)

// Play is a card played into the trick by a seat.
type Play struct {
	Seat int    `json:"seat"`
	Card string `json:"card"`
}

// Data is the game's part of a position. The cards are in Table.
type Data struct {
	// Phase is the stage, Turn the seat that decides (0 once the game is over).
	Phase string `json:"phase"`
	Turn  int    `json:"turn"`
	// RNG is the shuffling generator's state: secret in public views.
	RNG   uint64       `json:"rng,omitempty"`
	Table *cards.Table `json:"table,omitempty"`
	// Crossed maps a crossed box ("line:option") to the seat that crossed it.
	Crossed map[string]int `json:"crossed,omitempty"`
	// The terms, once every line has one box left.
	Trump  string `json:"trump,omitempty"`
	Points int    `json:"points,omitempty"`
	// Plays are the cards of the trick being played; LastTrick the one before.
	Plays      []Play `json:"plays,omitempty"`
	LastTrick  []Play `json:"last_trick,omitempty"`
	LastWinner int    `json:"last_winner,omitempty"`
	// Tricks counts each seat's tricks (index seat).
	Tricks [6]int   `json:"tricks"`
	Winner int      `json:"winner,omitempty"`
	Events []string `json:"events,omitempty"`
}

// CloneExt implements state.ExtClone.
func (d *Data) CloneExt() any { return d.clone() }

func (d *Data) clone() *Data {
	c := *d
	c.Table = d.Table.Clone()
	c.Crossed = make(map[string]int, len(d.Crossed))
	for k, v := range d.Crossed {
		c.Crossed[k] = v
	}
	c.Plays = append([]Play(nil), d.Plays...)
	c.LastTrick = append([]Play(nil), d.LastTrick...)
	c.Events = append([]string(nil), d.Events...)
	return &c
}

type publicExt struct {
	*Data
	View *View `json:"view"`
}

func (p *publicExt) CloneExt() any { return &publicExt{Data: p.clone(), View: p.View} }

func dataOf(st *state.BoardState) (*Data, error) {
	switch v := st.Ext().(type) {
	case *Data:
		return v, nil
	case *publicExt:
		return v.Data, nil
	case json.RawMessage:
		d := &Data{}
		if err := json.Unmarshal(v, d); err != nil {
			return nil, fmt.Errorf("tricks: bad position payload: %w", err)
		}
		if d.Table == nil {
			d.Table = cards.NewTable()
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("tricks: position has no game payload")
	default:
		return nil, fmt.Errorf("tricks: position payload has type %T", v)
	}
}

// eng is one position being read or changed.
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
			return s.Label
		}
	}
	return "Seat " + strconv.Itoa(seat)
}

func (e *eng) next(seat int) int { return seat%e.n + 1 }

// open lists the boxes of a line nobody has crossed off.
func (e *eng) open(line string) []string {
	var out []string
	for _, o := range lineBoxes[line] {
		if e.d.Crossed[line+":"+o] == 0 {
			out = append(out, o)
		}
	}
	return out
}

// deal shuffles the deck and deals every hand.
func (e *eng) deal() {
	d := e.d
	deck := d.Table.Zone("deck", 0)
	deck.Fill(e.cc.Deck("main"), false)
	rng := &cards.RNG{State: d.RNG}
	deck.Shuffle(rng)
	d.RNG = rng.State
	var hands []*cards.Zone
	for s := 1; s <= e.n; s++ {
		hands = append(hands, d.Table.Zone("hand", s))
	}
	cards.Deal(deck, hands, e.game.ParamInt("hand", 8))
	d.Phase, d.Turn, d.Crossed = phaseTerms, 1, map[string]int{}
	e.say("The cards are dealt: %s crosses off first.", e.label(1))
}

// handTypes lists the card types in a seat's hand, by suit and value.
func (e *eng) handTypes(seat int) []string {
	var out []string
	for _, c := range e.d.Table.Zone("hand", seat).Cards {
		out = append(out, catalog.TypeID(c.ID))
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := e.cc.Types[out[i]], e.cc.Types[out[j]]
		if a.Suit != b.Suit {
			return a.Suit < b.Suit
		}
		return a.Value < b.Value
	})
	return out
}

func (e *eng) instanceOf(seat int, typeID string) (string, bool) {
	for _, c := range e.d.Table.Zone("hand", seat).Cards {
		if catalog.TypeID(c.ID) == typeID {
			return c.ID, true
		}
	}
	return "", false
}

func (e *eng) suit(id string) string { return e.cc.Type(id).Suit }

// legal lists the decisions of the seat to move.
func (e *eng) legal() []state.Selection {
	d := e.d
	if d.Turn == 0 {
		return nil
	}
	var out []state.Selection
	switch d.Phase {
	case phaseTerms:
		for _, line := range lineOrder {
			if open := e.open(line); len(open) > 1 {
				for _, o := range open {
					out = append(out, state.Selection{Action: actCross, Piece: line + ":" + o})
				}
			}
		}
	case phaseTrick:
		hand := e.handTypes(d.Turn)
		var follow []string
		if len(d.Plays) > 0 {
			led := e.suit(d.Plays[0].Card)
			for _, tid := range hand {
				if e.cc.Types[tid].Suit == led {
					follow = append(follow, tid)
				}
			}
		}
		if len(follow) > 0 {
			hand = follow
		}
		for _, tid := range hand {
			out = append(out, state.Selection{Action: actPlay, Piece: tid})
		}
	}
	return out
}

// execute plays a legal decision.
func (e *eng) execute(mv state.Selection, seat int) {
	d := e.d
	switch mv.Action {
	case actCross:
		d.Crossed[mv.Piece] = seat
		line, option, _ := strings.Cut(mv.Piece, ":")
		e.say("%s crosses off %s on the %s line.", e.label(seat), option, line)
		for _, l := range lineOrder {
			if len(e.open(l)) > 1 {
				d.Turn = e.next(seat)
				return
			}
		}
		d.Trump = e.open(lineTrump)[0]
		d.Points, _ = strconv.Atoi(e.open(linePoints)[0])
		d.Phase, d.Turn = phaseTrick, 1
		e.say("Trump is %s, and a trick is worth %d: %s leads.", d.Trump, d.Points, e.label(1))
	case actPlay:
		id, _ := e.instanceOf(seat, mv.Piece)
		d.Table.Move(id, "hand", seat, "trick", 0, true)
		d.Plays = append(d.Plays, Play{Seat: seat, Card: id})
		e.say("%s plays %s.", e.label(seat), catalog.TypeID(id))
		if len(d.Plays) < e.n {
			d.Turn = e.next(seat)
			return
		}
		e.resolveTrick()
	}
}

// trickWinner is the seat that takes the trick.
func (e *eng) trickWinner(plays []Play) int {
	led := e.suit(plays[0].Card)
	best, bestRank, bestVal := 0, -1, -1
	for _, p := range plays {
		rank := 0
		switch e.suit(p.Card) {
		case e.d.Trump:
			rank = 2
		case led:
			rank = 1
		default:
			continue
		}
		if v := e.cc.Type(p.Card).Value; rank > bestRank || (rank == bestRank && v > bestVal) {
			best, bestRank, bestVal = p.Seat, rank, v
		}
	}
	return best
}

func (e *eng) resolveTrick() {
	d := e.d
	winner := e.trickWinner(d.Plays)
	d.Tricks[winner]++
	for _, p := range d.Plays {
		d.Table.Move(p.Card, "trick", 0, "won", winner, false)
	}
	d.LastTrick, d.LastWinner, d.Plays = d.Plays, winner, nil
	e.say("%s takes the trick.", e.label(winner))
	if d.Table.Zone("hand", winner).Len() > 0 {
		d.Turn = winner
		return
	}
	best, winners := -1, []int{}
	for s := 1; s <= e.n; s++ {
		switch pts := d.Tricks[s] * d.Points; {
		case pts > best:
			best, winners = pts, []int{s}
		case pts == best:
			winners = append(winners, s)
		}
	}
	d.Turn = 0
	if len(winners) == 1 {
		d.Winner = winners[0]
		e.say("%s wins with %d points.", e.label(d.Winner), best)
	} else {
		e.say("A draw at %d points.", best)
	}
}

func (e *eng) scoreLine() string {
	var parts []string
	for s := 1; s <= e.n; s++ {
		parts = append(parts, fmt.Sprintf("%s %d", e.label(s), e.d.Tricks[s]*e.d.Points))
	}
	return strings.Join(parts, ", ")
}

// --- the ruleset interface ---------------------------------------------------

// Start deals the cards.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	if game.Cards == nil {
		return nil, fmt.Errorf("tricks: the definition has no cards")
	}
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	d := &Data{RNG: seed, Table: cards.NewTable()}
	e := newEng(game, d)
	e.deal()
	d.Events = e.ev
	st := state.NewBoardState()
	st.SetExt(d)
	return st, nil
}

// ImpulseSeat is the seat that decides next, or 0 once the game is over.
func (rs *Ruleset) ImpulseSeat(game *catalog.CompiledGame, st *state.BoardState) int {
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
	out := state.Outcome{Ended: true, Reason: "points", Detail: newEng(game, d).scoreLine()}
	if d.Winner != 0 {
		w := d.Winner
		out.Winner = &w
	}
	return out
}

// Validate checks a position document.
func (rs *Ruleset) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	d, err := dataOf(st)
	if err != nil {
		return err
	}
	if n := len(game.Definition.Seats); d.Turn < 0 || d.Turn > n {
		return fmt.Errorf("tricks: bad turn %d", d.Turn)
	}
	if d.Phase != phaseTerms && d.Phase != phaseTrick {
		return fmt.Errorf("tricks: unknown phase %q", d.Phase)
	}
	seen := map[string]bool{}
	for k, z := range d.Table.Zones {
		for _, c := range z.Cards {
			if game.Cards.Type(c.ID) == nil || seen[c.ID] {
				return fmt.Errorf("tricks: card %q in %s is unknown or in two places", c.ID, k)
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
	return newEng(game, d).legal()
}

// Apply plays one decision.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Selection) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("tricks: the game is over")
	}
	e := newEng(game, d)
	if seat != d.Turn {
		return nil, fmt.Errorf("tricks: it is %s's decision", e.label(d.Turn))
	}
	want := rs.Format(game, mv)
	var legal state.Selection
	found := false
	for _, cand := range e.legal() {
		if rs.Format(game, cand) == want {
			legal, found = cand, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("tricks: %q is not a legal move now", want)
	}
	nd := d.clone()
	e.d = nd
	e.execute(legal, seat)
	nd.Events = e.ev
	ns := st.Clone()
	ns.SetExt(nd)
	ns.SetPly(st.Ply() + 1)
	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal), NextImpulseSeat: nd.Turn}
	if nd.Turn == 0 {
		res.GameEnded, res.NextImpulseSeat, res.EndReason, res.EndDetail = true, seat, "points", e.scoreLine()
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
	}
	return res, nil
}

// Format renders mv: cross:trump:red, or a card played (red-5).
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Selection) string {
	if mv.Action == actCross {
		return "cross:" + mv.Piece
	}
	return mv.Piece
}

// Parse reads what Format writes.
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Selection, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	if box, ok := strings.CutPrefix(t, "cross:"); ok {
		line, option, _ := strings.Cut(box, ":")
		for _, o := range lineBoxes[line] {
			if o == option {
				return state.Selection{Action: actCross, Piece: box}, nil
			}
		}
		return state.Selection{}, fmt.Errorf("tricks: no box %q on the board", box)
	}
	if game.Cards.Types[t] != nil {
		return state.Selection{Action: actPlay, Piece: t}, nil
	}
	return state.Selection{}, fmt.Errorf("tricks: cannot read %q as a move", text)
}

// Gestures: one action per decision, as the page's panels name them
// (cross-<line>-<box> on the board of choices, card-<type> on the table).
func (rs *Ruleset) Gestures(game *catalog.CompiledGame, mv state.Selection) []state.Gesture {
	if mv.Action == actCross {
		return []state.Gesture{{Kind: state.GestureAction, Value: "cross-" + strings.Replace(mv.Piece, ":", "-", 1)}}
	}
	return []state.Gesture{{Kind: state.GestureAction, Value: "card-" + mv.Piece}}
}

// Describe says how the game is won and written, for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	return state.RulesetInfo{
		WinRules:     []string{"When every card is played, each player scores their tricks times the points of the round; the most points wins, and equal points are a draw."},
		NotationHelp: []string{"Crossing a box of the board of terms: cross:trump:red, cross:points:2. Playing a card is its name, colour-value: blue-7."},
	}
}

// --- hidden information --------------------------------------------------------

// Redact attaches the viewer's view and strips the cards (the view holds them as
// far as the viewer may see them) and the shuffling generator.
func (rs *Ruleset) Redact(game *catalog.CompiledGame, st *state.BoardState, viewer int) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	pub := d.clone()
	pub.Table, pub.RNG = nil, 0
	out := st.Clone()
	out.SetExt(&publicExt{Data: pub, View: newEng(game, d).buildView(viewer)})
	return out
}

// RedactMove: every decision is public (a card played is seen by all).
func (rs *Ruleset) RedactMove(game *catalog.CompiledGame, st *state.BoardState, mv state.Selection, notation string) (state.Selection, string) {
	return mv, notation
}

// Determinize deals the cards the viewer cannot see out again at random.
func (rs *Ruleset) Determinize(game *catalog.CompiledGame, st *state.BoardState, viewer int, rng *rand.Rand) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	nd := d.clone()
	n := len(game.Definition.Seats)
	deck := nd.Table.Zone("deck", 0)
	pool := append([]cards.Card(nil), deck.Cards...)
	for s := 1; s <= n; s++ {
		if s != viewer {
			pool = append(pool, nd.Table.Zone("hand", s).Cards...)
		}
	}
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	for s := 1; s <= n; s++ {
		if s == viewer {
			continue
		}
		z := nd.Table.Zone("hand", s)
		k := z.Len()
		z.Cards, pool = append([]cards.Card(nil), pool[:k]...), pool[k:]
	}
	deck.Cards = pool
	out := st.Clone()
	out.SetExt(nd)
	return out
}

// --- the view ------------------------------------------------------------------

// View is what clients draw: the cards as the viewer may see them, the board
// of terms and each seat's counters.
type View struct {
	Phase        string               `json:"phase"`
	ImpulseSeat  int                  `json:"impulse_seat,omitempty"`
	Prompt       string               `json:"prompt"`
	Impulse      string               `json:"impulse,omitempty"`
	Round        int                  `json:"round"`
	Rounds       int                  `json:"rounds"`
	Table        []cards.ZoneView     `json:"table"`
	Grid         *Grid                `json:"grid"`
	Seats        map[string]*SeatView `json:"seats"`
	Labels       map[string]string    `json:"labels,omitempty"`
	PanelActions []string             `json:"panel_actions"`
}

// Grid is a board of choices, line by line.
type Grid struct {
	Lines []GridLine `json:"lines"`
}

// GridLine is one line of the board; Icon names its picture, as GridBox.Icon does.
type GridLine struct {
	ID    string    `json:"id"`
	Label string    `json:"label"`
	Hint  string    `json:"hint"`
	Icon  string    `json:"icon"`
	Boxes []GridBox `json:"boxes"`
}

// GridBox is one box: open, crossed (by a seat), or the one left (chosen).
// Icon is "suit:red" or "points:2" and ByIcon "seat:north": the page script
// draws them (web/page.js).
type GridBox struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Icon   string `json:"icon"`
	State  string `json:"state"`
	By     int    `json:"by,omitempty"`
	ByIcon string `json:"by_icon,omitempty"`
}

// SeatView is one seat's counters.
type SeatView struct {
	Counters []CounterView `json:"counters"`
}

// CounterView is a labelled number.
type CounterView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value int    `json:"value"`
}

var lineHints = map[string]string{
	lineTrump:  "The suit whose box stays open is trump.",
	linePoints: "What each trick is worth.",
}

func (e *eng) buildView(viewer int) *View {
	d := e.d
	v := &View{Phase: d.Phase, ImpulseSeat: d.Turn, Round: 1, Rounds: 1, Seats: map[string]*SeatView{}, PanelActions: []string{"card-", "cross-"}}
	v.Table = d.Table.View(e.cc, viewer)
	for i := range v.Table {
		if v.Table[i].ID == "trick" {
			for _, p := range d.Plays {
				v.Table[i].Captions = append(v.Table[i].Captions, e.label(p.Seat))
			}
		}
	}
	if len(d.LastTrick) > 0 {
		z := cards.ZoneView{ID: "last", Count: len(d.LastTrick)}
		for _, p := range d.LastTrick {
			z.Cards = append(z.Cards, cards.Card{ID: p.Card, Up: true})
			z.Captions = append(z.Captions, e.label(p.Seat))
		}
		v.Table = append(v.Table, z)
	}
	g := &Grid{}
	for _, line := range lineOrder {
		gl := GridLine{ID: line, Label: strings.ToUpper(line[:1]) + line[1:], Hint: lineHints[line], Icon: "line:" + line}
		open := e.open(line)
		for _, o := range lineBoxes[line] {
			gb := GridBox{ID: o, Label: o, Icon: line + ":" + o, State: "open"}
			if line == lineTrump {
				gb.Icon = "suit:" + o
				if sd, ok := e.cc.Def.Suits[o]; ok {
					gb.Label = sd.Label
				}
			}
			switch by := d.Crossed[line+":"+o]; {
			case by != 0:
				gb.State, gb.By, gb.ByIcon = "crossed", by, "seat:"+strings.ToLower(e.label(by))
			case len(open) == 1:
				gb.State = "chosen"
			}
			gl.Boxes = append(gl.Boxes, gb)
		}
		g.Lines = append(g.Lines, gl)
	}
	v.Grid = g
	switch {
	case d.Turn == 0 && d.Winner != 0:
		v.Prompt = e.label(d.Winner) + " wins"
	case d.Turn == 0:
		v.Prompt = "A draw"
	case d.Phase == phaseTerms:
		v.Prompt, v.Impulse = e.label(d.Turn)+": cross off a box", "Choose a box to cross off."
	default:
		v.Prompt, v.Impulse = e.label(d.Turn)+": play a card", "Choose a card to play."
	}
	for s := 1; s <= e.n; s++ {
		v.Seats[strconv.Itoa(s)] = &SeatView{Counters: []CounterView{
			{ID: "tricks", Label: "Tricks", Value: d.Tricks[s]},
			{ID: "cards", Label: "Cards in hand", Value: d.Table.Zone("hand", s).Len()},
		}}
	}
	return v
}
