package hive

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is Hive's state.Ruleset. A turn is one decision: place a tile from
// the supply, move a tile already in the hive, or (only when nothing else is
// possible) pass.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

const (
	actPlace = "place"
	actMove  = "move"
	actPass  = "pass"
)

// pc is one tile: its kind and the seat that owns it.
type pc struct{ kind, seat uint8 }

// eng is a decoded position: cells[i] is the stack of tiles on cell i,
// bottom first.
type eng struct {
	g      *geo
	d      *Data
	cells  [][]pc
	nums   [][]uint8        // nums[i][j] numbers the tile cells[i][j] for the notation
	res    [3][numKinds]int // tiles still in each seat's supply
	colors [3]string
	game   *catalog.CompiledGame
	ev     []string
}

func newEng(game *catalog.CompiledGame, st *state.BoardState, d *Data) *eng {
	g := geoOf(game)
	e := &eng{g: g, d: d, cells: make([][]pc, len(g.ids)), nums: make([][]uint8, len(g.ids)), game: game}
	for _, s := range game.Definition.Seats {
		if s.Seat >= 1 && s.Seat <= 2 {
			e.colors[s.Seat] = s.Color
		}
	}
	for _, id := range st.Occupied() {
		i, ok := g.index[id]
		if !ok {
			continue
		}
		for _, o := range st.Stack(id) {
			k, _ := kindOf(o.Piece)
			e.cells[i] = append(e.cells[i], pc{uint8(k), uint8(e.seatOf(o.Color))})
		}
	}
	for s := 1; s <= 2; s++ {
		for k := range kindName {
			e.res[s][k] = st.Reserve(e.colors[s], kindName[k])
		}
	}
	e.number()
	return e
}

// number gives every tile its number: the one the position records, or (for a
// position built by hand) the next free one of its kind in cell order.
func (e *eng) number() {
	var used [3][numKinds]map[uint8]bool
	for i, stack := range e.cells {
		known := e.d.Nums[string(e.g.ids[i])]
		if len(known) == len(stack) {
			e.nums[i] = append([]uint8(nil), known...)
			for j, p := range stack {
				if used[p.seat][p.kind] == nil {
					used[p.seat][p.kind] = map[uint8]bool{}
				}
				used[p.seat][p.kind][e.nums[i][j]] = true
			}
		}
	}
	for i, stack := range e.cells {
		if len(e.nums[i]) == len(stack) {
			continue
		}
		e.nums[i] = make([]uint8, len(stack))
		for j, p := range stack {
			if used[p.seat][p.kind] == nil {
				used[p.seat][p.kind] = map[uint8]bool{}
			}
			n := uint8(1)
			for used[p.seat][p.kind][n] {
				n++
			}
			used[p.seat][p.kind][n] = true
			e.nums[i][j] = n
		}
	}
}

// exportNums records the tiles' numbers in the position payload.
func (e *eng) exportNums() {
	e.d.Nums = map[string][]uint8{}
	for i, stack := range e.cells {
		if len(stack) > 0 {
			e.d.Nums[string(e.g.ids[i])] = append([]uint8(nil), e.nums[i]...)
		}
	}
}

func (e *eng) seatOf(color string) int {
	for s := 1; s <= 2; s++ {
		if e.colors[s] == color {
			return s
		}
	}
	return 0
}

func (e *eng) save(st *state.BoardState) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	for i, stack := range e.cells {
		if len(stack) == 0 {
			continue
		}
		occ := make([]state.Occupant, len(stack))
		for j, p := range stack {
			occ[j] = state.Occupant{Piece: kindName[p.kind], Color: e.colors[p.seat]}
		}
		st.SetStack(e.g.ids[i], occ)
	}
	for s := 1; s <= 2; s++ {
		for k := range kindName {
			st.SetReserve(e.colors[s], kindName[k], e.res[s][k])
		}
	}
}

