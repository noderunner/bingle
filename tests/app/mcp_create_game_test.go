// Tests of the MCP tools that list game definitions and create games, and that the rooms they make can be joined over REST.

package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// mcpGameDefSummary mirrors one entry of list_game_defs' result array, from
// docs/history/plan.md section 6.10: [{"id", "name", "seats_required"}] — the same
// wire shape as GET /api/game-defs (gameDefSummary, already defined in
// rest_create_game_test.go), but kept as its own type in this file so this
// file has no compile-time dependency on that one beyond what it already
// needs (baseURL helpers, room-code/response types it reuses directly).
type mcpGameDefSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SeatsRequired int    `json:"seats_required"`
}

// newMCPClient connects an MCP client to baseURL's "/mcp" endpoint over
// Streamable HTTP, performs the MCP initialize handshake, and registers
// cleanup to close the client. It fails the test (via t.Fatalf) if
// connecting or initializing doesn't succeed.
func newMCPClient(t *testing.T, baseURL string) *client.Client {
	t.Helper()

	c, err := client.NewStreamableHttpClient(baseURL + "/mcp")
	if err != nil {
		t.Fatalf("client.NewStreamableHttpClient(%q): %v", baseURL+"/mcp", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("client.Start: %v (is internal/mcpserver mounted at /mcp in internal/app's router?)", err)
	}

	var initReq mcp.InitializeRequest
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "bingle-test-client", Version: "0.0.1"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("client.Initialize: %v (is internal/mcpserver mounted at /mcp in internal/app's router?)", err)
	}

	return c
}

// callTool invokes toolName with args (may be nil) against c and returns the
// raw *mcp.CallToolResult. It fails the test on a *transport-level* error
// (connection refused, malformed JSON-RPC, etc.) via t.Fatalf, since that's
// never the expected way for a business-logic failure (e.g. an unknown
// game_def_id) to surface for an MCP tool call — see
// TestMCPCreateGame_UnknownGameDefID_IsToolLevelError below, which asserts
// the opposite: CallTool itself must return err == nil even when the tool
// reports a failure via result.IsError.
func callTool(t *testing.T, c *client.Client, toolName string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var req mcp.CallToolRequest
	req.Params.Name = toolName
	if args != nil {
		req.Params.Arguments = args
	}

	res, err := c.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("CallTool(%q): transport-level error: %v, want nil (business-logic failures must surface as result.IsError, not a transport error)", toolName, err)
	}
	if res == nil {
		t.Fatalf("CallTool(%q): result is nil with err == nil", toolName)
	}
	return res
}

// decodeToolResult unmarshals res's payload into out, trying (in order)
// res.RawStructuredContent, res.StructuredContent, and the text of
// res.Content[0] — mark3labs/mcp-go's CallToolResult supports returning
// data as structured content, raw structured content, or plain text content
// (verified via `go doc github.com/mark3labs/mcp-go/mcp.CallToolResult`),
// and docs/history/plan.md section 6.10 doesn't pin down which representation the
// coder uses for tool results, only the JSON shape of the data itself. It
// fails the test if none of those decode successfully.
func decodeToolResult(t *testing.T, res *mcp.CallToolResult, out any) {
	t.Helper()

	if len(res.RawStructuredContent) > 0 {
		if err := json.Unmarshal(res.RawStructuredContent, out); err == nil {
			return
		}
	}
	if res.StructuredContent != nil {
		if b, err := json.Marshal(res.StructuredContent); err == nil {
			if err := json.Unmarshal(b, out); err == nil {
				return
			}
		}
	}
	if len(res.Content) > 0 {
		if txt := mcp.GetTextFromContent(res.Content[0]); strings.TrimSpace(txt) != "" {
			if err := json.Unmarshal([]byte(txt), out); err == nil {
				return
			}
		}
	}

	t.Fatalf("could not decode tool result into %T: content=%+v structuredContent=%+v rawStructuredContent=%s",
		out, res.Content, res.StructuredContent, res.RawStructuredContent)
}

// toolErrorText returns a best-effort human-readable error message out of a
// tool result whose IsError is true, for assertion failure messages only
// (not itself asserted against any specific wording, since docs/history/plan.md
// section 6.10 doesn't pin down error text).
func toolErrorText(res *mcp.CallToolResult) string {
	if len(res.Content) > 0 {
		return mcp.GetTextFromContent(res.Content[0])
	}
	return "(no content)"
}

