package state

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/callback"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// InitialState builds the starting position of game: its setup pieces
// (several entries on one position build a stack, first entry at the
// bottom), each seat's reserve, and a zero ply counter.
func InitialState(game *catalog.CompiledGame) *BoardState {
	if rs, ok := RulesetOf(game); ok {
		st, err := rs.Start(game, randomSeed())
		if err != nil {
			// A ruleset's start cannot fail for a registered game; an
			// empty position keeps callers total.
			return NewBoardState()
		}
		return st
	}
	seed := uint64(0)
	if len(game.Deal) > 0 {
		seed = randomSeed()
	}
	return declaredStart(game, seed)
}

// declaredStart is a declared game's starting position, its cards dealt
// with seed (ADR 0119).
func declaredStart(game *catalog.CompiledGame, seed uint64) *BoardState {
	st := NewBoardState()
	for _, entry := range game.Definition.Setup {
		st.Push(board.PositionID(entry.Position), Occupant{Piece: entry.Piece, Color: entry.Color})
	}
	for _, seat := range game.Definition.Seats {
		for piece, n := range game.Definition.Reserves {
			st.SetReserve(seat.Color, piece, n)
		}
	}
	if len(game.Deal) > 0 {
		st.table = deal(game, seed)
	}
	return st
}

// MoveResult is the outcome of a successfully applied move.
type MoveResult struct {
	NewState *BoardState
	Captured *board.PositionID
	// Captures lists every captured position (a draughts-style jump
	// captures several); Captured is the first of them.
	Captures        []board.PositionID
	NextImpulseSeat int
	GameEnded       bool
	WinnerSeat      *int

	// Move is the normalized move that was applied.
	Move Selection
	// Notation is the move in the game's canonical notation.
	Notation string
	// EndReason says why the game ended (the winning rule's reason), or ""
	// while it is still in progress.
	EndReason string
	// EndDetail adds specifics, e.g. the final score of a scored game.
	EndDetail string
}

// rctx bundles what every action implementation needs about the move being
// considered: the game, the position before the move, and who is moving.
type rctx struct {
	game  *catalog.CompiledGame
	st    *BoardState
	seat  int
	color string
	// palette is the seat's declared colors, when it plays several (ADR
	// 0117); a piece in color or in one of them is the seat's.
	palette []string

	// mustJumpKnown/mustJumpVal cache whether a mandatory jump is pending.
	mustJumpKnown bool
	mustJumpVal   bool

	// limit, when positive, makes enumeration stop once that many legal
	// moves have been found (HasLegalMove only needs one); count is how
	// many have been.
	limit, count int
}

// enough reports whether enumeration may stop: the requested number of
// legal moves has been found.
func (c *rctx) enough() bool { return c.limit > 0 && c.count >= c.limit }

// executor applies an already-validated move to ns (a clone of the
// pre-move position) and reports the positions captured by it, if any.
type executor func(ns *BoardState) (captured []board.PositionID)

func newCtx(game *catalog.CompiledGame, st *BoardState, seat int) (*rctx, error) {
	seatDef, ok := game.Seat(seat)
	if !ok {
		return nil, fmt.Errorf("state: unknown seat %d", seat)
	}
	return &rctx{game: game, st: st, seat: seat, color: seatDef.Color, palette: seatDef.Colors}, nil
}

// mine reports whether a piece of color is the mover's: one of its seat's
// colours.
func (c *rctx) mine(color string) bool {
	return color == c.color || slices.Contains(c.palette, color)
}

// sameSide reports whether colours a and b are one seat's (or the same
// colour, for those no seat plays).
func (c *rctx) sameSide(a, b string) bool {
	if a == b {
		return true
	}
	sa, ok := c.game.SeatOfColor(a)
	sb, ok2 := c.game.SeatOfColor(b)
	return ok && ok2 && sa == sb
}

// seatView shows a movement primitive each piece in its seat's colour, so a
// seat's second colour is its own, not an opponent's (ADR 0117).
type seatView struct{ c *rctx }

func (v seatView) Occupant(id board.PositionID) (string, bool) {
	col, ok := v.c.st.Occupant(id)
	if !ok {
		return "", false
	}
	return v.c.game.SideColor(col), true
}

// mayPlace reports whether a placement from owner's reserve may be of piece:
// not when the piece type's own colour is another seat's (ADR 0117).
func (c *rctx) mayPlace(owner, piece string) bool {
	pc := c.game.Definition.Pieces[piece].Color
	if pc == "" || pc == owner {
		return true
	}
	if _, played := c.game.SeatOfColor(pc); !played {
		return true
	}
	return c.sameSide(pc, owner)
}