func (e *eng) height(i int) int {
	if i < 0 {
		return 0
	}
	return len(e.cells[i])
}

func (e *eng) top(i int) pc { return e.cells[i][len(e.cells[i])-1] }

func (e *eng) occupied(i int) bool { return i >= 0 && len(e.cells[i]) > 0 }

// pieces counts the tiles on the table.
func (e *eng) pieces() int {
	n := 0
	for _, s := range e.cells {
		n += len(s)
	}
	return n
}

// connected reports whether the occupied cells form one group (the One Hive
// rule). An empty table or a single tile is connected.
func (e *eng) connected() bool {
	start, total := -1, 0
	for i := range e.cells {
		if len(e.cells[i]) > 0 {
			total++
			if start < 0 {
				start = i
			}
		}
	}
	if total <= 1 {
		return true
	}
	seen := make([]bool, len(e.cells))
	queue := []int{start}
	seen[start] = true
	reached := 1
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		for _, n := range e.g.nbr[c] {
			if n >= 0 && !seen[n] && len(e.cells[n]) > 0 {
				seen[n] = true
				reached++
				queue = append(queue, n)
			}
		}
	}
	return reached == total
}

// flank returns the two cells that touch both a and its neighbour in
// direction d.
func (e *eng) flank(a, d int) (int, int) {
	return e.g.nbr[a][(d+1)%6], e.g.nbr[a][(d+5)%6]
}

// blocked reports whether a tile travelling from a to its neighbour in
// direction d at the given level cannot squeeze between the two stacks
// beside the step (the "freedom to move" rule): both must stand higher than
// the tile.
func (e *eng) blocked(a, d, level int) bool {
	f1, f2 := e.flank(a, d)
	h1, h2 := e.height(f1), e.height(f2)
	if h2 < h1 {
		h1 = h2
	}
	return h1 > level
}