// TestMCPListGameDefs_ReturnsDemoFreeplay covers half of backlog item 33's
// AC: calling the list_game_defs MCP tool with no args succeeds
// (IsError == false) and its result includes an entry for
// "demo-freeplay" with name "Demo: Free Movement & Capture" and
// seats_required == 2 — the MCP equivalent of
// TestGetGameDefs_ReturnsDemoFreeplay (rest_create_game_test.go).
func TestMCPListGameDefs_ReturnsDemoFreeplay(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "list_game_defs", nil)
	if res.IsError {
		t.Fatalf("list_game_defs IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var defs []mcpGameDefSummary
	decodeToolResult(t, res, &defs)

	found := false
	for _, d := range defs {
		if d.ID == "demo-freeplay" {
			found = true
			if d.Name != "Demo: Free Movement & Capture" {
				t.Errorf("demo-freeplay name = %q, want %q", d.Name, "Demo: Free Movement & Capture")
			}
			if d.SeatsRequired != 2 {
				t.Errorf("demo-freeplay seats_required = %d, want 2", d.SeatsRequired)
			}
		}
	}
	if !found {
		t.Errorf("list_game_defs result %+v does not contain an entry with id == %q", defs, "demo-freeplay")
	}
}

// TestMCPListGameDefs_CalledTwice_Idempotent is an edge case guarding
// against any accidental per-call mutation or one-shot state in the tool
// handler (e.g. a catalog that's only safely readable once): calling
// list_game_defs twice on the same client must return the same
// demo-freeplay entry both times.
func TestMCPListGameDefs_CalledTwice_Idempotent(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	for i := 0; i < 2; i++ {
		res := callTool(t, c, "list_game_defs", nil)
		if res.IsError {
			t.Fatalf("call %d: list_game_defs IsError = true, want false (error: %s)", i, toolErrorText(res))
		}
		var defs []mcpGameDefSummary
		decodeToolResult(t, res, &defs)

		found := false
		for _, d := range defs {
			if d.ID == "demo-freeplay" {
				found = true
			}
		}
		if !found {
			t.Errorf("call %d: list_game_defs result %+v does not contain an entry with id == %q", i, defs, "demo-freeplay")
		}
	}
}

// TestMCPCreateGame_ProducesRoomJoinableViaREST covers backlog item 33's AC
// itself, in full: calling the create_game MCP tool with
// {"game_def_id": "demo-freeplay", "player_name": "Alice"} succeeds
// (IsError == false) and returns a room_code (6 chars, drawn from
// roomcode.Alphabet), seat == 1, color == "black", status == "waiting" —
// then a PLAIN REST call (GET /api/games/{room_code}, via net/http against
// the SAME httptest.Server instance backing the MCP client) using that
// exact room_code succeeds with 200 and shows the same game: creator
// "Alice" in seat 1. This is the literal AC — a room created via MCP is
// joinable/queryable via REST using the same room code, because both
// surfaces share the same session.Manager/store.Store.
func TestMCPCreateGame_ProducesRoomJoinableViaREST(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "create_game", map[string]any{
		"game_def_id": "demo-freeplay",
		"player_name": "Alice",
	})
	if res.IsError {
		t.Fatalf("create_game IsError = true, want false (error: %s)", toolErrorText(res))
	}

	var created createGameResponse // reused from rest_create_game_test.go: {room_code, seat, color, status}
	decodeToolResult(t, res, &created)

	if !isRoomCode(created.RoomCode) {
		t.Errorf("room_code = %q, want a 6-character code drawn from roomcode.Alphabet", created.RoomCode)
	}
	if created.Seat != 1 {
		t.Errorf("seat = %d, want 1", created.Seat)
	}
	if created.Color != "black" {
		t.Errorf("color = %q, want %q", created.Color, "black")
	}
	if created.Status != "waiting" {
		t.Errorf("status = %q, want %q", created.Status, "waiting")
	}

	// The literal AC: the same room_code is joinable/queryable via the
	// REST API, over a plain net/http client against the same
	// httptest.Server instance the MCP client is talking to.
	resp, raw := getJSON(t, baseURL, "/api/games/"+created.RoomCode) // getJSON from rest_get_game_test.go
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/games/%s (created via MCP): status = %d, want %d (body: %s)", created.RoomCode, resp.StatusCode, http.StatusOK, raw)
	}

	var got getGameResponse // getGameResponse from rest_get_game_test.go
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode GET /api/games/%s response: %v (body: %s)", created.RoomCode, err, raw)
	}

	if got.GameDefID != "demo-freeplay" {
		t.Errorf("REST game_def_id = %q, want %q", got.GameDefID, "demo-freeplay")
	}
	if got.Status != "waiting" {
		t.Errorf("REST status = %q, want %q", got.Status, "waiting")
	}
	if len(got.Seats) != 2 {
		t.Fatalf("REST len(seats) = %d, want 2 (seats: %+v)", len(got.Seats), got.Seats)
	}

	var seat1 *getGameSeat
	for i := range got.Seats {
		if got.Seats[i].Seat == 1 {
			seat1 = &got.Seats[i]
		}
	}
	if seat1 == nil {
		t.Fatalf("REST response has no seat 1 entry: %+v", got.Seats)
	}
	if seat1.Name != "Alice" {
		t.Errorf("REST seat 1 name = %q, want %q (creator from the MCP create_game call)", seat1.Name, "Alice")
	}
	if seat1.Color != "black" {
		t.Errorf("REST seat 1 color = %q, want %q", seat1.Color, "black")
	}
}

