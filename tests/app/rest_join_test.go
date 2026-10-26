// Tests of POST /games/{code}/join: players, the default role, spectators and reconnects.

package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// joinResponse mirrors POST /api/games/{code}/join's 200 response shape
// from docs/history/plan.md section 6.8: {"role", "seat", "color", "status"}, with
// seat/color tagged omitempty (0 / "" when absent) — struct decoding alone
// can't distinguish "key absent" from "key present with zero value", so
// tests that need to assert genuine key-absence decode into map[string]any
// instead (see TestPostJoin_Spectator_OmitsSeatAndColor below).
type joinResponse struct {
	Role   string `json:"role"`
	Seat   int    `json:"seat,omitempty"`
	Color  string `json:"color,omitempty"`
	Status string `json:"status"`
}

// postJoin POSTs raw JSON (or, if rawBody != "", that literal string
// instead of marshaling anything) to baseURL+"/api/games/"+code+"/join"
// and returns the raw HTTP response along with its body bytes (already
// read and the response body closed).
func postJoin(t *testing.T, baseURL, code string, body any, rawBody string) (*http.Response, []byte) {
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

	resp, err := http.Post(baseURL+"/api/games/"+code+"/join", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/games/%s/join: %v", code, err)
	}
	defer func() { _ = resp.Body.Close() }()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, buf.Bytes()
}

// TestPostJoin_NewPlayerFillsLastSeat_ActivatesGame covers this backlog
// item's AC itself: POST /api/games (creator "Alice") then POST
// /api/games/{code}/join with {"player_name": "Bob", "role": "player"} ->
// 200 {"role": "player", "seat": 2, "color": "white", "status": "active"}
// (demo.json's seat 2 color is "white"). A following GET /api/games/{code}
// must then show status == "active" with both seats filled.
func TestPostJoin_NewPlayerFillsLastSeat_ActivatesGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Bob",
		"role":        "player",
	}, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var body joinResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}

	if body.Role != "player" {
		t.Errorf("role = %q, want %q", body.Role, "player")
	}
	if body.Seat != 2 {
		t.Errorf("seat = %d, want 2", body.Seat)
	}
	if body.Color != "white" {
		t.Errorf("color = %q, want %q", body.Color, "white")
	}
	if body.Status != "active" {
		t.Errorf("status = %q, want %q", body.Status, "active")
	}

	// A following GetState (via GET /api/games/{code}) must show
	// status == "active" and both seats filled.
	getResp, getRaw := getJSON(t, baseURL, "/api/games/"+roomCode)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/games/%s status = %d, want %d (body: %s)", roomCode, getResp.StatusCode, http.StatusOK, getRaw)
	}

	var getBody getGameResponse
	if err := json.Unmarshal(getRaw, &getBody); err != nil {
		t.Fatalf("decode GET /api/games/%s response: %v (body: %s)", roomCode, err, getRaw)
	}

	if getBody.Status != "active" {
		t.Errorf("GET /api/games/%s status = %q, want %q", roomCode, getBody.Status, "active")
	}
	if len(getBody.Seats) != 2 {
		t.Fatalf("len(seats) = %d, want 2 (seats: %+v)", len(getBody.Seats), getBody.Seats)
	}
	for _, s := range getBody.Seats {
		if strings.TrimSpace(s.Name) == "" {
			t.Errorf("seat %d has no occupant name, want it filled (seats: %+v)", s.Seat, getBody.Seats)
		}
	}
}

