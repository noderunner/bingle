package mycelium

import (
	"fmt"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is Mycelium's state.Ruleset.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

const (
	actPlace    = "place"
	actSpore    = "spore"
	actHyphae   = "hyphae"
	phaseSpore  = "spore"
	phaseHyphae = "hyphae"
)

// eng is a decoded position: cells[i] is the colour on cell i ("" for empty).
type eng struct {
	g     *geo
	d     *Data
	cells []string
	game  *catalog.CompiledGame
	ev    []string
}

func newEng(game *catalog.CompiledGame, st *state.BoardState, d *Data) *eng {
	g := geoOf(game)
	e := &eng{g: g, d: d, cells: make([]string, len(g.ids)), game: game}
	for _, id := range st.Occupied() {
		if i, ok := g.index[id]; ok {
			occ, _ := st.Get(id)
			e.cells[i] = occ.Color
		}
	}
	return e
}

func (e *eng) save(st *state.BoardState) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	for i, c := range e.cells {
		if c != "" {
			st.Set(e.g.ids[i], state.Occupant{Piece: c, Color: c})
		}
	}
}

func (e *eng) say(format string, args ...any) { e.ev = append(e.ev, fmt.Sprintf(format, args...)) }

func (e *eng) finish(winner int, reason string) {
	e.d.Turn, e.d.Winner, e.d.Reason = 0, winner, reason
}

// --- cluster geometry ------------------------------------------------------

// clusters returns every connected cluster of the given colour. Each cluster is
// a sorted list of cell indices.
func (e *eng) clusters(color string) [][]int {
	seen := make([]bool, len(e.cells))
	var out [][]int
	for i, c := range e.cells {
		if c != color || seen[i] {
			continue
		}
		group := []int{i}
		seen[i] = true
		for k := 0; k < len(group); k++ {
			for _, n := range e.g.nbr[group[k]] {
				if e.cells[n] == color && !seen[n] {
					seen[n] = true
					group = append(group, n)
				}
			}
		}
		sort.Ints(group)
		out = append(out, group)
	}
	return out
}

// clusterKey is a stable identifier for a cluster at a moment in time.
func clusterKey(phase string, group []int) string {
	ids := make([]string, len(group))
	for i, idx := range group {
		ids[i] = fmt.Sprintf("%d", idx)
	}
	return phase + "|" + strings.Join(ids, ",")
}

// isHyphae reports whether group (of colour color) is a hyphae cluster, i.e.
// touches at least one enemy node.
func (e *eng) isHyphae(group []int, color string) bool {
	for _, i := range group {
		for _, n := range e.g.nbr[i] {
			if e.cells[n] != "" && e.cells[n] != color {
				return true
			}
		}
	}
	return false
}

// strands returns every maximal straight line (strand) inside group. Each
// strand is a sorted list of cell indices in one of the six axial directions.
func (e *eng) strands(group []int) [][]int {
	set := make(map[int]bool, len(group))
	for _, i := range group {
		set[i] = true
	}
	var out [][]int
	seen := make(map[string]bool)
	for _, start := range group {
		c, _ := e.game.Board.Coord(e.g.ids[start])
		for _, d := range dirVec {
			line := []int{start}
			for step := 1; ; step++ {
				if nid, ok := e.game.Board.At(board.Coord{X: c.X + d[0]*step, Y: c.Y + d[1]*step}); ok {
					j := e.g.index[nid]
					if set[j] {
						line = append(line, j)
						continue
					}
				}
				break
			}
			for step := 1; ; step++ {
				if nid, ok := e.game.Board.At(board.Coord{X: c.X - d[0]*step, Y: c.Y - d[1]*step}); ok {
					j := e.g.index[nid]
					if set[j] {
						line = append([]int{j}, line...)
						continue
					}
				}
				break
			}
			sort.Ints(line)
			if len(line) < 2 {
				continue
			}
			key := fmt.Sprintf("%d|%d|%d", d[0], d[1], line[0])
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, append([]int(nil), line...))
		}
	}
	if len(group) == 1 {
		out = append(out, append([]int(nil), group...))
	}
	return out
}

// --- legal moves -----------------------------------------------------------

func (e *eng) legal() []state.Move {
	d := e.d
	if d.Turn == 0 {
		return nil
	}
	if d.Confluence == nil {
		return e.legalPlacements()
	}
	return e.legalConfluenceMoves()
}

