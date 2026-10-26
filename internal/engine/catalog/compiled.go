package catalog

import (
	"io/fs"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/callback"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/primitives"
)

// ResolvedPiece is one piece type's movement rule, resolved against the
// primitive registry at Register time.
type ResolvedPiece struct {
	// Primitive is nil for pieces that declare no movement primitive (they
	// move only through declared actions such as stack moves).
	Primitive primitives.MovementPrimitive
	Occupancy primitives.Occupancy

	// Tags is the piece's tag set, for O(1) membership tests.
	Tags map[string]bool
	// Coverable reports whether other pieces may be moved onto a square
	// topped by this piece (default true).
	Coverable bool
	// Pool is the reserve pool placed pieces of this type are drawn from.
	Pool string
	// Dirs restricts the directions this piece may step or jump in, per
	// seat; a seat missing from the map (or a nil map) is unrestricted.
	Dirs map[int][]board.Direction
	// Promotions are applied to a piece of this type that ends a move in
	// the rule's region.
	Promotions []ResolvedPromotion
}

// ResolvedPromotion is a compiled dsl.PromoteDef.
type ResolvedPromotion struct {
	Seat   int // 0 = every seat
	Region map[board.PositionID]bool
	To     string
}

// HasTag reports whether the piece carries tag.
func (p ResolvedPiece) HasTag(tag string) bool { return p.Tags[tag] }

// Action type names understood by the engine.
const (
	ActionStep      = "step"
	ActionPlace     = "place"
	ActionMoveStack = "move-stack"
	ActionJump      = "jump"
	ActionSlide     = "slide"
	ActionLeap      = "leap"
	ActionSwap      = "swap"
	ActionPass      = "pass"
)

// Source values for place actions.
const (
	SourceReserve = "reserve"
	SourceSupply  = "supply"
)

// ResolvedAction is a compiled dsl.ActionDef (or the implicit "step"
// action).
type ResolvedAction struct {
	ID    string
	Type  string
	Label string

	// place
	Pieces     []string
	Source     string
	Region     map[board.PositionID]bool // nil = anywhere
	HasGravity bool
	GravityDX  int
	GravityDY  int

	// move-stack
	CarryLimit int
	Flatten    dsl.FlattenDef

	// place: flip bracketed opposing lines (Flips) and whether a placement
	// must flip something to be legal (FlipsRequired); liberty captures
	// (Captures, with AllowSuicide) and the simple ko rule (Ko).
	// SwapPlies holds the plies at which a swap action is available.
	SwapPlies map[int]bool

	// CustodianLengths lists capturable run lengths for placements and
	// slides (nil = none); CountPool is credited with each capture's size;
	// Anvils holds squares that close a flank while empty.
	CustodianLengths []int
	CountPool        string
	Anvils           map[board.PositionID]bool

	// ForbidLanding holds squares the action's pieces may not land on.
	ForbidLanding map[board.PositionID]bool

	Flips         bool
	FlipsRequired bool
	Captures      bool
	AllowSuicide  bool
	Ko            bool

	// jump
	Mandatory bool
	Chain     bool

	// slide: SlideOccupancy is the destination rule, SlideDirs the allowed
	// directions per seat (a missing seat = every board direction),
	// MaxDistance the longest slide (0 = unlimited) and Then the follow-up.
	SlideOccupancy primitives.Occupancy
	SlideDirs      map[int][]board.Direction
	MaxDistance    int
	Then           *ResolvedThen

	// leap: how far a clone reaches (MaxDistance bounds the jump) and
	// whether landing infects adjacent opposing pieces.
	CloneDistance int
	Infect        bool

	// Explicit marks a step action declared in the definition (as opposed
	// to the implicit one built from pieces' movement): it names its own
	// pieces, directions and occupancy rule.
	Explicit bool
	// StepOccupancy and StepDirs configure an explicit step action; StepDirs
	// is per seat (a missing seat is unrestricted).
	StepOccupancy primitives.Occupancy
	StepDirs      map[int][]board.Direction
}

// ResolvedThen is a compiled dsl.ThenDef.
type ResolvedThen struct {
	Piece       string
	Dirs        map[int][]board.Direction
	MaxDistance int
}

