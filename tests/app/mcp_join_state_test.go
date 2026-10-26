// Tests of the MCP join and state tools: seats, roles, spectators, full games, and agreement with REST.

package app_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// mcpJoinResult mirrors join_game's result shape from docs/history/plan.md section
// 6.10: {"role","seat","color","status"}, with seat/color tagged omitempty
// (0 / "" when absent) mirroring REST's joinResponse (rest_join_test.go) —
// tests that need to assert genuine key-absence (not just a zero value)
// decode into map[string]any instead.
type mcpJoinResult struct {
	Role   string `json:"role"`
	Seat   int    `json:"seat,omitempty"`
	Color  string `json:"color,omitempty"`
	Status string `json:"status"`
}

// mcpGetStateResult mirrors get_state's result shape from docs/history/plan.md
// section 6.10: {"board_state","turn_seat","status","move_seq"} — no
// "at_seq" field (that's REST-only). board_state is decoded as
// json.RawMessage, mirroring REST's getGameStateResponse, so it can be
// compared structurally against the REST response rather than byte-for-byte.
type mcpGetStateResult struct {
	BoardState json.RawMessage `json:"board_state"`
	TurnSeat   *int            `json:"turn_seat"`
	Status     string          `json:"status"`
	MoveSeq    int             `json:"move_seq"`
}

// TestMCPJoinGame_NewPlayerFillsLastSeat_ActivatesGameAndVisibleViaREST
// covers join_game's happy path: a room created via REST (creator "Alice",
// seat 1), then a NEW player joining via the join_game MCP tool with
// role:"player" gets assigned seat 2/color "white" and activates the game
// (status: "active") — and a following plain REST GET /api/games/{code}
// confirms both seats are filled, mirroring
// TestPostJoin_NewPlayerFillsLastSeat_ActivatesGame's REST-only assertions.
func TestMCPJoinGame_NewPlayerFillsLastSeat_ActivatesGameAndVisibleViaREST(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "join_game", map[string]any{
		"room_code":   roomCode,
		"player_name": "Bob",
		"role":        "player",
	})
	if res.IsError {
		t.Fatalf("join_game IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var body mcpJoinResult
	decodeToolResult(t, res, &body)

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

	// The literal cross-surface check: a following plain REST GET must show
	// both seats filled and the game active.
	getResp, getRaw := getJSON(t, baseURL, "/api/games/"+roomCode)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/games/%s status = %d, want %d (body: %s)", roomCode, getResp.StatusCode, http.StatusOK, getRaw)
	}
	var getBody getGameResponse
	if err := json.Unmarshal(getRaw, &getBody); err != nil {
		t.Fatalf("decode GET /api/games/%s response: %v (body: %s)", roomCode, err, getRaw)
	}
	if getBody.Status != "active" {
		t.Errorf("REST status = %q, want %q", getBody.Status, "active")
	}
	if len(getBody.Seats) != 2 {
		t.Fatalf("REST len(seats) = %d, want 2 (seats: %+v)", len(getBody.Seats), getBody.Seats)
	}
	for _, s := range getBody.Seats {
		if s.Name == "" {
			t.Errorf("REST seat %d has no occupant name, want it filled (seats: %+v)", s.Seat, getBody.Seats)
		}
	}
}