func (e *eng) legalPlacements() []state.Move {
	color := seatColors[e.d.Turn]
	var out []state.Move
	for i, c := range e.cells {
		if c == "" {
			out = append(out, state.Move{Action: actPlace, Piece: color, To: e.g.ids[i]})
		}
	}
	return out
}

func (e *eng) legalConfluenceMoves() []state.Move {
	orig := e.d
	e.d = orig.clone()
	defer func() { e.d = orig }()
	e.resolveConfluenceAdvances()
	cf := e.d.Confluence
	if cf == nil || e.d.Turn == 0 {
		return nil
	}
	return e.movesForSeat(cf.NextSeat, cf)
}

// resolveConfluenceAdvances advances the confluence state in place until the
// next seat has a pending spread, the opponent does, or the confluence ends.
// It is used by Legal (on a clone) and by Apply (on the new state being built).
func (e *eng) resolveConfluenceAdvances() {
	for {
		cf := e.d.Confluence
		if cf == nil {
			return
		}
		if len(e.movesForSeat(cf.NextSeat, cf)) > 0 {
			e.d.Turn = cf.NextSeat
			return
		}
		opp := other(cf.NextSeat)
		if len(e.movesForSeat(opp, cf)) > 0 {
			cf.NextSeat = opp
			e.d.Turn = opp
			return
		}
		e.advanceConfluence()
	}
}

func (e *eng) movesForSeat(seat int, cf *Confluence) []state.Move {
	color := seatColors[seat]
	if color == "" {
		return nil
	}
	groups := e.clusters(color)
	var moves []state.Move
	for _, g := range groups {
		if len(g) != cf.Initiative {
			continue
		}
		key := clusterKey(cf.Phase, g)
		if cf.Spread != nil && cf.Spread[key] {
			continue
		}
		switch cf.Phase {
		case phaseSpore:
			if !e.isHyphae(g, color) {
				moves = append(moves, e.sporeMoves(g, seat)...)
			}
		case phaseHyphae:
			if e.isHyphae(g, color) {
				moves = append(moves, e.hyphaeMoves(g, seat)...)
			}
		}
	}
	return moves
}

// sporeMoves lists every valid spore spread from cluster group for seat.
func (e *eng) sporeMoves(group []int, seat int) []state.Move {
	color := seatColors[seat]
	var moves []state.Move
	for _, src := range group {
		c, _ := e.game.Board.Coord(e.g.ids[src])
		for _, d := range dirVec {
			for step := 1; ; step++ {
				nid, ok := e.game.Board.At(board.Coord{X: c.X + d[0]*step, Y: c.Y + d[1]*step})
				if !ok {
					break
				}
				j := e.g.index[nid]
				if e.cells[j] == "" {
					continue
				}
				if e.cells[j] != color {
					moves = append(moves, state.Move{Action: actSpore, From: e.g.ids[src], To: nid})
				}
				break
			}
		}
	}
	return moves
}

// hyphaeMoves lists every valid hyphae spread from cluster group for seat.
func (e *eng) hyphaeMoves(group []int, seat int) []state.Move {
	color := seatColors[seat]
	var moves []state.Move
	for _, strand := range e.strands(group) {
		if len(strand) == 1 {
			moves = append(moves, e.singleNodeHyphae(strand[0], color)...)
			continue
		}
		dir := e.strandDir(strand)
		if dir[0] == 0 && dir[1] == 0 {
			continue
		}
		// Targets extending past the far end of the strand.
		moves = append(moves, e.hyphaeLine(strand, dir, color)...)
		// Targets extending past the near end.
		rev := make([]int, len(strand))
		for i := range strand {
			rev[i] = strand[len(strand)-1-i]
		}
		oppDir := [2]int{-dir[0], -dir[1]}
		moves = append(moves, e.hyphaeLine(rev, oppDir, color)...)
	}
	return moves
}

// singleNodeHyphae lists hyphae spreads from a one-node strand.
func (e *eng) singleNodeHyphae(node int, color string) []state.Move {
	var moves []state.Move
	for _, d := range dirVec {
		line := []int{node}
		moves = append(moves, e.hyphaeLine(line, d, color)...)
	}
	return moves
}

