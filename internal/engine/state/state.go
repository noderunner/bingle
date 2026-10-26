// Package state implements BoardState, the live position -> stack map plus
// per-color reserves for one game in progress, and the rules engine that
// enumerates, validates and applies moves against a *catalog.CompiledGame.
//
// See docs/history/plan.md section 6.4a for the original single-occupant contract
// and docs/decisions/0015-engine-v2-stacks-and-actions.md for the stacking
// extension this package now implements.
package state

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/noderunner/bingle/internal/engine/board"
)

// Occupant is what's on one board position: which piece type (a key into
// catalog.CompiledGame.Pieces, e.g. "stone") and which color/seat owns it.
//
// Its (untagged, capitalised) JSON shape is the original v1 wire shape and
// is kept so the legacy flat "board_state" view stays byte-compatible.
type Occupant struct {
	Piece string
	Color string
}

// BoardState is the live, mutable game position for one game in progress:
// every occupied position's stack of pieces (bottom first, top last), each
// color's reserve ("hand") of unplaced pieces, and the number of plies
// played so far. Use NewBoardState; the zero value is not usable.
//
// The original single-occupant methods (Get, Set, Remove, Occupant, ...)
// keep working: Get/Occupant read the top piece, Set replaces a whole stack
// with one piece, Remove/RemoveOccupant clear it.
type BoardState struct {
	cells    map[board.PositionID][]Occupant
	reserves map[string]map[string]int
	ply      int

	// prev is a hash of the position before the last move (kept only for
	// games with a ko rule) and passes counts consecutive pass actions.
	prev   string
	passes int

	// ext is the game-specific payload of a ruleset game (terrain, unit
	// conditions, momentum, ...): a json.RawMessage straight from a document
	// until the ruleset decodes it, then the ruleset's own value (which
	// must implement ExtClone).
	ext any
}

// ExtClone is implemented by the payload a ruleset keeps in a position so
// that BoardState.Clone can copy it.
type ExtClone interface {
	CloneExt() any
}

// NewBoardState returns an empty, ready-to-use *BoardState.
func NewBoardState() *BoardState {
	return &BoardState{
		cells:    make(map[board.PositionID][]Occupant),
		reserves: make(map[string]map[string]int),
	}
}

// Get returns the top Occupant at id, and whether the position is occupied.
func (s *BoardState) Get(id board.PositionID) (Occupant, bool) {
	stack := s.cells[id]
	if len(stack) == 0 {
		return Occupant{}, false
	}
	return stack[len(stack)-1], true
}

// Set places occ at id as the sole piece there, replacing whatever (if
// anything, including a whole stack) was there.
func (s *BoardState) Set(id board.PositionID, occ Occupant) {
	s.cells[id] = []Occupant{occ}
}

// Remove clears any occupant (the whole stack) at id. It is a no-op if id
// was unoccupied.
func (s *BoardState) Remove(id board.PositionID) {
	delete(s.cells, id)
}

// Stack returns a copy of the stack at id, bottom piece first. It is nil
// when the position is empty.
func (s *BoardState) Stack(id board.PositionID) []Occupant {
	stack := s.cells[id]
	if len(stack) == 0 {
		return nil
	}
	out := make([]Occupant, len(stack))
	copy(out, stack)
	return out
}

// Height returns the number of pieces at id.
func (s *BoardState) Height(id board.PositionID) int {
	return len(s.cells[id])
}

// SetStack replaces the stack at id with a copy of stack (bottom first). An
// empty stack clears the position.
func (s *BoardState) SetStack(id board.PositionID, stack []Occupant) {
	if len(stack) == 0 {
		delete(s.cells, id)
		return
	}
	cp := make([]Occupant, len(stack))
	copy(cp, stack)
	s.cells[id] = cp
}

// Push puts occ on top of the stack at id.
func (s *BoardState) Push(id board.PositionID, occ Occupant) {
	s.cells[id] = append(s.cells[id], occ)
}

