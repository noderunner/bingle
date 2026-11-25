package app_test

import (
	"testing"
)

// A game played on two boards says which positions are on each (Alice
// chess); a game on one board says nothing of boards.
func TestRESTGameDef_BoardsOfAliceChess(t *testing.T) {
	baseURL := newRESTTestServer(t)
	type def struct {
		Board struct {
			Width  int `json:"width"`
			Boards []struct {
				Positions []string `json:"positions"`
			} `json:"boards"`
		} `json:"board"`
	}
	var alice def
	if code := doJSON(t, "GET", baseURL+"/api/game-defs/chess-alice", nil, &alice); code != 200 {
		t.Fatalf("GET chess-alice: %d", code)
	}
	b := alice.Board
	if b.Width != 16 || len(b.Boards) != 2 || len(b.Boards[0].Positions) != 64 || len(b.Boards[1].Positions) != 64 {
		t.Fatalf("chess-alice's board: width %d, %d boards", b.Width, len(b.Boards))
	}
	on := func(i int, id string) bool {
		for _, p := range b.Boards[i].Positions {
			if p == id {
				return true
			}
		}
		return false
	}
	if !on(0, "a1") || !on(0, "h8") || on(0, "i1") || !on(1, "i1") || !on(1, "p8") {
		t.Errorf("board A is not a to h and board B i to p: %v", b.Boards)
	}
	var chess def
	if code := doJSON(t, "GET", baseURL+"/api/game-defs/chess", nil, &chess); code != 200 {
		t.Fatalf("GET chess: %d", code)
	}
	if chess.Board.Boards != nil {
		t.Errorf("chess lists boards: %v", chess.Board.Boards)
	}
}
