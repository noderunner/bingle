// Package guest is what a game's rules are written against when they are to
// run as a WebAssembly module in an install pack (ADR 0147): a plain Go
// interface, and types of its own whose JSON is the engine's.
//
// It imports nothing from the engine, so a module carries only its game and
// the standard library, and what a pack depends on is the JSON on the wire
// rather than Go types. A game implements Rules and gives the package a New
// that makes them for one game:
//
//	func main() {}
//
//	func init() { guest.Register(mygame.New) }
//
// Built with GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared, that is a
// module the host in the parent package runs. Linked natively, the same New
// is a state.Ruleset through wasmrules.Native, which is how a pack's rules are
// tested with plain go test and measured against themselves.
//
// Rules keep nothing between calls: everything a call needs is in its
// arguments, and whatever a game must remember rides in Position.Ext. They
// must be deterministic, and must not let the order of a Go map reach an
// answer.
package guest

import "encoding/json"

// ABI is the version of the calls this package speaks.
const ABI = 0

// Rules are one game's rules: the nine calls of the engine's state.Ruleset,
// for the game New was given.
type Rules interface {
	// Start returns the opening position. seed is the game's only source of
	// chance: rules that need it later keep what they need in Position.Ext.
	Start(seed uint64) (*Position, error)
	// ImpulseSeat is the seat the position waits for, or 0 once it is over.
	ImpulseSeat(pos *Position) int
	// Legal lists every selection seat may make, in a deterministic order.
	Legal(pos *Position, seat int) []Selection
	// Apply checks and applies one selection, leaving pos as it was. It must
	// refuse exactly the selections Legal does not list.
	Apply(pos *Position, seat int, sel Selection) (*Result, error)
	// Format and Parse convert a selection to and from its notation.
	Format(sel Selection) string
	Parse(text string) (Selection, error)
	// Gestures are the gestures that build a legal selection.
	Gestures(sel Selection) []Gesture
	// Validate checks a position that did not come from Start or Apply (a
	// custom start).
	Validate(pos *Position) error
	// Outcome says whether the position is over, and who won.
	Outcome(pos *Position) Outcome
}

// New makes the rules of one game, once, from its definition and its board.
// A definition the rules cannot play is an error.
type New func(g *Game) (Rules, error)

// Game is one game of a pack: its base game or one of its variants.
type Game struct {
	ID string `json:"id"`
	// Definition is the game's definition as the catalog compiled it. Rules
	// decode what they need of it (seats, params).
	Definition json.RawMessage `json:"definition"`
	// Board is the board as the catalog built it.
	Board Board `json:"board"`
}

// Board is a board's positions, in the engine's order, and its named regions.
type Board struct {
	// Kind is "grid", "hex" or "explicit".
	Kind      string              `json:"kind"`
	Width     int                 `json:"width,omitempty"`
	Height    int                 `json:"height,omitempty"`
	Positions []Point             `json:"positions"`
	Regions   map[string][]string `json:"regions,omitempty"`
}

// Point is one position of a board.
type Point struct {
	ID string `json:"id"`
	// XY is its grid coordinate (x grows rightwards, y downwards), nil for a
	// position that has none.
	XY        *[2]int  `json:"xy,omitempty"`
	Neighbors []string `json:"neighbors"`
}

// Piece is one piece on the board: its type and its colour.
type Piece struct {
	Piece string `json:"piece"`
	Color string `json:"color"`
}

// Position is a position document: what is on each cell (bottom piece
// first), each colour's reserve, the plies played, the position's counters
// and the game's own data. Counters hold the platform's counts as well as a
// game's, so Apply carries them over: start from Clone.
type Position struct {
	Cells    map[string][]Piece        `json:"cells"`
	Reserves map[string]map[string]int `json:"reserves,omitempty"`
	Ply      int                       `json:"ply"`
	Counters map[string]int            `json:"counters,omitempty"`
	// Ext is the game's own data, which the host stores and hands back
	// without reading.
	Ext json.RawMessage `json:"ext,omitempty"`
}

// NewPosition returns an empty position.
func NewPosition() *Position { return &Position{Cells: map[string][]Piece{}} }

// Clone returns a copy of p that shares nothing with it but Ext, which is
// replaced and never changed in place.
func (p *Position) Clone() *Position {
	c := &Position{Cells: make(map[string][]Piece, len(p.Cells)), Ply: p.Ply, Ext: p.Ext}
	for id, stack := range p.Cells {
		c.Cells[id] = append([]Piece(nil), stack...)
	}
	if len(p.Reserves) > 0 {
		c.Reserves = make(map[string]map[string]int, len(p.Reserves))
		for color, hand := range p.Reserves {
			cp := make(map[string]int, len(hand))
			for piece, n := range hand {
				cp[piece] = n
			}
			c.Reserves[color] = cp
		}
	}
	if len(p.Counters) > 0 {
		c.Counters = make(map[string]int, len(p.Counters))
		for name, n := range p.Counters {
			c.Counters[name] = n
		}
	}
	return c
}

// Selection is one self-contained choice within an impulse (ADR 0114). Which
// fields matter is the game's own business.
type Selection struct {
	Action string   `json:"action,omitempty"`
	Piece  string   `json:"piece,omitempty"`
	From   string   `json:"from,omitempty"`
	To     string   `json:"to,omitempty"`
	Count  int      `json:"count,omitempty"`
	Path   []string `json:"path,omitempty"`
	Drops  []int    `json:"drops,omitempty"`
	Aim    string   `json:"aim,omitempty"`
	Card   string   `json:"card,omitempty"`
}

// The kinds of gesture.
const (
	GesturePiece  = "piece"  // choose a piece type
	GesturePos    = "pos"    // choose a board position
	GestureCount  = "count"  // choose a number
	GestureAction = "action" // an action with nothing else to choose
	GestureCard   = "card"   // choose a card
)

// Gesture is one thing a player does in an interface to build a selection.
type Gesture struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Fact is something a selection did that the game's definition declares
// (ADR 0129).
type Fact struct {
	Name string `json:"name"`
	Seat int    `json:"seat"`
}

// Outcome is how a position stands. With Ended set and no Winner, every seat
// drew, unless Drawn names the seats that share the draw or AllLost says
// that nobody did.
type Outcome struct {
	Ended   bool   `json:"ended,omitempty"`
	Winner  int    `json:"winner,omitempty"`
	Drawn   []int  `json:"drawn,omitempty"`
	AllLost bool   `json:"all_lost,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// Result is what a selection did.
type Result struct {
	// Position is the position after it.
	Position *Position
	// Captures are the cells it captured on.
	Captures []string
	// NextSeat is the seat the new position waits for. The host works out
	// from it where the impulse ends.
	NextSeat int
	// Notation is the selection's notation: Format's, when left empty.
	Notation string
	Facts    []Fact
	// Outcome is how the new position stands.
	Outcome
}
