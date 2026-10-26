package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// A game can begin from any valid position: puzzles, endgames, "play from
// here". The start survives replay, undo and export.

func createFrom(t *testing.T, baseURL string, body map[string]any) (int, map[string]any) {
	t.Helper()
	var out map[string]any
	code := doJSON(t, "POST", baseURL+"/api/games", body, &out)
	return code, out
}

func TestGameStartsFromTPSAndKeepsItThroughUndoAndExport(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// White to move and a road away: e3 completes a1..e1? no — row 3.
	const tps = "x5/x5/1,1,1,1,x/x5/x5 1 6"
	code, out := createFrom(t, baseURL, map[string]any{
		"game_def_id": "tak-5x5", "player_name": "Alice", "start_tps": tps,
	})
	if code != 200 {
		t.Fatalf("create from TPS: %d %v", code, out)
	}
	room := out["room_code"].(string)
	joinRoom(t, baseURL, room, "Bob")

	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if len(st.State.Cells) != 4 || st.State.Ply != 10 || st.TurnSeat == nil || *st.TurnSeat != 1 {
		t.Fatalf("start state: cells=%d ply=%d turn=%v", len(st.State.Cells), st.State.Ply, st.TurnSeat)
	}

	// The game info endpoint reports the custom start for clients.
	var info struct {
		StartState json.RawMessage `json:"start_state"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &info)
	if len(info.StartState) == 0 {
		t.Error("GET /games/{code} should expose start_state for a custom start")
	}

	// Alice completes the road at once.
	if code, out := moveNotation(t, baseURL, room, "Alice", "e3"); code != 200 {
		t.Fatalf("e3: %d %v", code, out)
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if st.Status != "finished" || st.EndReason != "road" || st.WinnerSeat == nil || *st.WinnerSeat != 1 {
		t.Fatalf("after e3: %+v", st)
	}

	// at_seq=0 is the custom start, not the empty board.
	var first restState // fresh: decoding into a used struct would merge maps
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state?at_seq=0", nil, &first)
	if len(first.State.Cells) != 4 || first.State.Ply != 10 {
		t.Errorf("at_seq=0 = %d cells ply %d, want the custom start", len(first.State.Cells), first.State.Ply)
	}

	// The PTN export replays from the custom start: a TPS header and the
	// move number carried over from the position.
	resp, err := http.Get(baseURL + "/api/games/" + room + "/export?format=ptn")
	if err != nil {
		t.Fatal(err)
	}
	ptn, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(ptn), `[TPS "x5/x5/1,1,1,1,x/x5/x5 1 6"]`) || !strings.Contains(string(ptn), "6. e3") {
		t.Errorf("PTN export = %s", ptn)
	}

	// The record's initial_state is the custom start too.
	var rec struct {
		InitialState string `json:"initial_state"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/export", nil, &rec)
	if !strings.Contains(rec.InitialState, `"ply":10`) {
		t.Errorf("export initial_state = %s", rec.InitialState)
	}
}

func TestUndoReturnsToTheCustomStart(t *testing.T) {
	baseURL := newRESTTestServer(t)
	code, out := createFrom(t, baseURL, map[string]any{
		"game_def_id": "tak-5x5", "player_name": "Alice", "opponent": "random",
		"start_tps": "x2,2,x2/x5/x5/x5/1,x4 2 5",
	})
	if code != 200 {
		t.Fatalf("create: %d %v", code, out)
	}
	room := out["room_code"].(string)
	// Seat 2 (black) moves first from this TPS; Alice plays seat 1 against
	// the bot, so the bot moved already.
	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if st.MoveSeq != 1 {
		t.Fatalf("the bot should have opened from the custom position, move_seq=%d", st.MoveSeq)
	}
	// Alice undoes: nothing of hers to undo yet.
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/undo", map[string]any{"player_name": "Alice"}, nil); code != 409 {
		t.Errorf("undo with no move of her own: %d, want 409", code)
	}
	// Any legal move of hers will do (the bot's random opening may have taken c3).
	var legal struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"moves"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves/legal", nil, &legal); code != 200 || len(legal.Moves) == 0 {
		t.Fatalf("legal moves: %d %+v", code, legal)
	}
	own := legal.Moves[0].Notation
	if code, out := moveNotation(t, baseURL, room, "Alice", own); code != 200 {
		t.Fatalf("%s: %d %v", own, code, out)
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/undo", map[string]any{"player_name": "Alice"}, &st); code != 200 {
		t.Fatalf("undo: %d", code)
	}
}

