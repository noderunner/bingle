package app_test

import (
	"testing"
	"time"
)

// The Council fixture (tests/testdata/council) plays through the API: a table
// of four, Object offered to the house that may use it, and the reaction
// settings listing it.
func TestCouncil_PlaysThroughTheAPI(t *testing.T) {
	base := newRESTTestServer(t)
	var created createGameResponse
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Alice"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	for _, name := range []string{"Bob", "Cara", "Dan"} {
		joinRoom(t, base, created.RoomCode, name)
	}
	var settings struct {
		Abilities []struct {
			Name string `json:"name"`
			Held bool   `json:"held"`
		} `json:"abilities"`
	}
	if code := doJSON(t, "GET", base+"/api/games/"+created.RoomCode+"/reactions?player_name=Alice", nil, &settings); code != 200 || len(settings.Abilities) != 1 || settings.Abilities[0].Name != "Object" {
		t.Fatalf("Alice's reactions: %d %+v", code, settings)
	}
	if code, out := moveNotation(t, base, created.RoomCode, "Alice", "claim@north"); code != 200 {
		t.Fatalf("claim: %d %v", code, out)
	}
	if code, out := moveNotation(t, base, created.RoomCode, "Bob", "go@north"); code != 200 {
		t.Fatalf("Bob's move: %d %v", code, out)
	}
	// Bob's move ended where Alice's envoy stands: she is asked, by default.
	var pending struct {
		Pending *struct {
			Ability  string `json:"ability"`
			Targeted bool   `json:"targeted"`
		} `json:"pending"`
	}
	doJSON(t, "GET", base+"/api/games/"+created.RoomCode+"/reactions?player_name=Alice", nil, &pending)
	if pending.Pending == nil || pending.Pending.Ability != "Object" || !pending.Pending.Targeted {
		t.Fatalf("Alice should be asked: %+v", pending.Pending)
	}
	if code, out := moveNotation(t, base, created.RoomCode, "Alice", "object"); code != 200 {
		t.Fatalf("object: %d %v", code, out)
	}
}

// The page draws the fixture's map of areas without a page script of the
// game's own (ADR 0124): its areas as the graph of their anchors, clickable.
func TestWebCouncil_TheMapIsDrawnWithoutAScript(t *testing.T) {
	base := newRESTTestServer(t)
	var created createGameResponse
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Alice", "seat_bots": map[string]string{"2": "random", "3": "random", "4": "random"}}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, base, created.RoomCode, "test-council", "Alice")
	waitJS(t, ctx, "five areas", `document.querySelectorAll('#board .cell').length === 5`, 15*time.Second)
	waitJS(t, ctx, "a move to make", `document.querySelectorAll('#board .cell.clickable, .action-buttons button').length > 0`, 10*time.Second)
}
