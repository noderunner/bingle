// Package council is a game only the tests play, never registered by the
// server: the public suite's vehicle for the parts of the platform that only
// a commercial game used to exercise, now that those games are built apart
// (game plugins plan, step 4). It has
//
//   - a table of four (lobbies, seats, voice, following a player);
//   - optional reactions opened after every other house's turn, aimed at a
//     house when the turn ended in an area where its envoy stands: Object,
//     and the Owl's Heckle, which asks on every occasion (state.Reactor: the
//     reaction settings, one choice per reaction, the session's automatic
//     passes, the clock of a seat that must react);
//   - a choice of house per seat from five, swapped in the lobby, with a pool
//     its players choose from, and a game derived for every table
//     (catalog.Deriver, state.SeatFactions, state.OwnSeatFactions, saved setups);
//   - a setup catalog (state.SetupCatalogs) and an option, the Court;
//   - a map of areas (board style "areas").
//
// It is not a game worth playing: every rule is there for a test.
package council

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/web/look"
)

// ID is the fixture's game id.
const ID = "test-council"

//go:embed council.yaml
var fsys embed.FS

//go:embed tutorials
var tutorialsFS embed.FS

var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(fsys, "council.yaml") })

// baseDefinition is the definition parsed, for deriving tables outside the catalog.
var baseDefinition = sync.OnceValue(func() *dsl.GameDefinition {
	def, err := dsl.Parse(definition())
	if err != nil {
		panic(err)
	}
	return def
})

//go:embed web
var webFS embed.FS

// Module returns the fixture's catalog.Module.
func Module() catalog.Module {
	tutorials, err := fs.Sub(tutorialsFS, "tutorials")
	if err != nil {
		panic(err)
	}
	scripts, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: ID, Definition: definition(), Rules: New(), Tutorials: tutorials, Scripts: scripts, Web: look.MustLoadFile(webFS, "web/web.yaml")}
}

// A house is what a seat plays.
type house struct {
	key, label, color string
	object            bool // may Object from the start
	heckle            bool // may Heckle
}

// Houses are the five houses, in the order the catalog lists them.
var houses = []house{
	{"crow", "Crow", "red", true, false},
	{"stag", "Stag", "blue", false, false},
	{"wolf", "Wolf", "green", false, false},
	{"ram", "Ram", "yellow", false, false},
	{"owl", "Owl", "white", true, true},
}

func houseOf(key string) (house, bool) {
	for _, h := range houses {
		if h.key == key {
			return h, true
		}
	}
	return house{}, false
}

// home is each seat's starting area.
var home = map[int]board.PositionID{1: "north", 2: "east", 3: "south", 4: "west"}

const (
	actGo      = "go"
	actClaim   = "claim"
	actObject  = "object"
	actHeckle  = "heckle"
	actDecline = "decline"
	piece      = "envoy"
)

// Abilities are the reactions, by id: their name and what they do.
var abilities = map[string]state.ReactionAbility{
	actObject: {ID: actObject, Name: "Object", Text: "After another house's turn, make it give back its last claim."},
	actHeckle: {ID: actHeckle, Name: "Heckle", Text: "After another house's turn, heckle it: nothing comes of it, but you are asked every time.", AskAlways: true},
}

