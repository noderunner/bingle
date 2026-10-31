package app_test

// The pie rule exchanges the players' seats (ADR 0056), and a seat belongs to
// whoever holds it (ADR 0067): after a swap each person's seat is still theirs
// alone, whichever number it now has, and undoing the swap gives the seats
// back — for guests and for signed-in accounts alike.

import (
	"net/http"
	"testing"
)

// pieSeats is GET /api/games/{code}'s seats, as the browser b sees them.
type pieSeat struct {
	Seat   int    `json:"seat"`
	Name   string `json:"name"`
	Handle string `json:"handle"`
	Guest  bool   `json:"guest"`
	Yours  bool   `json:"yours"`
}

func pieGame(t *testing.T, b *browser, room string) (turn int, seats []pieSeat) {
	t.Helper()
	var g struct {
		TurnSeat *int      `json:"turn_seat"`
		Seats    []pieSeat `json:"seats"`
	}
	if code := b.do("GET", "/api/games/"+room, nil, &g); code != 200 {
		t.Fatalf("game: %d", code)
	}
	if g.TurnSeat != nil {
		turn = *g.TurnSeat
	}
	return turn, g.Seats
}

func firstLegal(t *testing.T, b *browser, room string) string {
	t.Helper()
	var legal struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"moves"`
	}
	b.do("GET", "/api/games/"+room+"/moves/legal", nil, &legal)
	for _, m := range legal.Moves {
		if m.Notation != "swap" {
			return m.Notation
		}
	}
	t.Fatal("no legal move")
	return ""
}

// pieSwapOwnership plays an opening and a swap between ana and bob, then checks
// that every seat is still its owner's.
func pieSwapOwnership(t *testing.T, ana, bob *browser, accounts bool) {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := ana.do("POST", "/api/games", map[string]any{"game_def_id": "yavalath", "player_name": "Ana", "options": map[string]any{"pie-rule": true}}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	if code := bob.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	if code := ana.do("POST", "/api/games/"+room+"/moves", map[string]any{"player_name": "Ana", "notation": firstLegal(t, ana, room)}, nil); code != 200 {
		t.Fatalf("the opening: %d", code)
	}
	if code := bob.do("POST", "/api/games/"+room+"/moves", map[string]any{"player_name": "Bob", "notation": "swap"}, nil); code != 200 {
		t.Fatalf("the swap: %d", code)
	}

	// Bob sits in seat 1 now, Ana in seat 2, and it is Ana's move.
	turn, seats := pieGame(t, ana, room)
	if len(seats) != 2 || seats[0].Name != "Bob" || seats[1].Name != "Ana" || turn != 2 {
		t.Fatalf("after the swap: turn %d, seats %+v", turn, seats)
	}
	if seats[0].Yours || !seats[1].Yours {
		t.Errorf("Ana's view: seat 1 yours %v, seat 2 yours %v; want only seat 2", seats[0].Yours, seats[1].Yours)
	}
	if _, bs := pieGame(t, bob, room); !bs[0].Yours || bs[1].Yours {
		t.Errorf("Bob's view: %+v; want only seat 1 his", bs)
	}
	if accounts && (seats[0].Handle != "bob" || seats[1].Handle != "ana") {
		t.Errorf("the handles did not move with their players: %+v", seats)
	}
	if !accounts && (!seats[0].Guest || !seats[1].Guest) {
		t.Errorf("the guest marks did not stay: %+v", seats)
	}

	// Nobody can play the other's seat by its name.
	move := firstLegal(t, ana, room)
	if code := bob.do("POST", "/api/games/"+room+"/moves", map[string]any{"player_name": "Ana", "notation": move}, nil); code != http.StatusForbidden {
		t.Errorf("Bob plays Ana's new seat: %d, want 403", code)
	}
	// The swap was Bob's: he takes it back, from the seat he holds now, and
	// the seats go back to their first owners.
	if code := ana.do("POST", "/api/games/"+room+"/undo", map[string]any{"player_name": "Ana"}, nil); code == 200 {
		t.Errorf("Ana took back Bob's swap")
	}
	if code := bob.do("POST", "/api/games/"+room+"/undo", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("Bob takes the swap back: %d", code)
	}
	turn, seats = pieGame(t, ana, room)
	if seats[0].Name != "Ana" || seats[1].Name != "Bob" || turn != 2 || !seats[0].Yours || seats[1].Yours {
		t.Fatalf("after the undo: turn %d, seats %+v", turn, seats)
	}
	// Each plays their first seat again, and only theirs.
	if code := ana.do("POST", "/api/games/"+room+"/moves", map[string]any{"player_name": "Bob", "notation": firstLegal(t, ana, room)}, nil); code != http.StatusForbidden {
		t.Errorf("Ana plays Bob's seat after the undo: %d, want 403", code)
	}
	if code := bob.do("POST", "/api/games/"+room+"/moves", map[string]any{"player_name": "Bob", "notation": firstLegal(t, bob, room)}, nil); code != 200 {
		t.Errorf("Bob plays his own seat after the undo: %d", code)
	}
}

func TestPieRule_TheSwapKeepsEachGuestsSeatTheirs(t *testing.T) {
	base := newRESTTestServer(t)
	pieSwapOwnership(t, newBrowser(t, base), newBrowser(t, base), false)
}

func TestPieRule_TheSwapKeepsEachAccountsSeatTheirs(t *testing.T) {
	a, base := membersServer(t)
	pieSwapOwnership(t, member(t, a, base, "ana"), member(t, a, base, "bob"), true)
}
