package mcpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/noderunner/bingle/internal/auth"
)

// Option changes how the server is made.
type Option func(*options)

type options struct{ caps *auth.Caps }

// WithCaps makes every tool call keep to the capabilities (ADR 0071): the
// route rules ask only for play of everything under /mcp, so each tool asks
// for what its REST twin does.
func WithCaps(c *auth.Caps) Option { return func(o *options) { o.caps = c } }

// twin is the REST request that does what a tool does. The capability the
// tool needs is the one that route's rule asks for (auth.Need), so the two
// front doors cannot drift apart; a test fails for a tool with no entry. Ids
// in the paths are placeholders.
type twin struct{ method, path string }

const (
	roomPath  = "/api/games/x"
	batchPath = "/api/batches"
	tourPath  = "/api/tournaments/1"
)

var twins = map[string]twin{
	"list_game_defs":      {"GET", "/api/game-defs"},
	"get_game_info":       {"GET", "/api/game-defs/x"},
	"get_game_guide":      {"GET", "/api/game-defs/x/guide"},
	"create_game":         {"POST", "/api/games"},
	"import_game":         {"POST", "/api/games/import"},
	"join_game":           {"POST", roomPath + "/join"},
	"start_game":          {"POST", roomPath + "/start"},
	"add_bot":             {"POST", roomPath + "/bots"},
	"set_seat_faction":    {"POST", roomPath + "/seat-faction"},
	"get_state":           {"GET", roomPath + "/state"},
	"get_observation":     {"GET", roomPath + "/observe"},
	"get_moves":           {"GET", roomPath + "/moves"},
	"list_selections":     {"GET", roomPath + "/selections"},
	"make_move":           {"POST", roomPath + "/moves"},
	"undo_move":           {"POST", roomPath + "/undo"},
	"resign":              {"POST", roomPath + "/resign"},
	"draw":                {"POST", roomPath + "/draw"},
	"auto_play_impulse":   {"POST", roomPath + "/auto-play-impulse"},
	"vote_kick":           {"POST", roomPath + "/kick-vote"},
	"suggest_move":        {"POST", roomPath + "/hint"},
	"simulate":            {"POST", roomPath + "/simulate"},
	"export_game":         {"GET", roomPath + "/export"},
	"send_chat":           {"POST", roomPath + "/chat"},
	"get_chat":            {"GET", roomPath + "/chat"},
	"get_reactions":       {"GET", roomPath + "/reactions"},
	"set_reactions":       {"PUT", roomPath + "/reactions"},
	"list_tournaments":    {"GET", "/api/tournaments"},
	"get_tournament":      {"GET", tourPath},
	"enter_tournament":    {"POST", tourPath + "/enter"},
	"check_in_tournament": {"POST", tourPath + "/check-in"},
	"leave_tournament":    {"POST", tourPath + "/leave"},
	"get_leaderboard":     {"GET", "/api/ratings/x"},
	"get_badges":          {"GET", "/api/players/x/badges"},
	"list_badges":         {"GET", "/api/badges"},
	"list_bots":           {"GET", "/api/bots"},
	"list_puzzles":        {"GET", "/api/puzzles"},
	"env_reset":           {"POST", "/api/envs"},
	"env_step":            {"POST", "/api/envs/x/step"},
	"env_batch_step":      {"POST", "/api/envs/batch-step"},
	"env_spec":            {"GET", "/api/env-specs/x"},
	"env_observe":         {"GET", "/api/envs/x"},
	"env_fork":            {"POST", "/api/envs/x/fork"},
	"env_close":           {"DELETE", "/api/envs/x"},
	"env_list":            {"GET", "/api/envs"},
	"match_bots":          {"POST", "/api/matches"},
	"start_batch":         {"POST", batchPath},
	"get_batch":           {"GET", batchPath + "/1"},
	"list_batches":        {"GET", batchPath},
	"cancel_batch":        {"POST", batchPath + "/1/cancel"},
}

// guard refuses a tool call whose principal lacks the capability its REST
// twin needs, as REST does (403, the same words), and a tool with no twin.
func guard(caps *auth.Caps) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			t, ok := twins[request.Params.Name]
			if !ok {
				return refusal(errors.New("auth: that tool has no rule for who may use it")), nil
			}
			if err := caps.MayDo(ctx, t.method, t.path); err != nil {
				return refusal(err), nil
			}
			return next(ctx, request)
		}
	}
}

// refusal is a tool's answer when the call is refused for the principal's
// sake, with the status REST answers with in its structured content.
func refusal(err error) *mcp.CallToolResult {
	res := mcp.NewToolResultError(err.Error())
	res.StructuredContent = map[string]any{"error": err.Error(), "status": http.StatusForbidden}
	return res
}