// TestMCPJoinGame_RoleOmitted_DefaultsToPlayer mirrors
// TestPostJoin_RoleOmitted_DefaultsToPlayer: omitting the "role" argument
// entirely on join_game must behave exactly like role:"player", not be
// rejected and not be silently treated as a spectator.
func TestMCPJoinGame_RoleOmitted_DefaultsToPlayer(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "join_game", map[string]any{
		"room_code":   roomCode,
		"player_name": "Carol",
	})
	if res.IsError {
		t.Fatalf("join_game IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var body mcpJoinResult
	decodeToolResult(t, res, &body)

	if body.Role != "player" {
		t.Errorf("role = %q, want %q (role omitted from args should default to player)", body.Role, "player")
	}
	if body.Seat != 2 {
		t.Errorf("seat = %d, want 2", body.Seat)
	}
	if body.Color != "white" {
		t.Errorf("color = %q, want %q", body.Color, "white")
	}
	if body.Status != "active" {
		t.Errorf("status = %q, want %q (filling the last seat should activate the game)", body.Status, "active")
	}
}

// TestMCPJoinGame_Spectator_OmitsSeatAndColor covers join_game's spectator
// path: role:"spectator" succeeds, and the result's "seat"/"color" keys are
// genuinely ABSENT from the JSON payload (checked via map[string]any, not
// merely zero-valued — mirroring TestPostJoin_Spectator_OmitsSeatAndColor's
// REST assertion, since a struct decode alone can't distinguish "absent"
// from "present with zero value").
func TestMCPJoinGame_Spectator_OmitsSeatAndColor(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "join_game", map[string]any{
		"room_code":   roomCode,
		"player_name": "Dana",
		"role":        "spectator",
	})
	if res.IsError {
		t.Fatalf("join_game IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var asMap map[string]any
	decodeToolResult(t, res, &asMap)

	if role, _ := asMap["role"].(string); role != "spectator" {
		t.Errorf("role = %v, want %q", asMap["role"], "spectator")
	}
	if status, _ := asMap["status"].(string); status != "waiting" {
		t.Errorf("status = %v, want %q (game had only the creator seated)", asMap["status"], "waiting")
	}
	if _, present := asMap["seat"]; present {
		t.Errorf("result contains a \"seat\" key for a spectator, want it omitted entirely (result: %+v)", asMap)
	}
	if _, present := asMap["color"]; present {
		t.Errorf("result contains a \"color\" key for a spectator, want it omitted entirely (result: %+v)", asMap)
	}
}

// TestMCPJoinGame_SeatsFull_IsToolLevelError covers join_game's seats-full
// edge case, mirroring TestPostJoin_SeatsFull_Returns409's REST behavior via
// session.ErrSeatsFull: after both seats are filled (Alice via REST create,
// Bob via REST join), a THIRD distinct player name joining via join_game
// with role:"player" must be a tool-level error (IsError == true), not a
// transport-level failure and not a successful join.
func TestMCPJoinGame_SeatsFull_IsToolLevelError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL) // Alice (seat 1) + Bob (seat 2), via REST
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "join_game", map[string]any{
		"room_code":   roomCode,
		"player_name": "Carol",
		"role":        "player",
	})

	if !res.IsError {
		t.Errorf("join_game on a seats-full room: IsError = false, want true (content: %+v)", res.Content)
	}
	if toolErrorText(res) == "" {
		t.Errorf("join_game on a seats-full room: error content is empty, want a non-empty message")
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

// TestMCPJoinGame_BlankPlayerName_IsToolLevelError mirrors
// TestPostJoin_BlankPlayerName_Returns400: a blank/whitespace-only
// player_name must be a tool-level error, per docs/history/plan.md section 6.10's
// "blank/whitespace required string args get the same pre-call 400-
// equivalent rejection REST uses."
func TestMCPJoinGame_BlankPlayerName_IsToolLevelError(t *testing.T) {
	cases := []struct {
		name       string
		playerName string
	}{
		{name: "empty string", playerName: ""},
		{name: "whitespace only", playerName: "   "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := newRESTTestServer(t)
			roomCode := createRoom(t, baseURL, "Alice")
			c := newMCPClient(t, baseURL)

			res := callTool(t, c, "join_game", map[string]any{
				"room_code":   roomCode,
				"player_name": tc.playerName,
				"role":        "player",
			})

			if !res.IsError {
				t.Errorf("join_game with player_name=%q: IsError = false, want true (content: %+v)", tc.playerName, res.Content)
			}
		})
	}
}

// TestMCPJoinGame_UnknownRoomCode_IsToolLevelError mirrors
// TestPostJoin_UnknownRoomCode_Returns404: joining a room code that was
// never created must be a tool-level error (the underlying store.ErrNotFound
// reported via mcp.NewToolResultError, not a transport failure).
func TestMCPJoinGame_UnknownRoomCode_IsToolLevelError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "join_game", map[string]any{
		"room_code":   "ZZZZZZ",
		"player_name": "Alice",
		"role":        "player",
	})

	if !res.IsError {
		t.Errorf("join_game with unknown room_code: IsError = false, want true (content: %+v)", res.Content)
	}
}