// hyphaeLine lists hyphae spreads where the targets extend from the last cell
// of line in direction dir. The line must be a straight strand in direction dir.
func (e *eng) hyphaeLine(line []int, dir [2]int, color string) []state.Move {
	k := len(line)
	end := line[len(line)-1]
	endC, _ := e.game.Board.Coord(e.g.ids[end])
	var moves []state.Move
	var targets []board.PositionID
	for step := 1; step <= k; step++ {
		nid, ok := e.game.Board.At(board.Coord{X: endC.X + dir[0]*step, Y: endC.Y + dir[1]*step})
		if !ok {
			break
		}
		j := e.g.index[nid]
		if e.cells[j] == "" || e.cells[j] == color {
			break
		}
		targets = append(targets, nid)
		path := append([]board.PositionID(nil), targets...)
		moves = append(moves, state.Move{
			Action: actHyphae,
			From:   e.g.ids[line[0]],
			To:     e.g.ids[line[len(line)-1]],
			Path:   path,
		})
	}
	return moves
}

// strandDir returns the axial direction of strand (which must be a straight
// line with at least two cells).
func (e *eng) strandDir(strand []int) [2]int {
	c0, _ := e.game.Board.Coord(e.g.ids[strand[0]])
	c1, _ := e.game.Board.Coord(e.g.ids[strand[1]])
	dx, dy := c1.X-c0.X, c1.Y-c0.Y
	for _, d := range dirVec {
		if dx == d[0] && dy == d[1] {
			return d
		}
	}
	return [2]int{0, 0}
}

// --- confluence advancement ------------------------------------------------

// advanceConfluence moves to the next initiative or phase, or ends the
// confluence if nothing remains.
func (e *eng) advanceConfluence() {
	cf := e.d.Confluence
	cf.Initiative++
	cf.NextSeat = other(cf.Initiator)
	cf.Spread = map[string]bool{}
	maxSize := 0
	for _, g := range e.clusters(seatColors[1]) {
		if len(g) > maxSize {
			maxSize = len(g)
		}
	}
	for _, g := range e.clusters(seatColors[2]) {
		if len(g) > maxSize {
			maxSize = len(g)
		}
	}
	if cf.Initiative > maxSize {
		if cf.Phase == phaseSpore {
			cf.Phase = phaseHyphae
			cf.Initiative = 1
			cf.NextSeat = other(cf.Initiator)
			cf.Spread = map[string]bool{}
			e.d.Turn = cf.NextSeat
			return
		}
		e.endConfluence()
		return
	}
	e.d.Turn = cf.NextSeat
}

// endConfluence resolves the confluence: removes the centre node, checks for a
// winner, and returns to normal play.
func (e *eng) endConfluence() {
	cf := e.d.Confluence
	e.d.Confluence = nil
	initiator := cf.Initiator
	if e.cells[e.g.centerI] != "" {
		removed := e.cells[e.g.centerI]
		e.cells[e.g.centerI] = ""
		e.say("The centre is cleared; the %s node is removed.", removed)
	}
	count := [2]int{}
	for _, c := range e.cells {
		if c == seatColors[1] {
			count[0]++
		}
		if c == seatColors[2] {
			count[1]++
		}
	}
	switch {
	case count[0] == 0 && count[1] == 0:
		e.finish(other(initiator), "confluence")
		e.say("Both colonies are wiped out; the player who did not initiate the confluence wins.")
	case count[0] == 0:
		e.finish(2, "confluence")
		e.say("Black has no nodes left; Green wins.")
	case count[1] == 0:
		e.finish(1, "confluence")
		e.say("Green has no nodes left; Black wins.")
	default:
		e.d.Turn = other(initiator)
		e.say("The confluence ends. %s resumes normal play.", seatLabel(e.d.Turn))
	}
}

// --- ruleset interface -----------------------------------------------------

