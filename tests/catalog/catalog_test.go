// Tests of catalog registration: a definition compiles into a game with its board and resolved pieces, and one with an unknown primitive or a malformed document is rejected and cannot be fetched.

package catalog_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/callback"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/primitives"
)

// --- fixtures ----------------------------------------------------------------

// validGridDefJSON mirrors docs/history/plan.md section 6.1's annotated example: a
// 9x9 orthogonal4 grid, one piece type "stone" using the "step" primitive
// with "capture-or-empty" occupancy, 2 seats, round-robin turn order, no win
// condition, no callbacks.
const validGridDefJSON = `{
  "id": "demo-freeplay",
  "name": "Demo: Free Movement & Capture",
  "board": {
    "generator": { "type": "grid", "width": 9, "height": 9, "adjacency": "orthogonal4" }
  },
  "seats": [
    { "seat": 1, "color": "black" },
    { "seat": 2, "color": "white" }
  ],
  "pieces": {
    "stone": { "movement": { "primitive": "step", "occupancy": "capture-or-empty" } }
  },
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" }
}`

// validExplicitDefJSON uses positions/edges instead of a generator: 3
// positions, 2 edges (p1-p2, p2-p3), mirroring the shape verified directly
// against board.FromExplicit in tests/board/explicit_test.go.
const validExplicitDefJSON = `{
  "id": "explicit-game",
  "name": "Explicit Board Game",
  "board": {
    "positions": [{"id":"p1"},{"id":"p2"},{"id":"p3"}],
    "edges": [{"from":"p1","to":"p2"},{"from":"p2","to":"p3"}]
  },
  "seats": [
    { "seat": 1, "color": "black" }
  ],
  "pieces": {
    "stone": { "movement": { "primitive": "step", "occupancy": "empty-only" } }
  },
  "turn_order": { "type": "round-robin", "seats": [1] },
  "win_condition": { "type": "none" }
}`

// unknownPrimitiveDefJSON is otherwise well-formed but names a movement
// primitive ("teleport") that is not in the primitives registry, for piece
// type "stone" — this is backlog item 07's AC.
const unknownPrimitiveDefJSON = `{
  "id": "broken-primitive-game",
  "name": "Broken Primitive Game",
  "board": {
    "generator": { "type": "grid", "width": 3, "height": 3, "adjacency": "orthogonal4" }
  },
  "seats": [
    { "seat": 1, "color": "black" },
    { "seat": 2, "color": "white" }
  ],
  "pieces": {
    "stone": { "movement": { "primitive": "teleport", "occupancy": "capture-or-empty" } }
  },
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" }
}`

// malformedDefJSON is missing the required "seats" field entirely, so
// dsl.Parse itself must reject it before catalog ever reaches board/piece
// resolution.
const malformedDefJSON = `{
  "id": "malformed-game",
  "name": "Malformed Game",
  "board": {
    "generator": { "type": "grid", "width": 3, "height": 3, "adjacency": "orthogonal4" }
  },
  "pieces": {
    "stone": { "movement": { "primitive": "step", "occupancy": "empty-only" } }
  },
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" }
}`

// unsupportedGeneratorDefJSON is well-formed per dsl.Parse but names a board
// generator type ("triangle") that catalog does not implement (only "grid" and "hex" are
// implemented so far, per plan.md section 6.3a step 2).
const unsupportedGeneratorDefJSON = `{
  "id": "triangle-game",
  "name": "Hex Game",
  "board": {
    "generator": { "type": "triangle", "width": 3, "height": 3, "adjacency": "orthogonal4" }
  },
  "seats": [
    { "seat": 1, "color": "black" }
  ],
  "pieces": {
    "stone": { "movement": { "primitive": "step", "occupancy": "empty-only" } }
  },
  "turn_order": { "type": "round-robin", "seats": [1] },
  "win_condition": { "type": "none" }
}`

