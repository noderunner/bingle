// Tests of the JSON game-definition parser: the required fields, unknown fields, and variants.

package dsl_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

// validFixtureJSON mirrors docs/history/plan.md section 6.1's annotated example
// game-definition JSON verbatim (minus the jsonc comments, which are not
// valid JSON), including an explicit empty "callbacks" object.
const validFixtureJSON = `{
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
  "setup": [
    { "position": "0,0", "piece": "stone", "color": "black" },
    { "position": "8,8", "piece": "stone", "color": "white" }
  ],
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" },
  "callbacks": {}
}`

// wantFixture is the GameDefinition validFixtureJSON should decode to.
func wantFixture() *dsl.GameDefinition {
	return &dsl.GameDefinition{
		ID:   "demo-freeplay",
		Name: "Demo: Free Movement & Capture",
		Board: dsl.Board{
			Generator: &dsl.BoardGenerator{
				Type:      "grid",
				Width:     9,
				Height:    9,
				Adjacency: "orthogonal4",
			},
		},
		Seats: []dsl.SeatDef{
			{Seat: 1, Color: "black"},
			{Seat: 2, Color: "white"},
		},
		Pieces: map[string]dsl.PieceDef{
			"stone": {
				Movement: dsl.MovementDef{
					Primitive: "step",
					Occupancy: "capture-or-empty",
				},
			},
		},
		Setup: []dsl.SetupEntry{
			{Position: "0,0", Piece: "stone", Color: "black"},
			{Position: "8,8", Piece: "stone", Color: "white"},
		},
		TurnOrder: dsl.TurnOrderDef{
			Type:  "round-robin",
			Seats: []int{1, 2},
		},
		WinCondition: dsl.WinConditionDef{Type: "none"},
		Callbacks:    dsl.CallbackRefs{},
	}
}

// TestParse_HappyPath verifies backlog item 01's acceptance criterion
// directly: parsing a well-formed fixture shaped like plan.md 6.1's example
// returns no error and populates every field of GameDefinition to match the
// fixture.
func TestParse_HappyPath(t *testing.T) {
	got, err := dsl.Parse([]byte(validFixtureJSON))
	if err != nil {
		t.Fatalf("Parse() returned unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("Parse() returned nil *GameDefinition with nil error")
	}

	want := wantFixture()

	// Whole-struct comparison first for a clear top-level signal...
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() result mismatch.\n got: %#v\nwant: %#v", got, want)
	}

	// ...then field-by-field so a failure pinpoints exactly which part of
	// the fixture didn't round-trip.
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.Board.Generator == nil {
		t.Fatal("Board.Generator = nil, want non-nil")
	}
	if *got.Board.Generator != *want.Board.Generator {
		t.Errorf("Board.Generator = %+v, want %+v", *got.Board.Generator, *want.Board.Generator)
	}
	if len(got.Board.Positions) != 0 || len(got.Board.Edges) != 0 {
		t.Errorf("Board.Positions/Edges = %v/%v, want empty (generator-based board)", got.Board.Positions, got.Board.Edges)
	}
	if !reflect.DeepEqual(got.Seats, want.Seats) {
		t.Errorf("Seats = %+v, want %+v", got.Seats, want.Seats)
	}
	if !reflect.DeepEqual(got.Pieces, want.Pieces) {
		t.Errorf("Pieces = %+v, want %+v", got.Pieces, want.Pieces)
	}
	stone, ok := got.Pieces["stone"]
	if !ok {
		t.Fatal(`Pieces["stone"] missing`)
	}
	if stone.Movement.Primitive != "step" {
		t.Errorf("Pieces[stone].Movement.Primitive = %q, want %q", stone.Movement.Primitive, "step")
	}
	if stone.Movement.Occupancy != "capture-or-empty" {
		t.Errorf("Pieces[stone].Movement.Occupancy = %q, want %q", stone.Movement.Occupancy, "capture-or-empty")
	}
	if !reflect.DeepEqual(got.Setup, want.Setup) {
		t.Errorf("Setup = %+v, want %+v", got.Setup, want.Setup)
	}
	if got.TurnOrder.Type != "round-robin" {
		t.Errorf("TurnOrder.Type = %q, want %q", got.TurnOrder.Type, "round-robin")
	}
	if !reflect.DeepEqual(got.TurnOrder.Seats, []int{1, 2}) {
		t.Errorf("TurnOrder.Seats = %v, want %v", got.TurnOrder.Seats, []int{1, 2})
	}
	if got.WinCondition.Type != "none" {
		t.Errorf("WinCondition.Type = %q, want %q", got.WinCondition.Type, "none")
	}
	if !reflect.DeepEqual(got.Callbacks, dsl.CallbackRefs{}) {
		t.Errorf("Callbacks = %+v, want empty CallbackRefs{}", got.Callbacks)
	}
}

