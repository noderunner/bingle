package mcpserver

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/noderunner/bingle/internal/session"
)

// plyArg is the optional ply argument of a tool, nil when it was not given.
func plyArg(request mcp.CallToolRequest) *int {
	if _, ok := request.GetArguments()["ply"]; !ok {
		return nil
	}
	n := request.GetInt("ply", 0)
	return &n
}

type selectionsResult struct {
	Seat     int               `json:"seat"`
	Ply      int               `json:"ply"`
	Made     []string          `json:"made"`
	Options  []legalMoveResult `json:"options"`
	Complete bool              `json:"complete"`
	MayEnd   bool              `json:"may_end,omitempty"`
	State    json.RawMessage   `json:"state,omitempty"`
}

// handleListSelections implements the list_selections tool (ADR 0114).
func handleListSelections(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		roomCode, err := request.RequireString("room_code")
		if err != nil || strings.TrimSpace(roomCode) == "" {
			return mcp.NewToolResultError("room_code is required"), nil
		}
		view, err := mgr.Selections(ctx, roomCode, strings.TrimSpace(request.GetString("made", "")), plyArg(request))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out := selectionsResult{Seat: view.Seat, Ply: view.Ply, Made: view.Made, Options: legalMoveResults(view.Options),
			Complete: view.Complete, MayEnd: view.MayEnd}
		if view.State != "" {
			out.State = json.RawMessage(view.State)
		}
		return mcp.NewToolResultStructuredOnly(out), nil
	}
}
