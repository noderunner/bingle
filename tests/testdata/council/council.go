// Package council is a game only the tests play, never registered by the
// server: the public suite's vehicle for the parts of the platform that only
// a commercial game used to exercise, now that those games are built apart
// (game plugins plan, step 4). It has
//
//   - a table of four (lobbies, seats, voice, following a player);
//   - an optional reaction, Object, opened after every other house's turn and
//     aimed at a house when the turn ended in an area where its envoy stands
//     (state.Reactor: the reaction settings, the session's automatic passes,
//     the clock of a seat that must react);
//   - a choice of house per seat from five, swapped in the lobby, with a pool
//     its players choose from, and a game derived for every table
//     (catalog.Deriver, state.SeatCults, state.OwnSeatCults, saved setups);
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
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
)

// ID is the fixture's game id.
const ID = "test-council"

//go:embed council.yaml
var fsys embed.FS

var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(fsys, "council.yaml") })

// Module returns the fixture's catalog.Module.
func Module() catalog.Module {
	return catalog.Module{ID: ID, Definition: definition(), Rules: New()}
}

// A house is what a seat plays.
type house struct {
	key, label, color string
	object            bool // may Object from the start
}

// Houses are the five houses, in the order the catalog lists them.
var houses = []house{
	{"crow", "Crow", "red", true},
	{"stag", "Stag", "blue", false},
	{"wolf", "Wolf", "green", false},
	{"ram", "Ram", "yellow", false},
	{"owl", "Owl", "white", true},
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
	actDecline = "decline"
	piece      = "envoy"
)

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

// Window is an open Object: who may still decide, in order, after Trigger's
// turn that ended in Area.
type Window struct {
	Trigger int    `json:"trigger"`
	Area    string `json:"area"`
	Waiting []int  `json:"waiting"`
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
		w.Waiting = append([]int(nil), d.Window.Waiting...)
		c.Window = &w
	}
	return &c
}

func dataOf(st *state.BoardState) (*Data, error) {
	switch v := st.Ext().(type) {
	case *Data:
		return v, nil
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

// mayObject says whether seat may use Object now.
func mayObject(game *catalog.CompiledGame, d *Data, seat int) bool {
	seats, court := table(game)
	h, _ := houseOf(seats[seat-1])
	return h.object || (court && len(d.Claims[seat]) > 0)
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
		return d.Window.Waiting[0]
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
		return []state.Selection{{Action: actObject}, {Action: actDecline}}
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
	case actObject, actDecline:
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
		// Every other house that may Object is asked in turn, after this one.
		var waiting []int
		for s := next(seat, n); s != seat; s = next(s, n) {
			if mayObject(game, nd, s) {
				waiting = append(waiting, s)
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

// Format writes a move: go@hall, claim@hall, object, decline.
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
	case actObject, actDecline:
		if at == "" {
			return state.Selection{Action: act}, nil
		}
	}
	return state.Selection{}, fmt.Errorf("council: cannot read %q", text)
}

// Gestures: an area to move or claim, or the action's button.
func (rs *Ruleset) Gestures(game *catalog.CompiledGame, mv state.Selection) []state.Gesture {
	if mv.Action == actGo {
		return []state.Gesture{{Kind: "pos", Value: string(mv.To)}}
	}
	return []state.Gesture{{Kind: "action", Value: mv.Action}}
}

// Describe names the actions.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	return state.RulesetInfo{
		Actions: []state.RulesetAction{
			{ID: actClaim, Label: "Claim"},
			{ID: actObject, Label: "Object"},
			{ID: actDecline, Label: "Let it pass"},
		},
		NotationHelp: []string{"go@area moves your envoy; claim@area claims the area it stands in; object and decline answer an Object.", "Example: go@hall, then claim@hall."},
		WinRules:     []string{"The first house with three claims wins."},
	}
}

// --- reactions (state.Reactor) ---------------------------------------------

// PendingReaction is the Object awaited, if any.
func (rs *Ruleset) PendingReaction(game *catalog.CompiledGame, st *state.BoardState) *state.Reaction {
	d, err := dataOf(st)
	if err != nil || d.Window == nil || len(d.Window.Waiting) == 0 {
		return nil
	}
	seat := d.Window.Waiting[0]
	return &state.Reaction{
		Seat: seat, Trigger: d.Window.Trigger, Ability: "Object",
		Targeted: d.Envoys[seat] == d.Window.Area,
		Pass:     state.Selection{Action: actDecline},
	}
}

// ReactionAbilities lists Object for a house that may use it, or will at a
// table with the Court once it holds a claim.
func (rs *Ruleset) ReactionAbilities(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.ReactionAbility {
	seats, court := table(game)
	if seat < 1 || seat > len(seats) {
		return nil
	}
	h, _ := houseOf(seats[seat-1])
	if !h.object && !court {
		return nil
	}
	held := h.object
	if d, err := dataOf(st); err == nil && !held {
		held = mayObject(game, d, seat)
	}
	return []state.ReactionAbility{{ID: actObject, Name: "Object", Text: "After another house's turn, make it give back its last claim.", Held: held}}
}

// --- the table (catalog.Deriver, state.SeatCults, state.OwnSeatCults) -------

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

// SeatCultOptions is the table with house in seat: a house another open
// seat has changes places with it; one a taken seat has is refused.
func (rs *Ruleset) SeatCultOptions(game *catalog.CompiledGame, seat int, key string, taken map[int]bool) (map[string]any, error) {
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
			return nil, fmt.Errorf("%w: %s", state.ErrCultHeld, key)
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

// PlayersChooseCults: at a table with a pool, each player picks their house.
func (rs *Ruleset) PlayersChooseCults(game *catalog.CompiledGame) bool { return len(pool(game)) > 0 }

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
