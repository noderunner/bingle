// Tests of POST /games/{code}/moves: legal moves, and the error status for illegal moves, the wrong turn and games that are not active.

package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// moveResult mirrors one "move" object in POST /api/games/{code}/moves's
// 200 response, and one entry of GET /api/games/{code}/moves's response
// array, from docs/history/plan.md section 6.8: {"seq","seat","color","from","to",
// "captured"}. Captured is a pointer so the test can distinguish JSON null
// (no capture) from a present string value.
type moveResult struct {
	Seq      int     `json:"seq"`
	Seat     int     `json:"seat"`
	Color    string  `json:"color"`
	From     string  `json:"from"`
	To       string  `json:"to"`
	Captured *string `json:"captured"`
}

// makeMoveResponse mirrors POST /api/games/{code}/moves's 200 response body
// from docs/history/plan.md section 6.8: {"move": {...}, "state": {"board_state":
// <raw JSON>, "turn_seat", "status", "move_seq"}}.
type makeMoveResponse struct {
	Move  moveResult           `json:"move"`
	State getGameStateResponse `json:"state"`
}

// postMove POSTs raw JSON (or, if rawBody != "", that literal string
// instead of marshaling anything) to baseURL+"/api/games/"+code+"/moves"
// and returns the raw HTTP response along with its body bytes (already read
// and the response body closed).
func postMove(t *testing.T, baseURL, code string, body any, rawBody string) (*http.Response, []byte) {
	t.Helper()

	var payload []byte
	if rawBody != "" {
		payload = []byte(rawBody)
	} else {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal(body): %v", err)
		}
	}

	resp, err := http.Post(baseURL+"/api/games/"+code+"/moves", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/games/%s/moves: %v", code, err)
	}
	defer func() { _ = resp.Body.Close() }()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, buf.Bytes()
}

// setupActiveGame creates a test-freeplay room with Alice (seat 1, black),
// has Bob join to fill seat 2 (white) and activate the game, and returns
// the room code. Per the turn-order-init fix (backlog item 20), the newly
// activated game's turn_seat is seat 1 (black / Alice).
func setupActiveGame(t *testing.T, baseURL string) string {
	t.Helper()
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Bob",
		"role":        "player",
	}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup join status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}
	return roomCode
}

// getState is a small helper wrapping GET /api/games/{code}/state,
// decoding the response into getGameStateResponse and failing the test if
// the status isn't 200.
func getState(t *testing.T, baseURL, roomCode string) getGameStateResponse {
	t.Helper()
	resp, raw := getJSON(t, baseURL, "/api/games/"+roomCode+"/state")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/games/%s/state status = %d, want %d (body: %s)", roomCode, resp.StatusCode, http.StatusOK, raw)
	}
	var body getGameStateResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode GET /api/games/%s/state response: %v (body: %s)", roomCode, err, raw)
	}
	return body
}

// decodeOccupants decodes a board_state raw JSON object into
// map[string]occupant, failing the test on error. (occupant is declared in
// rest_get_game_test.go.)
func decodeOccupants(t *testing.T, boardState json.RawMessage) map[string]occupant {
	t.Helper()
	var occupants map[string]occupant
	if err := json.Unmarshal(boardState, &occupants); err != nil {
		t.Fatalf("decode board_state as map[string]occupant: %v (board_state: %s)", err, boardState)
	}
	return occupants
}