// Data is the position's own part.
type Data struct {
	// Turn is the house to move (0 once the game is over).
	Turn int `json:"turn"`
	// Envoys is where each seat's envoy stands, by seat.
	Envoys map[int]string `json:"envoys"`
	// Claims lists each seat's claimed areas, in the order claimed.
	Claims map[int][]string `json:"claims,omitempty"`
	// Window is the reaction being waited for, when one is.
	Window *Window `json:"window,omitempty"`
	Winner int     `json:"winner,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

// Window is an open reaction: who may still decide, and about which ability,
// in order, after Trigger's turn that ended in Area.
type Window struct {
	Trigger int    `json:"trigger"`
	Area    string `json:"area"`
	Waiting []Ask  `json:"waiting"`
}

// Ask is one question of a window: seat may use Ability.
type Ask struct {
	Seat    int    `json:"seat"`
	Ability string `json:"ability"`
}

// CloneExt implements state.ExtClone.
func (d *Data) CloneExt() any { return d.clone() }

func (d *Data) clone() *Data {
	c := *d
	c.Envoys = map[int]string{}
	for k, v := range d.Envoys {
		c.Envoys[k] = v
	}
	c.Claims = map[int][]string{}
	for k, v := range d.Claims {
		c.Claims[k] = append([]string(nil), v...)
	}
	if d.Window != nil {
		w := *d.Window
		w.Waiting = append([]Ask(nil), d.Window.Waiting...)
		c.Window = &w
	}
	return &c
}

// publicExt is a position shown to a viewer: its data, and the view the
// page draws from (a ruleset's ext.view).
type publicExt struct {
	*Data
	View View `json:"view"`
}

// View is what the page is told besides the position: the question of the
// impulse, the same for everyone.
type View struct {
	Prompt string `json:"prompt"`
}

// CloneExt implements state.ExtClone.
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
			return nil, fmt.Errorf("council: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, errors.New("council: the position has no payload")
	}
	return nil, fmt.Errorf("council: unexpected payload %T", st.Ext())
}

// Ruleset is the fixture's state.Ruleset.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

// table is the houses at a game's table, by seat, and whether the Court plays.
func table(game *catalog.CompiledGame) (seats []string, court bool) {
	def := game.Definition
	if l, ok := def.Params["houses"].([]any); ok {
		for _, x := range l {
			s, _ := x.(string)
			seats = append(seats, s)
		}
	}
	if len(seats) != len(def.Seats) {
		seats = []string{"crow", "stag", "wolf", "ram"}
	}
	court, _ = def.Params["court"].(bool)
	return seats, court
}

// held lists the reactions seat may use now, in the order it is asked them.
func held(game *catalog.CompiledGame, d *Data, seat int) []string {
	seats, court := table(game)
	h, _ := houseOf(seats[seat-1])
	var out []string
	if h.heckle {
		out = append(out, actHeckle)
	}
	if h.object || (court && len(d.Claims[seat]) > 0) {
		out = append(out, actObject)
	}
	return out
}

func next(seat, n int) int { return seat%n + 1 }

// Start puts every envoy home; the first seat moves.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	st := state.NewBoardState()
	d := &Data{Turn: 1, Envoys: map[int]string{}, Claims: map[int][]string{}}
	for _, s := range game.Definition.Seats {
		d.Envoys[s.Seat] = string(home[s.Seat])
	}
	save(game, st, d)
	return st, nil
}

// save draws the envoys as pieces in their areas and stores d.
func save(game *catalog.CompiledGame, st *state.BoardState, d *Data) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	seats := make([]int, 0, len(d.Envoys))
	for s := range d.Envoys {
		seats = append(seats, s)
	}
	sort.Ints(seats)
	for _, s := range seats {
		sd, _ := game.Seat(s)
		st.Push(board.PositionID(d.Envoys[s]), state.Occupant{Piece: piece, Color: sd.Color})
	}
	st.SetExt(d)
}

// ImpulseSeat is the house to move, or the one whose Object is awaited.
func (rs *Ruleset) ImpulseSeat(game *catalog.CompiledGame, st *state.BoardState) int {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	if d.Window != nil && len(d.Window.Waiting) > 0 {
		return d.Window.Waiting[0].Seat
	}
	return d.Turn
}

// Legal lists seat's moves.
func (rs *Ruleset) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Selection {
	d, err := dataOf(st)
	if err != nil || d.Turn == 0 || seat != rs.ImpulseSeat(game, st) {
		return nil
	}
	if d.Window != nil {
		return []state.Selection{{Action: d.Window.Waiting[0].Ability}, {Action: actDecline}}
	}
	at := board.PositionID(d.Envoys[seat])
	var out []state.Selection
	if claimedBy(d, string(at)) == 0 {
		out = append(out, state.Selection{Action: actClaim, To: at})
	}
	nbrs := game.Board.Neighbors(at)
	sort.Slice(nbrs, func(i, j int) bool { return nbrs[i] < nbrs[j] })
	for _, n := range nbrs {
		out = append(out, state.Selection{Action: actGo, To: n})
	}
	return out
}

func claimedBy(d *Data, area string) int {
	for s, l := range d.Claims {
		if slices.Contains(l, area) {
			return s
		}
	}
	return 0
}

// Apply plays one move.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Selection) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	want := rs.Format(game, mv)
	var legal state.Selection
	found := false
	for _, c := range rs.Legal(game, st, seat) {
		if rs.Format(game, c) == want {
			legal, found = c, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("council: %q is not a legal move for seat %d now", want, seat)
	}
	nd := d.clone()
	n := len(game.Definition.Seats)
	switch legal.Action {
	case actObject, actHeckle, actDecline:
		w := nd.Window
		if legal.Action == actObject {
			if l := nd.Claims[w.Trigger]; len(l) > 0 {
				nd.Claims[w.Trigger] = l[:len(l)-1]
			}
		}
		w.Waiting = w.Waiting[1:]
		if len(w.Waiting) == 0 {
			nd.Window = nil
		}
	default:
		if legal.Action == actGo {
			nd.Envoys[seat] = string(legal.To)
		} else {
			nd.Claims[seat] = append(nd.Claims[seat], string(legal.To))
		}
		nd.Turn = next(seat, n)
		// Every other house is asked about each reaction it holds, in turn.
		var waiting []Ask
		for s := next(seat, n); s != seat; s = next(s, n) {
			for _, a := range held(game, nd, s) {
				waiting = append(waiting, Ask{Seat: s, Ability: a})
			}
		}
		if len(waiting) > 0 {
			nd.Window = &Window{Trigger: seat, Area: nd.Envoys[seat], Waiting: waiting}
		}
	}
	ns := st.Clone()
	ns.SetPly(st.Ply() + 1)
	finish(game, nd, ns.Ply())
	save(game, ns, nd)
	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal), NextImpulseSeat: rs.ImpulseSeat(game, ns)}
	if nd.Turn == 0 {
		res.GameEnded, res.NextImpulseSeat, res.EndReason = true, seat, nd.Reason
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
	}
	return res, nil
}

// finish ends the game when a house holds enough claims, or at the move
// limit (once no reaction is waited for).
func finish(game *catalog.CompiledGame, d *Data, ply int) {
	if d.Window != nil {
		return
	}
	need := game.ParamInt("claims_to_win", 3)
	for s := 1; s <= len(game.Definition.Seats); s++ {
		if len(d.Claims[s]) >= need {
			d.Turn, d.Winner, d.Reason = 0, s, "claims"
			return
		}
	}
	if ply < game.ParamInt("max_plies", 200) {
		return
	}
	d.Turn, d.Reason = 0, "ply-limit"
	best, top := 0, -1
	for s := 1; s <= len(game.Definition.Seats); s++ {
		switch c := len(d.Claims[s]); {
		case c > top:
			best, top = s, c
		case c == top:
			best = 0
		}
	}
	d.Winner = best
}

// Outcome reports a finished game.
func (rs *Ruleset) Outcome(game *catalog.CompiledGame, st *state.BoardState) state.Outcome {
	d, err := dataOf(st)
	if err != nil || d.Turn != 0 {
		return state.Outcome{}
	}
	out := state.Outcome{Ended: true, Reason: d.Reason}
	if d.Winner != 0 {
		w := d.Winner
		out.Winner = &w
	}
	return out
}

// Validate checks a position document: every envoy in an area of the map.
func (rs *Ruleset) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	d, err := dataOf(st)
	if err != nil {
		return err
	}
	for s, a := range d.Envoys {
		if _, ok := game.Seat(s); !ok || !game.Board.HasPosition(board.PositionID(a)) {
			return fmt.Errorf("council: envoy of seat %d in %q", s, a)
		}
	}
	return nil
}

// Format writes a move: go@hall, claim@hall, object, heckle, decline.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Selection) string {
	if mv.To != "" {
		return mv.Action + "@" + string(mv.To)
	}
	return mv.Action
}

// Parse reads a move Format wrote.
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Selection, error) {
	text = strings.TrimSpace(text)
	act, at, _ := strings.Cut(text, "@")
	switch act {
	case actGo, actClaim:
		if !game.Board.HasPosition(board.PositionID(at)) {
			return state.Selection{}, fmt.Errorf("council: no area %q", at)
		}
		return state.Selection{Action: act, To: board.PositionID(at)}, nil
	case actObject, actHeckle, actDecline:
		if at == "" {
			return state.Selection{Action: act}, nil
		}
	}
	return state.Selection{}, fmt.Errorf("council: cannot read %q", text)
}

// Gestures: an area to move to; the Claim button, then the area (a move of
// two steps, for the tests of the impulse card); or an answer's button.
func (rs *Ruleset) Gestures(game *catalog.CompiledGame, mv state.Selection) []state.Gesture {
	switch mv.Action {
	case actGo:
		return []state.Gesture{{Kind: state.GesturePos, Value: string(mv.To)}}
	case actClaim:
		return []state.Gesture{{Kind: state.GestureAction, Value: actClaim}, {Kind: state.GesturePos, Value: string(mv.To)}}
	}
	return []state.Gesture{{Kind: state.GestureAction, Value: mv.Action}}
}

// Describe names the actions.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	return state.RulesetInfo{
		Actions: []state.RulesetAction{
			{ID: actClaim, Label: "Claim"},
			{ID: actObject, Label: "Object"},
			{ID: actHeckle, Label: "Heckle"},
			{ID: actDecline, Label: "Let it pass"},
		},
		NotationHelp: []string{"go@area moves your envoy; claim@area claims the area it stands in; object, heckle and decline answer a reaction.", "Example: go@hall, then claim@hall."},
		WinRules:     []string{"The first house with three claims wins."},
	}
}

// --- reactions (state.Reactor) ---------------------------------------------

// PendingReaction is the reaction awaited, if any.
func (rs *Ruleset) PendingReaction(game *catalog.CompiledGame, st *state.BoardState) *state.Reaction {
	d, err := dataOf(st)
	if err != nil || d.Window == nil || len(d.Window.Waiting) == 0 {
		return nil
	}
	ask := d.Window.Waiting[0]
	return &state.Reaction{
		Seat: ask.Seat, Trigger: d.Window.Trigger, Ability: abilities[ask.Ability].Name,
		Targeted: d.Envoys[ask.Seat] == d.Window.Area,
		Pass:     state.Selection{Action: actDecline},
	}
}

// ReactionAbilities lists seat's reactions, held or not: the Owl's Heckle,
// and Object for a house that may use it, or will at a table with the Court
// once it holds a claim.
func (rs *Ruleset) ReactionAbilities(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.ReactionAbility {
	seats, court := table(game)
	if seat < 1 || seat > len(seats) {
		return nil
	}
	h, _ := houseOf(seats[seat-1])
	now := map[string]bool{}
	if d, err := dataOf(st); err == nil {
		for _, a := range held(game, d, seat) {
			now[a] = true
		}
	}
	var out []state.ReactionAbility
	if h.heckle {
		a := abilities[actHeckle]
		a.Held = now[actHeckle]
		out = append(out, a)
	}
	if h.object || court {
		a := abilities[actObject]
		a.Held = now[actObject]
		out = append(out, a)
	}
	return out
}

// --- the table (catalog.Deriver, state.SeatFactions, state.OwnSeatFactions) -------

const (
	optHouses = "houses"
	optCourt  = "court"
	optPool   = "house-pool"
)

// Derive makes the table of options: a house per seat, the Court, and a pool
// of houses its players choose from in the lobby.
func (rs *Ruleset) Derive(base *dsl.GameDefinition, options map[string]any) (string, map[string]any, error) {
	n := len(base.Seats)
	var seats, pool []string
	court := false
	for k, v := range options {
		switch k {
		case optHouses, optPool:
			list, err := names(k, v)
			if err != nil {
				return "", nil, err
			}
			if k == optHouses {
				seats = list
			} else {
				pool = list
			}
		case optCourt:
			b, ok := v.(bool)
			if !ok {
				return "", nil, fmt.Errorf("council: option %q must be true or false", k)
			}
			court = b
		default:
			return "", nil, fmt.Errorf("council: unknown option %q", k)
		}
	}
	if seats == nil {
		seats = []string{"crow", "stag", "wolf", "ram"}
	}
	if len(seats) != n {
		return "", nil, fmt.Errorf("council: option %q needs %d houses", optHouses, n)
	}
	seen := map[string]bool{}
	for _, s := range seats {
		if _, ok := houseOf(s); !ok {
			return "", nil, fmt.Errorf("council: unknown house %q", s)
		}
		if seen[s] {
			return "", nil, fmt.Errorf("council: house %q sits twice", s)
		}
		seen[s] = true
	}
	for _, p := range pool {
		if _, ok := houseOf(p); !ok {
			return "", nil, fmt.Errorf("council: unknown house %q in the pool", p)
		}
	}
	for _, s := range seats {
		if len(pool) > 0 && !slices.Contains(pool, s) {
			return "", nil, fmt.Errorf("council: option %q must hold every house at the table", optPool)
		}
	}
	suffix := strings.Join(seats, "-")
	if court {
		suffix += "-court"
	}
	if len(pool) > 0 {
		suffix += "-pool-" + strings.Join(pool, "-")
	}
	return suffix, patchFor(base, seats, court, pool), nil
}

// FromSuffix rebuilds a derived table's patch.
func (rs *Ruleset) FromSuffix(base *dsl.GameDefinition, suffix string) (map[string]any, error) {
	parts := strings.Split(suffix, "-")
	n := len(base.Seats)
	if len(parts) < n {
		return nil, fmt.Errorf("council: bad table %q", suffix)
	}
	opts := map[string]any{optHouses: anyList(parts[:n])}
	rest := parts[n:]
	if len(rest) > 0 && rest[0] == "court" {
		opts[optCourt] = true
		rest = rest[1:]
	}
	if len(rest) > 0 {
		if rest[0] != "pool" || len(rest) < 2 {
			return nil, fmt.Errorf("council: bad table %q", suffix)
		}
		opts[optPool] = anyList(rest[1:])
	}
	got, patch, err := rs.Derive(base, opts)
	if err != nil {
		return nil, err
	}
	if got != suffix {
		return nil, fmt.Errorf("council: %q is not a table's name (it would be %q)", suffix, got)
	}
	return patch, nil
}

func patchFor(base *dsl.GameDefinition, seats []string, court bool, pool []string) map[string]any {
	ss := make([]any, len(seats))
	for i, key := range seats {
		h, _ := houseOf(key)
		ss[i] = map[string]any{"seat": i + 1, "color": h.color, "label": h.label}
	}
	params := map[string]any{}
	for k, v := range base.Params {
		params[k] = v
	}
	params["houses"] = anyList(seats)
	params["court"] = court
	if len(pool) > 0 {
		params["faction_pool"] = anyList(pool)
	} else {
		params["faction_pool"] = nil
	}
	return map[string]any{"seats": ss, "params": params}
}

func names(k string, v any) ([]string, error) {
	var out []string
	switch l := v.(type) {
	case []string:
		out = l
	case []any:
		for _, x := range l {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("council: option %q holds a value that is not a house", k)
			}
			out = append(out, s)
		}
	default:
		return nil, fmt.Errorf("council: option %q must be a list of houses", k)
	}
	for i := range out {
		out[i] = strings.ToLower(strings.TrimSpace(out[i]))
	}
	return out, nil
}

func anyList(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// pool is the houses the table's players choose from, if any.
func pool(game *catalog.CompiledGame) []string {
	var out []string
	if l, ok := game.Definition.Params["faction_pool"].([]any); ok {
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// SeatFactionOptions is the table with house in seat: a house another open
// seat has changes places with it; one a taken seat has is refused.
func (rs *Ruleset) SeatFactionOptions(game *catalog.CompiledGame, seat int, key string, taken map[int]bool) (map[string]any, error) {
	if _, ok := houseOf(key); !ok {
		return nil, fmt.Errorf("council: unknown house %q", key)
	}
	p := pool(game)
	if len(p) > 0 && !slices.Contains(p, key) {
		return nil, fmt.Errorf("council: house %q is not in this table's pool", key)
	}
	seats, court := table(game)
	if seats[seat-1] == key {
		return nil, nil
	}
	next := append([]string(nil), seats...)
	if j := slices.Index(seats, key); j >= 0 {
		if taken[j+1] {
			return nil, fmt.Errorf("%w: %s", state.ErrFactionHeld, key)
		}
		next[j] = seats[seat-1]
	}
	next[seat-1] = key
	opts := map[string]any{optHouses: anyList(next)}
	if court {
		opts[optCourt] = true
	}
	if len(p) > 0 {
		opts[optPool] = anyList(p)
	}
	return opts, nil
}

// PlayersChooseFactions: at a table with a pool, each player picks their house.
func (rs *Ruleset) PlayersChooseFactions(game *catalog.CompiledGame) bool { return len(pool(game)) > 0 }

// --- the setup catalog (state.SetupCatalogs) -------------------------------

// CatalogItem is one house of the setup catalog.
type CatalogItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// SetupCatalog lists the houses and the Court.
func (rs *Ruleset) SetupCatalog(game *catalog.CompiledGame) (any, error) {
	items := make([]CatalogItem, len(houses))
	for i, h := range houses {
		items[i] = CatalogItem{ID: h.key, Name: h.label, Color: h.color}
	}
	return map[string]any{
		"tabs": []map[string]any{
			{"id": "houses", "label": "Houses", "items": items},
			{"id": "options", "label": "Options", "items": []map[string]any{{"id": optCourt, "name": "The Court", "option": optCourt}}},
		},
	}, nil
}

// --- training environments (state.ActionLister, state.PlaneEncoder) --------

// areas are the map's areas in a fixed order.
var areas = []string{"north", "west", "hall", "east", "south"}

// ActionNotations is the fixed action space: going to or claiming each area,
// and the answers to a reaction.
func (rs *Ruleset) ActionNotations(game *catalog.CompiledGame) []string {
	var out []string
	for _, a := range areas {
		out = append(out, actGo+"@"+a, actClaim+"@"+a)
	}
	return append(out, actObject, actHeckle, actDecline)
}

// PlaneLayout is four planes of the areas in a row: the viewer's envoy, the
// others' envoys, the viewer's claims and the others' claims.
func (rs *Ruleset) PlaneLayout(game *catalog.CompiledGame) ([]string, int, int) {
	return []string{"my-envoy", "their-envoys", "my-claims", "their-claims"}, 1, len(areas)
}

// PlaneCells is the area at every cell of a plane.
func (rs *Ruleset) PlaneCells(game *catalog.CompiledGame) []string {
	return append([]string(nil), areas...)
}

// EncodePlanes fills the planes for viewer.
func (rs *Ruleset) EncodePlanes(game *catalog.CompiledGame, st *state.BoardState, viewer int) []float32 {
	n := len(areas)
	out := make([]float32, 4*n)
	d, err := dataOf(st)
	if err != nil {
		return out
	}
	at := map[string]int{}
	for i, a := range areas {
		at[a] = i
	}
	for s, a := range d.Envoys {
		if s == viewer {
			out[at[a]] = 1
		} else {
			out[n+at[a]]++
		}
	}
	for s, l := range d.Claims {
		for _, a := range l {
			if s == viewer {
				out[2*n+at[a]] = 1
			} else {
				out[3*n+at[a]] = 1
			}
		}
	}
	return out
}

// --- the view (state.HiddenInfo, with nothing hidden) -----------------------

// Redact shows everything, with the view's prompt.
func (rs *Ruleset) Redact(game *catalog.CompiledGame, st *state.BoardState, viewer int) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	prompt := "Move your envoy, or claim the area it stands in."
	if d.Window != nil && len(d.Window.Waiting) > 0 {
		prompt = abilities[d.Window.Waiting[0].Ability].Name + ", or let it pass."
	}
	out := st.Clone()
	out.SetExt(&publicExt{Data: d.clone(), View: View{Prompt: prompt}})
	return out
}

// RedactMove hides nothing.
func (rs *Ruleset) RedactMove(game *catalog.CompiledGame, st *state.BoardState, mv state.Selection, notation string) (state.Selection, string) {
	return mv, notation
}

// Determinize has nothing to sample.
func (rs *Ruleset) Determinize(game *catalog.CompiledGame, st *state.BoardState, viewer int, rng *rand.Rand) *state.BoardState {
	return st.Clone()
}
