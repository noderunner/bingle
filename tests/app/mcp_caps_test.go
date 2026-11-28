package app_test

// MCP's tools ask for the capability their REST twin does (ADR 0071, #376):
// the route rule asks only for play of everything under /mcp.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/config"
)

// mcpWith connects an MCP client with the headers given (none: a guest).
func mcpWith(t *testing.T, base string, headers map[string]string) *client.Client {
	t.Helper()
	var opts []transport.StreamableHTTPCOption
	if headers != nil {
		opts = append(opts, transport.WithHTTPHeaders(headers))
	}
	c, err := client.NewStreamableHttpClient(base+"/mcp", opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	var init mcp.InitializeRequest
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "bingle-test-agent", Version: "0.0.1"}
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatal(err)
	}
	return c
}

// refusedFor reports a tool's refusal for want: an error result whose status
// is 403 and whose words name the capability, as REST's do.
func refusedFor(t *testing.T, who string, res *mcp.CallToolResult, want string) {
	t.Helper()
	var out struct {
		Error  string `json:"error"`
		Status int    `json:"status"`
	}
	if !res.IsError {
		t.Errorf("%s: not refused", who)
		return
	}
	decodeToolResult(t, res, &out)
	if out.Status != http.StatusForbidden || !strings.Contains(out.Error, `needs "`+want+`"`) {
		t.Errorf("%s: %d %q, want 403 needing %q", who, out.Status, out.Error, want)
	}
}

func TestMCPBatchToolsNeedRunBatch(t *testing.T) {
	base, ana, admin := limitedServer(t, config.BatchLimits{})
	var tok struct{ Token string }
	if code := admin.do("POST", "/auth/tokens", map[string]any{"name": "an agent", "scopes": []string{"play"}}, &tok); code != 201 {
		t.Fatalf("token: %d", code)
	}
	member := map[string]string{"Cookie": auth.SessionCookie + "=" + ana, auth.PageHeader: "1"}
	for who, c := range map[string]*client.Client{
		"a guest":            mcpWith(t, base, nil),
		"a token (no scope)": mcpWith(t, base, map[string]string{"Authorization": "Bearer " + tok.Token}),
	} {
		refusedFor(t, who+" starting", callTool(t, c, "start_batch", map[string]any{"spec": hexBatch()}), auth.CapRunBatch)
		refusedFor(t, who+" cancelling", callTool(t, c, "cancel_batch", map[string]any{"batch_id": 1}), auth.CapRunBatch)
	}
	// An account, which holds run_batch, still may, and REST says the same of a guest.
	c := mcpWith(t, base, member)
	res := callTool(t, c, "start_batch", map[string]any{"spec": hexBatch()})
	if res.IsError {
		t.Fatalf("an account starting: %s", toolErrorText(res))
	}
	var made struct {
		ID int64 `json:"id"`
	}
	decodeToolResult(t, res, &made)
	if res := callTool(t, c, "cancel_batch", map[string]any{"batch_id": made.ID}); res.IsError {
		t.Errorf("an account cancelling: %s", toolErrorText(res))
	}
	if code := doJSONAs(t, "", "POST", base+"/api/batches", hexBatch(), nil); code != http.StatusForbidden {
		t.Errorf("a guest over REST: %d, want 403", code)
	}
	// Reading a batch is open over both doors.
	if res := callTool(t, mcpWith(t, base, nil), "list_batches", nil); res.IsError {
		t.Errorf("a guest listing: %s", toolErrorText(res))
	}
}

// The other tools that make something or take part: create_game and
// import_game need create_game, join_game join, send_chat chat_game, a
// tournament's check-in what entering does.
func TestMCPToolsNeedTheirTwinsCapability(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Addr = "127.0.0.1:0"
	a, base := serve(t, cfg)
	ctx := context.Background()
	for _, cp := range []string{auth.CapCreateGame, auth.CapJoin, auth.CapChatGame} {
		if err := a.Caps().Set(ctx, auth.ClassGuest, cp, false); err != nil {
			t.Fatal(err)
		}
	}
	c := mcpWith(t, base, nil)
	for tool, args := range map[string]map[string]any{
		"create_game":         {"game_def_id": "tic-tac-toe", "player_name": "Ann"},
		"import_game":         {"game_def_id": "tic-tac-toe", "player_name": "Ann"},
		"join_game":           {"room_code": "ABCD", "player_name": "Ann"},
		"send_chat":           {"room_code": "ABCD", "player_name": "Ann", "text": "hi"},
		"check_in_tournament": {"tournament_id": 1},
	} {
		want := map[string]string{"create_game": auth.CapCreateGame, "import_game": auth.CapCreateGame, "join_game": auth.CapJoin,
			"send_chat": auth.CapChatGame, "check_in_tournament": auth.CapEnterTournament}[tool]
		refusedFor(t, tool, callTool(t, c, tool, args), want)
	}
	// Play alone is no longer enough for what makes a game; a game that is
	// not refused for it stays so.
	if res := callTool(t, c, "list_game_defs", nil); res.IsError {
		t.Errorf("list_game_defs: %s", toolErrorText(res))
	}
	if err := a.Caps().Set(ctx, auth.ClassGuest, auth.CapCreateGame, true); err != nil {
		t.Fatal(err)
	}
	if res := callTool(t, c, "create_game", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ann"}); res.IsError {
		t.Errorf("create_game with the capability: %s", toolErrorText(res))
	}
}