// fixtureMap decodes validFixtureJSON into a generic map so individual keys
// can be deleted or overwritten to build "missing field" / "blank field"
// fixtures without duplicating the whole document per case.
func fixtureMap(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(validFixtureJSON), &m); err != nil {
		t.Fatalf("failed to decode base fixture into map: %v", err)
	}
	return m
}

func mustMarshal(t *testing.T, m map[string]json.RawMessage) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("failed to re-marshal fixture map: %v", err)
	}
	return b
}

func rawJSON(t *testing.T, v string) json.RawMessage {
	t.Helper()
	return json.RawMessage(v)
}

// TestParse_MissingRequiredField_KeyAbsent covers backlog item 01's edge
// case: for each of the 7 required top-level fields, a fixture with that
// key entirely absent (all other fields left valid) must yield a non-nil
// error from Parse.
func TestParse_MissingRequiredField_KeyAbsent(t *testing.T) {
	requiredFields := []string{"id", "name", "board", "seats", "pieces", "turn_order", "win_condition"}

	for _, field := range requiredFields {
		t.Run(field, func(t *testing.T) {
			m := fixtureMap(t)
			if _, ok := m[field]; !ok {
				t.Fatalf("test setup bug: base fixture has no key %q to remove", field)
			}
			delete(m, field)

			got, err := dsl.Parse(mustMarshal(t, m))
			if err == nil {
				t.Errorf("Parse() with %q key absent: expected non-nil error, got nil error and result %#v", field, got)
			}
			if got != nil {
				t.Errorf("Parse() with %q key absent: expected nil *GameDefinition on error, got %#v", field, got)
			}
		})
	}
}

// TestParse_MissingRequiredField_Blank covers the "present but blank/empty"
// variant of the same edge case: for each of the 7 required top-level
// fields, a fixture where that key is present but holds a blank string, an
// empty slice, an empty map, or an empty object (as appropriate to the
// field's type) must yield a non-nil error from Parse.
func TestParse_MissingRequiredField_Blank(t *testing.T) {
	cases := []struct {
		field string
		blank string // JSON literal to substitute for the field's value
	}{
		{"id", `""`},
		{"name", `""`},
		{"board", `{}`},
		{"seats", `[]`},
		{"pieces", `{}`},
		{"turn_order", `{"type": "", "seats": []}`},
		{"win_condition", `{"type": ""}`},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			m := fixtureMap(t)
			if _, ok := m[tc.field]; !ok {
				t.Fatalf("test setup bug: base fixture has no key %q to blank out", tc.field)
			}
			m[tc.field] = rawJSON(t, tc.blank)

			got, err := dsl.Parse(mustMarshal(t, m))
			if err == nil {
				t.Errorf("Parse() with %q blanked to %s: expected non-nil error, got nil error and result %#v", tc.field, tc.blank, got)
			}
			if got != nil {
				t.Errorf("Parse() with %q blanked to %s: expected nil *GameDefinition on error, got %#v", tc.field, tc.blank, got)
			}
		})
	}
}

// TestParse_AllOtherFieldsValid_OnlyOneFieldMissing double-checks that the
// error path in the two tests above is genuinely triggered by the single
// offending field, not by some other accidental invalidity: it re-parses
// the full valid fixture unmodified (a control) and confirms *that*
// succeeds, so the missing/blank variants' failures can only be attributed
// to the field under test.
func TestParse_AllOtherFieldsValid_OnlyOneFieldMissing(t *testing.T) {
	if _, err := dsl.Parse([]byte(validFixtureJSON)); err != nil {
		t.Fatalf("control: Parse() of unmodified valid fixture returned unexpected error: %v", err)
	}
}

// TestParse_MalformedJSON covers backlog item 01's third edge case: input
// that is not valid JSON at all must produce a non-nil error, not a panic
// or a zero-value result.
func TestParse_MalformedJSON(t *testing.T) {
	malformedInputs := map[string]string{
		"truncated_object":    `{"id": "demo-freeplay", "name": "Demo"`,
		"trailing_comma":      `{"id": "demo-freeplay",}`,
		"not_json_at_all":     `this is not json`,
		"empty_input":         ``,
		"bare_scalar":         `42`,
		"unterminated_string": `{"id": "demo-freeplay`,
	}

	for name, input := range malformedInputs {
		t.Run(name, func(t *testing.T) {
			got, err := dsl.Parse([]byte(input))
			if err == nil {
				t.Errorf("Parse(%q): expected non-nil error, got nil error and result %#v", input, got)
			}
			if got != nil {
				t.Errorf("Parse(%q): expected nil *GameDefinition on error, got %#v", input, got)
			}
		})
	}
}
