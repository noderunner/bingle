// Tests that a seat's legal moves, which can name its hand, go only to whoever holds the seat (ADR 0112).

package app_test

import (
	"fmt"
	"strings"
	"testing"
)

type secrecyLegal struct {
	Seat  int `json:"seat"`
	Moves []struct {
		Notation string `json:"notation"`
	} `json:"options"`
}

// tricksAtTheCards seats key in all four seats of a game of the Tricks fixture and plays until a seat to move has
// a choice of cards from its hand, the moves that would give the hand away.
func tricksAtTheCards(t *testing.T, base, key string) (string, secrecyLegal) {
	t.Helper()
	var created createGameResponse
	if code := doJSONAs(t, key, "POST", base+"/api/games", map[string]any{"game_def_id": "test-tricks", "player_name": "Seat 1", "seat": 1}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	for seat := 2; seat <= 4; seat++ {
		if code := doJSONAs(t, key, "POST", base+"/api/games/"+room+"/join", map[string]any{"player_name": fmt.Sprintf("Seat %d", seat), "seat": seat}, nil); code != 200 {
			t.Fatalf("join seat %d: %d", seat, code)
		}
	}
	for i := 0; i < 100; i++ {
		var legal secrecyLegal
		if code := doJSONAs(t, key, "GET", base+"/api/games/"+room+"/selections", nil, &legal); code != 200 {
			t.Fatalf("legal: %d", code)
		}
		if len(legal.Moves) == 0 {
			t.Fatalf("seat %d to move has no moves", legal.Seat)
		}
		// Everything before the first card is the board of terms ("cross:").
		if n := legal.Moves[0].Notation; !strings.HasPrefix(n, "cross:") {
			return room, legal
		}
		body := map[string]any{"player_name": fmt.Sprintf("Seat %d", legal.Seat), "notation": legal.Moves[0].Notation}
		if code := doJSONAs(t, key, "POST", base+"/api/games/"+room+"/moves", body, nil); code != 200 {
			t.Fatalf("move %q: %d", legal.Moves[0].Notation, code)
		}
	}
	t.Fatal("never reached a choice of cards")
	return "", secrecyLegal{}
}

func TestLegalMoves_AHandIsShownOnlyToWhoeverHoldsTheSeat(t *testing.T) {
	base := newRESTTestServer(t)
	owner, stranger := newGuestKey(t, base), newGuestKey(t, base)
	room, held := tricksAtTheCards(t, base, owner)
	url := base + "/api/games/" + room + "/selections"

	for who, key := range map[string]string{"nobody signed in": "", "another guest": stranger} {
		for _, query := range []string{"", fmt.Sprintf("?seat=%d", held.Seat)} {
			var seen secrecyLegal
			if code := doJSONAs(t, key, "GET", url+query, nil, &seen); code != 200 {
				t.Fatalf("%s, %q: %d", who, query, code)
			}
			if len(seen.Moves) != 0 {
				t.Errorf("%s, %q: sees seat %d's moves: %+v", who, query, held.Seat, seen.Moves)
			}
			if seen.Seat != held.Seat {
				t.Errorf("%s, %q: told seat %d is to move, want %d", who, query, seen.Seat, held.Seat)
			}
		}
	}

	var again secrecyLegal
	if code := doJSONAs(t, owner, "GET", url, nil, &again); code != 200 || len(again.Moves) != len(held.Moves) {
		t.Errorf("the seat's own holder: %d, %d moves, want %d", code, len(again.Moves), len(held.Moves))
	}

	// MCP is the other way in, and is anonymous here.
	res := callTool(t, newMCPClient(t, base), "list_selections", map[string]any{"room_code": room})
	var seen secrecyLegal
	decodeToolResult(t, res, &seen)
	if len(seen.Moves) != 0 {
		t.Errorf("MCP's list_selections shows seat %d's moves: %+v", held.Seat, seen.Moves)
	}
}

// Asking must not make a seat nobody owns the property of whoever asked.
func TestLegalMoves_AskingDoesNotTakeASeatNobodyOwns(t *testing.T) {
	base := newRESTTestServer(t)
	var created createGameResponse
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-tricks", "player_name": "Seat 1", "seat": 1}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	for seat := 2; seat <= 4; seat++ {
		if code := doJSON(t, "POST", base+"/api/games/"+room+"/join", map[string]any{"player_name": fmt.Sprintf("Seat %d", seat), "seat": seat}, nil); code != 200 {
			t.Fatalf("join seat %d: %d", seat, code)
		}
	}
	var legal secrecyLegal
	if code := doJSONAs(t, newGuestKey(t, base), "GET", base+"/api/games/"+room+"/selections", nil, &legal); code != 200 || len(legal.Moves) == 0 {
		t.Fatalf("a seat nobody owns hides nothing: %d, %d moves", code, len(legal.Moves))
	}
	body := map[string]any{"player_name": fmt.Sprintf("Seat %d", legal.Seat), "notation": legal.Moves[0].Notation}
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/moves", body, nil); code != 200 {
		t.Errorf("the seat's anonymous player can no longer move after a stranger asked: %d", code)
	}
}

// A game with nothing to hide still shows its moves to anybody, for analysis and spectators.
func TestLegalMoves_AnOpenGameShowsThemToAnybody(t *testing.T) {
	base := newRESTTestServer(t)
	key := newGuestKey(t, base)
	var created createGameResponse
	if code := doJSONAs(t, key, "POST", base+"/api/games", map[string]any{"game_def_id": "gomoku", "player_name": "Seat 1", "seat": 1}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if code := doJSONAs(t, key, "POST", base+"/api/games/"+created.RoomCode+"/join", map[string]any{"player_name": "Seat 2", "seat": 2}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	var legal secrecyLegal
	if code := doJSON(t, "GET", base+"/api/games/"+created.RoomCode+"/selections", nil, &legal); code != 200 || len(legal.Moves) == 0 {
		t.Errorf("an open game: %d, %d moves", code, len(legal.Moves))
	}
}