// noBoardSourceDefJSON has a "board" key present (satisfying dsl.Parse's
// required-top-level-field presence check, which only checks the key
// exists) but with neither "generator" nor "positions" inside it. Per
// dsl.Parse's own validateRequiredFields, this is actually caught by dsl
// itself (Board.Generator == nil && len(Positions) == 0 is treated as a
// missing "board" field) before catalog's board-resolution step ever runs.
// The AC under test here is purely observable: Register must return a
// non-nil error and the id must not become retrievable via Get — which
// holds regardless of which layer (dsl or catalog) produced the error.
const noBoardSourceDefJSON = `{
  "id": "no-board-game",
  "name": "No Board Game",
  "board": {},
  "seats": [
    { "seat": 1, "color": "black" }
  ],
  "pieces": {
    "stone": { "movement": { "primitive": "step", "occupancy": "empty-only" } }
  },
  "turn_order": { "type": "round-robin", "seats": [1] },
  "win_condition": { "type": "none" }
}`

// validExplicitDefJSONAlt is a second, distinct valid explicit-board
// definition (different id-independent shape) used to verify that
// re-registering an id overwrites the previous entry with this one.
const validExplicitDefJSONAlt = `{
  "id": "explicit-game-v2",
  "name": "Explicit Board Game V2",
  "board": {
    "positions": [{"id":"q1"},{"id":"q2"}],
    "edges": [{"from":"q1","to":"q2"}]
  },
  "seats": [
    { "seat": 1, "color": "black" }
  ],
  "pieces": {
    "rock": { "movement": { "primitive": "step", "occupancy": "empty-only" } }
  },
  "turn_order": { "type": "round-robin", "seats": [1] },
  "win_condition": { "type": "none" }
}`

// fakeOccupancy is a minimal test-only implementation of
// primitives.OccupancyView, used to actually invoke a catalog-resolved
// MovementPrimitive and prove it's the real "step" implementation, not a
// non-nil placeholder.
type fakeOccupancy map[board.PositionID]string

func (o fakeOccupancy) Occupant(id board.PositionID) (string, bool) {
	color, ok := o[id]
	return color, ok
}

var _ primitives.OccupancyView = fakeOccupancy{}

// fakeCallbacks is a distinguishable, non-trivial callback.GameCallbacks
// used to prove that Register/Get preserve the exact caller-supplied
// callbacks object (not a copy that loses behavior, and not silently
// replaced with NoopCallbacks).
type fakeCallbacks struct{}

// errSentinelValidateMove is the marker error fakeCallbacks.ValidateMove
// always returns, so tests can distinguish "the real fake ran" from "some
// other GameCallbacks (e.g. NoopCallbacks) ran".
var errSentinelValidateMove = errValidateMoveSentinel{}

type errValidateMoveSentinel struct{}

func (errValidateMoveSentinel) Error() string { return "fakeCallbacks: sentinel rejection" }

func (fakeCallbacks) ValidateMove(ctx callback.MoveContext) error {
	return errSentinelValidateMove
}

func (fakeCallbacks) OnMoveApplied(ctx callback.MoveContext) error {
	return nil
}

func (fakeCallbacks) CheckWinner(st callback.BoardView) (*int, bool) {
	return nil, false
}

var _ callback.GameCallbacks = fakeCallbacks{}

// --- 1. Happy path, grid board -----------------------------------------------