// TestPostJoin_RoleOmitted_DefaultsToPlayer covers docs/history/plan.md section
// 6.8's "role defaults to player if blank" rule: on a fresh game with only
// the creator seated, POSTing {"player_name": "Carol"} with the "role" key
// omitted entirely must behave exactly like role=player (assigns seat 2),
// not be rejected and not be silently treated as a spectator (which would
// omit seat/color from the response).
func TestPostJoin_RoleOmitted_DefaultsToPlayer(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Carol",
	}, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var body joinResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}

	if body.Role != "player" {
		t.Errorf("role = %q, want %q (role omitted from request should default to player)", body.Role, "player")
	}
	if body.Seat != 2 {
		t.Errorf("seat = %d, want 2 (role-omitted join should assign the next open seat)", body.Seat)
	}
	if body.Color != "white" {
		t.Errorf("color = %q, want %q", body.Color, "white")
	}
	if body.Status != "active" {
		t.Errorf("status = %q, want %q (filling the last seat should activate the game)", body.Status, "active")
	}
}

// TestPostJoin_Spectator_OmitsSeatAndColor covers spectator joins: 200,
// {"role": "spectator", "status": <whatever it already was>}, with
// "seat"/"color" genuinely ABSENT from the JSON body (checked by decoding
// into map[string]any and confirming the keys don't exist at all, not just
// checking for zero values, which omitempty could also produce for a real
// seat/color by coincidence).
func TestPostJoin_Spectator_OmitsSeatAndColor(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Dana",
		"role":        "spectator",
	}, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("decode response body as map: %v (body: %s)", err, raw)
	}

	if role, _ := asMap["role"].(string); role != "spectator" {
		t.Errorf("role = %v, want %q (body: %s)", asMap["role"], "spectator", raw)
	}
	if status, _ := asMap["status"].(string); status != "waiting" {
		t.Errorf("status = %v, want %q (game had only the creator seated) (body: %s)", asMap["status"], "waiting", raw)
	}
	if _, present := asMap["seat"]; present {
		t.Errorf("response contains a \"seat\" key for a spectator, want it omitted entirely (body: %s)", raw)
	}
	if _, present := asMap["color"]; present {
		t.Errorf("response contains a \"color\" key for a spectator, want it omitted entirely (body: %s)", raw)
	}
}

// TestPostJoin_Spectator_OnActiveGame_OmitsSeatAndColor is a second data
// point for the spectator case, on an already-"active" game (both seats
// filled), confirming the response's status field echoes the game's actual
// current status ("active", not "waiting") and still omits seat/color.
func TestPostJoin_Spectator_OnActiveGame_OmitsSeatAndColor(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	// Fill seat 2 so the game becomes active.
	joinResp, joinRaw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Bob",
		"role":        "player",
	}, "")
	if joinResp.StatusCode != http.StatusOK {
		t.Fatalf("setup join status = %d, want %d (body: %s)", joinResp.StatusCode, http.StatusOK, joinRaw)
	}

	resp, raw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Eve",
		"role":        "spectator",
	}, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("decode response body as map: %v (body: %s)", err, raw)
	}

	if role, _ := asMap["role"].(string); role != "spectator" {
		t.Errorf("role = %v, want %q (body: %s)", asMap["role"], "spectator", raw)
	}
	if status, _ := asMap["status"].(string); status != "active" {
		t.Errorf("status = %v, want %q (body: %s)", asMap["status"], "active", raw)
	}
	if _, present := asMap["seat"]; present {
		t.Errorf("response contains a \"seat\" key for a spectator, want it omitted entirely (body: %s)", raw)
	}
	if _, present := asMap["color"]; present {
		t.Errorf("response contains a \"color\" key for a spectator, want it omitted entirely (body: %s)", raw)
	}
}

