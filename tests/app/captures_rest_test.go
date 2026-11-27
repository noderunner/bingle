package app_test

import (
	"slices"
	"testing"
)

// A legal selection says which stones it would capture, so a client can show
// them before the move is confirmed, and a played move lists what it took
// (#269): on Go, black's a8 takes the white stone in the a9 corner.
func TestREST_SelectionsAndMovesListWhatTheyCapture(t *testing.T) {
	base := newRESTTestServer(t)
	ana, bob := newGuestKey(t, base), newGuestKey(t, base)
	var created createGameResponse
	if code := doJSONAs(t, ana, "POST", base+"/api/games", map[string]any{"game_def_id": "go-9x9", "player_name": "Ana"}, &created); code != 200 || created.Seat != 1 {
		t.Fatalf("create: %d %+v", code, created)
	}
	game := base + "/api/games/" + created.RoomCode
	if code := doJSONAs(t, bob, "POST", game+"/join", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	play := func(key, name, notation string) {
		t.Helper()
		var out map[string]any
		if code := doJSONAs(t, key, "POST", game+"/moves", map[string]any{"player_name": name, "notation": notation}, &out); code != 200 {
			t.Fatalf("%s plays %s: %d %v", name, notation, code, out)
		}
	}
	type option struct {
		Notation string   `json:"notation"`
		Captures []string `json:"captures"`
	}
	find := func(opts []option, notation string) option {
		t.Helper()
		for _, o := range opts {
			if o.Notation == notation {
				return o
			}
		}
		t.Fatalf("%s is not among the options", notation)
		return option{}
	}
	play(ana, "Ana", "b9")
	play(bob, "Bob", "a9")

	var sel struct {
		Options []option `json:"options"`
	}
	if code := doJSONAs(t, ana, "GET", game+"/selections", nil, &sel); code != 200 {
		t.Fatalf("selections: %d", code)
	}
	if o := find(sel.Options, "a8"); !slices.Equal(o.Captures, []string{"a9"}) {
		t.Errorf("a8 would capture %v, want [a9]", o.Captures)
	}
	if o := find(sel.Options, "e5"); o.Captures != nil {
		t.Errorf("e5 would capture %v, want nothing", o.Captures)
	}
	var sim struct {
		Legal []option `json:"legal_moves"`
	}
	if code := doJSONAs(t, ana, "POST", game+"/simulate", map[string]any{"moves": []string{"c9", "pass"}}, &sim); code != 200 {
		t.Fatalf("simulate: %d", code)
	}
	if o := find(sim.Legal, "a8"); !slices.Equal(o.Captures, []string{"a9"}) {
		t.Errorf("simulated, a8 would capture %v, want [a9]", o.Captures)
	}

	play(ana, "Ana", "a8")
	var moves []struct {
		Notation string   `json:"notation"`
		Captured *string  `json:"captured"`
		Captures []string `json:"captures"`
	}
	if code := doJSONAs(t, ana, "GET", game+"/moves", nil, &moves); code != 200 || len(moves) != 3 {
		t.Fatalf("moves: %d, %d of them", code, len(moves))
	}
	last := moves[2]
	if last.Captured == nil || *last.Captured != "a9" || !slices.Equal(last.Captures, []string{"a9"}) {
		t.Errorf("a8 is recorded as capturing %v / %v, want a9 / [a9]", last.Captured, last.Captures)
	}
	if moves[0].Captures != nil {
		t.Errorf("b9 is recorded as capturing %v", moves[0].Captures)
	}
}

// A legal selection says which pieces it would turn over (#293): Othello's
// opening d3 converts the white disc on d4, and simulate's legal moves say
// the same.
func TestREST_SelectionsListWhatTheyConvert(t *testing.T) {
	base := newRESTTestServer(t)
	ana, bob := newGuestKey(t, base), newGuestKey(t, base)
	var created createGameResponse
	if code := doJSONAs(t, ana, "POST", base+"/api/games", map[string]any{"game_def_id": "othello", "player_name": "Ana"}, &created); code != 200 || created.Seat != 1 {
		t.Fatalf("create: %d %+v", code, created)
	}
	game := base + "/api/games/" + created.RoomCode
	if code := doJSONAs(t, bob, "POST", game+"/join", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	type option struct {
		Notation string   `json:"notation"`
		Captures []string `json:"captures"`
		Converts []string `json:"converts"`
	}
	find := func(opts []option, notation string) option {
		t.Helper()
		for _, o := range opts {
			if o.Notation == notation {
				return o
			}
		}
		t.Fatalf("%s is not among the options", notation)
		return option{}
	}
	var sel struct {
		Options []option `json:"options"`
	}
	if code := doJSONAs(t, ana, "GET", game+"/selections", nil, &sel); code != 200 {
		t.Fatalf("selections: %d", code)
	}
	if o := find(sel.Options, "d3"); !slices.Equal(o.Converts, []string{"d4"}) || o.Captures != nil {
		t.Errorf("d3 would convert %v and capture %v, want [d4] and nothing", o.Converts, o.Captures)
	}
	var sim struct {
		Legal []option `json:"legal_moves"`
	}
	if code := doJSONAs(t, ana, "POST", game+"/simulate", map[string]any{"moves": []string{"d3"}}, &sim); code != 200 {
		t.Fatalf("simulate: %d", code)
	}
	// After d3, white's c3 outflanks d4 (now black) along the diagonal to e5.
	if o := find(sim.Legal, "c3"); !slices.Equal(o.Converts, []string{"d4"}) {
		t.Errorf("simulated, c3 would convert %v, want [d4]", o.Converts)
	}
}
