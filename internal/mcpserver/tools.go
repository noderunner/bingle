package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/wire"
)

// registerTools adds every MCP tool this package implements to mcpServer,
// each backed by mgr. Per docs/history/plan.md section 6.10, these are the room tools
// (list_game_defs, create_game, join_game, get_state, list_legal_moves,
// make_move, get_moves); registerPlayTools, registerEnvTools, registerLobbyTools
// and registerBatchTools add the rest.
func registerTools(mcpServer *server.MCPServer, mgr *session.Manager) {
	mcpServer.AddTool(
		mcp.NewTool("list_game_defs",
			mcp.WithDescription("List every registered game definition available to create a room for."),
		),
		handleListGameDefs(mgr),
	)

	mcpServer.AddTool(
		mcp.NewTool("create_game",
			mcp.WithDescription("Create a new game room for a given game definition, seating the caller as its creator."),
			mcp.WithString("game_def_id", mcp.Required(), mcp.Description("ID of the game definition to create a room for (see list_game_defs).")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Display name of the player creating the room.")),
			mcp.WithNumber("seat", mcp.Description("Seat to take (default 1). In most games seat 1 moves first.")),
			mcp.WithString("opponent", mcp.Description("Optional built-in bot to fill the other seat(s) and start immediately: one of the ids from list_bots (random, tactical, minimax, mcts). Omit to wait for a human or another agent to join.")),
			mcp.WithArray("also_seats", mcp.Description("Optional seats, besides yours, that you play too (a game against yourself): each is a player of its own named \"<you> · <faction or colour>\"."), mcp.WithNumberItems()),
			mcp.WithNumber("move_time_seconds", mcp.Description("Optional per-move time limit in seconds; a player who runs out of time loses. Omit or 0 for no limit.")),
			mcp.WithArray("clock_seconds", mcp.Description("Optional game clock (a chess clock) instead of a per-move limit, for a game of two seats: the seconds each seat starts with, in seat order (the two may differ for handicap play). A player whose clock runs out forfeits."), mcp.WithNumberItems()),
			mcp.WithBoolean("lobby", mcp.Description("Wait in a lobby until the host starts the game (start_game), even with every seat filled: other players join (with a seat of their choice), add bots and chat first.")),
			mcp.WithObject("options", mcp.Description("Setup choices of games that offer them (see get_game_info \"options\"). Cthulhu Wars: {\"factions\": [\"gc\",\"cc\",\"bg\",\"ys\",\"ww\"] (one per seat, in seat order), \"high-priests\": true}.")),
			mcp.WithString("start_tps", mcp.Description("Optional starting position in TPS (Tak) instead of the standard start; the TPS says who moves first. For puzzles and endgame practice.")),
			mcp.WithObject("start_state", mcp.Description("Optional starting position as a position document (the `state` object of an observation), for any game. Use start_seat to say who moves first.")),
			mcp.WithNumber("start_seat", mcp.Description("With start_state: the seat that moves first (default: the game's first seat).")),
		),
		handleCreateGame(mgr),
	)

	mcpServer.AddTool(
		mcp.NewTool("join_game",
			mcp.WithDescription("Join an existing game room as a player (taking a seat) or a spectator (read-only)."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game to join.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Display name of the player/spectator joining.")),
			mcp.WithString("role", mcp.Description(`Either "player" or "spectator"; defaults to "player" if omitted.`)),
		),
		handleJoinGame(mgr),
	)

	mcpServer.AddTool(
		mcp.NewTool("get_state",
			mcp.WithDescription("Get a game room's current position, turn, status, and move sequence number. board_state is a flat position -> top piece map; state is the complete position (full stacks bottom-to-top, reserves, ply). For a decision-ready view with legal moves use get_observation."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game to fetch state for.")),
		),
		handleGetState(mgr),
	)

	mcpServer.AddTool(
		mcp.NewTool("list_legal_moves",
			mcp.WithDescription("List every position a piece can legally move to right now, given its current position. Returns an empty array if there's no piece at that position (this is not a move attempt)."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game to query.")),
			mcp.WithString("from", mcp.Required(), mcp.Description("The position of the piece to query legal destinations for.")),
		),
		handleListLegalMoves(mgr),
	)

	mcpServer.AddTool(
		mcp.NewTool("make_move",
			mcp.WithDescription("Make a move for a seated player on that player's turn. Preferred form: pass `notation` using a string from get_observation's legal_moves (e.g. Tak PTN \"a1\", \"Sc3\", \"3c3>111\"). Alternatively give a structured move (action/piece/from/to/count/path/drops), or from+to for simple piece moves. If a bot opponent replies, its moves are returned in bot_moves."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game to move in.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Display name of the player making the move (must hold the seat whose turn it currently is).")),
			mcp.WithString("notation", mcp.Description("The move in the game's notation (see get_game_info notation_help).")),
			mcp.WithString("from", mcp.Description("The position of the piece or stack to move.")),
			mcp.WithString("to", mcp.Description("The position to move that piece to, or the square to place on.")),
			mcp.WithString("action", mcp.Description("Action id, for structured moves (e.g. \"place\" or \"move\").")),
			mcp.WithString("piece", mcp.Description("Piece type to place, for structured placements (e.g. \"flat\", \"wall\", \"cap\").")),
			mcp.WithNumber("count", mcp.Description("Number of pieces to pick up from the stack, for structured stack moves.")),
			mcp.WithArray("path", mcp.Description("Squares dropped onto in order, for structured stack moves."), mcp.WithStringItems()),
			mcp.WithArray("drops", mcp.Description("Pieces dropped on each path square, for structured stack moves."), mcp.WithNumberItems()),
			mcp.WithString("aim", mcp.Description("For compound moves (Amazons): the square the follow-up shot lands on, after from/to.")),
		),
		handleMakeMove(mgr),
	)

	mcpServer.AddTool(
		mcp.NewTool("get_moves",
			mcp.WithDescription("Get a game room's move history, optionally starting after a given sequence number."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game to fetch move history for.")),
			mcp.WithNumber("since_seq", mcp.Description("Only return moves with seq greater than this value; defaults to 0 (all moves).")),
		),
		handleGetMoves(mgr),
	)
}

// handleListGameDefs implements the list_game_defs tool: no args, respond
// with every registered game definition's id/name/seats_required.
func handleListGameDefs(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		defs := mgr.ListGameDefs(ctx)

		return mcp.NewToolResultStructuredOnly(wire.GameDefs(defs)), nil
	}
}

// createGameResult is the create_game tool's result shape, from
// session.CreateGameResult — the same wire shape as POST /api/games's
// createGameResponse (internal/api/games.go).
type createGameResult struct {
	RoomCode string      `json:"room_code"`
	Seat     int         `json:"seat"`
	Color    string      `json:"color"`
	Status   string      `json:"status"`
	BotMoves []wire.Move `json:"bot_moves,omitempty"`
}

// handleCreateGame implements the create_game tool: read {"game_def_id",
// "player_name"}, reject blank/whitespace-only values with a tool-level
// error before calling Manager (same pre-validation discipline as REST'S
// handleCreateGame), then create the game and respond with its
// room_code/seat/color/status. Business-logic failures (e.g. an unknown
// game_def_id, session.ErrGameDefNotFound) are reported as a tool-level
// error via mcp.NewToolResultError, not a transport-level error, per
// docs/history/plan.md section 6.10.
func handleCreateGame(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gameDefID, err := request.RequireString("game_def_id")
		if err != nil {
			return mcp.NewToolResultError("game_def_id is required: " + err.Error()), nil
		}
		playerName, err := request.RequireString("player_name")
		if err != nil {
			return mcp.NewToolResultError("player_name is required: " + err.Error()), nil
		}

		if strings.TrimSpace(gameDefID) == "" {
			return mcp.NewToolResultError("game_def_id is required"), nil
		}
		if strings.TrimSpace(playerName) == "" {
			return mcp.NewToolResultError("player_name is required"), nil
		}

		result, err := mgr.CreateGameWithOptions(ctx, session.CreateOptions{
			GameDefID:   gameDefID,
			CreatorName: playerName,
			Seat:        request.GetInt("seat", 0),
			Opponent:    strings.TrimSpace(request.GetString("opponent", "")),
			Also:        intsArg(request.GetArguments()["also_seats"]),

			MoveTimeSeconds: request.GetInt("move_time_seconds", 0),
			ClockSeconds:    intsArg(request.GetArguments()["clock_seconds"]),
			Lobby:           request.GetBool("lobby", false),
			Options:         optionsArg(request.GetArguments()["options"]),

			StartTPS:   strings.TrimSpace(request.GetString("start_tps", "")),
			StartState: startStateArg(request.GetArguments()["start_state"]),
			StartSeat:  request.GetInt("start_seat", 0),
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// An AI client answers each reaction window itself: nothing is passed
		// for it unless it asks (set_reactions).
		_ = mgr.AskEveryReaction(ctx, result.RoomCode, playerName)

		return mcp.NewToolResultStructuredOnly(createGameResult{
			RoomCode: result.RoomCode,
			Seat:     result.Seat,
			Color:    result.Color,
			Status:   result.Status,
			BotMoves: wire.MovesFromOutcomes(result.BotMoves),
		}), nil
	}
}

// intsArg reads a list of numbers.
func intsArg(v any) []int {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []int
	for _, x := range list {
		if f, ok := x.(float64); ok {
			out = append(out, int(f))
		}
	}
	return out
}

// optionsArg reads the options argument: an object, or the same object as a
// JSON string.
func optionsArg(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case string:
		var m map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(t)), &m); err == nil {
			return m
		}
	}
	return nil
}

// startStateArg turns a start_state argument — a position object, or the same
// document as a JSON string — into the JSON text the session layer parses.
func startStateArg(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// joinGameResult is the join_game tool's result shape, from
// session.JoinResult — the same wire shape as POST /games/{code}/join's
// joinGameResponse (internal/api/games.go). Seat/Color are tagged omitempty
// so a spectator's result (Seat == 0, Color == "") genuinely omits those
// keys rather than sending zero values.
type joinGameResult struct {
	Role   string `json:"role"`
	Seat   int    `json:"seat,omitempty"`
	Color  string `json:"color,omitempty"`
	Status string `json:"status"`
}

// handleJoinGame implements the join_game tool: read {"room_code",
// "player_name", "role"}, reject blank/whitespace-only room_code/player_name
// with a tool-level error before calling Manager, resolve role (blank/
// omitted defaults to "player"; anything other than "player"/"spectator" is
// a tool-level error), then join the room and respond with the resulting
// role/seat/color/status. Business-logic failures (e.g. an unknown
// room_code, session.ErrSeatsFull) are reported as a tool-level error via
// mcp.NewToolResultError, not a transport-level error, per docs/history/plan.md
// section 6.10.
func handleJoinGame(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		roomCode, err := request.RequireString("room_code")
		if err != nil {
			return mcp.NewToolResultError("room_code is required: " + err.Error()), nil
		}
		playerName, err := request.RequireString("player_name")
		if err != nil {
			return mcp.NewToolResultError("player_name is required: " + err.Error()), nil
		}
		rawRole := request.GetString("role", "")

		if strings.TrimSpace(roomCode) == "" {
			return mcp.NewToolResultError("room_code is required"), nil
		}
		if strings.TrimSpace(playerName) == "" {
			return mcp.NewToolResultError("player_name is required"), nil
		}

		var role session.Role
		switch strings.TrimSpace(rawRole) {
		case "", string(session.RolePlayer):
			role = session.RolePlayer
		case string(session.RoleSpectator):
			role = session.RoleSpectator
		default:
			return mcp.NewToolResultError(fmt.Sprintf("invalid role %q", rawRole)), nil
		}

		result, err := mgr.JoinGame(ctx, roomCode, playerName, role)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if result.Role == session.RolePlayer {
			_ = mgr.AskEveryReaction(ctx, roomCode, playerName)
		}

		return mcp.NewToolResultStructuredOnly(joinGameResult{
			Role:   string(result.Role),
			Seat:   result.Seat,
			Color:  result.Color,
			Status: result.Status,
		}), nil
	}
}

// getStateResult is the get_state tool's result shape, from
// session.StateSnapshot — the same wire shape as GET
// /games/{code}/state's getGameStateResponse (internal/api/games.go),
// minus at_seq support (a REST-only replay convenience, not part of the
// tool table). board_state is embedded as a raw JSON object (it's already a
// JSON string from state.BoardState.MarshalJSON), not a plain string field,
// so it appears nested rather than doubly-encoded.
type getStateResult struct {
	BoardState json.RawMessage `json:"board_state"`
	State      json.RawMessage `json:"state,omitempty"`
	TurnSeat   *int            `json:"turn_seat"`
	Status     string          `json:"status"`
	MoveSeq    int             `json:"move_seq"`
	WinnerSeat *int            `json:"winner_seat"`
	EndReason  string          `json:"end_reason,omitempty"`
	// DrawOfferSeat and TakebackSeat are the seats with an open draw offer
	// or take-back request (null when none).
	DrawOfferSeat *int `json:"draw_offer_seat"`
	TakebackSeat  *int `json:"takeback_seat"`
}

// handleGetState implements the get_state tool: read {"room_code"}, reject
// a blank/whitespace-only value with a tool-level error before calling
// Manager, then respond with the room's current board_state/turn_seat/
// status/move_seq (no at_seq support, unlike REST's .../state endpoint).
// Business-logic failures (e.g. an unknown room_code, store.ErrNotFound)
// are reported as a tool-level error via mcp.NewToolResultError, not a
// transport-level error, per docs/history/plan.md section 6.10.
func handleGetState(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		roomCode, err := request.RequireString("room_code")
		if err != nil {
			return mcp.NewToolResultError("room_code is required: " + err.Error()), nil
		}
		if strings.TrimSpace(roomCode) == "" {
			return mcp.NewToolResultError("room_code is required"), nil
		}

		snapshot, err := mgr.GetState(ctx, roomCode, nil)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultStructuredOnly(getStateResult{
			BoardState: json.RawMessage(snapshot.BoardState),
			State:      json.RawMessage(snapshot.State),
			TurnSeat:   snapshot.TurnSeat,
			Status:     snapshot.Status,
			MoveSeq:    snapshot.MoveSeq,
			WinnerSeat: snapshot.WinnerSeat,
			EndReason:  snapshot.EndReason,
		}), nil
	}
}

// handleListLegalMoves implements the list_legal_moves tool: read
// {"room_code", "from"}, reject blank/whitespace-only values with a
// tool-level error before calling Manager, then respond with a JSON array
// of every position that piece can legally move to right now. Always
// returns "[]" rather than "null" when there are none (an unoccupied `from`
// is not an error — this is an AI convenience query, not a move attempt).
// Business-logic failures (e.g. an unknown room_code, store.ErrNotFound)
// are reported as a tool-level error via mcp.NewToolResultError, not a
// transport-level error, per docs/history/plan.md section 6.10.
func handleListLegalMoves(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		roomCode, err := request.RequireString("room_code")
		if err != nil {
			return mcp.NewToolResultError("room_code is required: " + err.Error()), nil
		}
		from, err := request.RequireString("from")
		if err != nil {
			return mcp.NewToolResultError("from is required: " + err.Error()), nil
		}

		if strings.TrimSpace(roomCode) == "" {
			return mcp.NewToolResultError("room_code is required"), nil
		}
		if strings.TrimSpace(from) == "" {
			return mcp.NewToolResultError("from is required"), nil
		}

		positions, err := mgr.ListLegalMoves(ctx, roomCode, board.PositionID(from))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		out := make([]string, 0, len(positions))
		for _, pos := range positions {
			out = append(out, string(pos))
		}

		return mcp.NewToolResultStructuredOnly(out), nil
	}
}

// makeMoveResult is the make_move tool's result shape, from
// docs/history/plan.md section 6.10: {"move": {...}, "state": {...}} — the MCP
// equivalent of POST /games/{code}/moves' makeMoveResponse
// (internal/api/games.go). State reuses getStateResult's shape.
type makeMoveResult struct {
	Move     wire.Move      `json:"move"`
	State    getStateResult `json:"state"`
	BotMoves []wire.Move    `json:"bot_moves,omitempty"`
}

// handleMakeMove implements the make_move tool: read {"room_code",
// "player_name", "from", "to"}, reject blank/whitespace-only values with a
// tool-level error before calling Manager (same pre-validation discipline as
// handleCreateGame/handleJoinGame), then apply the move and respond with the
// resulting move/state. Business-logic failures (e.g. an illegal move,
// session.ErrGameNotActive, session.ErrNotYourTurn, store.ErrNotFound) are
// reported as a tool-level error via mcp.NewToolResultError, not a
// transport-level error, per docs/history/plan.md section 6.10. This call goes
// through the same Manager.MakeMove REST uses, so it also triggers
// hub.publish — an open spectator WebSocket connection to the room receives
// the resulting "move" event with no extra wiring needed here.
func handleMakeMove(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		roomCode, err := request.RequireString("room_code")
		if err != nil {
			return mcp.NewToolResultError("room_code is required: " + err.Error()), nil
		}
		playerName, err := request.RequireString("player_name")
		if err != nil {
			return mcp.NewToolResultError("player_name is required: " + err.Error()), nil
		}
		if strings.TrimSpace(roomCode) == "" {
			return mcp.NewToolResultError("room_code is required"), nil
		}
		if strings.TrimSpace(playerName) == "" {
			return mcp.NewToolResultError("player_name is required"), nil
		}

		notation := strings.TrimSpace(request.GetString("notation", ""))
		action := strings.TrimSpace(request.GetString("action", ""))
		piece := strings.TrimSpace(request.GetString("piece", ""))
		count := request.GetInt("count", 0)
		path := request.GetStringSlice("path", nil)
		aim := strings.TrimSpace(request.GetString("aim", ""))

		var outcome session.MoveOutcome
		switch {
		case notation != "":
			outcome, err = mgr.PlayNotation(ctx, roomCode, playerName, notation)
		case action != "" || piece != "" || count != 0 || len(path) > 0 || aim != "":
			mv := state.Move{
				Action: action,
				Piece:  piece,
				From:   board.PositionID(strings.TrimSpace(request.GetString("from", ""))),
				To:     board.PositionID(strings.TrimSpace(request.GetString("to", ""))),
				Count:  count,
				Drops:  request.GetIntSlice("drops", nil),
				Aim:    board.PositionID(aim),
			}
			for _, p := range path {
				mv.Path = append(mv.Path, board.PositionID(strings.TrimSpace(p)))
			}
			outcome, err = mgr.Play(ctx, roomCode, playerName, mv)
		default:
			from, ferr := request.RequireString("from")
			if ferr != nil {
				return mcp.NewToolResultError("from is required (or give notation): " + ferr.Error()), nil
			}
			to, terr := request.RequireString("to")
			if terr != nil {
				return mcp.NewToolResultError("to is required (or give notation): " + terr.Error()), nil
			}
			if strings.TrimSpace(from) == "" {
				return mcp.NewToolResultError("from is required"), nil
			}
			if strings.TrimSpace(to) == "" {
				return mcp.NewToolResultError("to is required"), nil
			}
			outcome, err = mgr.MakeMove(ctx, roomCode, playerName, board.PositionID(from), board.PositionID(to))
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		final := outcome
		for _, b := range outcome.BotMoves {
			final = b
		}
		var turnSeat *int
		if !final.GameEnded {
			turnSeat = &final.NextTurnSeat
		}

		return mcp.NewToolResultStructuredOnly(makeMoveResult{
			Move: wire.MoveFromOutcome(outcome),
			State: getStateResult{
				BoardState: json.RawMessage(final.BoardState),
				State:      json.RawMessage(final.State),
				TurnSeat:   turnSeat,
				Status:     final.Status,
				MoveSeq:    final.Seq,
				WinnerSeat: final.WinnerSeat,
				EndReason:  final.EndReason,
			},
			BotMoves: wire.MovesFromOutcomes(outcome.BotMoves),
		}), nil
	}
}

// handleGetMoves implements the get_moves tool: read {"room_code",
// "since_seq"} (since_seq optional, defaulting to 0, i.e. all moves), reject
// a blank/whitespace-only room_code with a tool-level error before calling
// Manager, then respond with a JSON array of moves in ascending seq order.
// Always returns "[]" rather than "null" when there are no moves.
// Business-logic failures (e.g. an unknown room_code, store.ErrNotFound) are
// reported as a tool-level error via mcp.NewToolResultError, not a
// transport-level error, per docs/history/plan.md section 6.10.
func handleGetMoves(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		roomCode, err := request.RequireString("room_code")
		if err != nil {
			return mcp.NewToolResultError("room_code is required: " + err.Error()), nil
		}
		if strings.TrimSpace(roomCode) == "" {
			return mcp.NewToolResultError("room_code is required"), nil
		}

		sinceSeq := request.GetInt("since_seq", 0)

		records, err := mgr.ListMoves(ctx, roomCode, sinceSeq)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		history := make([]wire.Move, 0, len(records))
		for _, rec := range records {
			history = append(history, wire.MoveFromRecord(rec))
		}

		return mcp.NewToolResultStructuredOnly(history), nil
	}
}