// placedIn is the colour a placement of piece takes when its reserve is
// owner's: the piece type's own colour when owner's seat plays it, and
// otherwise owner itself.
func (c *rctx) placedIn(owner, piece string) string {
	pc := c.game.Definition.Pieces[piece].Color
	if pc == "" || pc == owner {
		return owner
	}
	if seat, ok := c.game.SeatOfColor(owner); ok {
		if seat2, ok := c.game.SeatOfColor(pc); ok && seat2 == seat {
			return pc
		}
	}
	return owner
}

// inOpening reports whether the position is still within the opening phase.
func (c *rctx) inOpening() bool {
	op := c.game.Opening
	return op != nil && c.st.ply < op.Plies
}

// placedColor is the color a piece placed by the mover this ply takes: the
// mover's own, or the opponent's during an "opponent" opening.
func (c *rctx) placedColor() string {
	if c.inOpening() && c.game.Opening.Opponent {
		if opp, ok := c.game.Opponent(c.seat); ok {
			if def, ok := c.game.Seat(opp); ok {
				return def.Color
			}
		}
	}
	return c.color
}

// openingAllows reports why act (placing piece, if non-empty) is not
// allowed right now because of the opening phase, or nil.
func (c *rctx) openingAllows(act catalog.ResolvedAction, piece string) error {
	if !c.inOpening() {
		return nil
	}
	op := c.game.Opening
	if op.Actions != nil && !op.Actions[act.ID] {
		return fmt.Errorf("state: during the opening (first %d plies) only %s allowed", op.Plies, openingActionList(c.game))
	}
	if piece != "" && op.Pieces != nil && !op.Pieces[piece] {
		return fmt.Errorf("state: during the opening (first %d plies) %q may not be placed", op.Plies, piece)
	}
	return nil
}

func openingActionList(game *catalog.CompiledGame) string {
	var ids []string
	for _, a := range game.Actions {
		if game.Opening.Actions[a.ID] {
			ids = append(ids, fmt.Sprintf("%q", a.ID))
		}
	}
	switch len(ids) {
	case 0:
		return "no action is"
	case 1:
		return "the " + ids[0] + " action is"
	default:
		s := ""
		for i, id := range ids {
			if i > 0 {
				s += ", "
			}
			s += id
		}
		return "the actions " + s + " are"
	}
}

// validate checks mv (already normalized) against the rules and returns the
// executor that applies it.
func (c *rctx) validate(act catalog.ResolvedAction, mv Selection) (executor, error) {
	if act.Type != catalog.ActionJump && c.mustJump() {
		return nil, fmt.Errorf("state: a capture is available and must be taken")
	}
	switch act.Type {
	case catalog.ActionStep:
		return c.validateStep(act, mv)
	case catalog.ActionPlace:
		return c.validatePlace(act, mv)
	case catalog.ActionMoveStack:
		return c.validateStack(act, mv)
	case catalog.ActionJump:
		return c.validateJump(act, mv)
	case catalog.ActionSlide:
		return c.validateSlide(act, mv)
	case catalog.ActionLeap:
		return c.validateLeap(act, mv)
	case catalog.ActionSwap:
		if mv.Piece != "" || mv.From != "" || mv.To != "" || mv.Count != 0 || len(mv.Path) > 0 || len(mv.Drops) > 0 {
			return nil, fmt.Errorf("state: a swap takes no parameters")
		}
		if !act.SwapPlies[c.st.ply] {
			return nil, fmt.Errorf("state: swapping sides is not allowed at this point of the game")
		}
		// The pie rule: the players change seats, so the pieces stay exactly
		// where and what they are — the opening move now belongs to the player
		// who was second. The board does not change; the turn does not advance
		// either (see Apply): the mover's seat is to move again, held from now
		// on by the player who opened. The session layer exchanges the seats'
		// occupants when it records the move.
		return func(ns *BoardState) []board.PositionID { return nil }, nil
	case catalog.ActionCardMove:
		return c.validateCardMove(act, mv)
	case catalog.ActionPass:
		if mv.Piece != "" || mv.From != "" || mv.To != "" || mv.Count != 0 || len(mv.Path) > 0 || len(mv.Drops) > 0 {
			return nil, fmt.Errorf("state: a pass takes no parameters")
		}
		return func(*BoardState) []board.PositionID { return nil }, nil
	case catalog.ActionDone:
		if mv.Piece != "" || mv.From != "" || mv.To != "" || mv.Count != 0 || len(mv.Path) > 0 || len(mv.Drops) > 0 {
			return nil, fmt.Errorf("state: done takes no parameters")
		}
		if parts := turnParts(c.game, c.st); parts == nil || !canStop(parts, c.st) {
			return nil, fmt.Errorf("state: the turn cannot stop here: it needs more selections")
		}
		return func(*BoardState) []board.PositionID { return nil }, nil
	default:
		return nil, fmt.Errorf("state: unsupported action type %q", act.Type)
	}
}