// TestPostMove_IllegalMove_Returns400AndBoardUnchanged covers this backlog
// item's AC itself: POST /api/games/{code}/moves with a genuinely
// non-adjacent destination ("2,0" -> "5,5", nowhere near each other on the
// 9x9 orthogonal4 grid) returns HTTP 400 with a non-empty {"error"} body,
// and a following GET .../state shows move_seq still 0 and the exact same
// board_state (same occupied positions/colors) as immediately after setup
// — nothing was persisted.
func TestPostMove_IllegalMove_Returns400AndBoardUnchanged(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	before := getState(t, baseURL, roomCode)
	if before.MoveSeq != 0 {
		t.Fatalf("precondition failed: move_seq = %d before any move, want 0", before.MoveSeq)
	}
	beforeOccupants := decodeOccupants(t, before.BoardState)

	resp, raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "5,5",
	}, "")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
	}

	var errBody errorResponse
	if err := json.Unmarshal(raw, &errBody); err != nil {
		t.Fatalf("decode error response body: %v (body: %s)", err, raw)
	}
	if strings.TrimSpace(errBody.Error) == "" {
		t.Errorf("error field = %q, want a non-empty error message (body: %s)", errBody.Error, raw)
	}

	after := getState(t, baseURL, roomCode)
	if after.MoveSeq != 0 {
		t.Errorf("move_seq after rejected illegal move = %d, want 0 (unchanged)", after.MoveSeq)
	}
	if after.Status != "active" {
		t.Errorf("status after rejected illegal move = %q, want %q (unchanged)", after.Status, "active")
	}
	if after.TurnSeat == nil || *after.TurnSeat != 1 {
		t.Errorf("turn_seat after rejected illegal move = %v, want 1 (unchanged: still Alice/black's turn)", after.TurnSeat)
	}

	// board_state bytes should be byte-for-byte identical; fall back to a
	// structural occupant-map comparison in case of insignificant
	// whitespace/key-order differences in the raw encoding.
	if !bytes.Equal(before.BoardState, after.BoardState) {
		afterOccupants := decodeOccupants(t, after.BoardState)
		if len(afterOccupants) != len(beforeOccupants) {
			t.Fatalf("board_state occupant count changed: before %d, after %d (before: %+v, after: %+v)",
				len(beforeOccupants), len(afterOccupants), beforeOccupants, afterOccupants)
		}
		for pos, want := range beforeOccupants {
			got, ok := afterOccupants[pos]
			if !ok {
				t.Errorf("board_state after rejected move is missing occupant at %q, want %+v", pos, want)
				continue
			}
			if got != want {
				t.Errorf("board_state after rejected move at %q = %+v, want %+v (unchanged)", pos, got, want)
			}
		}
	}
}