// TestMCPGetState_MatchesRESTGetState_ForSameRoom covers backlog item 34's
// AC itself, in full: after activating a game (REST create + REST join) and
// making at least one legal move via REST (so state isn't the trivial
// initial board), a call to the get_state MCP tool for that room and a
// plain REST GET /api/games/{code}/state call for the SAME room must return
// EQUIVALENT data — same status, same turn_seat, same move_seq, and the same
// board_state (compared structurally, decoding both as
// map[string]occupant, since exact byte formatting between the two surfaces
// might differ even if semantically identical; a raw string/byte compare is
// deliberately NOT used here).
func TestMCPGetState_MatchesRESTGetState_ForSameRoom(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL) // Alice (seat 1, black) + Bob (seat 2, white), active

	moveResp, moveRaw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")
	if moveResp.StatusCode != http.StatusOK {
		t.Fatalf("setup move status = %d, want %d (body: %s)", moveResp.StatusCode, http.StatusOK, moveRaw)
	}

	// REST side: GET /api/games/{code}/state.
	restState := getState(t, baseURL, roomCode)

	// MCP side: the get_state tool, against the same room.
	c := newMCPClient(t, baseURL)
	res := callTool(t, c, "get_state", map[string]any{
		"room_code": roomCode,
	})
	if res.IsError {
		t.Fatalf("get_state IsError = true, want false (error: %s)", toolErrorText(res))
	}
	var mcpState mcpGetStateResult
	decodeToolResult(t, res, &mcpState)

	// Same status, turn_seat, move_seq.
	if mcpState.Status != restState.Status {
		t.Errorf("MCP status = %q, REST status = %q, want equal", mcpState.Status, restState.Status)
	}
	if (mcpState.TurnSeat == nil) != (restState.TurnSeat == nil) {
		t.Errorf("MCP turn_seat = %v, REST turn_seat = %v, want both nil or both non-nil", mcpState.TurnSeat, restState.TurnSeat)
	} else if mcpState.TurnSeat != nil && restState.TurnSeat != nil && *mcpState.TurnSeat != *restState.TurnSeat {
		t.Errorf("MCP turn_seat = %d, REST turn_seat = %d, want equal", *mcpState.TurnSeat, *restState.TurnSeat)
	}
	if mcpState.MoveSeq != restState.MoveSeq {
		t.Errorf("MCP move_seq = %d, REST move_seq = %d, want equal", mcpState.MoveSeq, restState.MoveSeq)
	}
	// Sanity: this is meant to be a non-trivial state (a move happened).
	if restState.MoveSeq == 0 {
		t.Fatalf("precondition failed: REST move_seq = 0 after a move was made, want >= 1")
	}

	// Same board_state, compared structurally (not byte-for-byte).
	mcpOccupants := decodeOccupants(t, mcpState.BoardState)
	restOccupants := decodeOccupants(t, restState.BoardState)

	if len(mcpOccupants) != len(restOccupants) {
		t.Fatalf("MCP board_state has %d occupants, REST board_state has %d, want equal (MCP: %+v, REST: %+v)",
			len(mcpOccupants), len(restOccupants), mcpOccupants, restOccupants)
	}
	for pos, want := range restOccupants {
		got, ok := mcpOccupants[pos]
		if !ok {
			t.Errorf("MCP board_state missing occupant at %q, want %+v (MCP occupants: %+v)", pos, want, mcpOccupants)
			continue
		}
		if got != want {
			t.Errorf("MCP board_state[%q] = %+v, REST board_state[%q] = %+v, want equal", pos, got, pos, want)
		}
	}
}

// TestMCPGetState_UnknownRoomCode_IsToolLevelError mirrors
// TestGetGameState_UnknownRoomCode_Returns404: get_state for a room code
// that was never created must be a tool-level error, not a transport
// failure and not a successful (empty/zero) result.
func TestMCPGetState_UnknownRoomCode_IsToolLevelError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "get_state", map[string]any{
		"room_code": "ZZZZZZ",
	})

	if !res.IsError {
		t.Errorf("get_state with unknown room_code: IsError = false, want true (content: %+v)", res.Content)
	}
	if toolErrorText(res) == "" {
		t.Errorf("get_state with unknown room_code: error content is empty, want a non-empty message")
	}
}

