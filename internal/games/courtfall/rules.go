package courtfall

import (
	"fmt"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset implements Courtfall (Third Edition) for the state package. It is
// stateless: every position carries its own data.
type Ruleset struct{}

// New returns the Courtfall ruleset.
func New() *Ruleset { return &Ruleset{} }

var _ state.Ruleset = (*Ruleset)(nil)

// Action ids carried by moves.
const (
	actTile     = "tile"
	actCourt    = "court"
	actDeploy   = "deploy"
	actRecover  = "recover"
	actActivate = "activate"
	actExert    = "exert"
	actClear    = "clear"
	actSurge    = "surge"
	actRouse    = "rouse"
	actTiles    = "tiles"
	actMove     = "move"
	actStrike   = "strike"
	actShoot    = "shoot"
	actShove    = "shove"
	actEnd      = "end"
	actRespond  = "respond"
	actReroll   = "reroll"
	actKeep     = "keep"
	actPush     = "push"
	actShift    = "shift"
	actStay     = "stay"
	actSeize    = "seize"
	actDone     = "done"
)

// unit is one piece on the board during an operation.
type unit struct {
	key   string
	seat  int
	piece string
	cell  int
}

// eng is a working view of one position: the data, the units and the tiles
// in indexed form. Apply works on a clone it owns and mutates through eng.
type eng struct {
	game  *catalog.CompiledGame
	g     *geo
	st    *state.BoardState
	d     *Data
	color [3]string // seat → color
	units []unit
	at    []int  // cell → index into units, -1 when empty
	tiles []byte // cell → 0 tan, 1 green, 2 blue, 3 red
	ev    []string

	vpWin, momCap, maxPlies int
}

var tileCode = map[string]byte{tileGreen: 1, tileBlue: 2, tileRed: 3}
var tileName = [4]string{"", tileGreen, tileBlue, tileRed}

func (rs *Ruleset) newEng(game *catalog.CompiledGame, st *state.BoardState, d *Data) *eng {
	e := &eng{game: game, g: geoOf(game), st: st, d: d}
	for _, s := range game.Definition.Seats {
		if s.Seat >= 1 && s.Seat <= 2 {
			e.color[s.Seat] = s.Color
		}
	}
	e.vpWin = game.ParamInt("vp_to_win", 5)
	e.momCap = game.ParamInt("momentum_cap", 5)
	e.maxPlies = game.ParamInt("max_plies", 2000)
	n := len(e.g.ids)
	e.at = make([]int, n)
	for i := range e.at {
		e.at[i] = -1
	}
	e.tiles = make([]byte, n)
	for id, t := range d.Tiles {
		if i, ok := e.g.index[board.PositionID(id)]; ok {
			e.tiles[i] = tileCode[t]
		}
	}
	e.loadUnits()
	return e
}

func (e *eng) loadUnits() {
	e.units = e.units[:0]
	for i := range e.at {
		e.at[i] = -1
	}
	for _, id := range e.st.Occupied() {
		occ, ok := e.st.Get(id)
		if !ok {
			continue
		}
		cell, ok := e.g.index[id]
		if !ok {
			continue
		}
		seat := 0
		for s := 1; s <= 2; s++ {
			if e.color[s] == occ.Color {
				seat = s
			}
		}
		if seat == 0 {
			continue
		}
		e.units = append(e.units, unit{key: unitKey(occ.Color, occ.Piece), seat: seat, piece: occ.Piece, cell: cell})
		e.at[cell] = len(e.units) - 1
	}
}

func (e *eng) unitAt(cell int) *unit {
	if cell < 0 || e.at[cell] < 0 {
		return nil
	}
	return &e.units[e.at[cell]]
}

func (e *eng) unitByKey(key string) *unit {
	for i := range e.units {
		if e.units[i].key == key {
			return &e.units[i]
		}
	}
	return nil
}

func (e *eng) unitOf(seat int, piece string) *unit {
	return e.unitByKey(unitKey(e.color[seat], piece))
}

func (e *eng) cond(u *unit) int { return e.d.Cond[u.key] }

func (e *eng) setCond(u *unit, c int) {
	if c <= 0 {
		delete(e.d.Cond, u.key)
		return
	}
	if e.d.Cond == nil {
		e.d.Cond = map[string]int{}
	}
	e.d.Cond[u.key] = c
}

func (e *eng) tile(cell int) byte { return e.tiles[cell] }

func (e *eng) setTile(cell int, t byte) {
	e.tiles[cell] = t
	id := string(e.g.ids[cell])
	if t == 0 {
		delete(e.d.Tiles, id)
		return
	}
	if e.d.Tiles == nil {
		e.d.Tiles = map[string]string{}
	}
	e.d.Tiles[id] = tileName[t]
}

func (e *eng) court(seat int) string { return e.d.Courts[seat-1] }

func (e *eng) cellName(cell int) string { return string(e.g.ids[cell]) }

func (e *eng) label(u *unit) string {
	return fmt.Sprintf("%s's %s", e.seatLabel(u.seat), unitName[u.piece])
}

func (e *eng) seatLabel(seat int) string {
	if sd, ok := e.game.Seat(seat); ok && sd.Label != "" {
		return sd.Label
	}
	return fmt.Sprintf("Seat %d", seat)
}

func (e *eng) say(format string, args ...any) {
	e.ev = append(e.ev, fmt.Sprintf(format, args...))
}

func (e *eng) addMom(seat, n int) {
	if n <= 0 || seat < 1 {
		return
	}
	v := e.d.Mom[seat-1] + n
	if v > e.momCap {
		v = e.momCap
	}
	e.d.Mom[seat-1] = v
}

func (e *eng) mom(seat int) int { return e.d.Mom[seat-1] }

// moveUnit relocates u on the position (the board state mirrors the units).
func (e *eng) moveUnit(u *unit, to int) {
	from := u.cell
	idx := e.at[from]
	occ, _ := e.st.Get(e.g.ids[from])
	e.st.Remove(e.g.ids[from])
	e.st.Set(e.g.ids[to], occ)
	e.at[from] = -1
	e.at[to] = idx
	e.units[idx].cell = to
}

// --- Ruleset interface ------------------------------------------------------

// Seeded reports that the start position carries a seed to record.
func (*Ruleset) Seeded() bool { return true }

// Start builds the empty arena awaiting its terrain draft.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	st := state.NewBoardState()
	for _, s := range game.Definition.Seats {
		for _, p := range unitTypes {
			st.SetReserve(s.Color, p, 1)
		}
	}
	if seed == 0 {
		seed = 0x2545f4914f6cdd1d
	}
	d := &Data{
		Phase: phaseTerrain,
		Left:  [3]int{4, 3, 3},
		RNG:   seed,
	}
	st.SetExt(d)
	return st, nil
}