// slides reports whether a tile on the ground at a may slide into the empty
// neighbour in direction d: it must stay in touch with the hive (one of the
// two cells beside the step is occupied) and fit through (not both).
func (e *eng) slides(a, d int) bool {
	b := e.g.nbr[a][d]
	if b < 0 || len(e.cells[b]) > 0 {
		return false
	}
	f1, f2 := e.flank(a, d)
	return e.occupied(f1) != e.occupied(f2)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// kindDests lists where a tile of the given movement kind, lifted from cell
// from (whose stack was ha high), could go. The tile is already lifted off.
func (e *eng) kindDests(kind, from, ha int) []int {
	var out []int
	switch kind {
	case kQueen, kPillbug:
		for d := 0; d < 6; d++ {
			if e.slides(from, d) {
				out = append(out, e.g.nbr[from][d])
			}
		}
	case kBeetle:
		for d := 0; d < 6; d++ {
			b := e.g.nbr[from][d]
			if b < 0 {
				continue
			}
			hb := len(e.cells[b])
			if ha == 1 && hb == 0 {
				if !e.slides(from, d) {
					continue
				}
			} else if e.blocked(from, d, maxInt(ha-1, hb)) {
				continue
			}
			out = append(out, b)
		}
	case kSpider:
		visited := map[int]bool{from: true}
		var walk func(cur, depth int)
		walk = func(cur, depth int) {
			if depth == 3 {
				out = append(out, cur)
				return
			}
			for d := 0; d < 6; d++ {
				if e.slides(cur, d) {
					if b := e.g.nbr[cur][d]; !visited[b] {
						visited[b] = true
						walk(b, depth+1)
						delete(visited, b)
					}
				}
			}
		}
		walk(from, 0)
	case kAnt:
		seen := map[int]bool{from: true}
		queue := []int{from}
		for len(queue) > 0 {
			c := queue[0]
			queue = queue[1:]
			for d := 0; d < 6; d++ {
				if e.slides(c, d) {
					if b := e.g.nbr[c][d]; !seen[b] {
						seen[b] = true
						queue = append(queue, b)
						out = append(out, b)
					}
				}
			}
		}
	case kHopper:
		for d := 0; d < 6; d++ {
			cur := e.g.nbr[from][d]
			if !e.occupied(cur) {
				continue
			}
			for e.occupied(cur) {
				cur = e.g.nbr[cur][d]
			}
			if cur >= 0 {
				out = append(out, cur)
			}
		}
	case kLadybug:
		// Two steps over the hive, then one down onto an empty cell.
		for d1 := 0; d1 < 6; d1++ {
			b1 := e.g.nbr[from][d1]
			if !e.occupied(b1) || e.blocked(from, d1, maxInt(ha-1, e.height(b1))) {
				continue
			}
			l1 := e.height(b1)
			for d2 := 0; d2 < 6; d2++ {
				b2 := e.g.nbr[b1][d2]
				if !e.occupied(b2) || e.blocked(b1, d2, maxInt(l1, e.height(b2))) {
					continue
				}
				l2 := e.height(b2)
				for d3 := 0; d3 < 6; d3++ {
					b3 := e.g.nbr[b2][d3]
					if b3 < 0 || e.occupied(b3) || b3 == from || e.blocked(b2, d3, l2) {
						continue
					}
					out = append(out, b3)
				}
			}
		}
	}
	return out
}

// destsOf lists the cells the top tile of cell i can move to by itself.
func (e *eng) destsOf(i int) []int {
	stack := e.cells[i]
	ha := len(stack)
	p := stack[ha-1]
	e.cells[i] = stack[:ha-1]
	defer func() { e.cells[i] = stack }()
	if ha == 1 && !e.connected() {
		return nil // lifting it would split the hive
	}
	kind := int(p.kind)
	if kind != kMosquito {
		return e.kindDests(kind, i, ha)
	}
	if ha > 1 {
		return e.kindDests(kBeetle, i, ha) // on top of the hive it is a beetle
	}
	// On the ground it moves like each kind of tile it touches.
	var out []int
	var copied [numKinds]bool
	for _, n := range e.g.nbr[i] {
		if !e.occupied(n) {
			continue
		}
		if k := int(e.top(n).kind); k != kMosquito && !copied[k] {
			copied[k] = true
			out = append(out, e.kindDests(k, i, ha)...)
		}
	}
	return out
}

// canThrow reports whether the tile on cell i can use the Pillbug's special
// ability: it is a Pillbug, or a Mosquito touching one, and nothing covers it.
func (e *eng) canThrow(i int) bool {
	if len(e.cells[i]) != 1 {
		return false
	}
	switch e.top(i).kind {
	case kPillbug:
		return true
	case kMosquito:
		for _, n := range e.g.nbr[i] {
			if e.occupied(n) && e.top(n).kind == kPillbug {
				return true
			}
		}
	}
	return false
}

// throws lists (from, to) pairs the Pillbug ability of the tile on p allows:
// a lone tile beside it is lifted over it onto an empty cell beside it.
func (e *eng) throws(p int) [][2]int {
	var out [][2]int
	for dt := 0; dt < 6; dt++ {
		t := e.g.nbr[p][dt]
		if t < 0 || len(e.cells[t]) != 1 || string(e.g.ids[t]) == e.d.Moved {
			continue
		}
		stack := e.cells[t]
		e.cells[t] = nil
		if e.connected() {
			// Up onto the Pillbug (level 1), then down to an empty cell.
			back := (dt + 3) % 6 // direction from t to p
			if !e.blocked(t, back, 1) {
				for db := 0; db < 6; db++ {
					b := e.g.nbr[p][db]
					if b < 0 || b == t || len(e.cells[b]) > 0 || e.blocked(p, db, 1) {
						continue
					}
					out = append(out, [2]int{t, b})
				}
			}
		}
		e.cells[t] = stack
	}
	return out
}

// movements lists the moves of the tiles already on the table.
func (e *eng) movements(seat int) []state.Move {
	if e.res[seat][kQueen] > 0 {
		return nil // nothing may move before the Queen is down
	}
	var out []state.Move
	seen := map[[2]int]bool{}
	add := func(from, to int) {
		k := [2]int{from, to}
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, state.Move{Action: actMove, From: e.g.ids[from], To: e.g.ids[to]})
	}
	var thrown [][2]int
	for i := range e.cells {
		if len(e.cells[i]) == 0 || int(e.top(i).seat) != seat {
			continue
		}
		for _, to := range e.destsOf(i) {
			add(i, to)
		}
		if e.canThrow(i) {
			thrown = append(thrown, e.throws(i)...)
		}
	}
	for _, m := range thrown {
		add(m[0], m[1])
	}
	return out
}