func TestRegister_GridBoard_HappyPath(t *testing.T) {
	cat := catalog.NewCatalog()

	if err := cat.Register("demo-freeplay", []byte(validGridDefJSON), nil); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	cg, ok := cat.Get("demo-freeplay")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "demo-freeplay")
	}
	if cg == nil {
		t.Fatalf("Get(%q) returned nil *CompiledGame with ok=true", "demo-freeplay")
	}

	// Board: grid positions present.
	for _, pos := range []board.PositionID{"0,0", "8,8", "4,4", "3,5"} {
		if !cg.Board.HasPosition(pos) {
			t.Errorf("Board.HasPosition(%s) = false, want true", pos)
		}
	}
	// Out-of-range position must not exist (9x9 grid is 0..8 per axis).
	if cg.Board.HasPosition("9,9") {
		t.Errorf("Board.HasPosition(9,9) = true, want false (out of 9x9 range)")
	}

	// Pieces: "stone" resolved with the right occupancy.
	stone, ok := cg.Pieces["stone"]
	if !ok {
		t.Fatalf("Pieces[%q] missing", "stone")
	}
	if stone.Occupancy != primitives.OccupancyCaptureOrEmpty {
		t.Errorf("Pieces[%q].Occupancy = %q, want %q", "stone", stone.Occupancy, primitives.OccupancyCaptureOrEmpty)
	}
	if stone.Primitive == nil {
		t.Fatalf("Pieces[%q].Primitive = nil, want the resolved step primitive", "stone")
	}

	// Prove it's the *real* step primitive: invoke it directly on two
	// graph-adjacent positions with an empty destination.
	from := board.PositionID("4,4")
	to := board.PositionID("4,5")
	if !cg.Board.HasEdge(from, to) {
		t.Fatalf("fixture error: expected HasEdge(%s,%s) = true on the generated grid", from, to)
	}
	occ := fakeOccupancy{from: "black"} // destination empty
	res := stone.Primitive(cg.Board, occ, stone.Occupancy, from, to, "black")
	if !res.Allowed {
		t.Errorf("invoking resolved Primitive: Allowed = false, want true (reason: %q) — did Register resolve a real primitive?", res.Reason)
	}

	// Callbacks: nil was passed to Register, so this must be a non-nil
	// no-op default.
	if cg.Callbacks == nil {
		t.Fatalf("Callbacks = nil, want non-nil default (callback.NoopCallbacks)")
	}
	if err := cg.Callbacks.ValidateMove(callback.MoveContext{}); err != nil {
		t.Errorf("Callbacks.ValidateMove(zero-value context) = %v, want nil (expected no-op default)", err)
	}
}

// --- 2. Happy path, explicit board -------------------------------------------

func TestRegister_ExplicitBoard_HappyPath(t *testing.T) {
	cat := catalog.NewCatalog()

	if err := cat.Register("explicit-game", []byte(validExplicitDefJSON), nil); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	cg, ok := cat.Get("explicit-game")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "explicit-game")
	}

	for _, pos := range []board.PositionID{"p1", "p2", "p3"} {
		if !cg.Board.HasPosition(pos) {
			t.Errorf("Board.HasPosition(%s) = false, want true", pos)
		}
	}
	if cg.Board.HasPosition("p4") {
		t.Errorf("Board.HasPosition(p4) = true, want false (not in fixture)")
	}

	if !cg.Board.HasEdge("p1", "p2") {
		t.Errorf("Board.HasEdge(p1,p2) = false, want true")
	}
	if !cg.Board.HasEdge("p2", "p3") {
		t.Errorf("Board.HasEdge(p2,p3) = false, want true")
	}
	if cg.Board.HasEdge("p1", "p3") {
		t.Errorf("Board.HasEdge(p1,p3) = true, want false (not a given edge)")
	}
}

// --- 3. The AC itself: unknown primitive name --------------------------------

func TestRegister_UnknownPrimitive_ReturnsErrorAndNotRetrievable(t *testing.T) {
	cat := catalog.NewCatalog()

	err := cat.Register("broken-primitive-game", []byte(unknownPrimitiveDefJSON), nil)
	if err == nil {
		t.Fatalf("Register() error = nil, want non-nil (unknown primitive %q for piece %q)", "teleport", "stone")
	}
	if !strings.Contains(err.Error(), "stone") {
		t.Errorf("Register() error = %q, want it to name the piece type %q", err.Error(), "stone")
	}
	if !strings.Contains(err.Error(), "teleport") {
		t.Errorf("Register() error = %q, want it to name the unknown primitive %q", err.Error(), "teleport")
	}

	cg, ok := cat.Get("broken-primitive-game")
	if ok {
		t.Errorf("Get(%q) ok = true, want false (registration failed)", "broken-primitive-game")
	}
	if cg != nil {
		t.Errorf("Get(%q) = %v, want nil *CompiledGame", "broken-primitive-game", cg)
	}
}

// --- 4. Malformed/invalid definition JSON ------------------------------------