// ToMove says whose decision the position waits for.
func (rs *Ruleset) ToMove(game *catalog.CompiledGame, st *state.BoardState) int {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	return toMove(d)
}

func toMove(d *Data) int {
	switch d.Phase {
	case phaseTerrain, phaseDeploy:
		return 1 + d.N%2
	case phaseCourt:
		if d.N == 0 {
			return 2
		}
		return 1
	case phasePlay:
		switch d.Stage {
		case stageRespond:
			if d.Con != nil {
				return d.Con.TSeat
			}
		case stageReroll:
			if d.Con != nil {
				if d.Con.Step == 0 {
					return d.Con.ASeat
				}
				return d.Con.TSeat
			}
		case stagePush:
			if len(d.Pushes) > 0 {
				return d.Pushes[0].By
			}
		}
		return d.Turn
	}
	return 0
}

// Outcome reports a finished game.
func (rs *Ruleset) Outcome(game *catalog.CompiledGame, st *state.BoardState) state.Outcome {
	d, err := dataOf(st)
	if err != nil || d.Phase != phaseOver {
		return state.Outcome{}
	}
	out := state.Outcome{Ended: true, Reason: d.Reason, Detail: fmt.Sprintf("%d–%d VP", d.VP[0], d.VP[1])}
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
	switch d.Phase {
	case phaseTerrain, phaseCourt, phaseDeploy, phasePlay, phaseOver:
	default:
		return fmt.Errorf("courtfall: unknown phase %q", d.Phase)
	}
	g := geoOf(game)
	for id, t := range d.Tiles {
		i, ok := g.index[board.PositionID(id)]
		if !ok {
			return fmt.Errorf("courtfall: tile on unknown hex %q", id)
		}
		if tileIndex(t) < 0 {
			return fmt.Errorf("courtfall: unknown tile %q at %s", t, id)
		}
		if i == g.eye {
			return fmt.Errorf("courtfall: the Eye must stay Open")
		}
		if g.zone[i] != 0 {
			return fmt.Errorf("courtfall: %s is in a deployment zone and must stay Open", id)
		}
	}
	for _, n := range d.Left {
		if n < 0 {
			return fmt.Errorf("courtfall: negative tile count")
		}
	}
	for i, c := range d.Courts {
		if c != "" && !validCourt(c) {
			return fmt.Errorf("courtfall: seat %d has unknown court %q", i+1, c)
		}
	}
	for k, c := range d.Cond {
		if c < 0 || c > condExhausted {
			return fmt.Errorf("courtfall: unit %s has condition %d", k, c)
		}
	}
	seen := map[string]bool{}
	for _, id := range st.Occupied() {
		stack := st.Stack(id)
		if len(stack) != 1 {
			return fmt.Errorf("courtfall: hex %s holds %d units", id, len(stack))
		}
		k := unitKey(stack[0].Color, stack[0].Piece)
		if seen[k] {
			return fmt.Errorf("courtfall: duplicate unit %s", k)
		}
		seen[k] = true
	}
	if d.Phase == phasePlay {
		if d.Turn != 1 && d.Turn != 2 {
			return fmt.Errorf("courtfall: play phase without a turn seat")
		}
		if len(seen) != 2*len(unitTypes) {
			return fmt.Errorf("courtfall: play phase needs all ten units on the board")
		}
		switch d.Stage {
		case stageRecover, stageMain, stageSeize, stageShift:
		case stageAct:
			if d.Act == nil {
				return fmt.Errorf("courtfall: act stage without an activation")
			}
		case stageRespond, stageReroll:
			if d.Con == nil {
				return fmt.Errorf("courtfall: %s stage without a contest", d.Stage)
			}
		case stagePush:
			if len(d.Pushes) == 0 {
				return fmt.Errorf("courtfall: push stage without a push")
			}
		default:
			return fmt.Errorf("courtfall: unknown stage %q", d.Stage)
		}
	}
	return nil
}

