package state

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/primitives"
)

// --- step ---------------------------------------------------------------

// validateStep implements the implicit "step" action: the top piece at
// From moves to To under its piece type's movement primitive. This is the
// original v1 move, kept behaviourally identical for single-piece squares.
func (c *rctx) validateStep(act catalog.ResolvedAction, mv Move) (executor, error) {
	if mv.Piece != "" || mv.Count != 0 || len(mv.Path) > 0 || len(mv.Drops) > 0 {
		return nil, fmt.Errorf("state: a step move takes only from and to")
	}
	// Occupant at from.
	occupant, ok := c.st.Get(mv.From)
	if !ok {
		return nil, fmt.Errorf("state: no piece at %s", mv.From)
	}

	// Occupant's color must match the seat's color.
	if occupant.Color != c.color {
		return nil, fmt.Errorf("state: piece at %s belongs to color %q, not seat %d's color %q", mv.From, occupant.Color, c.seat, c.color)
	}

	// Resolve the piece's movement rule.
	resolved, ok := c.game.Pieces[occupant.Piece]
	if !ok {
		return nil, fmt.Errorf("state: unknown piece type %q at %s", occupant.Piece, mv.From)
	}

	var prim primitives.MovementPrimitive
	var occupancy primitives.Occupancy
	if act.Explicit {
		listed := false
		for _, p := range act.Pieces {
			if p == occupant.Piece {
				listed = true
			}
		}
		if !listed {
			return nil, fmt.Errorf("state: action %q cannot move a %s", act.ID, pieceLabel(c.game, occupant.Piece))
		}
		if !c.stepDirAllowed(act.StepDirs[c.seat], act.StepDirs != nil && act.StepDirs[c.seat] != nil, mv.From, mv.To) {
			return nil, fmt.Errorf("state: a %s cannot move from %s to %s that way (action %q)", pieceLabel(c.game, occupant.Piece), mv.From, mv.To, act.ID)
		}
		prim, _ = primitives.Get(catalog.ActionStep)
		occupancy = act.StepOccupancy
	} else {
		if resolved.Primitive == nil {
			return nil, fmt.Errorf("state: piece %q at %s has no step movement", occupant.Piece, mv.From)
		}
		if !c.dirAllowed(resolved, mv.From, mv.To) {
			return nil, fmt.Errorf("state: a %s cannot move from %s to %s (wrong direction)", pieceLabel(c.game, occupant.Piece), mv.From, mv.To)
		}
		prim, occupancy = resolved.Primitive, resolved.Occupancy
	}

	result := prim(c.game.Board, c.st, occupancy, mv.From, mv.To, occupant.Color)
	if !result.Allowed {
		return nil, fmt.Errorf("state: move rejected: %s", result.Reason)
	}

	from, to := mv.From, mv.To
	return func(ns *BoardState) []board.PositionID {
		// Lift the top piece off its square; whatever is beneath stays.
		stack := ns.Stack(from)
		if len(stack) <= 1 {
			ns.Remove(from)
		} else {
			ns.SetStack(from, stack[:len(stack)-1])
		}
		if result.Captured != nil {
			ns.Remove(*result.Captured)
		}
		ns.Push(to, Occupant{Piece: occupant.Piece, Color: occupant.Color})
		c.applyPromotion(ns, to)
		if result.Captured != nil {
			return []board.PositionID{*result.Captured}
		}
		return nil
	}, nil
}

// stepDirAllowed reports whether the step from `from` to `to` follows one of
// dirs (when restricted is false, any step is allowed).
func (c *rctx) stepDirAllowed(dirs []board.Direction, restricted bool, from, to board.PositionID) bool {
	if !restricted {
		return true
	}
	fc, ok1 := c.game.Board.Coord(from)
	tc, ok2 := c.game.Board.Coord(to)
	if !ok1 || !ok2 {
		return true
	}
	dx, dy := tc.X-fc.X, tc.Y-fc.Y
	for _, d := range dirs {
		if d.DX == dx && d.DY == dy {
			return true
		}
	}
	return false
}

// dirAllowed reports whether rp may travel from `from` to the adjacent
// square `to` for the moving seat, per its direction restrictions.
func (c *rctx) dirAllowed(rp catalog.ResolvedPiece, from, to board.PositionID) bool {
	dirs, restricted := rp.Dirs[c.seat]
	if !restricted {
		return true
	}
	fc, ok1 := c.game.Board.Coord(from)
	tc, ok2 := c.game.Board.Coord(to)
	if !ok1 || !ok2 {
		return true
	}
	dx, dy := tc.X-fc.X, tc.Y-fc.Y
	for _, d := range dirs {
		if d.DX == dx && d.DY == dy {
			return true
		}
	}
	return false
}

