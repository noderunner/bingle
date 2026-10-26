// Tests of the MCP make_move tool: moves reach WebSocket spectators, and illegal moves, wrong turns and inactive games are tool-level errors.

package app_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// mcpMoveResult mirrors one "move" object in make_move's result and one
// entry of get_moves' result array, from docs/history/plan.md section 6.10:
// {"seq","seat","color","from","to","captured"} — the same wire shape as
// REST's moveResult (rest_make_move_test.go), kept as its own type here so
// this file has no compile-time dependency beyond what it already needs.
type mcpMoveResult struct {
	Seq      int     `json:"seq"`
	Seat     int     `json:"seat"`
	Color    string  `json:"color"`
	From     string  `json:"from"`
	To       string  `json:"to"`
	Captured *string `json:"captured"`
}

// mcpMakeMoveResult mirrors make_move's full result shape from docs/history/plan.md
// section 6.10: {"move": {...}, "state": {"board_state","turn_seat","status",
// "move_seq"}} — the MCP equivalent of REST's makeMoveResponse
// (rest_make_move_test.go).
type mcpMakeMoveResult struct {
	Move  mcpMoveResult     `json:"move"`
	State mcpGetStateResult `json:"state"` // mcpGetStateResult from mcp_join_state_test.go
}

// TestMCPMakeMove_LegalMove_BroadcastsToOpenSpectatorWebSocket covers backlog
// item 35's AC itself, in full: with an active game (Alice seat 1/black, Bob
// seat 2/white) and an already-open spectator WebSocket connection to that
// room (initial "joined" event already consumed), Alice (holding seat 1, on
// seat 1's turn) calls the make_move MCP tool with a legal, non-capturing
// move. The tool result must show move.seq==1, the submitted seat/color/
// from/to, captured==nil, state.turn_seat==2, and state.status=="active".
// THEN — without the test re-polling REST — the NEXT message read off the
// SAME, STILL-OPEN WebSocket connection (bounded by wsReadTimeout via
// readFirstEvent, so a genuine failure here reports cleanly rather than
// hanging) must be a "move" event whose data matches the MCP call: this is
// the literal AC, proving an MCP-initiated move broadcasts over WS exactly
// like a REST-initiated one does (docs/history/plan.md section 6.10's "falls out for
// free" note, since make_move calls the same Manager.MakeMove REST uses).
func TestMCPMakeMove_LegalMove_BroadcastsToOpenSpectatorWebSocket(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL) // Alice (seat 1, black) + Bob (seat 2, white), active

	conn := dialSpectator(t, baseURL, roomCode, "Watcher")

	c := newMCPClient(t, baseURL)
	res := callTool(t, c, "make_move", map[string]any{
		"room_code":   roomCode,
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	})
	if res.IsError {
		t.Fatalf("make_move IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var result mcpMakeMoveResult
	decodeToolResult(t, res, &result)

	if result.Move.Seq != 1 {
		t.Errorf("move.seq = %d, want 1", result.Move.Seq)
	}
	if result.Move.Seat != 1 {
		t.Errorf("move.seat = %d, want 1", result.Move.Seat)
	}
	if result.Move.Color != "black" {
		t.Errorf("move.color = %q, want %q", result.Move.Color, "black")
	}
	if result.Move.From != "2,0" {
		t.Errorf("move.from = %q, want %q", result.Move.From, "2,0")
	}
	if result.Move.To != "1,0" {
		t.Errorf("move.to = %q, want %q", result.Move.To, "1,0")
	}
	if result.Move.Captured != nil {
		t.Errorf("move.captured = %v, want nil (JSON null; nothing was at 1,0)", *result.Move.Captured)
	}
	if result.State.TurnSeat == nil || *result.State.TurnSeat != 2 {
		t.Errorf("state.turn_seat = %v, want 2 (Bob/white's turn next)", result.State.TurnSeat)
	}
	if result.State.Status != "active" {
		t.Errorf("state.status = %q, want %q", result.State.Status, "active")
	}
	if result.State.MoveSeq != 1 {
		t.Errorf("state.move_seq = %d, want 1", result.State.MoveSeq)
	}

	// The literal AC: the SAME still-open spectator WebSocket connection must
	// receive a "move" event, with no re-polling of REST/MCP in between.
	ev := readFirstEvent(t, conn)

	if ev.Type != "move" {
		t.Fatalf("event type = %q, want %q (raw data: %s)", ev.Type, "move", ev.Data)
	}

	var data wsMoveData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		t.Fatalf("decode move event data: %v (raw: %s)", err, ev.Data)
	}

	if data.Seq != 1 {
		t.Errorf("ws data.seq = %d, want 1", data.Seq)
	}
	if data.Seat != 1 {
		t.Errorf("ws data.seat = %d, want 1", data.Seat)
	}
	if data.Color != "black" {
		t.Errorf("ws data.color = %q, want %q", data.Color, "black")
	}
	if data.From != "2,0" {
		t.Errorf("ws data.from = %q, want %q", data.From, "2,0")
	}
	if data.To != "1,0" {
		t.Errorf("ws data.to = %q, want %q", data.To, "1,0")
	}
	if data.Captured != nil {
		t.Errorf("ws data.captured = %v, want nil", *data.Captured)
	}
	if data.TurnSeat == nil || *data.TurnSeat != 2 {
		t.Errorf("ws data.turn_seat = %v, want 2", data.TurnSeat)
	}
	if data.Status != "active" {
		t.Errorf("ws data.status = %q, want %q", data.Status, "active")
	}

	occupants := decodeOccupants(t, data.BoardState)
	if _, stillAtOld := occupants["2,0"]; stillAtOld {
		t.Errorf("ws data.board_state still has an occupant at \"2,0\": %+v", occupants)
	}
	got, ok := occupants["1,0"]
	if !ok {
		t.Fatalf("ws data.board_state has no occupant at \"1,0\": %+v", occupants)
	}
	if got != (occupant{Piece: "stone", Color: "black"}) {
		t.Errorf("ws data.board_state[\"1,0\"] = %+v, want {Piece: stone, Color: black}", got)
	}
}