// Start returns the empty board; Black (player 1) moves first.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	st := state.NewBoardState()
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
		return fmt.Errorf("mycelium: bad turn %d", d.Turn)
	}
	if cf := d.Confluence; cf != nil {
		if cf.Initiator != 1 && cf.Initiator != 2 {
			return fmt.Errorf("mycelium: bad confluence initiator %d", cf.Initiator)
		}
		if cf.Phase != phaseSpore && cf.Phase != phaseHyphae {
			return fmt.Errorf("mycelium: bad confluence phase %q", cf.Phase)
		}
		if cf.NextSeat != 1 && cf.NextSeat != 2 {
			return fmt.Errorf("mycelium: bad confluence next seat %d", cf.NextSeat)
		}
		if cf.Initiative < 1 {
			return fmt.Errorf("mycelium: bad confluence initiative %d", cf.Initiative)
		}
	}
	for _, id := range st.Occupied() {
		occ, _ := st.Get(id)
		if ownerOf(occ.Color) == 0 || occ.Piece != occ.Color {
			return fmt.Errorf("mycelium: bad node %s/%s at %s", occ.Color, occ.Piece, id)
		}
		if st.Height(id) > 1 {
			return fmt.Errorf("mycelium: two nodes on %s", id)
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
	e := newEng(game, st, d)
	nd := d.clone()
	e.d = nd
	e.ev = nil
	ns := st.Clone()
	// The confluence auto-advances through initiative orders that have no
	// pending spreads; resolve those advances on the new state before checking
	// the requested move.
	e.resolveConfluenceAdvances()
	if nd.Turn == 0 {
		return nil, fmt.Errorf("mycelium: the game is over")
	}
	if seat != nd.Turn {
		return nil, fmt.Errorf("mycelium: it is %s's turn", seatLabel(nd.Turn))
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
		return nil, fmt.Errorf("mycelium: %q is not a legal move now", want)
	}
	// legal() works on a clone; put e back on nd for execution.
	e.d = nd
	e.ev = nil
	e.execute(legal, seat)
	// A newly triggered or advanced confluence may have no pending spreads and
	// should resolve immediately (including ending the game).
	e.resolveConfluenceAdvances()
	nd.Events = e.ev
	ns.SetExt(nd)
	e.save(ns)
	ns.SetPly(st.Ply() + 1)
	if nd.Turn != 0 && nd.Confluence == nil && ns.Ply() >= game.ParamInt("max_plies", 500) {
		e.finish(0, "ply-limit")
		e.say("The move limit is reached; the game is drawn.")
		nd.Events = e.ev
		ns.SetExt(nd)
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

func (e *eng) execute(mv state.Move, seat int) {
	d := e.d
	switch mv.Action {
	case actPlace:
		i := e.g.index[mv.To]
		e.cells[i] = mv.Piece
		d.Last = string(mv.To)
		e.say("%s places a node on %s.", seatLabel(seat), mv.To)
		if i == e.g.centerI {
			d.Confluence = &Confluence{
				Initiator:  seat,
				Phase:      phaseSpore,
				Initiative: 1,
				NextSeat:   other(seat),
				Spread:     map[string]bool{},
			}
			d.Turn = other(seat)
			e.say("A confluence begins in the centre!")
		} else {
			d.Turn = other(seat)
		}
	case actSpore:
		e.applySpore(mv, seat)
		if e.d.Confluence != nil {
			e.d.Confluence.NextSeat = other(seat)
			e.d.Turn = other(seat)
		}
	case actHyphae:
		e.applyHyphae(mv, seat)
		if e.d.Confluence != nil {
			e.d.Confluence.NextSeat = other(seat)
			e.d.Turn = other(seat)
		}
	}
}

func (e *eng) applySpore(mv state.Move, seat int) {
	cf := e.d.Confluence
	src := e.g.index[mv.From]
	color := e.cells[src]
	if cf != nil {
		for _, g := range e.clusters(color) {
			if contains(g, src) && len(g) == cf.Initiative && cf.Phase == phaseSpore && !e.isHyphae(g, color) {
				cf.Spread[clusterKey(phaseSpore, g)] = true
				break
			}
		}
	}
	dst := e.g.index[mv.To]
	e.cells[src] = ""
	e.cells[dst] = color
	e.say("%s's spore at %s spreads to %s.", seatLabel(seat), mv.From, mv.To)
	if cf != nil {
		e.resolveConfluenceAdvances()
	}
}

func (e *eng) applyHyphae(mv state.Move, seat int) {
	cf := e.d.Confluence
	color := seatColors[seat]
	if cf != nil {
		strandStart := e.g.index[mv.From]
		for _, g := range e.clusters(color) {
			if contains(g, strandStart) && len(g) == cf.Initiative && cf.Phase == phaseHyphae && e.isHyphae(g, color) {
				cf.Spread[clusterKey(phaseHyphae, g)] = true
				break
			}
		}
	}
	for _, t := range mv.Path {
		e.cells[e.g.index[t]] = color
	}
	e.say("%s's hyphae strand at %s spreads, converting %d node(s).", seatLabel(seat), mv.From, len(mv.Path))
	if cf != nil {
		e.resolveConfluenceAdvances()
	}
}

func contains(group []int, x int) bool {
	for _, v := range group {
		if v == x {
			return true
		}
	}
	return false
}