// TestMCPGetState_ImmediatelyAfterCreation_ReturnsWaitingWithZeroMoveSeq is a
// second, simpler data point for get_state (no moves made, game not even
// active yet): status "waiting", move_seq 0, turn_seat nil (JSON null), and
// board_state reflecting the initial setup — mirroring
// TestGetGameState_ImmediatelyAfterCreation_ReturnsInitialBoardState.
func TestMCPGetState_ImmediatelyAfterCreation_ReturnsWaitingWithZeroMoveSeq(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "get_state", map[string]any{
		"room_code": roomCode,
	})
	if res.IsError {
		t.Fatalf("get_state IsError = true, want false (error: %s)", toolErrorText(res))
	}
	var mcpState mcpGetStateResult
	decodeToolResult(t, res, &mcpState)

	if mcpState.Status != "waiting" {
		t.Errorf("status = %q, want %q", mcpState.Status, "waiting")
	}
	if mcpState.MoveSeq != 0 {
		t.Errorf("move_seq = %d, want 0", mcpState.MoveSeq)
	}
	if mcpState.TurnSeat != nil {
		t.Errorf("turn_seat = %v, want nil (JSON null)", *mcpState.TurnSeat)
	}

	occupants := decodeOccupants(t, mcpState.BoardState)
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
			t.Errorf("board_state missing occupant at %q, want %+v", pos, want)
			continue
		}
		if got != want {
			t.Errorf("board_state[%q] = %+v, want %+v", pos, got, want)
		}
	}
}

// TestMCPListLegalMoves_UnmovedStartingStone_ReturnsExactlyThreeDestinations
// mirrors TestGetLegalMoves_UnmovedStartingStone_ReturnsExactlyThreeDestinations:
// right after game creation/activation, before any moves, black's stone at
// "4,0" has exactly 3 empty graph neighbors on the 9x9 orthogonal4 grid —
// "3,0", "5,0", "4,1" — and list_legal_moves must return exactly those 3, no
// more, no fewer.
func TestMCPListLegalMoves_UnmovedStartingStone_ReturnsExactlyThreeDestinations(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "list_legal_moves", map[string]any{
		"room_code": roomCode,
		"from":      "4,0",
	})
	if res.IsError {
		t.Fatalf("list_legal_moves IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var got []string
	decodeToolResult(t, res, &got)

	want := []string{"3,0", "5,0", "4,1"}
	gotSorted := sortedStrings(got)
	wantSorted := sortedStrings(want)

	if len(gotSorted) != len(wantSorted) {
		t.Fatalf("list_legal_moves(from=4,0) = %v, want exactly %v", got, want)
	}
	for i := range gotSorted {
		if gotSorted[i] != wantSorted[i] {
			t.Fatalf("list_legal_moves(from=4,0) = %v, want exactly %v", got, want)
		}
	}
}

// TestMCPListLegalMoves_EmptyPosition_ReturnsEmptyArrayNotError mirrors
// TestGetLegalMoves_EmptyPosition_ReturnsEmptyArray: a `from` position with
// no occupant is not an error (this is an AI convenience query, not a move
// attempt, per docs/history/plan.md section 6.8/6.10) — IsError must be false, and
// the result must be an empty array (not null, not an error).
func TestMCPListLegalMoves_EmptyPosition_ReturnsEmptyArrayNotError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "list_legal_moves", map[string]any{
		"room_code": roomCode,
		"from":      "5,5",
	})
	if res.IsError {
		t.Fatalf("list_legal_moves(from=5,5) IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var got []string
	decodeToolResult(t, res, &got)

	if len(got) != 0 {
		t.Errorf("list_legal_moves(from=5,5) = %v, want an empty array (no occupant there)", got)
	}
}

// TestMCPListLegalMoves_UnknownRoomCode_IsToolLevelError mirrors
// TestGetLegalMoves_UnknownRoomCode_Returns404: an unknown room code must be
// a tool-level error, not an empty array and not a transport failure.
func TestMCPListLegalMoves_UnknownRoomCode_IsToolLevelError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "list_legal_moves", map[string]any{
		"room_code": "ZZZZZZ",
		"from":      "0,0",
	})

	if !res.IsError {
		t.Errorf("list_legal_moves with unknown room_code: IsError = false, want true (content: %+v)", res.Content)
	}
}