// TestMCPMakeMove_WrongTurn_IsToolLevelError covers make_move's wrong-turn
// edge case, mirroring TestPostMove_WrongTurn_Returns403AndStateUnchanged:
// after the game is active (Alice seat 1/black holds the first turn), Bob
// (seat 2/white) attempting a move must be a tool-level error (session's
// ErrNotYourTurn), not a transport failure and not a successful move.
func TestMCPMakeMove_WrongTurn_IsToolLevelError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL) // Alice seat 1/black holds the first turn
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "make_move", map[string]any{
		"room_code":   roomCode,
		"player_name": "Bob",
		"from":        "2,8",
		"to":          "1,8",
	})

	if !res.IsError {
		t.Errorf("make_move out of turn: IsError = false, want true (content: %+v)", res.Content)
	}
	if toolErrorText(res) == "" {
		t.Errorf("make_move out of turn: error content is empty, want a non-empty message")
	}

	// No state change: still move_seq 0, still Alice/seat 1's turn.
	after := getState(t, baseURL, roomCode)
	if after.MoveSeq != 0 {
		t.Errorf("move_seq after rejected out-of-turn move = %d, want 0 (unchanged)", after.MoveSeq)
	}
	if after.TurnSeat == nil || *after.TurnSeat != 1 {
		t.Errorf("turn_seat after rejected out-of-turn move = %v, want 1 (unchanged)", after.TurnSeat)
	}
}

// TestMCPMakeMove_GameNotActive_IsToolLevelError covers make_move on a game
// that isn't active yet (only the creator has joined, so it's still
// "waiting") — a tool-level error (session's ErrGameNotActive), mirroring
// TestPostMove_GameNotActive_Returns409.
func TestMCPMakeMove_GameNotActive_IsToolLevelError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice") // no second player joined -> still "waiting"
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "make_move", map[string]any{
		"room_code":   roomCode,
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	})

	if !res.IsError {
		t.Errorf("make_move on a non-active game: IsError = false, want true (content: %+v)", res.Content)
	}
	if toolErrorText(res) == "" {
		t.Errorf("make_move on a non-active game: error content is empty, want a non-empty message")
	}
}

