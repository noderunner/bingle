package app_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// A person may play every seat of a table: each seat besides their own is a
// player of its own, named after them and the seat, so the game still has
// someone to move for every seat — a mock game alone.

type soloInfo struct {
	Status   string `json:"status"`
	TurnSeat int    `json:"turn_seat"`
	Seats    []struct {
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
	// Taking back: the last move of Alice's own seat, and the alias's after it.
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/undo", map[string]any{"player_name": "Alice"}, nil); code != 200 {
		t.Errorf("undo: %d", code)
	}
	var st soloInfo
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &st)
	if st.TurnSeat != 1 {
		t.Errorf("after the undo it is seat %d's turn, want 1", st.TurnSeat)
	}
}

func TestSoloTableOfThreeCultsWithABot(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars-3p", "player_name": "Alice", "also_seats": []int{3}, "seat_bots": map[string]string{"2": "random"},
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if created.Status != "active" {
		t.Fatalf("status %q", created.Status)
	}
	var info soloInfo
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &info)
	if info.Seats[1].Bot != "random" || info.Seats[2].Name != "Alice · Black Goat" {
		t.Fatalf("seats %+v", info.Seats)
	}
	// An unknown seat is refused.
	bad := map[string]any{"game_def_id": "cthulhu-wars-3p", "player_name": "Bob", "also_seats": []int{9}}
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
	// Alice plays Great Cthulhu (Devolve) and Crawling Chaos herself.
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 3, 3, 0, 0}, [5]uint8{0, 1 << 1, 0, 0, 0},
		func(d *cthulhuwars.Data) { d.Turn.Seat = 2 })
	var created createGameResponse
	body := map[string]any{
		"game_def_id": "cthulhu-wars", "player_name": "Alice", "also_seats": []int{2},
		"seat_bots":   map[string]string{"3": "random", "4": "random"},
		"start_state": json.RawMessage(doc), "start_seat": 2,
	}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if code, out := moveNotation(t, base, created.RoomCode, "Alice · Crawling Chaos", "recruit@asia"); code != 200 {
		t.Fatalf("recruit: %d %v", code, out)
	}
	// Nothing is aimed at Great Cthulhu, but Alice is asked all the same.
	var st struct {
		TurnSeat int `json:"turn_seat"`
	}
	doJSON(t, "GET", base+"/api/games/"+created.RoomCode, nil, &st)
	if st.TurnSeat != 1 {
		t.Fatalf("seat %d is to move: the reaction was passed for her", st.TurnSeat)
	}
	if code, out := moveNotation(t, base, created.RoomCode, "Alice", "done"); code != 200 {
		t.Fatalf("pass: %d %v", code, out)
	}
}