// applyPromotion turns the top piece at pos into its promoted type when it
// has just arrived in a promotion region.
func (c *rctx) applyPromotion(ns *BoardState, pos board.PositionID) {
	stack := ns.cells[pos]
	if len(stack) == 0 {
		return
	}
	top := &stack[len(stack)-1]
	rp, ok := c.game.Pieces[top.Piece]
	if !ok {
		return
	}
	for _, pr := range rp.Promotions {
		if (pr.Seat == 0 || pr.Seat == c.seat) && pr.Region[pos] {
			top.Piece = pr.To
			return
		}
	}
}

// promotesAt reports whether a piece of rp arriving at pos would promote.
func (c *rctx) promotesAt(rp catalog.ResolvedPiece, pos board.PositionID) bool {
	for _, pr := range rp.Promotions {
		if (pr.Seat == 0 || pr.Seat == c.seat) && pr.Region[pos] {
			return true
		}
	}
	return false
}

func (c *rctx) enumerateSteps(act catalog.ResolvedAction, keep func(catalog.ResolvedAction, Move, bool)) {
	for _, from := range c.game.Board.Positions() {
		if c.enough() {
			return
		}
		occ, ok := c.st.Get(from)
		if !ok || occ.Color != c.color {
			continue
		}
		rp, known := c.game.Pieces[occ.Piece]
		if !known {
			continue
		}
		if act.Explicit {
			listed := false
			for _, p := range act.Pieces {
				if p == occ.Piece {
					listed = true
				}
			}
			if !listed {
				continue
			}
		} else if rp.Primitive == nil {
			continue
		}
		for _, to := range c.game.Board.Neighbors(from) {
			if act.Explicit {
				dirs := act.StepDirs[c.seat]
				if !c.stepDirAllowed(dirs, act.StepDirs != nil && dirs != nil, from, to) {
					continue
				}
			} else if !c.dirAllowed(rp, from, to) {
				continue
			}
			keep(act, Move{Action: act.ID, From: from, To: to}, false)
		}
	}
}

// --- place --------------------------------------------------------------