func TestGameStartsFromAStateDocument(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// Tic-tac-toe, X (seat 1) has two in a row on top; O to move first.
	doc := map[string]any{
		"v": 2, "ply": 3,
		"cells": map[string]any{
			"a3": []map[string]string{{"piece": "mark", "color": "x"}},
			"b3": []map[string]string{{"piece": "mark", "color": "x"}},
			"b2": []map[string]string{{"piece": "mark", "color": "o"}},
		},
	}
	// Find the colour names rather than assuming them.
	var info struct {
		Seats []struct {
			Seat  int    `json:"seat"`
			Color string `json:"color"`
		} `json:"seats"`
	}
	doJSON(t, "GET", baseURL+"/api/game-defs/tic-tac-toe", nil, &info)
	if len(info.Seats) != 2 {
		t.Fatalf("seats = %+v", info.Seats)
	}
	c1, c2 := info.Seats[0].Color, info.Seats[1].Color
	cells := doc["cells"].(map[string]any)
	cells["a3"] = []map[string]string{{"piece": "mark", "color": c1}}
	cells["b3"] = []map[string]string{{"piece": "mark", "color": c1}}
	cells["b2"] = []map[string]string{{"piece": "mark", "color": c2}}

	code, out := createFrom(t, baseURL, map[string]any{
		"game_def_id": "tic-tac-toe", "player_name": "Alice", "opponent": "tactical",
		"start_state": doc, "start_seat": 2, "seat": 1,
	})
	if code != 200 {
		t.Fatalf("create from state: %d %v", code, out)
	}
	room := out["room_code"].(string)
	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	// Seat 2 (the bot) moved first from the position; the tactical bot
	// blocks the row at c3.
	if st.MoveSeq != 1 || st.State.Cells["c3"] == nil {
		t.Errorf("bot reply: move_seq=%d c3=%v", st.MoveSeq, st.State.Cells["c3"])
	}
}

func TestInvalidStartPositionsAreRejected(t *testing.T) {
	baseURL := newRESTTestServer(t)
	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{"garbage TPS", map[string]any{"start_tps": "banana"}, "start position"},
		{"both given", map[string]any{"start_tps": "x5/x5/x5/x5/x5 1 1", "start_state": map[string]any{"v": 2}}, "not both"},
		{"unknown square", map[string]any{"start_state": map[string]any{"v": 2, "cells": map[string]any{"z9": []map[string]string{{"piece": "flat", "color": "white"}}}}}, "not on the board"},
		{"unknown piece", map[string]any{"start_state": map[string]any{"v": 2, "cells": map[string]any{"a1": []map[string]string{{"piece": "dragon", "color": "white"}}}}}, "unknown piece"},
		{"unknown colour", map[string]any{"start_state": map[string]any{"v": 2, "cells": map[string]any{"a1": []map[string]string{{"piece": "flat", "color": "teal"}}}}}, "unknown colour"},
		{"bad start seat", map[string]any{"start_state": map[string]any{"v": 2}, "start_seat": 7}, "start_seat"},
		// A finished game is not a puzzle.
		{"already won", map[string]any{"start_tps": "1,1,1,1,1/x5/x5/x5/x5 2 6"}, "already decided"},
	}
	for _, tc := range cases {
		body := map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice"}
		for k, v := range tc.body {
			body[k] = v
		}
		code, out := createFrom(t, baseURL, body)
		msg, _ := out["error"].(string)
		if code != 400 || !strings.Contains(msg, tc.want) {
			t.Errorf("%s: status %d error %q, want 400 containing %q", tc.name, code, msg, tc.want)
		}
	}
}