// placements lists the tiles seat may add from its supply.
func (e *eng) placements(seat int) []state.Move {
	forced := e.res[seat][kQueen] > 0 && e.d.Turns[seat-1] >= 3
	n := e.pieces()
	var cells []int
	switch n {
	case 0:
		cells = []int{e.g.center}
	default:
		for i := range e.cells {
			if len(e.cells[i]) > 0 {
				continue
			}
			touches, ok := false, true
			for _, nb := range e.g.nbr[i] {
				if !e.occupied(nb) {
					continue
				}
				touches = true
				// Only the second tile of the game may touch an enemy.
				if n > 1 && int(e.top(nb).seat) != seat {
					ok = false
					break
				}
			}
			if touches && ok {
				cells = append(cells, i)
			}
		}
	}
	var out []state.Move
	for k := 0; k < numKinds; k++ {
		if e.res[seat][k] == 0 || (forced && k != kQueen) {
			continue
		}
		for _, c := range cells {
			out = append(out, state.Move{Action: actPlace, Piece: kindName[k], To: e.g.ids[c]})
		}
	}
	return out
}

// legal lists the moves of the seat to move.
func (e *eng) legal() []state.Move {
	d := e.d
	if d.Turn == 0 {
		return nil
	}
	out := append(e.placements(d.Turn), e.movements(d.Turn)...)
	if len(out) == 0 {
		out = append(out, state.Move{Action: actPass})
	}
	for i := range out {
		e.annotate(&out[i], d.Turn)
	}
	return out
}

// surrounded counts the occupied cells beside seat's Queen, or -1 if it is
// not on the table yet.
func (e *eng) surrounded(seat int) int {
	for i, stack := range e.cells {
		for _, p := range stack {
			if int(p.kind) == kQueen && int(p.seat) == seat {
				n := 0
				for _, nb := range e.g.nbr[i] {
					if e.occupied(nb) {
						n++
					}
				}
				return n
			}
		}
	}
	return -1
}

func other(seat int) int { return 3 - seat }

func seatLabel(seat int) string {
	if seat == 1 {
		return "White"
	}
	return "Black"
}

// --- ruleset interface ----------------------------------------------------

// Start returns the empty table with both supplies full; White moves first.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	st := state.NewBoardState()
	for _, s := range game.Definition.Seats {
		for pool, n := range game.Definition.Reserves {
			st.SetReserve(s.Color, pool, n)
		}
	}
	st.SetExt(&Data{Turn: 1})
	return st, nil
}

// ToMove is the seat to move, or 0 once the game is over.
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
	out := state.Outcome{Ended: true, Reason: d.Reason}
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
	if d.Turn < 0 || d.Turn > 2 {
		return fmt.Errorf("hive: bad turn %d", d.Turn)
	}
	e := newEng(game, st, d)
	for _, id := range st.Occupied() {
		for _, o := range st.Stack(id) {
			if _, ok := kindOf(o.Piece); !ok || e.seatOf(o.Color) == 0 {
				return fmt.Errorf("hive: bad tile %s/%s at %s", o.Color, o.Piece, id)
			}
		}
	}
	return nil
}