// validatePlace implements the "place" action: put a piece on an empty
// square, taking it from the placing color's reserve (or the unlimited
// supply).
func (c *rctx) validatePlace(act catalog.ResolvedAction, mv Move) (executor, error) {
	if mv.Piece == "" {
		return nil, fmt.Errorf("state: placement needs a piece type")
	}
	allowed := false
	for _, p := range act.Pieces {
		if p == mv.Piece {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("state: action %q cannot place piece %q", act.ID, mv.Piece)
	}
	if mv.From != "" || mv.Count != 0 || len(mv.Path) > 0 || len(mv.Drops) > 0 {
		return nil, fmt.Errorf("state: a placement takes only a piece and a destination square")
	}
	if err := c.placementSquareOK(act, mv.To); err != nil {
		return nil, err
	}

	owner := c.placedColor()
	pool := c.game.Pool(mv.Piece)
	if act.Source == catalog.SourceReserve && c.st.Reserve(owner, pool) <= 0 {
		return nil, fmt.Errorf("state: no %s left in the %s reserve", pieceLabel(c.game, mv.Piece), owner)
	}

	var captured []board.PositionID
	if act.Captures {
		var err error
		captured, err = c.libertyOutcome(act, mv.To, owner, mv.Piece)
		if err != nil {
			return nil, err
		}
	}

	var flips []board.PositionID
	if act.Flips {
		flips = c.flipsFor(mv.To, owner)
		if act.FlipsRequired && len(flips) == 0 {
			return nil, fmt.Errorf("state: a piece placed at %s would flip nothing; a placement must bracket and flip at least one opposing piece", mv.To)
		}
	}

	var custodian []board.PositionID
	if len(act.CustodianLengths) > 0 {
		custodian = c.custodianCaptures(act, mv.To, owner)
	}

	piece, to, source := mv.Piece, mv.To, act.Source
	return func(ns *BoardState) []board.PositionID {
		if source == catalog.SourceReserve {
			ns.SetReserve(owner, pool, ns.Reserve(owner, pool)-1)
		}
		ns.Set(to, Occupant{Piece: piece, Color: owner})
		for _, f := range flips {
			ns.SetOccupant(f, owner)
		}
		for _, p := range captured {
			ns.Remove(p)
		}
		if len(custodian) > 0 {
			for _, p := range custodian {
				ns.Remove(p)
			}
			if act.CountPool != "" {
				ns.SetReserve(owner, act.CountPool, ns.Reserve(owner, act.CountPool)+len(custodian))
			}
			captured = append(append([]board.PositionID(nil), captured...), custodian...)
		}
		return captured
	}, nil
}

// custodianCaptures returns the opposing pieces a piece of color placed at
// `to` captures by custodian: along each board direction, a run of opposing
// pieces whose length is one of the action's lengths, closed off by a piece
// of the placer's own colour.
func (c *rctx) custodianCaptures(act catalog.ResolvedAction, to board.PositionID, color string) []board.PositionID {
	g := c.game.Board
	tc, ok := g.Coord(to)
	if !ok {
		return nil
	}
	wanted := map[int]bool{}
	for _, l := range act.CustodianLengths {
		wanted[l] = true
	}
	var out []board.PositionID
	for _, d := range c.game.Directions {
		var run []board.PositionID
		prev := to
		closed := false
		for n := 1; n <= 9; n++ {
			p, ok := g.At(board.Coord{X: tc.X + n*d.DX, Y: tc.Y + n*d.DY})
			if !ok || !g.HasEdge(prev, p) {
				break
			}
			top, occupied := c.st.Get(p)
			if !occupied {
				// An empty anvil square (corner, throne) closes the flank.
				closed = len(run) > 0 && act.Anvils[p]
				break
			}
			if top.Color == color {
				closed = len(run) > 0
				break
			}
			run = append(run, p)
			prev = p
		}
		if closed && wanted[len(run)] {
			out = append(out, run...)
		}
	}
	return out
}

// placementSquareOK checks the square-level preconditions of a placement:
// the square exists, is empty, lies in the action's region and (with
// gravity) is supported.
func (c *rctx) placementSquareOK(act catalog.ResolvedAction, to board.PositionID) error {
	if !c.game.Board.HasPosition(to) {
		return fmt.Errorf("state: unknown position %q", to)
	}
	if _, occupied := c.st.Get(to); occupied {
		return fmt.Errorf("state: %s is already occupied", to)
	}
	if act.Region != nil && !act.Region[to] {
		return fmt.Errorf("state: %s is outside the area where pieces may be placed", to)
	}
	if act.HasGravity {
		below, onBoard := c.game.Board.Offset(to, act.GravityDX, act.GravityDY)
		if onBoard {
			if _, supported := c.st.Get(below); !supported {
				return fmt.Errorf("state: a piece placed at %s would fall: %s is empty", to, below)
			}
		}
	}
	return nil
}

// libertyOutcome evaluates a Go-style placement of a stone of color at `to`:
// it returns the positions of every stone removed (opposing groups left
// without liberties, or — when suicide is allowed — the placer's own group)
// and rejects suicide and ko.
func (c *rctx) libertyOutcome(act catalog.ResolvedAction, to board.PositionID, color, piece string) ([]board.PositionID, error) {
	g := c.game.Board
	removed := map[board.PositionID]bool{}

	colorAt := func(p board.PositionID) string {
		if p == to {
			return color
		}
		if removed[p] {
			return ""
		}
		top, ok := c.st.Get(p)
		if !ok {
			return ""
		}
		return top.Color
	}
	// group flood-fills the connected stones of col from start; it reports
	// the members and whether any member touches an empty square.
	group := func(start board.PositionID, col string) ([]board.PositionID, bool) {
		members := []board.PositionID{start}
		seen := map[board.PositionID]bool{start: true}
		liberty := false
		for i := 0; i < len(members); i++ {
			for _, n := range g.Neighbors(members[i]) {
				switch cn := colorAt(n); {
				case cn == "":
					liberty = true
				case cn == col && !seen[n]:
					seen[n] = true
					members = append(members, n)
				}
			}
		}
		return members, liberty
	}

	// Opposing groups touching the new stone that have no liberties left.
	var captured []board.PositionID
	for _, n := range g.Neighbors(to) {
		cn := colorAt(n)
		if cn == "" || cn == color || removed[n] {
			continue
		}
		members, liberty := group(n, cn)
		if liberty {
			continue
		}
		for _, m := range members {
			if !removed[m] {
				removed[m] = true
				captured = append(captured, m)
			}
		}
	}

	// The placed stone's own group, after the captures are lifted.
	if own, liberty := group(to, color); !liberty {
		if !act.AllowSuicide {
			return nil, fmt.Errorf("state: playing at %s would be suicide (the group would have no liberties)", to)
		}
		for _, m := range own {
			if !removed[m] {
				removed[m] = true
				captured = append(captured, m)
			}
		}
	}

	if act.Ko && c.st.prev != "" {
		after := hashCells(c.game, func(p board.PositionID) []Occupant {
			if removed[p] {
				return nil
			}
			if p == to {
				return []Occupant{{Piece: piece, Color: color}}
			}
			return c.st.cells[p]
		})
		if after == c.st.prev {
			return nil, fmt.Errorf("state: playing at %s would recreate the previous position (ko)", to)
		}
	}
	return captured, nil
}

// flipsFor returns the squares that would be flipped by placing a piece of
// color at to: in every board direction, a run of one or more opposing
// pieces directly followed by a piece of color.
func (c *rctx) flipsFor(to board.PositionID, color string) []board.PositionID {
	g := c.game.Board
	tc, ok := g.Coord(to)
	if !ok {
		return nil
	}
	var out []board.PositionID
	for _, d := range c.game.Directions {
		var run []board.PositionID
		for i := 1; ; i++ {
			p, ok := g.At(board.Coord{X: tc.X + i*d.DX, Y: tc.Y + i*d.DY})
			if !ok {
				run = nil
				break
			}
			top, occupied := c.st.Get(p)
			if !occupied {
				run = nil
				break
			}
			if top.Color == color {
				break
			}
			run = append(run, p)
		}
		out = append(out, run...)
	}
	return out
}

func (c *rctx) enumeratePlacements(act catalog.ResolvedAction, keep func(catalog.ResolvedAction, Move, bool)) {
	positions := c.game.Board.Positions()
	// Without captures, flips, ko or custodian rules a placement is legal
	// exactly when the square-level preconditions hold, so the full
	// validation (and its executor closure) can be skipped.
	simple := !act.Captures && !act.Flips && !act.Ko && len(act.CustodianLengths) == 0 && !c.mustJump()
	for _, piece := range act.Pieces {
		// Cheap necessary conditions first; validatePlace remains the
		// authority for everything else.
		if act.Source == catalog.SourceReserve && c.st.Reserve(c.placedColor(), c.game.Pool(piece)) <= 0 {
			continue
		}
		for _, to := range positions {
			if c.enough() {
				return
			}
			if len(c.st.cells[to]) > 0 {
				continue
			}
			if simple {
				if c.placementSquareOK(act, to) != nil {
					continue
				}
				keep(act, Move{Action: act.ID, Piece: piece, To: to}, true)
				continue
			}
			keep(act, Move{Action: act.ID, Piece: piece, To: to}, false)
		}
	}
}

// --- move-stack ---------------------------------------------------------

// validateStack implements the "move-stack" action (Tak-style): pick up the
// top Count pieces of a stack the mover controls, carry them in a straight
// line over len(Path) squares, dropping Drops[i] pieces (bottom of the
// carried group first) on Path[i]. Squares topped by uncoverable pieces
// block the move, except that a flattener may flatten a flattenable piece
// under the configured conditions.
func (c *rctx) validateStack(act catalog.ResolvedAction, mv Move) (executor, error) {
	g := c.game.Board

	if mv.Piece != "" {
		return nil, fmt.Errorf("state: a stack move does not name a piece")
	}
	if !g.HasPosition(mv.From) {
		return nil, fmt.Errorf("state: unknown position %q", mv.From)
	}
	stack := c.st.cells[mv.From]
	if len(stack) == 0 {
		return nil, fmt.Errorf("state: no piece at %s", mv.From)
	}
	top := stack[len(stack)-1]
	if top.Color != c.color {
		return nil, fmt.Errorf("state: the stack at %s is controlled by %q, not seat %d (%q)", mv.From, top.Color, c.seat, c.color)
	}

	if mv.Count < 1 {
		return nil, fmt.Errorf("state: a stack move must pick up at least one piece")
	}
	if mv.Count > len(stack) {
		return nil, fmt.Errorf("state: the stack at %s has only %d piece(s); cannot pick up %d", mv.From, len(stack), mv.Count)
	}
	if mv.Count > act.CarryLimit {
		return nil, fmt.Errorf("state: cannot carry more than %d pieces (tried %d)", act.CarryLimit, mv.Count)
	}
	if len(mv.Path) == 0 {
		return nil, fmt.Errorf("state: a stack move needs at least one destination square")
	}
	if len(mv.Drops) != len(mv.Path) {
		return nil, fmt.Errorf("state: drops must give one count per destination square (%d squares, %d drops)", len(mv.Path), len(mv.Drops))
	}
	sum := 0
	for i, d := range mv.Drops {
		if d < 1 {
			return nil, fmt.Errorf("state: at least one piece must be dropped on every square (square %d drops %d)", i+1, d)
		}
		sum += d
	}
	if sum != mv.Count {
		return nil, fmt.Errorf("state: drops add up to %d but %d piece(s) were picked up; every carried piece must be dropped", sum, mv.Count)
	}

	// The path must run straight away from the origin, one square at a
	// time, along a board direction.
	fromC, ok := g.Coord(mv.From)
	if !ok {
		return nil, fmt.Errorf("state: board has no coordinates for %s", mv.From)
	}
	firstC, ok := g.Coord(mv.Path[0])
	if !ok {
		return nil, fmt.Errorf("state: unknown position %q", mv.Path[0])
	}
	dx, dy := firstC.X-fromC.X, firstC.Y-fromC.Y
	if !g.HasEdge(mv.From, mv.Path[0]) || !isDirection(c.game.Directions, dx, dy) {
		return nil, fmt.Errorf("state: %s is not adjacent to %s", mv.Path[0], mv.From)
	}
	for i, p := range mv.Path {
		want, ok := g.At(board.Coord{X: fromC.X + (i+1)*dx, Y: fromC.Y + (i+1)*dy})
		if !ok {
			return nil, fmt.Errorf("state: the move runs off the board")
		}
		if want != p {
			return nil, fmt.Errorf("state: stack moves must travel in a straight line (expected %s, got %s)", want, p)
		}
	}

	// Walk the squares: every one must be enterable.
	carried := stack[len(stack)-mv.Count:]
	groups := make([][]Occupant, len(mv.Path))
	flattened := make([]bool, len(mv.Path))
	offset := 0
	for i, p := range mv.Path {
		group := carried[offset : offset+mv.Drops[i]]
		offset += mv.Drops[i]
		groups[i] = group

		destTop, occupied := c.st.Get(p)
		if !occupied {
			continue
		}
		rp, known := c.game.Pieces[destTop.Piece]
		if known && rp.Coverable {
			continue
		}
		// Uncoverable: only a permitted flattening may enter.
		def := c.game.Definition.Pieces[destTop.Piece]
		lander := group[0] // the piece that actually lands on the square
		canFlatten := false
		for _, by := range def.Stack.FlattenableBy {
			if by == lander.Piece {
				canFlatten = true
				break
			}
		}
		if canFlatten && act.Flatten.AloneOnly && len(group) != 1 {
			canFlatten = false
		}
		if canFlatten && act.Flatten.FinalSquareOnly && i != len(mv.Path)-1 {
			canFlatten = false
		}
		if !canFlatten {
			return nil, fmt.Errorf("state: cannot move onto %s: a %s there cannot be covered", p, pieceLabel(c.game, destTop.Piece))
		}
		flattened[i] = true
	}

	from, count, path := mv.From, mv.Count, clonePath(mv.Path)
	flattenTo := func(piece string) string { return c.game.Definition.Pieces[piece].Stack.FlattenTo }
	return func(ns *BoardState) []board.PositionID {
		remaining := ns.Stack(from)
		remaining = remaining[:len(remaining)-count]
		ns.SetStack(from, remaining)
		for i, p := range path {
			dest := ns.Stack(p)
			if flattened[i] && len(dest) > 0 {
				dest[len(dest)-1].Piece = flattenTo(dest[len(dest)-1].Piece)
			}
			dest = append(dest, groups[i]...)
			ns.SetStack(p, dest)
		}
		return nil
	}, nil
}

func isDirection(dirs []board.Direction, dx, dy int) bool {
	for _, d := range dirs {
		if d.DX == dx && d.DY == dy {
			return true
		}
	}
	return false
}

func pieceLabel(game *catalog.CompiledGame, piece string) string {
	if l := game.Definition.Pieces[piece].Label; l != "" {
		return l
	}
	return piece
}

// enumerateStackMoves generates every legal stack move for the mover and
// hands each to keep.
//
// Candidates are built along each direction only as far as a move can
// continue: every square except the last must be enterable by an ordinary
// drop, so any composition of the carried pieces over such a path is legal
// by construction and is passed on as well-formed. Only when the last square
// holds an uncoverable piece does legality depend on the drops (the
// flattener must land on it, alone if required), and those candidates go
// through the shared validateStack.
func (c *rctx) enumerateStackMoves(act catalog.ResolvedAction, keep func(catalog.ResolvedAction, Move, bool)) {
	g := c.game.Board
	var arena intArena
	// Which piece types can be flattened by anything (computed once: the
	// definition lookup copies a large struct).
	flattenableAny := map[string]bool{}
	for name, pd := range c.game.Definition.Pieces {
		flattenableAny[name] = len(pd.Stack.FlattenableBy) > 0
	}
	for _, from := range g.Positions() {
		if c.enough() {
			return
		}
		stack := c.st.cells[from]
		if len(stack) == 0 || stack[len(stack)-1].Color != c.color {
			continue
		}
		if _, ok := g.Coord(from); !ok {
			continue
		}
		maxCarry := len(stack)
		if maxCarry > act.CarryLimit {
			maxCarry = act.CarryLimit
		}
		for _, d := range c.game.Directions {
			// The squares available along this direction, up to the point
			// where a move can no longer continue: every square but the
			// last must be enterable by an ordinary drop, and the last may
			// additionally be a flattenable blocker.
			var line []board.PositionID
			for i, p := range g.Ray(from, d.DX, d.DY) {
				top, occupied := c.st.Get(p)
				enterable := true
				flattenable := false
				if occupied {
					rp, known := c.game.Pieces[top.Piece]
					if !known || !rp.Coverable {
						enterable = false
						flattenable = flattenableAny[top.Piece]
					}
				}
				if !enterable && !flattenable {
					break
				}
				line = append(line, p)
				if !enterable || i+1 >= maxCarry {
					break // a blocker can only ever be the final square
				}
			}
			if len(line) == 0 {
				continue
			}
			// The final square of a full-length line may be a blocker; then
			// only moves whose last drop is a permitted flattener survive.
			lastIsBlocker := false
			var flattenableBy []string
			if top, occupied := c.st.Get(line[len(line)-1]); occupied {
				if rp, known := c.game.Pieces[top.Piece]; !known || !rp.Coverable {
					lastIsBlocker = true
					flattenableBy = c.game.Definition.Pieces[top.Piece].Stack.FlattenableBy
				}
			}
			for count := 1; count <= maxCarry; count++ {
				for squares := 1; squares <= count && squares <= len(line); squares++ {
					wellFormed := !lastIsBlocker || squares != len(line)
					forEachComposition(count, squares, func(drops []int) {
						if !wellFormed {
							// Cheap necessary condition (validateStack stays
							// the authority): the piece landing on the
							// blocker must be allowed to flatten it, alone
							// if the action says so.
							lastDrop := drops[len(drops)-1]
							lander := stack[len(stack)-lastDrop].Piece
							ok := false
							for _, by := range flattenableBy {
								if by == lander {
									ok = true
									break
								}
							}
							if !ok || (act.Flatten.AloneOnly && lastDrop != 1) {
								return
							}
						}
						keep(act, Move{
							Action: act.ID,
							From:   from,
							To:     line[squares-1],
							Count:  count,
							// Paths of one line share a backing array; they
							// are never mutated, and the capacity limit stops
							// appends from clobbering a sibling.
							Path:  line[:squares:squares],
							Drops: arena.take(drops),
						}, wellFormed)
					})
				}
			}
		}
	}
}

// intArena hands out small immutable int slices carved from larger blocks,
// so enumerating thousands of stack moves does one allocation per ~hundred
// moves instead of one per move.
type intArena struct{ block []int }

func (a *intArena) take(src []int) []int {
	n := len(src)
	if len(a.block) < n {
		size := 512
		if n > size {
			size = n
		}
		a.block = make([]int, size)
	}
	out := a.block[:n:n]
	a.block = a.block[n:]
	copy(out, src)
	return out
}

// forEachComposition calls fn with every way to write total as an ordered
// sum of exactly parts positive integers. The slice passed to fn is reused.
func forEachComposition(total, parts int, fn func([]int)) {
	buf := make([]int, parts)
	var rec func(i, left int)
	rec = func(i, left int) {
		if i == parts-1 {
			buf[i] = left
			fn(buf)
			return
		}
		// Leave at least one for each remaining part.
		for v := 1; v <= left-(parts-1-i); v++ {
			buf[i] = v
			rec(i+1, left-v)
		}
	}
	if parts < 1 || total < parts {
		return
	}
	rec(0, total)
}
