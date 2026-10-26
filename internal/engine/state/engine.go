package state

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"

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
	st := NewBoardState()
	for _, entry := range game.Definition.Setup {
		st.Push(board.PositionID(entry.Position), Occupant{Piece: entry.Piece, Color: entry.Color})
	}
	for _, seat := range game.Definition.Seats {
		for piece, n := range game.Definition.Reserves {
			st.SetReserve(seat.Color, piece, n)
		}
	}
	return st
}

// MoveResult is the outcome of a successfully applied move.
type MoveResult struct {
	NewState *BoardState
	Captured *board.PositionID
	// Captures lists every captured position (a draughts-style jump
	// captures several); Captured is the first of them.
	Captures     []board.PositionID
	NextTurnSeat int
	GameEnded    bool
	WinnerSeat   *int

	// Move is the normalized move that was applied.
	Move Move
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
	return &rctx{game: game, st: st, seat: seat, color: seatDef.Color}, nil
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
func (c *rctx) validate(act catalog.ResolvedAction, mv Move) (executor, error) {
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
		colors := c.game.Colors()
		if len(colors) != 2 {
			return nil, fmt.Errorf("state: swap needs exactly two colours")
		}
		a, b := colors[0], colors[1]
		return func(ns *BoardState) []board.PositionID {
			for id, stack := range ns.cells {
				for i, o := range stack {
					switch o.Color {
					case a:
						stack[i].Color = b
					case b:
						stack[i].Color = a
					}
				}
				ns.cells[id] = stack
			}
			// The reserves change hands too.
			ra, rb := ns.reserves[a], ns.reserves[b]
			ns.reserves[a], ns.reserves[b] = rb, ra
			return nil
		}, nil
	case catalog.ActionPass:
		if mv.Piece != "" || mv.From != "" || mv.To != "" || mv.Count != 0 || len(mv.Path) > 0 || len(mv.Drops) > 0 {
			return nil, fmt.Errorf("state: a pass takes no parameters")
		}
		return func(*BoardState) []board.PositionID { return nil }, nil
	default:
		return nil, fmt.Errorf("state: unsupported action type %q", act.Type)
	}
}

func (c *rctx) moveContext(board callback.BoardView, mv Move) callback.MoveContext {
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
func Apply(game *catalog.CompiledGame, st *BoardState, seat int, mv Move) (*MoveResult, error) {
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
	newState.ply++
	if game.KeepsHistory {
		newState.prev = positionHash(game, st)
	}
	if act.Type == catalog.ActionPass {
		newState.passes = st.passes + 1
	} else {
		newState.passes = 0
	}

	postCtx := ctx
	postCtx.Board = newState
	if err := game.Callbacks.OnMoveApplied(postCtx); err != nil {
		return nil, err
	}

	nextSeat, err := nextTurnSeat(game, seat)
	if err != nil {
		return nil, err
	}
	if game.Definition.TurnOrder.PassWhenStuck {
		nextSeat = skipStuckSeats(game, newState, nextSeat)
	}

	outcome, err := evaluateWin(game, newState, seat, nextSeat)
	if err != nil {
		return nil, err
	}

	return &MoveResult{
		NewState:     newState,
		Captured:     captured,
		Captures:     captures,
		NextTurnSeat: nextSeat,
		GameEnded:    outcome.Ended,
		WinnerSeat:   outcome.Winner,
		Move:         mv,
		Notation:     FormatMove(game, mv),
		EndReason:    outcome.Reason,
		EndDetail:    outcome.Detail,
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

// LegalMoves enumerates every move seat may legally make in st, in a
// deterministic order (actions in declaration order; within an action,
// board reading order). Moves rejected by the game's ValidateMove callback
// are excluded. It returns nil for an unknown seat.
//
// Every returned move is accepted by Apply, and Apply accepts no move this
// function omits: both go through the same per-action validation.
func LegalMoves(game *catalog.CompiledGame, st *BoardState, seat int) []Move {
	return legalMoves(game, st, seat, 0)
}

// legalMoves is LegalMoves that stops after limit moves when limit > 0.
func legalMoves(game *catalog.CompiledGame, st *BoardState, seat int, limit int) []Move {
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

	var out []Move
	// keep appends mv if it is legal. wellFormed reports that the caller
	// built mv so that validation cannot fail (see enumerateStackMoves);
	// everything else is validated by the same code Apply uses.
	keep := func(act catalog.ResolvedAction, mv Move, wellFormed bool) {
		if c.openingAllows(act, mv.Piece) != nil {
			return
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
		case catalog.ActionPass:
			keep(act, Move{Action: act.ID}, true)
		case catalog.ActionSwap:
			if act.SwapPlies[c.st.ply] {
				keep(act, Move{Action: act.ID}, false)
			}
		}
	}
	return out
}

// HasLegalMove reports whether seat has at least one legal move in st.
func HasLegalMove(game *catalog.CompiledGame, st *BoardState, seat int) bool {
	return len(legalMoves(game, st, seat, 1)) > 0
}

// ApplyMove validates and applies a single legacy step move by seat, from
// `from` to `to`; it is Apply with a Move{From, To}. See docs/history/plan.md
// section 6.4a for the original 12-step algorithm.
func ApplyMove(game *catalog.CompiledGame, st *BoardState, seat int, from, to board.PositionID) (*MoveResult, error) {
	return Apply(game, st, seat, Move{Action: catalog.ActionStep, From: from, To: to})
}

// skipStuckSeats starting from `candidate` returns the first seat, in turn
// order, that has a legal move; if nobody has one it returns candidate.
func skipStuckSeats(game *catalog.CompiledGame, st *BoardState, candidate int) int {
	seat := candidate
	for i := 0; i < len(game.Definition.TurnOrder.Seats); i++ {
		if HasLegalMove(game, st, seat) {
			return seat
		}
		next, err := nextTurnSeat(game, seat)
		if err != nil {
			return candidate
		}
		seat = next
	}
	return candidate
}

// TurnAfter returns the seat to move in st when seat has just moved: the next
// seat in turn order, skipping seats with no legal move when the game passes
// stuck seats. It mirrors what Apply puts in MoveResult.NextTurnSeat.
func TurnAfter(game *catalog.CompiledGame, st *BoardState, seat int) (int, error) {
	if rs, ok := RulesetOf(game); ok {
		if next := rs.ToMove(game, st); next != 0 {
			return next, nil
		}
		return seat, nil
	}
	next, err := nextTurnSeat(game, seat)
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

// nextTurnSeat computes the seat whose turn follows seat, per
// game.Definition.TurnOrder. Only turn_order.type == "round-robin" is
// supported; any other value is an error.
func nextTurnSeat(game *catalog.CompiledGame, seat int) (int, error) {
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