// Legal lists the moves of seat.
func (rs *Ruleset) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Move {
	d, err := dataOf(st)
	if err != nil || d.Turn == 0 || seat != d.Turn {
		return nil
	}
	return newEng(game, st, d).legal()
}

// Apply plays one decision.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("hive: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("hive: it is %s's turn", seatLabel(d.Turn))
	}
	e := newEng(game, st, d)
	var legal state.Move
	found := false
	for _, cand := range e.legal() {
		if sameMove(mv, cand) {
			legal, found = cand, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("hive: %q is not a legal move now", rs.Format(game, mv))
	}
	nd := d.clone()
	e.d = nd
	e.execute(legal, seat)
	e.exportNums()
	nd.Events = e.ev
	ns := st.Clone()
	ns.SetExt(nd)
	e.save(ns)
	ns.SetPly(st.Ply() + 1)
	if nd.Turn != 0 && ns.Ply() >= game.ParamInt("max_plies", 300) {
		nd.Turn, nd.Reason = 0, "ply-limit"
		e.say("The move limit is reached: the game is a draw.")
		nd.Events = e.ev
	}
	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal), NextTurnSeat: nd.Turn}
	if nd.Turn == 0 {
		res.GameEnded = true
		res.NextTurnSeat = seat
		res.EndReason = nd.Reason
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
	}
	return res, nil
}

func (e *eng) say(format string, args ...any) { e.ev = append(e.ev, fmt.Sprintf(format, args...)) }

// execute plays a legal move and settles whether the game is over.
func (e *eng) execute(mv state.Move, seat int) {
	d := e.d
	d.Last = nil
	switch mv.Action {
	case actPlace:
		k, _ := kindOf(mv.Piece)
		to := e.g.index[mv.To]
		e.nums[to] = append(e.nums[to], uint8(e.startCount(k)-e.res[seat][k]+1))
		e.res[seat][k]--
		e.cells[to] = append(e.cells[to], pc{uint8(k), uint8(seat)})
		d.Moved, d.Passes = "", 0
		d.Last = []string{string(mv.To)}
		e.say("%s places a %s on %s.", seatLabel(seat), kindLabel[k], mv.To)
	case actMove:
		from, to := e.g.index[mv.From], e.g.index[mv.To]
		stack := e.cells[from]
		p := stack[len(stack)-1]
		num := e.nums[from][len(stack)-1]
		e.cells[from] = stack[:len(stack)-1]
		e.nums[from] = e.nums[from][:len(stack)-1]
		e.cells[to] = append(e.cells[to], p)
		e.nums[to] = append(e.nums[to], num)
		d.Moved, d.Passes = string(mv.To), 0
		d.Last = []string{string(mv.From), string(mv.To)}
		e.say("%s moves a %s from %s to %s.", seatLabel(seat), kindLabel[p.kind], mv.From, mv.To)
	case actPass:
		d.Moved = ""
		d.Passes++
		e.say("%s has no move and passes.", seatLabel(seat))
	}
	d.Turns[seat-1]++
	mine, theirs := e.surrounded(seat) == 6, e.surrounded(other(seat)) == 6
	switch {
	case mine && theirs:
		d.Turn, d.Reason = 0, "both-queens"
		e.say("Both Queen Bees are surrounded: the game is a draw.")
	case theirs:
		d.Turn, d.Winner, d.Reason = 0, seat, "queen"
		e.say("%s's Queen Bee is surrounded: %s wins!", seatLabel(other(seat)), seatLabel(seat))
	case mine:
		d.Turn, d.Winner, d.Reason = 0, other(seat), "queen"
		e.say("%s surrounds their own Queen Bee and loses.", seatLabel(seat))
	case d.Passes >= 2:
		d.Turn, d.Reason = 0, "stalemate"
		e.say("Neither player can move: the game is a draw.")
	default:
		d.Turn = other(seat)
	}
}