// Occupied returns every occupied position, sorted lexicographically.
func (s *BoardState) Occupied() []board.PositionID {
	ids := make([]board.PositionID, 0, len(s.cells))
	for id, stack := range s.cells {
		if len(stack) > 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// Reserve returns how many pieces of the given type color has in hand.
func (s *BoardState) Reserve(color, piece string) int {
	return s.reserves[color][piece]
}

// SetReserve sets color's count of piece in hand (clamped at zero).
func (s *BoardState) SetReserve(color, piece string, n int) {
	if n < 0 {
		n = 0
	}
	if s.reserves[color] == nil {
		s.reserves[color] = make(map[string]int)
	}
	s.reserves[color][piece] = n
}

// ReserveTotal returns the total number of pieces color has in hand.
func (s *BoardState) ReserveTotal(color string) int {
	total := 0
	for _, n := range s.reserves[color] {
		total += n
	}
	return total
}

// Reserves returns a deep copy of every color's hand: color -> piece -> n.
func (s *BoardState) Reserves() map[string]map[string]int {
	out := make(map[string]map[string]int, len(s.reserves))
	for color, hand := range s.reserves {
		cp := make(map[string]int, len(hand))
		for p, n := range hand {
			cp[p] = n
		}
		out[color] = cp
	}
	return out
}

// Ply returns how many plies (single player actions) have been applied.
func (s *BoardState) Ply() int { return s.ply }

// SetPly overrides the ply counter (used when constructing a position).
func (s *BoardState) SetPly(n int) { s.ply = n }

// Clone returns a deep, independent copy of s: mutating the clone never
// affects s, and vice versa.
func (s *BoardState) Clone() *BoardState {
	clone := NewBoardState()
	for id, stack := range s.cells {
		cp := make([]Occupant, len(stack))
		copy(cp, stack)
		clone.cells[id] = cp
	}
	for color, hand := range s.reserves {
		cp := make(map[string]int, len(hand))
		for p, n := range hand {
			cp[p] = n
		}
		clone.reserves[color] = cp
	}
	clone.ply = s.ply
	clone.prev = s.prev
	clone.passes = s.passes
	switch v := s.ext.(type) {
	case nil:
	case ExtClone:
		clone.ext = v.CloneExt()
	default:
		// A json.RawMessage is never mutated in place, so it is shared.
		clone.ext = v
	}
	return clone
}

// Ext returns the ruleset payload: nil, a json.RawMessage (undecoded, from a
// document) or the value a ruleset stored with SetExt.
func (s *BoardState) Ext() any { return s.ext }

// SetExt stores the ruleset payload. v must implement ExtClone and marshal
// to JSON, or be a json.RawMessage.
func (s *BoardState) SetExt(v any) { s.ext = v }

// Passes returns the number of consecutive pass actions just played.
func (s *BoardState) Passes() int { return s.passes }

// Occupant satisfies primitives.OccupancyView: color-only read access to
// the top piece, for invoking a piece's movement primitive.
func (s *BoardState) Occupant(id board.PositionID) (color string, occupied bool) {
	occ, ok := s.Get(id)
	if !ok {
		return "", false
	}
	return occ.Color, true
}

// SetOccupant satisfies callback.BoardView. It sets the top piece's Color
// and preserves its Piece type (or leaves Piece empty if id was previously
// unoccupied) — see docs/history/plan.md section 6.4a's note on why this shim
// doesn't try to guess a piece type from a bare color string.
func (s *BoardState) SetOccupant(id board.PositionID, color string) {
	stack := s.cells[id]
	if len(stack) == 0 {
		s.cells[id] = []Occupant{{Color: color}}
		return
	}
	stack[len(stack)-1].Color = color
}

// RemoveOccupant satisfies callback.BoardView.
func (s *BoardState) RemoveOccupant(id board.PositionID) {
	delete(s.cells, id)
}

// --- JSON ---------------------------------------------------------------

// pieceJSON is the v2 wire shape of one piece.
type pieceJSON struct {
	Piece string `json:"piece"`
	Color string `json:"color"`
}

// stateJSON is the v2 persisted/wire shape of a BoardState: the complete
// position including stacks, reserves and the ply counter. "v" tells it
// apart from the v1 flat occupant map.
type stateJSON struct {
	V        int                       `json:"v"`
	Cells    map[string][]pieceJSON    `json:"cells"`
	Reserves map[string]map[string]int `json:"reserves,omitempty"`
	Ply      int                       `json:"ply"`
	Prev     string                    `json:"prev,omitempty"`
	Passes   int                       `json:"passes,omitempty"`
	Ext      json.RawMessage           `json:"ext,omitempty"`
}

// StateVersion is the current persisted state document version.
const StateVersion = 2

// MarshalJSON encodes the complete state (stacks, reserves, ply) as a v2
// document. This is the persistence format; use LegacyJSON for the v1 flat
// top-of-stack view.
func (s *BoardState) MarshalJSON() ([]byte, error) {
	doc := stateJSON{
		V:      StateVersion,
		Cells:  make(map[string][]pieceJSON, len(s.cells)),
		Ply:    s.ply,
		Prev:   s.prev,
		Passes: s.passes,
	}
	switch v := s.ext.(type) {
	case nil:
	case json.RawMessage:
		doc.Ext = v
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("state: marshal ruleset payload: %w", err)
		}
		doc.Ext = raw
	}
	for id, stack := range s.cells {
		if len(stack) == 0 {
			continue
		}
		pieces := make([]pieceJSON, len(stack))
		for i, o := range stack {
			pieces[i] = pieceJSON(o)
		}
		doc.Cells[string(id)] = pieces
	}
	for color, hand := range s.reserves {
		if len(hand) == 0 {
			continue
		}
		if doc.Reserves == nil {
			doc.Reserves = make(map[string]map[string]int)
		}
		cp := make(map[string]int, len(hand))
		for p, n := range hand {
			cp[p] = n
		}
		doc.Reserves[color] = cp
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("state: marshal BoardState: %w", err)
	}
	return data, nil
}

// LegacyJSON encodes the v1 flat view: a JSON object keyed by position whose
// values are the top piece's {"Piece","Color"}. Stacks and reserves are not
// represented; this exists so v1 clients keep working.
func (s *BoardState) LegacyJSON() ([]byte, error) {
	out := make(map[string]Occupant, len(s.cells))
	for id, stack := range s.cells {
		if len(stack) == 0 {
			continue
		}
		out[string(id)] = stack[len(stack)-1]
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("state: marshal legacy BoardState: %w", err)
	}
	return data, nil
}

// UnmarshalJSON decodes data into s, replacing its contents entirely. It
// accepts both the v2 document (see MarshalJSON) and the v1 flat occupant
// map, so databases written by earlier versions keep loading.
func (s *BoardState) UnmarshalJSON(data []byte) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("state: unmarshal BoardState: %w", err)
	}

	if rawV, ok := probe["v"]; ok {
		var v int
		if err := json.Unmarshal(rawV, &v); err == nil {
			return s.unmarshalV2(data, v)
		}
	}

	var in map[string]Occupant
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("state: unmarshal BoardState: %w", err)
	}
	cells := make(map[board.PositionID][]Occupant, len(in))
	for id, occ := range in {
		cells[board.PositionID(id)] = []Occupant{occ}
	}
	s.cells = cells
	s.reserves = make(map[string]map[string]int)
	s.ply = 0
	s.ext = nil
	return nil
}

func (s *BoardState) unmarshalV2(data []byte, version int) error {
	if version != StateVersion {
		return fmt.Errorf("state: unsupported state document version %d", version)
	}
	var doc stateJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("state: unmarshal BoardState: %w", err)
	}
	cells := make(map[board.PositionID][]Occupant, len(doc.Cells))
	for id, pieces := range doc.Cells {
		if len(pieces) == 0 {
			continue
		}
		stack := make([]Occupant, len(pieces))
		for i, p := range pieces {
			stack[i] = Occupant(p)
		}
		cells[board.PositionID(id)] = stack
	}
	reserves := make(map[string]map[string]int, len(doc.Reserves))
	for color, hand := range doc.Reserves {
		cp := make(map[string]int, len(hand))
		for p, n := range hand {
			cp[p] = n
		}
		reserves[color] = cp
	}
	s.cells = cells
	s.reserves = reserves
	s.ply = doc.Ply
	s.prev = doc.Prev
	s.passes = doc.Passes
	s.ext = nil
	if len(doc.Ext) > 0 && string(doc.Ext) != "null" {
		s.ext = doc.Ext
	}
	return nil
}