func (c *rctx) moveContext(board callback.BoardView, mv Selection) callback.MoveContext {
	return callback.MoveContext{
		GameID: c.game.Definition.ID,
		Board:  board,
		Seat:   c.seat,
		Color:  c.color,
		From:   mv.From,
		To:     mv.To,
		Action: mv.Action,
		Piece:  mv.Piece,
		Count:  mv.Count,
		Path:   clonePath(mv.Path),
		Drops:  cloneInts(mv.Drops),
	}
}

// Apply validates and applies a single move by seat against game's compiled
// rules and st (the position before the move). On any rejection it returns
// a non-nil error and a nil *MoveResult, leaving st completely unmutated.
//
// The steps are: resolve the seat; normalize the move (inferring its
// action); enforce the opening phase; validate against the action's rules;
// run the game's ValidateMove callback against the pre-move position;
// execute on a clone; run OnMoveApplied; compute the next seat; evaluate
// the win condition.
func Apply(game *catalog.CompiledGame, st *BoardState, seat int, mv Selection) (*MoveResult, error) {
	if rs, ok := RulesetOf(game); ok {
		return rs.Apply(game, st, seat, mv)
	}
	c, err := newCtx(game, st, seat)
	if err != nil {
		return nil, err
	}

	mv, err = mv.Normalize(game)
	if err != nil {
		return nil, err
	}
	act, known := game.Action(mv.Action)
	if !known && mv.Action == catalog.ActionStep {
		// A bare from/to move: take the first explicit step action that
		// accepts it.
		var firstErr error
		for _, cand := range game.Actions {
			if (cand.Type != catalog.ActionStep || !cand.Explicit) && cand.Type != catalog.ActionSlide && cand.Type != catalog.ActionLeap {
				continue
			}
			trial := mv
			trial.Action = cand.ID
			if err := c.openingAllows(cand, ""); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if _, err := c.validate(cand, trial); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			act, mv, known = cand, trial, true
			break
		}
		if !known {
			return nil, firstErr
		}
	}

	if err := c.openingAllows(act, mv.Piece); err != nil {
		return nil, err
	}
	parts := turnParts(game, st)
	part, piece := 0, ""
	if parts != nil && act.Type != catalog.ActionDone {
		piece = pieceOf(st, mv)
		var ok bool
		if part, ok = partFor(parts, st, act, piece); !ok {
			return nil, fmt.Errorf("state: %q has no place in the turn at this point", act.ID)
		}
	}

	exec, err := c.validate(act, mv)
	if err != nil {
		return nil, err
	}

	ctx := c.moveContext(st, mv)
	if err := game.Callbacks.ValidateMove(ctx); err != nil {
		return nil, err
	}

	newState := st.Clone()
	captures := exec(newState)
	var captured *board.PositionID
	if len(captures) > 0 {
		first := captures[0]
		captured = &first
	}
	// A turn of several selections goes on until it ends (ADR 0115); the ply,
	// ko's position and passes count whole turns.
	endsTurn, start := true, ""
	if game.KeepsHistory {
		// ko looks at the position before the opponent's last turn.
		if st.turn != nil {
			start = st.turn.Start
		} else {
			start = positionHash(game, st)
		}
	}
	if parts != nil && act.Type != catalog.ActionDone && act.Type != catalog.ActionPass && act.Type != catalog.ActionSwap {
		prog := progress(st)
		prog.Start = start
		if part > prog.Part {
			prog.Part, prog.Made, prog.Pieces = part, 0, nil
		}
		prog.Made++
		if parts[part].DistinctPiece && piece != "" {
			prog.Pieces = append(slices.Clone(prog.Pieces), piece)
		}
		newState.turn = &prog
		endsTurn = (part == len(parts)-1 && prog.Made >= parts[part].Max) || !goesOn(game, newState, seat)
	}
	if endsTurn {
		newState.turn = nil
		newState.ply++
		if game.KeepsHistory {
			newState.prev = start
		}
		if act.Type == catalog.ActionPass {
			newState.passes = st.passes + 1
		} else {
			newState.passes = 0
		}
		// The effects at the end of a turn come before it is judged; a swap
		// changes nothing on the board (ADR 0116).
		if len(game.EndOfTurn) > 0 && act.Type != catalog.ActionSwap {
			if taken := endOfTurn(game, newState, seat); len(taken) > 0 {
				captures = append(captures, taken...)
				if captured == nil {
					first := taken[0]
					captured = &first
				}
			}
		}
	}

	postCtx := ctx
	postCtx.Board = newState
	if err := game.Callbacks.OnMoveApplied(postCtx); err != nil {
		return nil, err
	}

	nextSeat, err := nextImpulseSeat(game, seat)
	if err != nil {
		return nil, err
	}
	if !endsTurn {
		nextSeat = seat // the turn goes on, and the impulse with it
	} else if act.Type == catalog.ActionSwap {
		// The pie rule: the players change seats, so the turn stays with the
		// mover's seat — the player who opened holds it now and replies.
		nextSeat = seat
	}
	if endsTurn && game.Definition.TurnOrder.PassWhenStuck {
		nextSeat = skipStuckSeats(game, newState, nextSeat)
	}

	outcome, err := evaluateWin(game, newState, seat, nextSeat)
	if err != nil {
		return nil, err
	}

	return &MoveResult{
		NewState:        newState,
		Captured:        captured,
		Captures:        captures,
		NextImpulseSeat: nextSeat,
		GameEnded:       outcome.Ended,
		WinnerSeat:      outcome.Winner,
		Move:            mv,
		Notation:        FormatMove(game, mv),
		EndReason:       outcome.Reason,
		EndDetail:       outcome.Detail,
	}, nil
}

