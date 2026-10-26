package mcpserver

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/noderunner/bingle/internal/session"
)

// registerLobbyTools adds the tools of the table's social side: starting a
// game that waits in its lobby, the chat, and reaction settings.
func registerLobbyTools(s *server.MCPServer, mgr *session.Manager) {
	s.AddTool(
		mcp.NewTool("start_game",
			mcp.WithDescription("Start a game that is waiting in its lobby (create_game with lobby=true). Only the host — the player who created the game — may, and every seat must be filled (add_bot fills open seats). A game created without a lobby starts by itself when its last seat fills."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name (the host).")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			room, player, errRes := requireRoomAndPlayer(request)
			if errRes != nil {
				return errRes, nil
			}
			snap, err := mgr.StartGame(ctx, room, player)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructuredOnly(map[string]any{"status": snap.Status, "turn_seat": snap.TurnSeat, "move_seq": snap.MoveSeq}), nil
		},
	)

	s.AddTool(
		mcp.NewTool("set_seat_cult",
			mcp.WithDescription("Change the cult (faction) of an open seat of a game that waits in its lobby (Cthulhu Wars: gc, cc, bg, ys, ww, sl). A cult played by another open seat changes places with it; seats a player or bot holds keep theirs. Any player at the table may."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name.")),
			mcp.WithNumber("seat", mcp.Required(), mcp.Description("The open seat to change.")),
			mcp.WithString("cult", mcp.Required(), mcp.Description("The cult the seat should play: gc, cc, bg, ys, ww or sl.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			room, player, errRes := requireRoomAndPlayer(request)
			if errRes != nil {
				return errRes, nil
			}
			cult, err := request.RequireString("cult")
			if err != nil || strings.TrimSpace(cult) == "" {
				return mcp.NewToolResultError("cult is required"), nil
			}
			snap, err := mgr.SetSeatCult(ctx, room, player, request.GetInt("seat", 0), cult)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructuredOnly(map[string]any{"status": snap.Status, "game_def_id": snap.GameDefID, "seats": snap.Seats}), nil
		},
	)

	s.AddTool(
		mcp.NewTool("send_chat",
			mcp.WithDescription("Say something in a game's chat, as a seated player or as a spectator (any name). At most 500 characters; floods are refused."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Who speaks.")),
			mcp.WithString("text", mcp.Required(), mcp.Description("The message.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			room, player, errRes := requireRoomAndPlayer(request)
			if errRes != nil {
				return errRes, nil
			}
			text, err := request.RequireString("text")
			if err != nil || strings.TrimSpace(text) == "" {
				return mcp.NewToolResultError("text is required"), nil
			}
			line, err := mgr.SendChat(ctx, room, player, text)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructuredOnly(line), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_chat",
			mcp.WithDescription("Read a game's chat: the latest messages, or those after message id `after`."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithNumber("after", mcp.Description("Only messages with an id greater than this.")),
			mcp.WithNumber("limit", mcp.Description("At most this many messages (default 100).")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			room, err := request.RequireString("room_code")
			if err != nil || strings.TrimSpace(room) == "" {
				return mcp.NewToolResultError("room_code is required"), nil
			}
			lines, err := mgr.ChatLog(ctx, room, int64(request.GetInt("after", 0)), request.GetInt("limit", 100))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructuredOnly(map[string]any{"messages": lines}), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_reactions",
			mcp.WithDescription("Your faction's reaction abilities in a game (Devolve, Necrophagy, Madness…: abilities used on another player's turn; 'held' says whether the spellbook is earned yet), when the game asks you about them, and the reaction waiting for your answer, if any. As an AI client you are asked about every reaction (ask_all) unless you choose otherwise with set_reactions."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			room, player, errRes := requireRoomAndPlayer(request)
			if errRes != nil {
				return errRes, nil
			}
			set, err := mgr.ReactionSettings(ctx, room, player)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructuredOnly(set), nil
		},
	)

	s.AddTool(
		mcp.NewTool("set_reactions",
			mcp.WithDescription("Choose when the game asks you about your reactions; the rest are declined for you automatically. targeted=true asks when an action is aimed at you (you, one of your units, or an area where you have a unit or a Gate); ask_all=true asks after every other player's action; ask_seats lists the seats whose actions always ask. With ability (an id from get_reactions) the choice is for that reaction alone — Madness and the Unspeakable Oath can differ; without it, for every reaction. Arguments you leave out keep their value."),
			mcp.WithString("ability", mcp.Description("The id of one reaction (see get_reactions); leave out to choose for all.")),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name.")),
			mcp.WithBoolean("targeted", mcp.Description("Ask when an action is aimed at you.")),
			mcp.WithBoolean("ask_all", mcp.Description("Ask after any other player's action.")),
			mcp.WithArray("ask_seats", mcp.Description("Seat numbers whose actions always ask you."), mcp.WithNumberItems()),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			room, player, errRes := requireRoomAndPlayer(request)
			if errRes != nil {
				return errRes, nil
			}
			args := request.GetArguments()
			var upd session.ReactionUpdate
			if v, ok := args["ability"].(string); ok {
				upd.Ability = v
			}
			if v, ok := args["targeted"].(bool); ok {
				upd.Targeted = &v
			}
			if v, ok := args["ask_all"].(bool); ok {
				upd.AskAll = &v
			}
			if raw, ok := args["ask_seats"].([]any); ok {
				seats := []int{}
				for _, v := range raw {
					if f, ok := v.(float64); ok {
						seats = append(seats, int(f))
					}
				}
				upd.AskSeats = &seats
			}
			set, err := mgr.SetReactions(ctx, room, player, upd)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructuredOnly(set), nil
		},
	)
}