// TestMCPMakeMove_IllegalMove_IsToolLevelErrorAndBoardUnchanged covers
// make_move with a genuinely non-adjacent destination ("2,0" -> "5,5",
// nowhere near each other on the 9x9 orthogonal4 grid), mirroring
// TestPostMove_IllegalMove_Returns400AndBoardUnchanged: a tool-level error,
// and a following plain REST GET .../state shows nothing changed (move_seq
// still 0, same board_state).
func TestMCPMakeMove_IllegalMove_IsToolLevelErrorAndBoardUnchanged(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	before := getState(t, baseURL, roomCode)
	if before.MoveSeq != 0 {
		t.Fatalf("precondition failed: move_seq = %d before any move, want 0", before.MoveSeq)
	}

	c := newMCPClient(t, baseURL)
	res := callTool(t, c, "make_move", map[string]any{
		"room_code":   roomCode,
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "5,5",
	})

	if !res.IsError {
		t.Errorf("make_move with a non-adjacent destination: IsError = false, want true (content: %+v)", res.Content)
	}
	if toolErrorText(res) == "" {
		t.Errorf("make_move with a non-adjacent destination: error content is empty, want a non-empty message")
	}

	after := getState(t, baseURL, roomCode)
	if after.MoveSeq != before.MoveSeq {
		t.Errorf("move_seq after rejected illegal move = %d, want %d (unchanged)", after.MoveSeq, before.MoveSeq)
	}
	if after.Status != "active" {
		t.Errorf("status after rejected illegal move = %q, want %q (unchanged)", after.Status, "active")
	}

	beforeOccupants := decodeOccupants(t, before.BoardState)
	afterOccupants := decodeOccupants(t, after.BoardState)
	if len(afterOccupants) != len(beforeOccupants) {
		t.Fatalf("board_state occupant count changed: before %d, after %d", len(beforeOccupants), len(afterOccupants))
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

// TestMCPMakeMove_UnknownRoomCode_IsToolLevelError mirrors
// TestPostMove_UnknownRoomCode_Returns404: make_move for a room code that was
// never created must be a tool-level error, not a transport failure.
func TestMCPMakeMove_UnknownRoomCode_IsToolLevelError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "make_move", map[string]any{
		"room_code":   "ZZZZZZ",
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	})

	if !res.IsError {
		t.Errorf("make_move with unknown room_code: IsError = false, want true (content: %+v)", res.Content)
	}
	if toolErrorText(res) == "" {
		t.Errorf("make_move with unknown room_code: error content is empty, want a non-empty message")
	}
}

