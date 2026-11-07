package app_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// A person may play every seat of a table: each seat besides their own is a
// player of its own, named after them and the seat, so the game still has
// someone to move for every seat — a mock game alone.

type soloInfo struct {
	Status      string `json:"status"`
	ImpulseSeat int    `json:"impulse_seat"`
	Seats       []struct {
		Seat  int    `json:"seat"`
		Name  string `json:"name"`
		Label string `json:"label"`
		Bot   string `json:"bot"`
	} `json:"seats"`
}

func TestSoloTwoPlayerGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
		Seat     int    `json:"seat"`
		Status   string `json:"status"`
	}
	body := map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice", "also_seats": []int{2}}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if created.Status != "active" {
		t.Fatalf("status %q: a game with every seat taken starts at once", created.Status)
	}
	var info soloInfo
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &info)
	if len(info.Seats) != 2 || info.Seats[0].Name != "Alice" || !strings.HasPrefix(info.Seats[1].Name, "Alice · ") || info.Seats[1].Bot != "" {
		t.Fatalf("seats %+v", info.Seats)
	}
	// Each alias moves for its own seat; the creator's own name moves seat 1.
	moves := []struct {
		who, n string
	}{{"Alice", "b2"}, {info.Seats[1].Name, "a1"}, {"Alice", "a3"}}
	for _, m := range moves {
		if code, out := moveNotation(t, baseURL, created.RoomCode, m.who, m.n); code != 200 {
			t.Fatalf("%s plays %s: %d %v", m.who, m.n, code, out)
		}
	}
	// The wrong name is still refused: it is seat 2's turn.
	if code, _ := moveNotation(t, baseURL, created.RoomCode, "Alice", "c3"); code != 403 {
		t.Errorf("seat 1 moved out of turn: %d", code)
	}
	// Playing both sides does not change the rule: a move stands once the turn has
	// passed, even to oneself.
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/undo", map[string]any{"player_name": "Alice"}, nil); code != 409 {
		t.Errorf("undo: %d, want 409", code)
	}
}

// A person may play several seats of a table, with bots in others.
func TestSoloTableOfSeveralSeatsWithABot(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
	}
	body := map[string]any{
		"game_def_id": "test-council", "player_name": "Alice", "also_seats": []int{3}, "seat_bots": map[string]string{"2": "random", "4": "random"},
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if created.Status != "active" {
		t.Fatalf("status %q", created.Status)
	}
	var info soloInfo
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &info)
	if info.Seats[1].Bot != "random" || info.Seats[2].Name != "Alice · Wolf" {
		t.Fatalf("seats %+v", info.Seats)
	}
	// An unknown seat is refused.
	bad := map[string]any{"game_def_id": "test-council", "player_name": "Bob", "also_seats": []int{9}}
	if code := doJSON(t, "POST", baseURL+"/api/games", bad, nil); code != 400 {
		t.Errorf("seat 9: %d, want 400", code)
	}
}

func TestSoloTableWaitsInTheLobbyUntilStarted(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
	}
	body := map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice", "also_seats": []int{2}, "lobby": true}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if created.Status != "waiting" {
		t.Fatalf("status %q, want waiting for the host", created.Status)
	}
	var started struct {
		Status string `json:"status"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/start", map[string]any{"player_name": "Alice"}, &started); code != 200 || started.Status != "active" {
		t.Fatalf("start: %d %+v", code, started)
	}
}

// A person who plays several seats of a game against themselves is asked about
// every reaction (the page offers them no settings, and nobody else waits).
func TestAGameAgainstOneselfAsksEveryReaction(t *testing.T) {
	base := newRESTTestServer(t)
	// Alice plays the Crow (who may Object) and the Ram herself.
	var created createGameResponse
	body := map[string]any{
		"game_def_id": councilReactionTable, "player_name": "Alice", "also_seats": []int{4},
		"seat_bots":   map[string]string{"2": "random", "3": "random"},
		"start_state": json.RawMessage(reactionStart(t)), "start_seat": 4,
	}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if code, out := moveNotation(t, base, created.RoomCode, "Alice · Ram", "go@hall"); code != 200 {
		t.Fatalf("move: %d %v", code, out)
	}
	// Nothing is aimed at the Crow, but Alice is asked all the same.
	var st struct {
		ImpulseSeat int `json:"impulse_seat"`
	}
	doJSON(t, "GET", base+"/api/games/"+created.RoomCode, nil, &st)
	if st.ImpulseSeat != 1 {
		t.Fatalf("seat %d is to move: the reaction was passed for her", st.ImpulseSeat)
	}
	if code, out := moveNotation(t, base, created.RoomCode, "Alice", "decline"); code != 200 {
		t.Fatalf("pass: %d %v", code, out)
	}
}
