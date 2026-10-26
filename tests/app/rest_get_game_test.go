// Tests of reading a game over REST: a new game's seats and state, and the 404 for an unknown room.

package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// getGameResponse mirrors GET /api/games/{code}'s 200 response shape from
// docs/history/plan.md section 6.8: {"game_def_id", "status", "seats": [{"seat",
// "color", "name"}], "turn_seat"} — deliberately no board_state/move_seq,
// those live at .../state.
type getGameResponse struct {
	GameDefID string        `json:"game_def_id"`
	Status    string        `json:"status"`
	Seats     []getGameSeat `json:"seats"`
	TurnSeat  *int          `json:"turn_seat"`
}

type getGameSeat struct {
	Seat  int    `json:"seat"`
	Color string `json:"color"`
	Name  string `json:"name"`
}

// getGameStateResponse mirrors GET /api/games/{code}/state's 200 response
// shape from docs/history/plan.md section 6.8: {"board_state": <raw JSON>,
// "turn_seat", "status", "move_seq"}. board_state is decoded as
// json.RawMessage specifically so the test can assert it's a JSON object
// (starts with '{'), not a doubly-encoded JSON string (which would start
// with '"').
type getGameStateResponse struct {
	BoardState json.RawMessage `json:"board_state"`
	TurnSeat   *int            `json:"turn_seat"`
	Status     string          `json:"status"`
	MoveSeq    int             `json:"move_seq"`
}

// occupant mirrors internal/engine/state.Occupant's JSON shape (no json
// tags on that struct, so its fields serialize under their Go names,
// "Piece"/"Color").
type occupant struct {
	Piece string
	Color string
}

// createRoom is a small helper wrapping POST /api/games for this file's
// tests: it creates a demo-freeplay game for playerName and returns the
// resulting room code, failing the test on any non-200 response.
func createRoom(t *testing.T, baseURL, playerName string) string {
	t.Helper()

	_, raw := postGame(t, baseURL, map[string]string{
		"game_def_id": "demo-freeplay",
		"player_name": playerName,
	}, "")

	var body createGameResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode POST /api/games response: %v (body: %s)", err, raw)
	}
	if body.RoomCode == "" {
		t.Fatalf("POST /api/games returned empty room_code (body: %s)", raw)
	}
	return body.RoomCode
}

// getJSON GETs path (relative to baseURL) and returns the raw HTTP response
// plus its body bytes, already read with the response body closed.
func getJSON(t *testing.T, baseURL, path string) (*http.Response, []byte) {
	t.Helper()

	resp, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body for GET %s: %v", path, err)
	}
	return resp, raw
}

// TestGetGame_ImmediatelyAfterCreation_ReturnsWaitingWithOneSeatFilled
// covers GET /api/games/{code} right after POST /api/games: 200,
// game_def_id == "demo-freeplay", status == "waiting", exactly 2 seats
// declared (seat 1 filled with the creator's name, seat 2 open/empty name),
// and turn_seat is JSON null (the game hasn't activated — only one seat is
// filled).
func TestGetGame_ImmediatelyAfterCreation_ReturnsWaitingWithOneSeatFilled(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := getJSON(t, baseURL, "/api/games/"+roomCode)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	// Confirm turn_seat is literally JSON null in the wire payload (present
	// with a null value), not merely absent from the object.
	var raw2 map[string]json.RawMessage
	if err := json.Unmarshal(raw, &raw2); err != nil {
		t.Fatalf("decode response body as map: %v (body: %s)", err, raw)
	}
	turnSeatRaw, ok := raw2["turn_seat"]
	if !ok {
		t.Errorf("response body missing \"turn_seat\" key entirely (body: %s)", raw)
	} else if string(turnSeatRaw) != "null" {
		t.Errorf("turn_seat raw JSON = %s, want literal null (body: %s)", turnSeatRaw, raw)
	}

	var body getGameResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}

	if body.GameDefID != "demo-freeplay" {
		t.Errorf("game_def_id = %q, want %q", body.GameDefID, "demo-freeplay")
	}
	if body.Status != "waiting" {
		t.Errorf("status = %q, want %q", body.Status, "waiting")
	}
	if body.TurnSeat != nil {
		t.Errorf("turn_seat = %v, want nil (JSON null)", *body.TurnSeat)
	}
	if len(body.Seats) != 2 {
		t.Fatalf("len(seats) = %d, want 2 (seats: %+v)", len(body.Seats), body.Seats)
	}

	var seat1, seat2 *getGameSeat
	for i := range body.Seats {
		switch body.Seats[i].Seat {
		case 1:
			seat1 = &body.Seats[i]
		case 2:
			seat2 = &body.Seats[i]
		}
	}
	if seat1 == nil {
		t.Fatalf("no seat 1 entry in seats: %+v", body.Seats)
	}
	if seat2 == nil {
		t.Fatalf("no seat 2 entry in seats: %+v", body.Seats)
	}
	if seat1.Name != "Alice" {
		t.Errorf("seat 1 name = %q, want %q", seat1.Name, "Alice")
	}
	if seat1.Color != "black" {
		t.Errorf("seat 1 color = %q, want %q", seat1.Color, "black")
	}
	if seat2.Name != "" {
		t.Errorf("seat 2 name = %q, want \"\" (unfilled)", seat2.Name)
	}
	if seat2.Color != "white" {
		t.Errorf("seat 2 color = %q, want %q", seat2.Color, "white")
	}
}