// TestMCPGetMoves_AfterThreeMoves_ReturnsExactlyThreeInAscendingSeq covers
// get_moves' happy path, mirroring
// TestGetMoves_AfterThreeMoves_ReturnsExactlyThreeInAscendingSeq: after 3
// moves submitted via REST (both surfaces share the same persisted history,
// per docs/history/plan.md section 6.10), get_moves with no since_seq returns an
// array of exactly those 3 entries, seq 1/2/3 in ascending order, each
// matching what was actually submitted.
func TestMCPGetMoves_AfterThreeMoves_ReturnsExactlyThreeInAscendingSeq(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)
	submitted := submitThreeMoves(t, baseURL, roomCode)

	c := newMCPClient(t, baseURL)
	res := callTool(t, c, "get_moves", map[string]any{
		"room_code": roomCode,
	})
	if res.IsError {
		t.Fatalf("get_moves IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var history []mcpMoveResult
	decodeToolResult(t, res, &history)

	if len(history) != 3 {
		t.Fatalf("len(history) = %d, want exactly 3 (history: %+v)", len(history), history)
	}

	wantSeqs := []int{1, 2, 3}
	for i, entry := range history {
		if entry.Seq != wantSeqs[i] {
			t.Errorf("history[%d].seq = %d, want %d (ascending order)", i, entry.Seq, wantSeqs[i])
		}
		want := submitted[i]
		if entry.Seat != want.Seat {
			t.Errorf("history[%d].seat = %d, want %d", i, entry.Seat, want.Seat)
		}
		if entry.Color != want.Color {
			t.Errorf("history[%d].color = %q, want %q", i, entry.Color, want.Color)
		}
		if entry.From != want.From {
			t.Errorf("history[%d].from = %q, want %q", i, entry.From, want.From)
		}
		if entry.To != want.To {
			t.Errorf("history[%d].to = %q, want %q", i, entry.To, want.To)
		}
		if entry.Captured != nil {
			t.Errorf("history[%d].captured = %v, want nil (all three moves here are non-capturing)", i, *entry.Captured)
		}
	}
}

// TestMCPGetMoves_MixedRESTAndMCPMoves_SharedHistory is a second data point
// for get_moves' happy path, driving moves through BOTH surfaces (REST then
// MCP) to confirm they share the same persisted history: seq 1 via REST,
// seq 2 via the make_move MCP tool, then get_moves must return both, in
// order.
func TestMCPGetMoves_MixedRESTAndMCPMoves_SharedHistory(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	restResp, restRaw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")
	if restResp.StatusCode != http.StatusOK {
		t.Fatalf("setup REST move status = %d, want %d (body: %s)", restResp.StatusCode, http.StatusOK, restRaw)
	}

	c := newMCPClient(t, baseURL)
	moveRes := callTool(t, c, "make_move", map[string]any{
		"room_code":   roomCode,
		"player_name": "Bob",
		"from":        "2,8",
		"to":          "1,8",
	})
	if moveRes.IsError {
		t.Fatalf("setup MCP make_move IsError = true, want false (error: %s)", toolErrorText(moveRes))
	}

	res := callTool(t, c, "get_moves", map[string]any{
		"room_code": roomCode,
	})
	if res.IsError {
		t.Fatalf("get_moves IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var history []mcpMoveResult
	decodeToolResult(t, res, &history)

	if len(history) != 2 {
		t.Fatalf("len(history) = %d, want exactly 2 (history: %+v)", len(history), history)
	}
	if history[0].Seq != 1 || history[0].Seat != 1 || history[0].From != "2,0" || history[0].To != "1,0" {
		t.Errorf("history[0] = %+v, want the REST-submitted move (seq 1, seat 1, 2,0 -> 1,0)", history[0])
	}
	if history[1].Seq != 2 || history[1].Seat != 2 || history[1].From != "2,8" || history[1].To != "1,8" {
		t.Errorf("history[1] = %+v, want the MCP-submitted move (seq 2, seat 2, 2,8 -> 1,8)", history[1])
	}
}

// TestMCPGetMoves_SinceLastSeq_ReturnsEmptyArrayNotError covers get_moves
// with since_seq set to the last move's seq: not an error, just an empty
// array — mirroring TestGetMoves_SinceSeqBeyondLast_ReturnsEmptyArray's
// "empty is not an error" assertion (here using the exact last seq rather
// than a seq beyond it, to also confirm the boundary itself excludes that
// move, not merely large values).
func TestMCPGetMoves_SinceLastSeq_ReturnsEmptyArrayNotError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)
	submitted := submitThreeMoves(t, baseURL, roomCode)
	lastSeq := submitted[len(submitted)-1].Seq

	c := newMCPClient(t, baseURL)
	res := callTool(t, c, "get_moves", map[string]any{
		"room_code": roomCode,
		"since_seq": lastSeq,
	})
	if res.IsError {
		t.Fatalf("get_moves(since_seq=%d) IsError = true, want false (error: %s)", lastSeq, toolErrorText(res))
	}

	var history []mcpMoveResult
	decodeToolResult(t, res, &history)

	if len(history) != 0 {
		t.Errorf("get_moves(since_seq=%d) = %+v, want an empty array (nothing after the last move)", lastSeq, history)
	}
}

// TestMCPGetMoves_NewGameNoMoves_ReturnsEmptyArray covers a brand-new game
// with zero moves made yet (not even activated — only the creator has
// joined): get_moves must succeed (IsError == false) with an empty array,
// not an error, mirroring TestGetMoves_NewGameNoMoves_ReturnsEmptyArray.
func TestMCPGetMoves_NewGameNoMoves_ReturnsEmptyArray(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "get_moves", map[string]any{
		"room_code": roomCode,
	})
	if res.IsError {
		t.Fatalf("get_moves on a new game IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var history []mcpMoveResult
	decodeToolResult(t, res, &history)

	if len(history) != 0 {
		t.Errorf("get_moves on a new game = %+v, want an empty array", history)
	}
}

// TestMCPGetMoves_UnknownRoomCode_IsToolLevelError mirrors
// TestGetMoves_UnknownRoomCode_Returns404: get_moves for a room code that was
// never created must be a tool-level error, not an empty array and not a
// transport failure.
func TestMCPGetMoves_UnknownRoomCode_IsToolLevelError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "get_moves", map[string]any{
		"room_code": "ZZZZZZ",
	})

	if !res.IsError {
		t.Errorf("get_moves with unknown room_code: IsError = false, want true (content: %+v)", res.Content)
	}
	if toolErrorText(res) == "" {
		t.Errorf("get_moves with unknown room_code: error content is empty, want a non-empty message")
	}
}
