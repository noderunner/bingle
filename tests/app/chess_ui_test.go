package app_test

import (
	"sync"
	"testing"
	"time"
)

// The UI can play chess: the board shows the 32 pieces, two clicks move a
// pawn, the bot answers, and a pawn on the last rank becomes the piece its
// button names.
func TestWebChess_MovesAndPromotesThroughTheUI(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "chess", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, "chess", "Alice")
	waitJS(t, ctx, "board", `document.querySelectorAll('#board .cell').length === 64`, 15*time.Second)
	waitJS(t, ctx, "32 pieces", `document.querySelectorAll('#board .piece').length === 32`, 10*time.Second)
	clickCellUntil(t, ctx, "e2", `document.querySelectorAll('#board .cell.clickable').length > 0`, 10*time.Second)
	clickCell(t, ctx, "e4")
	for deadline := time.Now().Add(20 * time.Second); cfGet(t, baseURL, created.RoomCode).MoveSeq < 2; time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d moves, want e2-e4 and the answer", cfGet(t, baseURL, created.RoomCode).MoveSeq)
		}
	}
	waitJS(t, ctx, "a pawn on e4", `document.querySelector('#board .cell[data-position="e4"] .piece') !== null`, 10*time.Second)

	// A pawn about to promote, with White to move.
	start := map[string]any{"v": 2, "cells": map[string]any{
		"e1": []any{map[string]any{"piece": "king", "color": "white"}},
		"b7": []any{map[string]any{"piece": "pawn", "color": "white"}},
		"h8": []any{map[string]any{"piece": "king", "color": "black"}},
	}, "ext": map[string]any{"turn": 1, "fullmove": 40}}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "chess", "player_name": "Alice", "opponent": "random", "start_state": start, "start_seat": 1}, &created); code != 200 {
		t.Fatalf("create from a position: %d", code)
	}
	openGameAs(t, ctx, baseURL, created.RoomCode, "chess", "Alice")
	waitJS(t, ctx, "3 pieces", `document.querySelectorAll('#board .piece').length === 3`, 15*time.Second)
	clickCellUntil(t, ctx, "b7", `document.querySelectorAll('#board .cell.clickable').length > 0`, 10*time.Second)
	clickCell(t, ctx, "b8")
	waitJS(t, ctx, "the four pieces to choose", `document.querySelectorAll('.action-buttons button').length === 4`, 10*time.Second)
	clickSel(t, ctx, `.action-buttons button[data-action="knight"]`)
	waitJS(t, ctx, "a knight on b8", `(function () { var p = document.querySelector('#board .cell[data-position="b8"] .piece'); return p && /knight/.test(p.getAttribute('class') + ' ' + (p.getAttribute('data-piece') || '')); })()`, 10*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