// TestGetGameState_ImmediatelyAfterCreation_ReturnsInitialBoardState covers
// this backlog item's AC itself: GET /api/games/{code}/state, called right
// after POST /api/games with no moves made yet, returns 200 with
// status == "waiting", move_seq == 0, turn_seat == null, and board_state
// decoded as a JSON *object* (not an escaped JSON string) whose occupied
// positions match internal/games/demo/demo.json's setup entries exactly:
// 6 stones total, including (2,0)=black and (2,8)=white.
func TestGetGameState_ImmediatelyAfterCreation_ReturnsInitialBoardState(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := getJSON(t, baseURL, "/api/games/"+roomCode+"/state")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var body getGameStateResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}

	if body.Status != "waiting" {
		t.Errorf("status = %q, want %q", body.Status, "waiting")
	}
	if body.MoveSeq != 0 {
		t.Errorf("move_seq = %d, want 0", body.MoveSeq)
	}
	if body.TurnSeat != nil {
		t.Errorf("turn_seat = %v, want nil (JSON null)", *body.TurnSeat)
	}

	if len(body.BoardState) == 0 {
		t.Fatalf("board_state is empty (body: %s)", raw)
	}
	// board_state must be embedded as a raw JSON object, not a
	// doubly-encoded string: the first non-whitespace byte must be '{',
	// never '"'.
	trimmed := trimLeadingWhitespace(body.BoardState)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		t.Fatalf("board_state does not start with '{' (looks like a JSON string, not an object): %s", body.BoardState)
	}

	var occupants map[string]occupant
	if err := json.Unmarshal(body.BoardState, &occupants); err != nil {
		t.Fatalf("decode board_state as map[string]occupant: %v (board_state: %s)", err, body.BoardState)
	}

	wantOccupants := map[string]occupant{
		"2,0": {Piece: "stone", Color: "black"},
		"4,0": {Piece: "stone", Color: "black"},
		"6,0": {Piece: "stone", Color: "black"},
		"2,8": {Piece: "stone", Color: "white"},
		"4,8": {Piece: "stone", Color: "white"},
		"6,8": {Piece: "stone", Color: "white"},
	}

	if len(occupants) != len(wantOccupants) {
		t.Errorf("len(board_state occupants) = %d, want %d (occupants: %+v)", len(occupants), len(wantOccupants), occupants)
	}
	for pos, want := range wantOccupants {
		got, ok := occupants[pos]
		if !ok {
			t.Errorf("board_state missing occupant at %q, want %+v (occupants: %+v)", pos, want, occupants)
			continue
		}
		if got != want {
			t.Errorf("board_state[%q] = %+v, want %+v", pos, got, want)
		}
	}
}

// trimLeadingWhitespace trims leading JSON whitespace (space, tab, CR, LF)
// from b, mirroring what encoding/json itself tolerates before a value.
func trimLeadingWhitespace(b []byte) []byte {
	i := 0
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\r', '\n':
			i++
			continue
		}
		break
	}
	return b[i:]
}

// TestGetGame_UnknownRoomCode_Returns404 covers GET /api/games/{code} for a
// room code that was never created: 404 with a non-empty {"error": "..."}
// body.
func TestGetGame_UnknownRoomCode_Returns404(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := getJSON(t, baseURL, "/api/games/ZZZZZZ")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusNotFound, raw)
	}

	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}
	if body.Error == "" {
		t.Errorf("error field is empty, want a non-empty error message (body: %s)", raw)
	}
}

// TestGetGameState_UnknownRoomCode_Returns404 is
// TestGetGame_UnknownRoomCode_Returns404's counterpart for
// GET /api/games/{code}/state.
func TestGetGameState_UnknownRoomCode_Returns404(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := getJSON(t, baseURL, "/api/games/ZZZZZZ/state")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusNotFound, raw)
	}

	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}
	if body.Error == "" {
		t.Errorf("error field is empty, want a non-empty error message (body: %s)", raw)
	}
}

// TestGetGameState_AtSeq1BeforeAnyMoves_Returns404 exercises the ?at_seq=N
// query-param path (even though this backlog item is mainly about the
// no-at_seq case): immediately after creation, no move with seq=1 has ever
// been recorded, so GET .../state?at_seq=1 must be a definite 404 (per
// store.GetStateAtSeq's contract), not silently ignored (which would wrongly
// return the current/initial state with a 200) and not a panic/500.
func TestGetGameState_AtSeq1BeforeAnyMoves_Returns404(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := getJSON(t, baseURL, "/api/games/"+roomCode+"/state?at_seq=1")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusNotFound, raw)
	}

	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}
	if body.Error == "" {
		t.Errorf("error field is empty, want a non-empty error message (body: %s)", raw)
	}
}