// TestPostMove_LegalMove_Returns200WithMoveAndState covers a legal,
// non-capturing move succeeding: "2,0" -> "1,0" is graph-adjacent (orthogonal4)
// and "1,0" is unoccupied at game start. Asserts the full move/state
// response shape from docs/history/plan.md section 6.8.
func TestPostMove_LegalMove_Returns200WithMoveAndState(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	resp, raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var body makeMoveResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}

	if body.Move.Seq != 1 {
		t.Errorf("move.seq = %d, want 1", body.Move.Seq)
	}
	if body.Move.Seat != 1 {
		t.Errorf("move.seat = %d, want 1", body.Move.Seat)
	}
	if body.Move.Color != "black" {
		t.Errorf("move.color = %q, want %q", body.Move.Color, "black")
	}
	if body.Move.From != "2,0" {
		t.Errorf("move.from = %q, want %q", body.Move.From, "2,0")
	}
	if body.Move.To != "1,0" {
		t.Errorf("move.to = %q, want %q", body.Move.To, "1,0")
	}
	if body.Move.Captured != nil {
		t.Errorf("move.captured = %v, want nil (JSON null; nothing was at 1,0)", *body.Move.Captured)
	}

	// Confirm captured is literally JSON null in the wire payload, not
	// merely a coincidentally-nil pointer from a missing key.
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("decode response body as map: %v (body: %s)", err, raw)
	}
	var moveMap map[string]json.RawMessage
	if err := json.Unmarshal(asMap["move"], &moveMap); err != nil {
		t.Fatalf("decode move object as map: %v (move: %s)", err, asMap["move"])
	}
	capturedRaw, ok := moveMap["captured"]
	if !ok {
		t.Errorf("move object missing \"captured\" key entirely (move: %s)", asMap["move"])
	} else if string(capturedRaw) != "null" {
		t.Errorf("move.captured raw JSON = %s, want literal null", capturedRaw)
	}

	if body.State.MoveSeq != 1 {
		t.Errorf("state.move_seq = %d, want 1", body.State.MoveSeq)
	}
	if body.State.TurnSeat == nil || *body.State.TurnSeat != 2 {
		t.Errorf("state.turn_seat = %v, want 2 (Bob/white's turn next)", body.State.TurnSeat)
	}
	if body.State.Status != "active" {
		t.Errorf("state.status = %q, want %q", body.State.Status, "active")
	}

	if len(body.State.BoardState) == 0 {
		t.Fatalf("state.board_state is empty (body: %s)", raw)
	}
	trimmed := trimLeadingWhitespace(body.State.BoardState)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		t.Fatalf("state.board_state does not start with '{' (looks like a JSON string, not an object): %s", body.State.BoardState)
	}

	occupants := decodeOccupants(t, body.State.BoardState)
	if _, stillAtOld := occupants["2,0"]; stillAtOld {
		t.Errorf("board_state still has an occupant at \"2,0\" after moving away from it: %+v", occupants)
	}
	got, ok := occupants["1,0"]
	if !ok {
		t.Fatalf("board_state has no occupant at \"1,0\" after moving there: %+v", occupants)
	}
	if got != (occupant{Piece: "stone", Color: "black"}) {
		t.Errorf("board_state[\"1,0\"] = %+v, want {Piece: stone, Color: black}", got)
	}

	// A following GET .../state must reflect the same persisted result.
	after := getState(t, baseURL, roomCode)
	if after.MoveSeq != 1 {
		t.Errorf("GET .../state move_seq = %d, want 1", after.MoveSeq)
	}
	if after.TurnSeat == nil || *after.TurnSeat != 2 {
		t.Errorf("GET .../state turn_seat = %v, want 2", after.TurnSeat)
	}
}

// TestPostMove_WrongTurn_Returns403AndStateUnchanged covers docs/history/plan.md
// section 7's "move submitted by a name that is not the seat whose
// turn_seat it is -> 403, no state change" edge case: after Alice's legal
// move above hands the turn to Bob (seat 2), Alice attempts a second move
// immediately -> 403 with a non-empty {"error"}, and a following GET
// .../state shows move_seq still 1 (unchanged by the rejected attempt).
func TestPostMove_WrongTurn_Returns403AndStateUnchanged(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	setupResp, setupRaw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")
	if setupResp.StatusCode != http.StatusOK {
		t.Fatalf("setup move status = %d, want %d (body: %s)", setupResp.StatusCode, http.StatusOK, setupRaw)
	}

	// Now it's Bob's (seat 2, white) turn; Alice tries to move again.
	resp, raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "4,0",
		"to":          "3,0",
	}, "")

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusForbidden, raw)
	}

	var errBody errorResponse
	if err := json.Unmarshal(raw, &errBody); err != nil {
		t.Fatalf("decode error response body: %v (body: %s)", err, raw)
	}
	if strings.TrimSpace(errBody.Error) == "" {
		t.Errorf("error field = %q, want a non-empty error message (body: %s)", errBody.Error, raw)
	}

	after := getState(t, baseURL, roomCode)
	if after.MoveSeq != 1 {
		t.Errorf("move_seq after rejected out-of-turn move = %d, want 1 (unchanged)", after.MoveSeq)
	}
	if after.TurnSeat == nil || *after.TurnSeat != 2 {
		t.Errorf("turn_seat after rejected out-of-turn move = %v, want 2 (unchanged: still Bob/white's turn)", after.TurnSeat)
	}
}

// TestPostMove_GameNotActive_Returns409 covers a brand new game (created,
// but nobody has joined yet — still "waiting") rejecting any move attempt
// with 409 and a non-empty {"error"} body.
func TestPostMove_GameNotActive_Returns409(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusConflict, raw)
	}

	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}
	if strings.TrimSpace(body.Error) == "" {
		t.Errorf("error field = %q, want a non-empty error message (body: %s)", body.Error, raw)
	}
}