// --- legal moves ------------------------------------------------------------

// Legal lists seat's decisions.
func (rs *Ruleset) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Move {
	d, err := dataOf(st)
	if err != nil || toMove(d) != seat || seat == 0 {
		return nil
	}
	e := rs.newEng(game, st, d)
	return e.legal()
}

func (e *eng) legal() []state.Move {
	d := e.d
	switch d.Phase {
	case phaseTerrain:
		return e.legalTiles()
	case phaseCourt:
		out := make([]state.Move, 0, len(courts))
		for _, c := range courts {
			out = append(out, state.Move{Action: actCourt, Piece: c})
		}
		return out
	case phaseDeploy:
		return e.legalDeploys()
	case phasePlay:
		switch d.Stage {
		case stageRecover:
			return e.legalRecover()
		case stageMain:
			return e.legalMain()
		case stageAct:
			return e.legalAct()
		case stageRespond:
			return e.legalRespond()
		case stageReroll:
			return e.legalReroll()
		case stagePush:
			return e.legalPush()
		case stageShift:
			return e.legalShift()
		case stageSeize:
			out := []state.Move{}
			if e.canSeize() {
				out = append(out, state.Move{Action: actSeize})
			}
			return append(out, state.Move{Action: actDone})
		}
	}
	return nil
}

// --- setup ------------------------------------------------------------------

// specialsAround counts the special tiles adjacent to cell.
func (e *eng) specialsAround(cell int) int {
	n := 0
	for d := 0; d < 6; d++ {
		if nb := e.g.nbr[cell][d]; nb >= 0 && e.tiles[nb] != 0 {
			n++
		}
	}
	return n
}

// layoutOK checks the draft constraints around the given hexes: no special
// tile in a deployment zone or on the Eye, no two Hazards adjacent and no
// special tile touching more than two other special tiles. (Read literally
// — "no more than two special tiles adjacent to any single tile" for every
// tile, tan ones included — the last rule leaves room for only eight of the
// ten tiles on this arena, so it is applied to the special tiles.)
func (e *eng) layoutOK(cells []int) bool {
	for _, c := range cells {
		t := e.tiles[c]
		if t == 0 {
			continue
		}
		if e.g.zone[c] != 0 || c == e.g.eye {
			return false
		}
		if e.specialsAround(c) > 2 {
			return false
		}
		if t == 3 {
			for d := 0; d < 6; d++ {
				if nb := e.g.nbr[c][d]; nb >= 0 && e.tiles[nb] == 3 {
					return false
				}
			}
		}
	}
	return true
}