func TestRegister_MalformedDefinition_ReturnsErrorAndNotRetrievable(t *testing.T) {
	cat := catalog.NewCatalog()

	err := cat.Register("malformed-game", []byte(malformedDefJSON), nil)
	if err == nil {
		t.Fatalf("Register() error = nil, want non-nil (definition missing required \"seats\" field)")
	}

	cg, ok := cat.Get("malformed-game")
	if ok {
		t.Errorf("Get(%q) ok = true, want false", "malformed-game")
	}
	if cg != nil {
		t.Errorf("Get(%q) = %v, want nil *CompiledGame", "malformed-game", cg)
	}
}

func TestRegister_InvalidJSONSyntax_ReturnsErrorAndNotRetrievable(t *testing.T) {
	cat := catalog.NewCatalog()

	err := cat.Register("not-json", []byte("{not valid json"), nil)
	if err == nil {
		t.Fatalf("Register() error = nil, want non-nil (invalid JSON syntax)")
	}

	cg, ok := cat.Get("not-json")
	if ok {
		t.Errorf("Get(%q) ok = true, want false", "not-json")
	}
	if cg != nil {
		t.Errorf("Get(%q) = %v, want nil *CompiledGame", "not-json", cg)
	}
}

// --- 5. Unsupported board generator type -------------------------------------

func TestRegister_UnsupportedGeneratorType_ReturnsErrorAndNotRetrievable(t *testing.T) {
	cat := catalog.NewCatalog()

	err := cat.Register("triangle-game", []byte(unsupportedGeneratorDefJSON), nil)
	if err == nil {
		t.Fatalf("Register() error = nil, want non-nil (generator type %q is not implemented)", "triangle")
	}

	cg, ok := cat.Get("triangle-game")
	if ok {
		t.Errorf("Get(%q) ok = true, want false", "triangle-game")
	}
	if cg != nil {
		t.Errorf("Get(%q) = %v, want nil *CompiledGame", "triangle-game", cg)
	}
}

// --- 6. Board with neither generator nor positions ---------------------------

func TestRegister_BoardWithNeitherGeneratorNorPositions_ReturnsErrorAndNotRetrievable(t *testing.T) {
	cat := catalog.NewCatalog()

	err := cat.Register("no-board-game", []byte(noBoardSourceDefJSON), nil)
	if err == nil {
		t.Fatalf("Register() error = nil, want non-nil (board has neither generator nor positions)")
	}

	cg, ok := cat.Get("no-board-game")
	if ok {
		t.Errorf("Get(%q) ok = true, want false", "no-board-game")
	}
	if cg != nil {
		t.Errorf("Get(%q) = %v, want nil *CompiledGame", "no-board-game", cg)
	}
}

// --- 7. Explicit callbacks preserved -----------------------------------------

func TestRegister_ExplicitCallbacksPreserved(t *testing.T) {
	cat := catalog.NewCatalog()

	cb := fakeCallbacks{}
	if err := cat.Register("callback-game", []byte(validExplicitDefJSON), cb); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	cg, ok := cat.Get("callback-game")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "callback-game")
	}
	if cg.Callbacks == nil {
		t.Fatalf("Callbacks = nil, want the fakeCallbacks instance passed to Register")
	}

	err := cg.Callbacks.ValidateMove(callback.MoveContext{})
	if err != errSentinelValidateMove {
		t.Errorf("Callbacks.ValidateMove(...) = %v, want the fakeCallbacks sentinel error %v (Callbacks must be exactly the fake passed in, not e.g. NoopCallbacks)", err, errSentinelValidateMove)
	}
}

// --- 8. Atomicity / no-clobber on failure ------------------------------------