// TestPostJoin_SeatsFull_Returns409 covers docs/history/plan.md's SeatsFull edge
// case: after both seats are filled (Alice + Bob), a THIRD distinct player
// name joining with role=player must return 409 with a non-empty
// {"error"} body, and must not disturb the already-filled seats.
func TestPostJoin_SeatsFull_Returns409(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	fillResp, fillRaw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Bob",
		"role":        "player",
	}, "")
	if fillResp.StatusCode != http.StatusOK {
		t.Fatalf("setup join status = %d, want %d (body: %s)", fillResp.StatusCode, http.StatusOK, fillRaw)
	}

	resp, raw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Carol",
		"role":        "player",
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

	// The rejected join must not have disturbed the existing seating.
	getResp, getRaw := getJSON(t, baseURL, "/api/games/"+roomCode)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/games/%s status = %d, want %d (body: %s)", roomCode, getResp.StatusCode, http.StatusOK, getRaw)
	}
	var getBody getGameResponse
	if err := json.Unmarshal(getRaw, &getBody); err != nil {
		t.Fatalf("decode GET /api/games/%s response: %v (body: %s)", roomCode, err, getRaw)
	}
	for _, s := range getBody.Seats {
		if s.Seat == 1 && s.Name != "Alice" {
			t.Errorf("seat 1 name = %q, want %q (unchanged by the rejected join)", s.Name, "Alice")
		}
		if s.Seat == 2 && s.Name != "Bob" {
			t.Errorf("seat 2 name = %q, want %q (unchanged by the rejected join)", s.Name, "Bob")
		}
	}
}

// TestPostJoin_UnknownRoomCode_Returns404 covers an unknown room code ->
// 404 with a non-empty {"error"} body, mirroring GET /api/games/{code}'s
// same rule (docs/history/plan.md section 6.8's statusForError mapping via
// store.ErrNotFound).
func TestPostJoin_UnknownRoomCode_Returns404(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := postJoin(t, baseURL, "ZZZZZZ", map[string]string{
		"player_name": "Alice",
		"role":        "player",
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

// TestPostJoin_BlankPlayerName_Returns400 covers docs/history/plan.md section 6.8's
// "blank/whitespace-only player_name ... is rejected with 400 before even
// calling Manager" rule for the join endpoint too (already covered for
// create in rest_create_game_test.go), for both a totally empty name and a
// whitespace-only one, and for both role values.
func TestPostJoin_BlankPlayerName_Returns400(t *testing.T) {
	cases := []struct {
		name       string
		playerName string
		role       string
	}{
		{name: "empty string, player role", playerName: "", role: "player"},
		{name: "whitespace only, player role", playerName: "   ", role: "player"},
		{name: "empty string, spectator role", playerName: "", role: "spectator"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := newRESTTestServer(t)
			roomCode := createRoom(t, baseURL, "Alice")

			resp, raw := postJoin(t, baseURL, roomCode, map[string]string{
				"player_name": tc.playerName,
				"role":        tc.role,
			}, "")

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
			}
		})
	}
}

// TestPostJoin_MalformedJSON_Returns400 covers robustness: malformed JSON
// in the request body must not panic or 500 the server — it's a 400,
// mirroring TestPostGames_MalformedJSON_Returns400.
func TestPostJoin_MalformedJSON_Returns400(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := postJoin(t, baseURL, roomCode, nil, `{"player_name": "Bob", "role": `)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
	}
}

// TestPostJoin_ReconnectExistingName_ReturnsSameSeat is a smoke check that
// the REST layer correctly passes through Manager.JoinGame's reconnect
// behavior (already covered in depth at the session-layer, per docs/history/plan.md
// section 6.4c): joining again with the EXACT name already on seat 1
// ("Alice") returns 200 with seat: 1 (not a new seat, not an error, not
// treated as a fresh join that would error out on a "taken" seat).
func TestPostJoin_ReconnectExistingName_ReturnsSameSeat(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"role":        "player",
	}, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var body joinResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}

	if body.Role != "player" {
		t.Errorf("role = %q, want %q", body.Role, "player")
	}
	if body.Seat != 1 {
		t.Errorf("seat = %d, want 1 (reconnecting to the same name's existing seat)", body.Seat)
	}
	if body.Color != "black" {
		t.Errorf("color = %q, want %q", body.Color, "black")
	}
	// Reconnecting doesn't fill a new seat, so status must remain "waiting".
	if body.Status != "waiting" {
		t.Errorf("status = %q, want %q (reconnect must not change game status)", body.Status, "waiting")
	}
}