// touched lists cell and its neighbours.
func (e *eng) touched(cells ...int) []int {
	seen := map[int]bool{}
	var out []int
	add := func(c int) {
		if c >= 0 && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	for _, c := range cells {
		add(c)
		for d := 0; d < 6; d++ {
			add(e.g.nbr[c][d])
		}
	}
	return out
}

// placeable reports whether a tile of the color could go on cell under the
// draft constraints alone (not counting whether the draft can still finish).
// Only the new tile and its special neighbours can break a constraint.
func (e *eng) placeable(cell int, code byte) bool {
	if e.tiles[cell] != 0 || cell == e.g.eye || e.g.zone[cell] != 0 || e.at[cell] >= 0 {
		return false
	}
	around := 0
	for d := 0; d < 6; d++ {
		nb := e.g.nbr[cell][d]
		if nb < 0 || e.tiles[nb] == 0 {
			continue
		}
		around++
		if code == 3 && e.tiles[nb] == 3 {
			return false // two Hazards touching
		}
		if e.specialsAround(nb) >= 2 {
			return false // the neighbour would touch a third special tile
		}
	}
	return around <= 2
}

// completable reports whether the tiles still to lay (left) can all be
// placed on the current layout, in some order. The draft only allows moves
// that keep this true, so every game gets all ten tiles down.
func (e *eng) completable(left [3]int) bool {
	total := left[0] + left[1] + left[2]
	if total == 0 {
		return true
	}
	var buf [37]int
	cands := buf[:0]
	for c := range e.g.ids {
		if e.tiles[c] == 0 && c != e.g.eye && e.g.zone[c] == 0 && e.at[c] < 0 {
			cands = append(cands, c)
		}
	}
	var failed map[[3]uint64]bool
	var special, hazard uint64
	for c, t := range e.tiles {
		if t != 0 {
			special |= 1 << uint(c)
		}
		if t == 3 {
			hazard |= 1 << uint(c)
		}
	}
	var fill func(pos int, left [3]int, total int) bool
	fill = func(pos int, left [3]int, total int) bool {
		if total == 0 {
			return true
		}
		if len(cands)-pos < total {
			return false
		}
		key := [3]uint64{uint64(pos) | uint64(left[0])<<8 | uint64(left[1])<<16 | uint64(left[2])<<24, special, hazard}
		if failed[key] {
			return false
		}
		c := cands[pos]
		for ci := 0; ci < 3; ci++ {
			if left[ci] == 0 {
				continue
			}
			code := byte(ci + 1)
			if !e.placeable(c, code) {
				continue
			}
			e.tiles[c] = code
			special |= 1 << uint(c)
			if code == 3 {
				hazard |= 1 << uint(c)
			}
			left[ci]--
			ok := fill(pos+1, left, total-1)
			left[ci]++
			special &^= 1 << uint(c)
			hazard &^= 1 << uint(c)
			e.tiles[c] = 0
			if ok {
				return true
			}
		}
		if fill(pos+1, left, total) {
			return true
		}
		if failed == nil {
			failed = map[[3]uint64]bool{}
		}
		failed[key] = true
		return false
	}
	return fill(0, left, total)
}

func (e *eng) canTile(cell int, color string) bool {
	ci := tileIndex(color)
	if ci < 0 || e.d.Left[ci] == 0 {
		return false
	}
	code := tileCode[color]
	if !e.placeable(cell, code) {
		return false
	}
	left := e.d.Left
	left[ci]--
	e.tiles[cell] = code
	ok := e.completable(left)
	e.tiles[cell] = 0
	return ok
}

func (e *eng) anyTileMove() bool {
	for ci, color := range tileColors {
		if e.d.Left[ci] == 0 {
			continue
		}
		for c := range e.g.ids {
			if e.canTile(c, color) {
				return true
			}
		}
	}
	return false
}

func (e *eng) legalTiles() []state.Move {
	var out []state.Move
	for ci, color := range tileColors {
		if e.d.Left[ci] == 0 {
			continue
		}
		for c, id := range e.g.ids {
			if e.canTile(c, color) {
				out = append(out, state.Move{Action: actTile, Piece: color, To: id})
			}
		}
	}
	return out
}

func (e *eng) legalDeploys() []state.Move {
	seat := toMove(e.d)
	var out []state.Move
	for _, p := range unitTypes {
		if e.st.Reserve(e.color[seat], p) == 0 {
			continue
		}
		for c, id := range e.g.ids {
			if int(e.g.zone[c]) == seat && e.at[c] < 0 {
				out = append(out, state.Move{Action: actDeploy, Piece: p, To: id})
			}
		}
	}
	return out
}

// --- main stage -------------------------------------------------------------

func (e *eng) ownUnits(seat int) []*unit {
	var out []*unit
	for _, p := range unitTypes {
		if u := e.unitOf(seat, p); u != nil {
			out = append(out, u)
		}
	}
	return out
}

// recoverOptions are the units a free recovery may work on.
func (e *eng) recoverOptions(seat int) []*unit {
	var out []*unit
	gale := e.court(seat) == courtGale
	for _, u := range e.ownUnits(seat) {
		switch e.cond(u) {
		case condFatigued:
			out = append(out, u)
		case condExhausted:
			if gale {
				out = append(out, u)
			}
		}
	}
	return out
}

func (e *eng) legalRecover() []state.Move {
	var out []state.Move
	for _, u := range e.recoverOptions(e.d.Turn) {
		out = append(out, state.Move{Action: actRecover, Piece: u.piece, From: e.g.ids[u.cell]})
	}
	return out
}

// priceOf is the Momentum cost of buying n VP in one Clear Turn: 2, 3, 4…
func priceOf(n int) int {
	total := 0
	for i := 0; i < n; i++ {
		total += 2 + i
	}
	return total
}

// eyeIncome reports whether seat's units hold the Eye for the payout.
func (e *eng) eyeIncome(seat int) bool {
	if e.g.eye < 0 {
		return false
	}
	u := e.unitAt(e.g.eye)
	return u != nil && u.seat == seat && e.cond(u) != condExhausted
}

// maxBuy is how many VP seat could buy in a Clear Turn taken now.
func (e *eng) maxBuy(seat int) int {
	m := e.mom(seat)
	if e.eyeIncome(seat) {
		m++
		if m > e.momCap {
			m = e.momCap
		}
	}
	n := 0
	for priceOf(n+1) <= m {
		n++
	}
	return n
}

func (e *eng) legalMain() []state.Move {
	seat := e.d.Turn
	var out []state.Move
	own := e.ownUnits(seat)
	for _, u := range own {
		if e.cond(u) == condFresh {
			out = append(out, state.Move{Action: actActivate, Piece: u.piece, From: e.g.ids[u.cell]})
		}
	}
	for _, u := range own {
		if e.cond(u) == condFatigued {
			out = append(out, state.Move{Action: actExert, Piece: u.piece, From: e.g.ids[u.cell]})
		}
	}
	if e.mom(seat) >= 1 {
		for _, u := range own {
			if e.cond(u) == condExhausted {
				continue
			}
			for d := 0; d < 6; d++ {
				if nb := e.g.nbr[u.cell][d]; nb >= 0 && e.at[nb] < 0 {
					out = append(out, state.Move{Action: actSurge, Piece: u.piece, From: e.g.ids[u.cell], To: e.g.ids[nb]})
				}
			}
		}
	}
	if e.mom(seat) >= 2 {
		for _, u := range own {
			if e.cond(u) == condExhausted {
				out = append(out, state.Move{Action: actRouse, Piece: u.piece, From: e.g.ids[u.cell]})
			}
		}
	}
	if e.court(seat) == courtStone && !e.d.StoneUsed[seat-1] {
		out = append(out, e.legalSwaps()...)
	}
	for n, max := 0, e.maxBuy(seat); n <= max; n++ {
		out = append(out, state.Move{Action: actClear, Count: n})
	}
	return out
}

// swappable reports whether the tiles at a and b may trade places.
func (e *eng) canSwap(a, b int) bool {
	if a == b || a == e.g.eye || b == e.g.eye {
		return false
	}
	if e.at[a] >= 0 || e.at[b] >= 0 || e.g.dist[a][b] > 2 {
		return false
	}
	if e.tiles[a] == e.tiles[b] {
		return false
	}
	e.tiles[a], e.tiles[b] = e.tiles[b], e.tiles[a]
	ok := e.layoutOK(e.touched(a, b))
	e.tiles[a], e.tiles[b] = e.tiles[b], e.tiles[a]
	return ok
}

func (e *eng) legalSwaps() []state.Move {
	var out []state.Move
	for a := range e.g.ids {
		for b := a + 1; b < len(e.g.ids); b++ {
			if e.canSwap(a, b) {
				out = append(out, state.Move{Action: actTiles, From: e.g.ids[a], To: e.g.ids[b]})
			}
		}
	}
	return out
}

func (e *eng) canSeize() bool {
	d := e.d
	return d.Streak < 2 && e.mom(d.Turn) >= 2
}

// --- Apply ------------------------------------------------------------------

// moveKey is a move's identity for matching: the fields that are decisions,
// not the ones derived from the position.
func moveKey(mv state.Move) string {
	var b strings.Builder
	b.WriteString(mv.Action)
	switch mv.Action {
	case actTile, actDeploy:
		b.WriteString("|" + mv.Piece + "|" + string(mv.To))
	case actCourt, actRecover, actActivate, actExert, actRouse:
		b.WriteString("|" + mv.Piece)
	case actClear, actRespond, actReroll:
		fmt.Fprintf(&b, "|%d", mv.Count)
	case actSurge:
		b.WriteString("|" + mv.Piece + "|" + string(mv.To))
	case actTiles:
		a, c := string(mv.From), string(mv.To)
		if a > c {
			a, c = c, a
		}
		b.WriteString("|" + a + "|" + c)
	case actMove, actShoot, actShove, actShift:
		b.WriteString("|" + string(mv.To))
	case actStrike:
		fmt.Fprintf(&b, "|%s|%d", mv.To, mv.Count)
	case actPush:
		fmt.Fprintf(&b, "|%d", mv.Count)
	}
	return b.String()
}

// Apply validates and applies one decision.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Phase == phaseOver {
		return nil, fmt.Errorf("courtfall: the game is over")
	}
	next := toMove(d)
	if seat != next {
		return nil, fmt.Errorf("courtfall: it is not seat %d's decision (waiting for seat %d: %s)", seat, next, rs.waiting(game, st, d))
	}
	ns := st.Clone()
	nd := d.clone()
	ns.SetExt(nd)
	e := rs.newEng(game, ns, nd)

	want := moveKey(mv)
	var legal state.Move
	found := false
	if mv.Action == actTile && nd.Phase == phaseTerrain {
		// The draft's legal list is the expensive one (every tile is
		// checked for whether the draft can still be finished): validate
		// just the one placement.
		if cell, ok := e.g.index[mv.To]; ok && e.canTile(cell, mv.Piece) {
			legal, found = state.Move{Action: actTile, Piece: mv.Piece, To: mv.To}, true
		}
	} else {
		for _, cand := range e.legal() {
			if moveKey(cand) == want {
				legal, found = cand, true
				break
			}
		}
	}
	if found {
		switch legal.Action {
		case actMove, actStrike, actShoot, actShove, actShift:
			// The unit is implied by the activation; a move that names a
			// different one is not this decision.
			if mv.Piece != "" && mv.Piece != legal.Piece {
				found = false
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("courtfall: %q is not legal now — %s", rs.Format(game, mv), rs.waiting(game, st, d))
	}
	// Reload: legal() does not mutate, but be explicit that e owns ns.
	e.ev = nil
	e.execute(legal)
	nd.Events = e.ev
	ns.SetPly(st.Ply() + 1)

	if nd.Phase != phaseOver && ns.Ply() >= e.maxPlies {
		switch {
		case nd.VP[0] > nd.VP[1]:
			e.finish(1, "ply-limit")
		case nd.VP[1] > nd.VP[0]:
			e.finish(2, "ply-limit")
		default:
			e.finish(0, "ply-limit")
		}
		nd.Events = e.ev
	}

	res := &state.MoveResult{
		NewState: ns,
		Move:     legal,
		Notation: rs.Format(game, legal),
	}
	if nd.Phase == phaseOver {
		res.GameEnded = true
		res.NextTurnSeat = seat
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
		res.EndReason = nd.Reason
		res.EndDetail = fmt.Sprintf("%d–%d VP", nd.VP[0], nd.VP[1])
	} else {
		res.NextTurnSeat = toMove(nd)
	}
	return res, nil
}

// waiting describes the decision a position is waiting for.
func (rs *Ruleset) waiting(game *catalog.CompiledGame, st *state.BoardState, d *Data) string {
	e := rs.newEng(game, st, d)
	return e.prompt()
}

// execute applies a (legal, normalised) move to the working position.
func (e *eng) execute(mv state.Move) {
	d := e.d
	switch mv.Action {
	case actTile:
		e.doTile(mv)
	case actCourt:
		e.doCourt(mv)
	case actDeploy:
		e.doDeploy(mv)
	case actRecover:
		u := e.unitOf(d.Turn, mv.Piece)
		e.doRecover(u)
		d.Stage = stageMain
	case actActivate, actExert:
		u := e.unitOf(d.Turn, mv.Piece)
		e.startActivation(u, mv.Action == actExert)
	case actClear:
		e.doClear(mv.Count)
	case actSurge:
		e.doSurge(mv)
	case actRouse:
		u := e.unitOf(d.Turn, mv.Piece)
		d.Mom[d.Turn-1] -= 2
		e.setCond(u, condFresh)
		e.say("%s spends 2 Momentum to Rouse the %s: its black stone is removed.", e.seatLabel(d.Turn), unitName[u.piece])
	case actTiles:
		e.doSwapTiles(mv)
	case actMove:
		e.doMove(mv)
	case actStrike:
		e.doStrike(mv)
	case actShoot:
		e.doShoot(mv)
	case actShove:
		e.doShove(mv)
	case actEnd:
		e.say("%s ends the activation.", e.label(e.unitOf(d.Turn, d.Act.Unit)))
		e.finishActivation()
	case actRespond:
		e.doRespond(mv)
	case actReroll:
		e.doReroll(mv.Count)
	case actKeep:
		e.d.Con.Step++
		e.nextReroll()
	case actPush:
		e.doPush(mv)
	case actShift:
		e.doShift(mv)
	case actStay:
		e.say("%s holds its ground.", e.label(e.unitOf(d.Turn, d.Act.Unit)))
		e.afterAction()
	case actSeize:
		d.Mom[d.Turn-1] -= 2
		e.say("%s spends 2 Momentum to Seize the Tempo and takes another turn.", e.seatLabel(d.Turn))
		e.beginTurn(d.Turn, d.Streak+1)
	case actDone:
		e.passTurn()
	}
}

func (e *eng) doTile(mv state.Move) {
	d := e.d
	seat := toMove(d)
	cell := e.g.index[mv.To]
	e.setTile(cell, tileCode[mv.Piece])
	d.Left[tileIndex(mv.Piece)]--
	d.N++
	e.say("%s lays %s at %s.", e.seatLabel(seat), tileLabel(mv.Piece), mv.To)
	if d.Left == [3]int{} || !e.anyTileMove() {
		d.Phase = phaseCourt
		d.N = 0
		if d.Left != [3]int{} {
			e.say("No further tile can be laid; the draft ends.")
		}
	}
}

func tileLabel(color string) string {
	switch color {
	case tileGreen:
		return "Overgrowth"
	case tileBlue:
		return "Mire"
	case tileRed:
		return "Hazard"
	}
	return color
}

func courtLabel(c string) string {
	if c == "" {
		return "no Court"
	}
	return strings.ToUpper(c[:1]) + c[1:] + " Court"
}

func (e *eng) doCourt(mv state.Move) {
	d := e.d
	seat := toMove(d)
	d.Courts[seat-1] = mv.Piece
	d.N++
	e.say("%s chooses the %s.", e.seatLabel(seat), courtLabel(mv.Piece))
	if d.N >= 2 {
		d.Phase = phaseDeploy
		d.N = 0
	}
}

func (e *eng) doDeploy(mv state.Move) {
	d := e.d
	seat := toMove(d)
	color := e.color[seat]
	e.st.Set(mv.To, state.Occupant{Piece: mv.Piece, Color: color})
	e.st.SetReserve(color, mv.Piece, e.st.Reserve(color, mv.Piece)-1)
	cell := e.g.index[mv.To]
	e.units = append(e.units, unit{key: unitKey(color, mv.Piece), seat: seat, piece: mv.Piece, cell: cell})
	e.at[cell] = len(e.units) - 1
	d.N++
	e.say("%s deploys the %s at %s.", e.seatLabel(seat), unitName[mv.Piece], mv.To)
	if d.N >= 2*len(unitTypes) {
		d.Phase = phasePlay
		d.N = 0
		e.say("Deployment complete. %s takes the first turn.", e.seatLabel(2))
		e.beginTurn(2, 1)
	}
}

// beginTurn starts seat's turn: the free recovery, then the main stage.
func (e *eng) beginTurn(seat, streak int) {
	d := e.d
	d.Turn = seat
	d.Streak = streak
	d.Act = nil
	d.Con = nil
	d.Pushes = nil
	opts := e.recoverOptions(seat)
	switch len(opts) {
	case 0:
		d.Stage = stageMain
	case 1:
		e.doRecover(opts[0])
		d.Stage = stageMain
	default:
		d.Stage = stageRecover
	}
}

func (e *eng) doRecover(u *unit) {
	seat := u.seat
	switch e.cond(u) {
	case condFatigued:
		e.setCond(u, condFresh)
		e.say("%s recovers: the %s's white stone is removed.", e.seatLabel(seat), unitName[u.piece])
	case condExhausted:
		e.setCond(u, condFatigued)
		e.say("%s recovers: the %s's black stone becomes white.", e.seatLabel(seat), unitName[u.piece])
	}
}

func (e *eng) startActivation(u *unit, exert bool) {
	d := e.d
	left := 2
	verb := "activates"
	if exert {
		left = 1
		verb = "exerts"
	}
	d.Act = &Activation{Unit: u.piece, Left: left, Exert: exert}
	d.Stage = stageAct
	e.say("%s %s the %s.", e.seatLabel(u.seat), verb, unitName[u.piece])
}

func (e *eng) doClear(n int) {
	d := e.d
	seat := d.Turn
	e.say("%s takes a Clear Turn.", e.seatLabel(seat))
	if e.eyeIncome(seat) {
		before := e.mom(seat)
		e.addMom(seat, 1)
		if e.mom(seat) > before {
			e.say("The Eye pays out 1 Momentum.")
		}
	}
	if n > 0 {
		cost := priceOf(n)
		d.Mom[seat-1] -= cost
		d.VP[seat-1] += n
		e.say("%s buys %d VP for %d Momentum (now %d VP).", e.seatLabel(seat), n, cost, d.VP[seat-1])
	}
	cleared := 0
	for _, u := range e.ownUnits(seat) {
		if e.cond(u) != condFresh {
			e.setCond(u, condFresh)
			cleared++
		}
	}
	if cleared > 0 {
		e.say("%s catches their breath: %d stone-bearing unit(s) are Fresh again.", e.seatLabel(seat), cleared)
	}
	d.StoneUsed[seat-1] = false
	if d.VP[seat-1] >= e.vpWin {
		e.say("%s reaches %d VP and wins as champion.", e.seatLabel(seat), d.VP[seat-1])
		e.finish(seat, "victory")
		return
	}
	e.endTurn()
}

func (e *eng) finish(winner int, reason string) {
	d := e.d
	d.Phase = phaseOver
	d.Stage = ""
	d.Act = nil
	d.Con = nil
	d.Pushes = nil
	d.Winner = winner
	d.Reason = reason
	if winner == 0 {
		e.say("The game ends in a draw.")
	}
}

func (e *eng) endTurn() {
	d := e.d
	d.Act = nil
	d.Con = nil
	if e.canSeize() {
		d.Stage = stageSeize
		return
	}
	e.passTurn()
}

func (e *eng) passTurn() {
	d := e.d
	e.beginTurn(3-d.Turn, 1)
}

func (e *eng) doSurge(mv state.Move) {
	d := e.d
	u := e.unitOf(d.Turn, mv.Piece)
	to := e.g.index[mv.To]
	d.Mom[d.Turn-1]--
	e.say("%s spends 1 Momentum to Surge the %s to %s.", e.seatLabel(d.Turn), unitName[u.piece], mv.To)
	e.moveUnit(u, to)
	if e.tile(to) == 3 {
		e.terrainHit(u, 0, "enters a Hazard")
	}
}

func (e *eng) doSwapTiles(mv state.Move) {
	d := e.d
	a, b := e.g.index[mv.From], e.g.index[mv.To]
	ta, tb := e.tiles[a], e.tiles[b]
	e.setTile(a, tb)
	e.setTile(b, ta)
	d.StoneUsed[d.Turn-1] = true
	e.say("%s swaps the tiles at %s and %s.", e.seatLabel(d.Turn), mv.From, mv.To)
}

// --- activation ---------------------------------------------------------------

// activeUnit is the unit spending its actions.
func (e *eng) activeUnit() *unit {
	if e.d.Act == nil {
		return nil
	}
	return e.unitOf(e.d.Turn, e.d.Act.Unit)
}

// reach lists the hexes u may move to with one Move action.
func (e *eng) reach(u *unit) []int {
	var out []int
	switch u.piece {
	case "knight":
		for c := range e.g.ids {
			if c != u.cell && e.at[c] < 0 && int(e.g.dist[u.cell][c]) <= unitMove[u.piece] {
				out = append(out, c)
			}
		}
	case "rook":
		for d := 0; d < 6; d++ {
			for i, c := range e.g.ray[u.cell][d] {
				if i >= unitMove[u.piece] || e.at[c] >= 0 {
					break
				}
				out = append(out, c)
				if t := e.tile(c); t == 2 || t == 3 {
					break
				}
			}
		}
	default:
		dist := map[int]int{u.cell: 0}
		queue := []int{u.cell}
		for len(queue) > 0 {
			c := queue[0]
			queue = queue[1:]
			if dist[c] >= unitMove[u.piece] {
				continue
			}
			for d := 0; d < 6; d++ {
				nb := e.g.nbr[c][d]
				if nb < 0 || e.at[nb] >= 0 {
					continue
				}
				if _, seen := dist[nb]; seen {
					continue
				}
				dist[nb] = dist[c] + 1
				out = append(out, nb)
				if t := e.tile(nb); t == 2 || t == 3 {
					continue // Mire and Hazard end the move
				}
				queue = append(queue, nb)
			}
		}
		sort.Ints(out)
	}
	return out
}

// shootTargets lists the enemy units u (an Arbalest) may shoot.
func (e *eng) shootTargets(u *unit) []*unit {
	var out []*unit
	for d := 0; d < 6; d++ {
		for i, c := range e.g.ray[u.cell][d] {
			if i >= 4 {
				break
			}
			if o := e.unitAt(c); o != nil {
				if o.seat != u.seat && i >= 1 {
					out = append(out, o)
				}
				break
			}
			if e.tile(c) == 1 {
				break // Overgrowth blocks the line
			}
		}
	}
	return out
}

func (e *eng) enemiesAdjacent(u *unit) []*unit {
	var out []*unit
	for d := 0; d < 6; d++ {
		if nb := e.g.nbr[u.cell][d]; nb >= 0 {
			if o := e.unitAt(nb); o != nil && o.seat != u.seat {
				out = append(out, o)
			}
		}
	}
	return out
}

func (e *eng) legalAct() []state.Move {
	u := e.activeUnit()
	var out []state.Move
	if u == nil {
		return []state.Move{{Action: actEnd}}
	}
	from := e.g.ids[u.cell]
	if e.d.Act.Left > 0 && e.cond(u) != condExhausted {
		for _, c := range e.reach(u) {
			out = append(out, state.Move{Action: actMove, Piece: u.piece, From: from, To: e.g.ids[c]})
		}
		cv := e.cv(u)
		for _, t := range e.enemiesAdjacent(u) {
			for n := 0; n <= cv; n++ {
				out = append(out, state.Move{Action: actStrike, Piece: u.piece, From: from, To: e.g.ids[t.cell], Count: n})
			}
		}
		if u.piece == "bishop" {
			for _, t := range e.shootTargets(u) {
				out = append(out, state.Move{Action: actShoot, Piece: u.piece, From: from, To: e.g.ids[t.cell]})
			}
		}
		for _, t := range e.enemiesAdjacent(u) {
			if e.shoveUseful(u, t) {
				out = append(out, state.Move{Action: actShove, Piece: u.piece, From: from, To: e.g.ids[t.cell]})
			}
		}
	}
	return append(out, state.Move{Action: actEnd})
}

// shoveUseful reports whether shoving t could move it at all: it can be
// pushed and the first hex behind it is not blocked by another unit.
func (e *eng) shoveUseful(u, t *unit) bool {
	if !e.pushable(t) {
		return false
	}
	d := e.g.dirBetween(u.cell, t.cell)
	if d < 0 {
		return false
	}
	next := e.g.nbr[t.cell][d]
	return next < 0 || e.at[next] < 0
}

func (e *eng) doMove(mv state.Move) {
	d := e.d
	u := e.activeUnit()
	to := e.g.index[mv.To]
	from := u.cell
	d.Act.Left--
	d.Act.Charge = ""
	e.say("%s moves from %s to %s.", e.label(u), e.cellName(from), mv.To)
	e.moveUnit(u, to)
	if u.piece == "rook" && int(e.g.dist[from][to]) >= 2 {
		d.Act.Charge = e.cellName(from)
	}
	if e.tile(to) == 3 {
		e.terrainHit(u, 0, "enters a Hazard")
	}
	e.afterAction()
}

// finishActivation applies the stone the activation earns and ends the turn.
func (e *eng) finishActivation() {
	d := e.d
	u := e.activeUnit()
	if u != nil {
		before := e.cond(u)
		if d.Act.Exert {
			e.setCond(u, condExhausted)
		} else if before < condExhausted {
			e.setCond(u, before+1)
		}
		switch e.cond(u) {
		case condFatigued:
			e.say("The %s gains a white stone (Fatigued).", unitName[u.piece])
		case condExhausted:
			if before != condExhausted {
				e.say("The %s gains a black stone (Exhausted).", unitName[u.piece])
			}
		}
	}
	e.endTurn()
}

// afterAction continues the activation once an action (and anything it set
// off) is fully resolved.
func (e *eng) afterAction() {
	d := e.d
	u := e.activeUnit()
	if u == nil || e.cond(u) == condExhausted || d.Act.Left <= 0 {
		if u != nil && e.cond(u) == condExhausted && d.Act.Left > 0 {
			e.say("The %s is Exhausted and cannot act any more.", unitName[u.piece])
		}
		e.finishActivation()
		return
	}
	d.Stage = stageAct
}