// TestPostMove_UnknownRoomCode_Returns404 covers an unknown room code ->
// 404 with a non-empty {"error"} body, mirroring the other endpoints'
// same rule (docs/history/plan.md section 6.8's statusForError mapping via
// store.ErrNotFound).
func TestPostMove_UnknownRoomCode_Returns404(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := postMove(t, baseURL, "ZZZZZZ", map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusNotFound, raw)
	}

	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}
	if strings.TrimSpace(body.Error) == "" {
		t.Errorf("error field = %q, want a non-empty error message (body: %s)", body.Error, raw)
	}
}

// TestPostMove_MalformedJSON_Returns400 covers robustness: malformed JSON
// in the request body must not panic or 500 the server — it's a 400,
// mirroring TestPostGames_MalformedJSON_Returns400 and
// TestPostJoin_MalformedJSON_Returns400.
func TestPostMove_MalformedJSON_Returns400(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	resp, raw := postMove(t, baseURL, roomCode, nil, `{"player_name": "Alice", "from": "2,0", "to": `)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
	}
}

// TestPostMove_MalformedJSON_NotEmptyBody guards specifically against a
// non-JSON body at all (e.g. plain text), which must also be a 400, not a
// 500 or a hang/panic.
func TestPostMove_MalformedJSON_NotEmptyBody(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	resp, raw := postMove(t, baseURL, roomCode, nil, `this is not json at all`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
	}
}

// TestPostMove_BlankRequiredFields_Returns400 covers blank/whitespace-only
// required fields in the move request. Blank "from"/"to" reach
// state.ApplyMove (there's no piece at "" and/or "" is not adjacent to
// anything), which rejects them as an illegal move — a 400 via
// statusForError's default case, per docs/history/plan.md section 7 ("Move where
// `from` has no piece ... -> 400"). Blank player_name is validated the same
// way POST /games and POST /games/{code}/join already do (games.go rejects
// blank/whitespace-only player_name with 400 before calling Manager) —
// mirroring that established handler style for consistency across all
// three POST endpoints in this package.
func TestPostMove_BlankRequiredFields_Returns400(t *testing.T) {
	cases := []struct {
		name       string
		playerName string
		from       string
		to         string
	}{
		{name: "blank player_name", playerName: "", from: "2,0", to: "1,0"},
		{name: "whitespace-only player_name", playerName: "   ", from: "2,0", to: "1,0"},
		{name: "blank from", playerName: "Alice", from: "", to: "1,0"},
		{name: "blank to", playerName: "Alice", from: "2,0", to: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := newRESTTestServer(t)
			roomCode := setupActiveGame(t, baseURL)

			resp, raw := postMove(t, baseURL, roomCode, map[string]string{
				"player_name": tc.playerName,
				"from":        tc.from,
				"to":          tc.to,
			}, "")

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
			}
		})
	}
}

// TestPostMove_SpectatorCannotMove covers docs/history/plan.md section 7's
// "Spectator attempts POST /api/games/{code}/moves -> 403 (no seat to move
// from)" edge case: a spectator holds no seat, so Manager.MakeMove can't
// match their name to game.TurnSeat, resulting in ErrNotYourTurn -> 403.
func TestPostMove_SpectatorCannotMove(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	joinResp, joinRaw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Eve",
		"role":        "spectator",
	}, "")
	if joinResp.StatusCode != http.StatusOK {
		t.Fatalf("setup spectator join status = %d, want %d (body: %s)", joinResp.StatusCode, http.StatusOK, joinRaw)
	}

	resp, raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Eve",
		"from":        "2,0",
		"to":          "1,0",
	}, "")

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusForbidden, raw)
	}

	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}
	if strings.TrimSpace(body.Error) == "" {
		t.Errorf("error field = %q, want a non-empty error message (body: %s)", body.Error, raw)
	}
}