func TestRegister_FailedReregistration_LeavesOriginalEntryUntouched(t *testing.T) {
	cat := catalog.NewCatalog()

	if err := cat.Register("g1", []byte(validGridDefJSON), nil); err != nil {
		t.Fatalf("initial Register() error = %v, want nil", err)
	}

	original, ok := cat.Get("g1")
	if !ok || original == nil {
		t.Fatalf("Get(%q) after successful Register = (%v, %v), want (non-nil, true)", "g1", original, ok)
	}

	// Now attempt to overwrite "g1" with a broken definition.
	err := cat.Register("g1", []byte(unknownPrimitiveDefJSON), nil)
	if err == nil {
		t.Fatalf("re-Register() with unknown primitive error = nil, want non-nil")
	}

	after, ok := cat.Get("g1")
	if !ok {
		t.Fatalf("Get(%q) after failed re-Register ok = false, want true (original entry must survive)", "g1")
	}
	if after == nil {
		t.Fatalf("Get(%q) after failed re-Register = nil, want the original CompiledGame", "g1")
	}

	// The surviving entry must still be the ORIGINAL valid compiled game,
	// not some partially-applied broken attempt.
	if !after.Board.HasPosition("0,0") {
		t.Errorf("after failed re-Register, Board.HasPosition(0,0) = false, want true (original grid board)")
	}
	stone, ok := after.Pieces["stone"]
	if !ok {
		t.Fatalf("after failed re-Register, Pieces[%q] missing (original piece lost)", "stone")
	}
	if stone.Occupancy != primitives.OccupancyCaptureOrEmpty {
		t.Errorf("after failed re-Register, Pieces[%q].Occupancy = %q, want %q (original piece definition)", "stone", stone.Occupancy, primitives.OccupancyCaptureOrEmpty)
	}
	if stone.Primitive == nil {
		t.Fatalf("after failed re-Register, Pieces[%q].Primitive = nil, want the original resolved step primitive", "stone")
	}
}

// --- 9. Get for a never-registered id ----------------------------------------

func TestGet_NeverRegisteredID_ReturnsNilFalse(t *testing.T) {
	cat := catalog.NewCatalog()

	cg, ok := cat.Get("does-not-exist")
	if ok {
		t.Errorf("Get(%q) ok = true, want false", "does-not-exist")
	}
	if cg != nil {
		t.Errorf("Get(%q) = %v, want nil", "does-not-exist", cg)
	}
}

func TestGet_NeverRegisteredID_OnNonEmptyCatalog_ReturnsNilFalse(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.Register("some-game", []byte(validGridDefJSON), nil); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	cg, ok := cat.Get("some-other-id-never-registered")
	if ok {
		t.Errorf("Get(%q) ok = true, want false", "some-other-id-never-registered")
	}
	if cg != nil {
		t.Errorf("Get(%q) = %v, want nil", "some-other-id-never-registered", cg)
	}
}

// --- 10. Re-registering the same id with a new valid definition overwrites --

func TestRegister_SameIDReregisteredWithNewValidDefinition_Overwrites(t *testing.T) {
	cat := catalog.NewCatalog()

	if err := cat.Register("g2", []byte(validExplicitDefJSON), nil); err != nil {
		t.Fatalf("first Register() error = %v, want nil", err)
	}
	first, ok := cat.Get("g2")
	if !ok || first == nil {
		t.Fatalf("Get(%q) after first Register = (%v, %v), want (non-nil, true)", "g2", first, ok)
	}
	if !first.Board.HasPosition("p1") {
		t.Fatalf("fixture error: expected first-registered board to have position p1")
	}

	if err := cat.Register("g2", []byte(validExplicitDefJSONAlt), nil); err != nil {
		t.Fatalf("second Register() error = %v, want nil", err)
	}
	second, ok := cat.Get("g2")
	if !ok || second == nil {
		t.Fatalf("Get(%q) after second Register = (%v, %v), want (non-nil, true)", "g2", second, ok)
	}

	// The newest definition's board must be in effect...
	if !second.Board.HasPosition("q1") {
		t.Errorf("after re-registering %q, Board.HasPosition(q1) = false, want true (newest definition)", "g2")
	}
	if !second.Board.HasPosition("q2") {
		t.Errorf("after re-registering %q, Board.HasPosition(q2) = false, want true (newest definition)", "g2")
	}
	// ...and the old definition's board/pieces must no longer be present.
	if second.Board.HasPosition("p1") {
		t.Errorf("after re-registering %q, Board.HasPosition(p1) = true, want false (stale entry from the old definition)", "g2")
	}
	if _, stillThere := second.Pieces["stone"]; stillThere {
		t.Errorf("after re-registering %q, Pieces[%q] still present, want it replaced by the new definition's pieces (%q)", "g2", "stone", "rock")
	}
	if _, hasRock := second.Pieces["rock"]; !hasRock {
		t.Errorf("after re-registering %q, Pieces[%q] missing, want it present from the new definition", "g2", "rock")
	}
}