// positionHash is a canonical string of the board's contents (not reserves),
// used by the ko rule to recognise a recreated position.
func positionHash(game *catalog.CompiledGame, st *BoardState) string {
	return hashCells(game, func(id board.PositionID) []Occupant { return st.cells[id] })
}

func hashCells(game *catalog.CompiledGame, at func(board.PositionID) []Occupant) string {
	var b []byte
	for _, id := range game.Board.Positions() {
		for _, o := range at(id) {
			b = append(b, o.Color...)
			b = append(b, ':')
			b = append(b, o.Piece...)
			b = append(b, ',')
		}
		b = append(b, ';')
	}
	return string(b)
}

// LegalSelections enumerates every move seat may legally make in st, in a
// deterministic order (actions in declaration order; within an action,
// board reading order). Moves rejected by the game's ValidateMove callback
// are excluded. It returns nil for an unknown seat.
//
// Every returned move is accepted by Apply, and Apply accepts no move this
// function omits: both go through the same per-action validation.
func LegalSelections(game *catalog.CompiledGame, st *BoardState, seat int) []Selection {
	return legalSelections(game, st, seat, 0)
}

// legalSelections is LegalMoves that stops after limit moves when limit > 0.
func legalSelections(game *catalog.CompiledGame, st *BoardState, seat int, limit int) []Selection {
	if rs, ok := RulesetOf(game); ok {
		moves := rs.Legal(game, st, seat)
		if limit > 0 && len(moves) > limit {
			moves = moves[:limit]
		}
		return moves
	}
	c, err := newCtx(game, st, seat)
	if err != nil {
		return nil
	}
	c.limit = limit

	// Games without callbacks (the usual case) skip the per-move callback
	// context entirely.
	_, noCallbacks := game.Callbacks.(callback.NoopCallbacks)

	var out []Selection
	// keep appends mv if it is legal. wellFormed reports that the caller
	// built mv so that validation cannot fail (see enumerateStackMoves);
	// everything else is validated by the same code Apply uses.
	parts := turnParts(game, st)
	keep := func(act catalog.ResolvedAction, mv Selection, wellFormed bool) {
		if c.openingAllows(act, mv.Piece) != nil {
			return
		}
		if parts != nil && act.Type != catalog.ActionDone {
			if _, ok := partFor(parts, st, act, pieceOf(st, mv)); !ok {
				return
			}
		}
		if !wellFormed {
			if _, err := c.validate(act, mv); err != nil {
				return
			}
		}
		if !noCallbacks && game.Callbacks.ValidateMove(c.moveContext(st, mv)) != nil {
			return
		}
		out = append(out, mv)
		c.count++
	}

	for _, act := range game.Actions {
		if c.enough() {
			break
		}
		switch act.Type {
		case catalog.ActionStep:
			c.enumerateSteps(act, keep)
		case catalog.ActionPlace:
			c.enumeratePlacements(act, keep)
		case catalog.ActionMoveStack:
			c.enumerateStackMoves(act, keep)
		case catalog.ActionJump:
			c.enumerateJumps(act, keep)
		case catalog.ActionSlide:
			c.enumerateSlides(act, keep)
		case catalog.ActionLeap:
			c.enumerateLeaps(act, keep)
		case catalog.ActionCardMove:
			c.enumerateCardMoves(act, keep)
		case catalog.ActionPass:
			keep(act, Selection{Action: act.ID}, true)
		case catalog.ActionSwap:
			if act.SwapPlies[c.st.ply] {
				keep(act, Selection{Action: act.ID}, false)
			}
		case catalog.ActionDone:
			if parts != nil && canStop(parts, st) {
				keep(act, Selection{Action: act.ID}, true)
			}
		}
	}
	return out
}

