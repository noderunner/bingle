package app_test

import (
	"strings"
	"testing"
)

func takeback(t *testing.T, baseURL, room, name, action string) (int, restState, map[string]any) {
	t.Helper()
	var st restState
	var raw map[string]any
	code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/takeback", map[string]any{"player_name": name, "action": action}, &raw)
	if code == 200 {
		doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	}
	return code, st, raw
}

func TestTakebackWithConsent(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	// Nothing to take back yet, and while nobody has replied Alice should
	// simply undo.
	if code, _, _ := takeback(t, baseURL, room, "Alice", "request"); code != 409 {
		t.Errorf("request before moving: %d, want 409", code)
	}
	moveNotation(t, baseURL, room, "Alice", "a5")
	if code, _, out := takeback(t, baseURL, room, "Alice", "request"); code != 409 || !strings.Contains(out["error"].(string), "undo it directly") {
		t.Errorf("request when undo works: %d %v", code, out)
	}
	moveNotation(t, baseURL, room, "Bob", "e1")
	moveNotation(t, baseURL, room, "Alice", "c3")
	moveNotation(t, baseURL, room, "Bob", "c4")

	// Alice asks; only Bob may answer.
	if code, st, out := takeback(t, baseURL, room, "Alice", "request"); code != 200 || st.MoveSeq != 4 {
		t.Fatalf("request: %d %v seq %d", code, out, st.MoveSeq)
	}
	var game struct {
		TakebackSeat  *int `json:"takeback_seat"`
		TakebackToSeq int  `json:"takeback_to_seq"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if game.TakebackSeat == nil || *game.TakebackSeat != 1 || game.TakebackToSeq != 2 {
		t.Errorf("game = %+v, want seat 1 asking to rewind to 2 moves", game)
	}
	if code, _, _ := takeback(t, baseURL, room, "Alice", "accept"); code != 409 {
		t.Errorf("asker accepting her own request: %d, want 409", code)
	}
	if code, _, _ := takeback(t, baseURL, room, "Alice", "request"); code != 409 {
		t.Errorf("a second request: %d, want 409", code)
	}

	// Bob declines: nothing changes and the request closes.
	if code, st, _ := takeback(t, baseURL, room, "Bob", "decline"); code != 200 || st.MoveSeq != 4 {
		t.Fatalf("decline: %d seq %d", code, st.MoveSeq)
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if game.TakebackSeat != nil {
		t.Errorf("request still open after decline: %+v", game)
	}

	// Ask again and let a move withdraw it.
	takeback(t, baseURL, room, "Alice", "request")
	if code, out := moveNotation(t, baseURL, room, "Alice", "d3"); code != 200 {
		t.Fatalf("d3: %d %v", code, out)
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if game.TakebackSeat != nil {
		t.Errorf("a move should withdraw the request: %+v", game)
	}
	if code, _, _ := takeback(t, baseURL, room, "Bob", "accept"); code != 409 {
		t.Errorf("accepting a withdrawn request: %d, want 409", code)
	}

	// Now Bob replies, Alice asks again and Bob accepts: back to before d3.
	moveNotation(t, baseURL, room, "Bob", "d4")
	if code, _, out := takeback(t, baseURL, room, "Alice", "request"); code != 200 {
		t.Fatalf("request 3: %d %v", code, out)
	}
	code, st, out := takeback(t, baseURL, room, "Bob", "accept")
	if code != 200 {
		t.Fatalf("accept: %d %v", code, out)
	}
	if st.MoveSeq != 4 || st.TurnSeat == nil || *st.TurnSeat != 1 || st.State.Cells["d3"] != nil || st.State.Cells["d4"] != nil {
		t.Errorf("after accept: seq %d turn %v d3=%v d4=%v", st.MoveSeq, st.TurnSeat, st.State.Cells["d3"], st.State.Cells["d4"])
	}
	// Alice can play a different move there.
	if code, out := moveNotation(t, baseURL, room, "Alice", "b2"); code != 200 {
		t.Errorf("replay after takeback: %d %v", code, out)
	}
}

func TestTakebackNotOfferedAgainstABot(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice", "opponent": "random"}, &created)
	moveNotation(t, baseURL, created.RoomCode, "Alice", "a5")
	if code, _, out := takeback(t, baseURL, created.RoomCode, "Alice", "request"); code != 409 {
		t.Errorf("takeback vs bot: %d %v, want 409 (undo works directly)", code, out)
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/undo", map[string]any{"player_name": "Alice"}, nil); code != 200 {
		t.Errorf("undo vs bot: %d", code)
	}
}
