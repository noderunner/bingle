package app_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// chessFrom creates a game of a chess variant against a bot, from start
// (nil for the variant's own) with White to move, and opens it as Alice.
func chessFrom(t *testing.T, ctx context.Context, baseURL, id string, start map[string]any) string {
	t.Helper()
	body := map[string]any{"game_def_id": id, "player_name": "Alice", "opponent": "random"}
	if start != nil {
		body["start_state"], body["start_seat"] = start, 1
	}
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create %s: %d", id, code)
	}
	openGameAs(t, ctx, baseURL, created.RoomCode, "chess", "Alice")
	return created.RoomCode
}

func piece(color, kind string) []any {
	return []any{map[string]any{"piece": kind, "color": color}}
}

// The clicks of the variants that add one: a Crazyhouse drop from the hand,
// Chess960's castling by king and rook, and an Alice move onto the second
// board.
func TestWebChessVariants_ClicksOfTheirOwn(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok && e.ExceptionDetails != nil {
			mu.Lock()
			jsErrors = append(jsErrors, e.ExceptionDetails.Error())
			mu.Unlock()
		}
	})
	pieceOn := func(sq, kind string) string {
		return `(function () { var p = document.querySelector('#board .cell[data-position="` + sq + `"] .piece'); return p && /` + kind + `/.test(p.getAttribute('class') + ' ' + (p.getAttribute('data-piece') || '')); })()`
	}

	// Crazyhouse: a knight in hand dropped on f3.
	chessFrom(t, ctx, baseURL, "chess-crazyhouse", map[string]any{"v": 2,
		"cells":    map[string]any{"e1": piece("white", "king"), "e8": piece("black", "king"), "a2": piece("white", "pawn"), "h7": piece("black", "pawn")},
		"reserves": map[string]any{"white": map[string]any{"knight": 1}},
		"ext":      map[string]any{"turn": 1, "fullmove": 20},
	})
	waitJS(t, ctx, "the knight in hand", `document.querySelector('.hand-piece.choosable[data-piece="knight"]') !== null`, 15*time.Second)
	clickSel(t, ctx, `.hand-piece.choosable[data-piece="knight"]`)
	clickCellUntil(t, ctx, "f3", pieceOn("f3", "knight"), 10*time.Second)

	// Chess960: the king on g1 castles by clicking the rook on h1.
	chessFrom(t, ctx, baseURL, "chess-960", map[string]any{"v": 2,
		"cells": map[string]any{"g1": piece("white", "king"), "h1": piece("white", "rook"), "g8": piece("black", "king"), "a7": piece("black", "pawn")},
		"ext":   map[string]any{"turn": 1, "castling": "K", "fullmove": 20},
	})
	waitJS(t, ctx, "4 pieces", `document.querySelectorAll('#board .piece').length === 4`, 15*time.Second)
	clickCellUntil(t, ctx, "g1", `document.querySelectorAll('#board .cell.clickable').length > 0`, 10*time.Second)
	clickCellUntil(t, ctx, "h1", pieceOn("f1", "rook"), 10*time.Second)

	// Alice: two boards of 64 squares; the knight on g1 goes to f3 and ends on n3.
	chessFrom(t, ctx, baseURL, "chess-alice", nil)
	waitJS(t, ctx, "two boards", `document.querySelectorAll('#board .cell').length === 128`, 15*time.Second)
	clickCellUntil(t, ctx, "g1", `document.querySelectorAll('#board .cell.clickable').length > 0`, 10*time.Second)
	clickCellUntil(t, ctx, "n3", pieceOn("n3", "knight"), 10*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