// HasLegalMove reports whether seat has at least one legal move in st.
func HasLegalMove(game *catalog.CompiledGame, st *BoardState, seat int) bool {
	return len(legalSelections(game, st, seat, 1)) > 0
}

// ApplyMove validates and applies a single legacy step move by seat, from
// `from` to `to`; it is Apply with a Move{From, To}. See docs/history/plan.md
// section 6.4a for the original 12-step algorithm.
func ApplyMove(game *catalog.CompiledGame, st *BoardState, seat int, from, to board.PositionID) (*MoveResult, error) {
	return Apply(game, st, seat, Selection{Action: catalog.ActionStep, From: from, To: to})
}

// skipStuckSeats starting from `candidate` returns the first seat, in turn
// order, that has a legal move; if nobody has one it returns candidate.
func skipStuckSeats(game *catalog.CompiledGame, st *BoardState, candidate int) int {
	seat := candidate
	for i := 0; i < len(game.Definition.TurnOrder.Seats); i++ {
		if HasLegalMove(game, st, seat) {
			return seat
		}
		next, err := nextImpulseSeat(game, seat)
		if err != nil {
			return candidate
		}
		seat = next
	}
	return candidate
}

// ImpulseAfter returns the seat to move in st when seat has just moved: the next
// seat in turn order, skipping seats with no legal move when the game passes
// stuck seats. It mirrors what Apply puts in MoveResult.NextImpulseSeat.
func ImpulseAfter(game *catalog.CompiledGame, st *BoardState, seat int) (int, error) {
	if rs, ok := RulesetOf(game); ok {
		if next := rs.ImpulseSeat(game, st); next != 0 {
			return next, nil
		}
		return seat, nil
	}
	if st.turn != nil {
		return seat, nil // a turn of several selections is still going
	}
	next, err := nextImpulseSeat(game, seat)
	if err != nil {
		return 0, err
	}
	if game.Definition.TurnOrder.PassWhenStuck {
		next = skipStuckSeats(game, st, next)
	}
	return next, nil
}

// FirstSeat returns the seat that moves first.
func FirstSeat(game *catalog.CompiledGame) int {
	if seats := game.Definition.TurnOrder.Seats; len(seats) > 0 {
		return seats[0]
	}
	return 1
}

// nextImpulseSeat computes the seat whose turn follows seat, per
// game.Definition.TurnOrder. Only turn_order.type == "round-robin" is
// supported; any other value is an error.
func nextImpulseSeat(game *catalog.CompiledGame, seat int) (int, error) {
	turnOrder := game.Definition.TurnOrder
	if turnOrder.Type != "round-robin" {
		return 0, fmt.Errorf("state: unsupported turn_order.type %q", turnOrder.Type)
	}

	for i, s := range turnOrder.Seats {
		if s == seat {
			next := turnOrder.Seats[(i+1)%len(turnOrder.Seats)]
			return next, nil
		}
	}
	return 0, fmt.Errorf("state: seat %d not found in turn_order.seats", seat)
}

// randomSeed draws a seed for a new ruleset game from the system's entropy.
func randomSeed() uint64 {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		return 0x9e3779b97f4a7c15
	}
	return binary.LittleEndian.Uint64(b[:])
}