// TestMCPCreateGame_TwoCalls_ReturnDifferentRoomCodes is an edge case
// mirroring TestPostGames_TwoCalls_ReturnDifferentRoomCodes: two separate
// create_game MCP calls (different player names) must produce two distinct
// room codes.
func TestMCPCreateGame_TwoCalls_ReturnDifferentRoomCodes(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res1 := callTool(t, c, "create_game", map[string]any{
		"game_def_id": "demo-freeplay",
		"player_name": "Alice",
	})
	if res1.IsError {
		t.Fatalf("first create_game IsError = true, want false (error: %s)", toolErrorText(res1))
	}
	var created1 createGameResponse
	decodeToolResult(t, res1, &created1)

	res2 := callTool(t, c, "create_game", map[string]any{
		"game_def_id": "demo-freeplay",
		"player_name": "Bob",
	})
	if res2.IsError {
		t.Fatalf("second create_game IsError = true, want false (error: %s)", toolErrorText(res2))
	}
	var created2 createGameResponse
	decodeToolResult(t, res2, &created2)

	if created1.RoomCode == "" || created2.RoomCode == "" {
		t.Fatalf("expected non-empty room codes, got %q and %q", created1.RoomCode, created2.RoomCode)
	}
	if created1.RoomCode == created2.RoomCode {
		t.Errorf("two separate create_game calls returned the same room_code %q, want two distinct codes", created1.RoomCode)
	}
}

// TestMCPCreateGame_UnknownGameDefID_IsToolLevelError covers backlog item
// 33's third scenario: create_game with an unknown game_def_id must report
// a TOOL-LEVEL error (result.IsError == true, with CallTool's own Go error
// return value == nil) rather than a transport-level failure — MCP tool
// calls are RPCs that succeed at the transport level even when the
// business logic fails (docs/history/plan.md section 6.10). callTool itself already
// asserts err == nil (it t.Fatalfs on a transport error), so this test's
// job is to assert IsError == true and that no game was actually created
// (no crash, no room code to speak of).
func TestMCPCreateGame_UnknownGameDefID_IsToolLevelError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "create_game", map[string]any{
		"game_def_id": "nonexistent-game-def",
		"player_name": "Alice",
	})

	if !res.IsError {
		t.Errorf("create_game with unknown game_def_id: IsError = false, want true (content: %+v)", res.Content)
	}
	if strings.TrimSpace(toolErrorText(res)) == "" {
		t.Errorf("create_game with unknown game_def_id: error content is empty, want a non-empty message")
	}
}

// TestMCPCreateGame_BlankPlayerName_IsToolLevelError is an edge case
// mirroring TestPostGames_BlankPlayerName_Returns400 (rest_create_game_test.go):
// a blank/whitespace-only player_name must be rejected as a tool-level
// error (per docs/history/plan.md section 6.10, "blank/whitespace required string
// args get the same pre-call 400-equivalent rejection REST uses"), not a
// transport error and not a successful call.
func TestMCPCreateGame_BlankPlayerName_IsToolLevelError(t *testing.T) {
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
			c := newMCPClient(t, baseURL)

			res := callTool(t, c, "create_game", map[string]any{
				"game_def_id": "demo-freeplay",
				"player_name": tc.playerName,
			})

			if !res.IsError {
				t.Errorf("create_game with player_name=%q: IsError = false, want true (content: %+v)", tc.playerName, res.Content)
			}
		})
	}
}

// TestMCPCreateGame_BlankGameDefID_IsToolLevelError is create_game's other
// required-field edge case: a blank game_def_id must also be a tool-level
// error, mirroring TestPostGames_BlankGameDefID_Returns400.
func TestMCPCreateGame_BlankGameDefID_IsToolLevelError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "create_game", map[string]any{
		"game_def_id": "",
		"player_name": "Alice",
	})

	if !res.IsError {
		t.Errorf("create_game with blank game_def_id: IsError = false, want true (content: %+v)", res.Content)
	}
}

// TestMCPCreateGame_MissingArguments_DoesNotCrashClient is a defensive edge
// case: calling create_game with no arguments at all (neither field
// present, as opposed to present-but-blank) must still come back as a
// well-formed tool result the client can read — either a tool-level error,
// or (least likely, but not this test's business to rule out) an MCP
// protocol-level error surfaced as a Go error from CallTool — but it must
// never hang, panic, or crash the server such that the client gets no
// response at all.
func TestMCPCreateGame_MissingArguments_DoesNotCrashClient(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var req mcp.CallToolRequest
	req.Params.Name = "create_game"
	// No Arguments set at all.

	res, err := c.CallTool(ctx, req)
	if err != nil {
		// A protocol-level rejection (e.g. missing required params) is an
		// acceptable outcome here; a hang/timeout is not, and the ctx
		// deadline above ensures we'd see a clear timeout error rather
		// than the test process hanging.
		t.Logf("CallTool with no arguments returned a transport-level error (acceptable): %v", err)
		return
	}
	if res == nil {
		t.Fatalf("CallTool with no arguments: result is nil with err == nil")
	}
	if !res.IsError {
		t.Errorf("CallTool with no arguments: IsError = false, want true (missing required game_def_id/player_name), content: %+v", res.Content)
	}
}