// CompiledGame is a fully-resolved, playable game: the parsed Definition,
// its compiled board Graph, every piece type's ResolvedPiece, and the
// GameCallbacks to invoke (NoopCallbacks if the registrant passed none).
type CompiledGame struct {
	Definition *dsl.GameDefinition
	Board      *board.Graph
	Pieces     map[string]ResolvedPiece // keyed by piece type name (dsl.GameDefinition.Pieces' keys)
	Callbacks  callback.GameCallbacks

	// Assets holds the module's files (guide pages, images), served to
	// clients under /api/game-defs/{id}/assets; nil when it has none.
	Assets fs.FS

	// Rules is the Go ruleset of a game whose definition says
	// engine: "ruleset" (nil for declarative games). It is opaque here —
	// the state package asserts it to its Ruleset interface — so catalog
	// need not import state.
	Rules any

	// Cards is the expanded card definitions of a card game (nil when the
	// definition has no "cards").
	Cards *CompiledCards

	// PieceTypes lists every piece type name, sorted.
	PieceTypes []string
	// Actions lists the game's actions in evaluation order: the implicit
	// "step" action first (when any piece has a movement primitive), then
	// the declared actions in declaration order.
	Actions []ResolvedAction
	// Directions are the board's named step directions (grid boards).
	Directions []board.Direction
	// Opening is non-nil when the turn order has an opening phase.
	Opening *ResolvedOpening
	// Forward is each seat's forward vector on a grid (when declared).
	Forward map[int][2]int
	// KeepsHistory is true when some rule (ko) compares positions with the
	// one before the last move, so applying a move must record it.
	KeepsHistory bool
}

// ResolvedOpening is a compiled dsl.OpeningDef.
type ResolvedOpening struct {
	Plies    int
	Actions  map[string]bool // nil = any
	Pieces   map[string]bool // nil = any
	Opponent bool            // placed pieces take the opponent's color/reserve
}

// Pool returns the reserve pool piece draws from (the piece type's own name
// unless its definition names a shared pool).
func (g *CompiledGame) Pool(piece string) string {
	if p, ok := g.Pieces[piece]; ok && p.Pool != "" {
		return p.Pool
	}
	return piece
}

// Action returns the action with the given ID.
func (g *CompiledGame) Action(id string) (ResolvedAction, bool) {
	for _, a := range g.Actions {
		if a.ID == id {
			return a, true
		}
	}
	return ResolvedAction{}, false
}

// Seat returns the definition of the given seat number.
func (g *CompiledGame) Seat(seat int) (dsl.SeatDef, bool) {
	for _, s := range g.Definition.Seats {
		if s.Seat == seat {
			return s, true
		}
	}
	return dsl.SeatDef{}, false
}

// SeatOfColor returns the seat that plays color.
func (g *CompiledGame) SeatOfColor(color string) (int, bool) {
	for _, s := range g.Definition.Seats {
		if s.Color == color {
			return s.Seat, true
		}
	}
	return 0, false
}

// Colors lists every seat's color, in seat declaration order.
func (g *CompiledGame) Colors() []string {
	out := make([]string, len(g.Definition.Seats))
	for i, s := range g.Definition.Seats {
		out[i] = s.Color
	}
	return out
}

// ParamInt reads an integer entry of the definition's params (JSON numbers
// arrive as float64), or def when it is absent or not a whole number.
func (g *CompiledGame) ParamInt(name string, def int) int {
	switch v := g.Definition.Params[name].(type) {
	case float64:
		if v == float64(int(v)) {
			return int(v)
		}
	case int:
		return v
	}
	return def
}

// ParamString reads a string entry of the definition's params.
func (g *CompiledGame) ParamString(name, def string) string {
	if v, ok := g.Definition.Params[name].(string); ok {
		return v
	}
	return def
}

// Opponent returns the opposing seat in a two-seat game.
func (g *CompiledGame) Opponent(seat int) (int, bool) {
	if len(g.Definition.Seats) != 2 {
		return 0, false
	}
	for _, s := range g.Definition.Seats {
		if s.Seat != seat {
			return s.Seat, true
		}
	}
	return 0, false
}
