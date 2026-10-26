package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/noderunner/bingle/internal/envs"
	"github.com/noderunner/bingle/internal/puzzles"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/wire"
)

// registerPlayTools adds the tools beyond the original v1 set that make
// persisted rooms pleasant for an agent to play: rules, observation, undo,
// resign, simulation and export.
func registerPlayTools(s *server.MCPServer, mgr *session.Manager) {
	s.AddTool(
		mcp.NewTool("get_game_info",
			mcp.WithDescription("Describe a game: its rules text, board, piece types, available actions, reserves, win rules and move notation (with examples). Read this before playing an unfamiliar game."),
			mcp.WithString("game_def_id", mcp.Required(), mcp.Description("ID of the game definition (see list_game_defs).")),
		),
		handleGetGameInfo(mgr),
	)

	s.AddTool(
		mcp.NewTool("get_game_guide",
			mcp.WithDescription("Read a game's full rules guide as Markdown pages (the tabs of the rules window): overview, the round, actions, battle, factions, spellbooks, the map… Games without a hand-written guide get one generated from their definition. Use `tab` to read a single page."),
			mcp.WithString("game_def_id", mcp.Required(), mcp.Description("ID of the game definition (see list_game_defs).")),
			mcp.WithString("tab", mcp.Description("Page id to return (default: every page).")),
		),
		handleGetGameGuide(mgr),
	)

	s.AddTool(
		mcp.NewTool("add_bot",
			mcp.WithDescription("Seat a built-in bot (see list_bots) in an open seat — the lowest, or `seat` — of a game that is still waiting for players. Only someone already seated may do this; a game without a lobby starts when the last seat is filled. The bot is named after the faction or colour of its seat and its kind (\"Black Goat (MCTS)\")."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name (you must hold a seat).")),
			mcp.WithString("bot", mcp.Required(), mcp.Description("Bot id, e.g. \"random\", \"tactical\" or \"mcts\".")),
			mcp.WithNumber("seat", mcp.Description("The open seat the bot takes (default: the lowest open seat).")),
		),
		handleAddBot(mgr),
	)

	s.AddTool(
		mcp.NewTool("list_bots",
			mcp.WithDescription("List the built-in computer opponents that create_game (opponent=...) and env_reset (opponent=...) accept."),
		),
		func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultStructuredOnly(mgr.Bots().List()), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_observation",
			mcp.WithDescription("Get everything needed to choose a move in one call: a text rendering of the board, the position (stacks/reserves), whose turn it is, and — when it is your turn — the exact list of legal moves in notation. After the game ends it reports the result and a reward (+1 win, -1 loss, 0 draw) from your seat."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name (whose perspective to report).")),
		),
		handleGetObservation(mgr),
	)

	s.AddTool(
		mcp.NewTool("get_legal_moves",
			mcp.WithDescription("List every legal move for the player to move (or for `seat`), as notation strings plus structured fields. Unlike list_legal_moves this needs no starting square."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithNumber("seat", mcp.Description("Seat to list moves for; defaults to the seat whose turn it is.")),
		),
		handleGetLegalMoves(mgr),
	)

	s.AddTool(
		mcp.NewTool("undo_move",
			mcp.WithDescription("Take back your most recent move (and any built-in bot replies to it). Only possible while no other human has moved since."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name.")),
		),
		handleUndoMove(mgr),
	)

	s.AddTool(
		mcp.NewTool("resign",
			mcp.WithDescription("Resign the game; your opponent wins."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name.")),
		),
		handleResign(mgr),
	)

	s.AddTool(
		mcp.NewTool("draw",
			mcp.WithDescription("Offer, withdraw, accept or decline a draw with a human opponent. Accepting ends the game as a draw; any move withdraws an outstanding offer. Bots never answer offers."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name.")),
			mcp.WithString("action", mcp.Required(), mcp.Description("One of: offer, cancel, accept, decline.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			room, player, errRes := requireRoomAndPlayer(request)
			if errRes != nil {
				return errRes, nil
			}
			action, err := request.RequireString("action")
			if err != nil {
				return mcp.NewToolResultError("action is required"), nil
			}
			snap, err := mgr.Draw(ctx, room, player, action)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructuredOnly(stateResultFromSnapshot(snap)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("takeback",
			mcp.WithDescription("Ask a human opponent to let you take back your last move after they have replied (undo_move works on its own until then), or answer their request. Accepting rewinds to the position before the requester's move, with the requester to move; any move withdraws an outstanding request."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name.")),
			mcp.WithString("action", mcp.Required(), mcp.Description("One of: request, cancel, accept, decline.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			room, player, errRes := requireRoomAndPlayer(request)
			if errRes != nil {
				return errRes, nil
			}
			action, err := request.RequireString("action")
			if err != nil {
				return mcp.NewToolResultError("action is required"), nil
			}
			snap, err := mgr.Takeback(ctx, room, player, action)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructuredOnly(stateResultFromSnapshot(snap)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("suggest_move",
			mcp.WithDescription("Ask a built-in bot (default: the stronger of minimax and MCTS for the game, about a second of thought) which move it would play — in the current position, or after `at_seq` plus an optional variation. Only available in games against bots, or after the game has ended, so it can never be used against another person."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithString("player_name", mcp.Required(), mcp.Description("Your player name.")),
			mcp.WithNumber("at_seq", mcp.Description("Position after this move number (default: current).")),
			mcp.WithArray("moves", mcp.Description("A variation to play out first, in notation."), mcp.WithStringItems()),
			mcp.WithString("bot", mcp.Description("Which bot to ask: random, tactical, minimax or mcts (default: minimax in games where piece counts matter, such as Tak, otherwise mcts).")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			room, player, errRes := requireRoomAndPlayer(request)
			if errRes != nil {
				return errRes, nil
			}
			var at *int
			if _, given := request.GetArguments()["at_seq"]; given {
				n := request.GetInt("at_seq", 0)
				at = &n
			}
			hint, err := mgr.Hint(ctx, room, player, at, request.GetStringSlice("moves", nil), strings.TrimSpace(request.GetString("bot", "")))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			out := map[string]any{
				"notation": hint.Notation, "steps": hint.Steps, "turn_seat": hint.TurnSeat, "bot": hint.Bot,
			}
			text := fmt.Sprintf("Suggested move: %s", hint.Notation)
			if hint.Estimated {
				out["win_rate"] = hint.WinRate
				alts := make([]map[string]any, 0, len(hint.Alternatives))
				for _, a := range hint.Alternatives {
					alts = append(alts, map[string]any{"notation": a.Notation, "win_rate": a.WinRate})
				}
				out["alternatives"] = alts
				text += fmt.Sprintf(" (estimated %.0f%% to win)", hint.WinRate*100)
			}
			return mcp.NewToolResultStructured(out, text), nil
		},
	)

	s.AddTool(
		mcp.NewTool("import_game",
			mcp.WithDescription("Load a recorded game (a PTN document or a plain list of moves) into a new room with full history, so it can be replayed, analysed with simulate/suggest_move, or exported. Moves are validated by the rules engine; an illegal move is reported by number. Both seats are filled by the named players, so the room is for reviewing, not playing."),
			mcp.WithString("text", mcp.Required(), mcp.Description("PTN (with or without headers) or moves separated by whitespace. Move numbers, results and {comments} are ignored; a [TPS \"...\"] header sets the start position.")),
			mcp.WithString("game_def_id", mcp.Description("Game id; optional for PTN with a [Size] header (Tak).")),
			mcp.WithString("player1", mcp.Description("Name of seat 1 (default: PTN Player1 header, else Player 1).")),
			mcp.WithString("player2", mcp.Description("Name of seat 2.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			text, err := request.RequireString("text")
			if err != nil || strings.TrimSpace(text) == "" {
				return mcp.NewToolResultError("text is required"), nil
			}
			res, err := mgr.ImportGame(ctx, session.ImportOptions{
				Text: text, GameDefID: strings.TrimSpace(request.GetString("game_def_id", "")),
				Player1: request.GetString("player1", ""), Player2: request.GetString("player2", ""),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructured(map[string]any{
				"room_code": res.RoomCode, "game_def_id": res.GameDefID, "status": res.Status,
				"moves": res.Moves, "players": res.Players,
			}, fmt.Sprintf("Imported %d moves of %s into room %s (%s).", res.Moves, res.GameDefID, res.RoomCode, res.Status)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("simulate",
			mcp.WithDescription("What-if analysis: apply a sequence of moves (in notation, alternating seats) to the current position — or to the position after move `at_seq` — without changing the game. Returns the resulting position, whether it is terminal, and the legal moves there."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
			mcp.WithArray("moves", mcp.Required(), mcp.Description("Moves to apply in order, in the game's notation."), mcp.WithStringItems()),
			mcp.WithNumber("at_seq", mcp.Description("Start from the position after this move number (0 = initial position); defaults to the current position.")),
		),
		handleSimulate(mgr),
	)

	s.AddTool(
		mcp.NewTool("export_game",
			mcp.WithDescription("Export a complete game record: players, every move in notation, initial and final position, result, and (for Tak) a PTN document. Useful for building training datasets."),
			mcp.WithString("room_code", mcp.Required(), mcp.Description("The room code of the game.")),
		),
		handleExportGame(mgr),
	)
}

func requireRoomAndPlayer(request mcp.CallToolRequest) (room, player string, errRes *mcp.CallToolResult) {
	room, err := request.RequireString("room_code")
	if err != nil || strings.TrimSpace(room) == "" {
		return "", "", mcp.NewToolResultError("room_code is required")
	}
	player, err = request.RequireString("player_name")
	if err != nil || strings.TrimSpace(player) == "" {
		return "", "", mcp.NewToolResultError("player_name is required")
	}
	return room, player, nil
}

func handleGetGameInfo(mgr *session.Manager) server.ToolHandlerFunc {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("game_def_id")
		if err != nil || strings.TrimSpace(id) == "" {
			return mcp.NewToolResultError("game_def_id is required"), nil
		}
		info, err := mgr.GameInfo(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultStructured(info, rulesText(info)), nil
	}
}

func handleGetGameGuide(mgr *session.Manager) server.ToolHandlerFunc {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("game_def_id")
		if err != nil || strings.TrimSpace(id) == "" {
			return mcp.NewToolResultError("game_def_id is required"), nil
		}
		guide, err := mgr.Guide(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if tab := strings.TrimSpace(request.GetString("tab", "")); tab != "" {
			var kept []session.GuideTab
			for _, t := range guide.Tabs {
				if t.ID == tab {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				return mcp.NewToolResultError(fmt.Sprintf("no guide page %q (pages: %s)", tab, guidePageIDs(guide))), nil
			}
			guide.Tabs = kept
		}
		var text strings.Builder
		for i, t := range guide.Tabs {
			if i > 0 {
				text.WriteString("\n\n---\n\n")
			}
			text.WriteString(t.Markdown)
		}
		return mcp.NewToolResultStructured(guide, text.String()), nil
	}
}

func guidePageIDs(g session.Guide) string {
	ids := make([]string, len(g.Tabs))
	for i, t := range g.Tabs {
		ids[i] = t.ID
	}
	return strings.Join(ids, ", ")
}

func handleAddBot(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		room, player, errRes := requireRoomAndPlayer(request)
		if errRes != nil {
			return errRes, nil
		}
		bot, err := request.RequireString("bot")
		if err != nil || strings.TrimSpace(bot) == "" {
			return mcp.NewToolResultError("bot is required"), nil
		}
		res, err := mgr.AddBotAt(ctx, room, player, bot, request.GetInt("seat", 0))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultStructuredOnly(res), nil
	}
}

// observationResult is the get_observation tool's result shape.
type observationResult struct {
	RoomCode   string          `json:"room_code"`
	GameDefID  string          `json:"game_def_id"`
	Status     string          `json:"status"`
	Seat       int             `json:"seat"`
	Color      string          `json:"color"`
	YourTurn   bool            `json:"your_turn"`
	TurnSeat   *int            `json:"turn_seat"`
	MoveSeq    int             `json:"move_seq"`
	Ply        int             `json:"ply"`
	BoardText  string          `json:"board_text"`
	TPS        string          `json:"tps,omitempty"`
	State      json.RawMessage `json:"state"`
	LegalMoves []string        `json:"legal_moves"`
	LastMove   *wire.Move      `json:"last_move"`
	Done       bool            `json:"done"`
	WinnerSeat *int            `json:"winner_seat"`
	EndReason  string          `json:"end_reason,omitempty"`
	Result     string          `json:"result,omitempty"`
	Reward     float64         `json:"reward"`

	// Timed games: the per-move limit and what is left for the player to
	// move.
	MoveTimeSeconds int  `json:"move_time_seconds,omitempty"`
	SecondsLeft     *int `json:"seconds_left,omitempty"`
}

func handleGetObservation(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		room, player, errRes := requireRoomAndPlayer(request)
		if errRes != nil {
			return errRes, nil
		}
		o, err := mgr.Observe(ctx, room, player)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		res := observationResult{
			RoomCode: o.RoomCode, GameDefID: o.GameDefID, Status: o.Status, Seat: o.Seat, Color: o.Color,
			YourTurn: o.YourTurn, TurnSeat: o.TurnSeat, MoveSeq: o.MoveSeq, Ply: o.Ply,
			BoardText: o.BoardText, TPS: o.TPS, State: json.RawMessage(o.State), LegalMoves: o.LegalMoves,
			Done: o.Done, WinnerSeat: o.WinnerSeat, EndReason: o.EndReason, Result: o.Result, Reward: o.Reward,
			MoveTimeSeconds: o.MoveTimeLimit, SecondsLeft: o.SecondsLeft,
		}
		if o.LastMove != nil {
			mr := wire.MoveFromRecord(*o.LastMove)
			res.LastMove = &mr
		}

		var b strings.Builder
		fmt.Fprintf(&b, "%s — room %s — status: %s\n", o.GameDefID, o.RoomCode, o.Status)
		switch {
		case o.Seat == 0:
			b.WriteString("You are not seated (spectating).\n")
		case o.Done:
			fmt.Fprintf(&b, "Game over — you are seat %d (%s): %s", o.Seat, o.Color, orDash(o.Result))
			if o.EndReason != "" {
				fmt.Fprintf(&b, " by %s", o.EndReason)
			}
			b.WriteString(".\n")
		case o.YourTurn:
			fmt.Fprintf(&b, "You are seat %d (%s) — YOUR TURN (move %d).\n", o.Seat, o.Color, o.MoveSeq+1)
		default:
			fmt.Fprintf(&b, "You are seat %d (%s) — waiting for the opponent.\n", o.Seat, o.Color)
		}
		if o.LastMove != nil {
			fmt.Fprintf(&b, "Last move: %s (seat %d)\n", o.LastMove.Notation, o.LastMove.Seat)
		}
		b.WriteString(o.BoardText)
		b.WriteByte('\n')
		if o.TPS != "" {
			fmt.Fprintf(&b, "TPS: %s\n", o.TPS)
		}
		if o.YourTurn {
			fmt.Fprintf(&b, "Legal moves (%d): %s\n", len(o.LegalMoves), strings.Join(o.LegalMoves, " "))
		}
		return mcp.NewToolResultStructured(res, b.String()), nil
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// legalMoveResult is one entry of get_legal_moves' result.
type legalMoveResult struct {
	Notation string   `json:"notation"`
	Action   string   `json:"action"`
	Piece    string   `json:"piece,omitempty"`
	From     string   `json:"from,omitempty"`
	To       string   `json:"to,omitempty"`
	Count    int      `json:"count,omitempty"`
	Path     []string `json:"path,omitempty"`
	Drops    []int    `json:"drops,omitempty"`
	Aim      string   `json:"aim,omitempty"`
}

func legalMoveResults(moves []session.LegalMove) []legalMoveResult {
	out := make([]legalMoveResult, len(moves))
	for i, lm := range moves {
		r := legalMoveResult{
			Notation: lm.Notation, Action: lm.Move.Action, Piece: lm.Move.Piece,
			From: string(lm.Move.From), To: string(lm.Move.To), Count: lm.Move.Count, Drops: lm.Move.Drops,
			Aim: string(lm.Move.Aim),
		}
		for _, p := range lm.Move.Path {
			r.Path = append(r.Path, string(p))
		}
		out[i] = r
	}
	return out
}

func handleGetLegalMoves(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		room, err := request.RequireString("room_code")
		if err != nil || strings.TrimSpace(room) == "" {
			return mcp.NewToolResultError("room_code is required"), nil
		}
		seat, moves, err := mgr.LegalMoves(ctx, room, request.GetInt("seat", 0))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultStructuredOnly(struct {
			Seat  int               `json:"seat"`
			Moves []legalMoveResult `json:"moves"`
		}{Seat: seat, Moves: legalMoveResults(moves)}), nil
	}
}

func handleUndoMove(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		room, player, errRes := requireRoomAndPlayer(request)
		if errRes != nil {
			return errRes, nil
		}
		out, err := mgr.Undo(ctx, room, player)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultStructuredOnly(struct {
			Removed []int          `json:"removed"`
			State   getStateResult `json:"state"`
		}{Removed: out.Removed, State: stateResultFromSnapshot(out.Snapshot)}), nil
	}
}

func stateResultFromSnapshot(s session.StateSnapshot) getStateResult {
	return getStateResult{
		BoardState: json.RawMessage(s.BoardState),
		State:      json.RawMessage(s.State),
		TurnSeat:   s.TurnSeat,
		Status:     s.Status,
		MoveSeq:    s.MoveSeq,
		WinnerSeat: s.WinnerSeat,
		EndReason:  s.EndReason,

		DrawOfferSeat: s.DrawOfferSeat,
		TakebackSeat:  s.TakebackSeat,
	}
}

func handleResign(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		room, player, errRes := requireRoomAndPlayer(request)
		if errRes != nil {
			return errRes, nil
		}
		snap, err := mgr.Resign(ctx, room, player)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultStructuredOnly(stateResultFromSnapshot(snap)), nil
	}
}

func handleSimulate(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		room, err := request.RequireString("room_code")
		if err != nil || strings.TrimSpace(room) == "" {
			return mcp.NewToolResultError("room_code is required"), nil
		}
		moves, err := request.RequireStringSlice("moves")
		if err != nil {
			return mcp.NewToolResultError("moves is required: " + err.Error()), nil
		}
		var atSeq *int
		if _, present := request.GetArguments()["at_seq"]; present {
			n := request.GetInt("at_seq", 0)
			atSeq = &n
		}
		sim, err := mgr.Simulate(ctx, room, atSeq, moves)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultStructuredOnly(struct {
			BoardState json.RawMessage   `json:"board_state"`
			State      json.RawMessage   `json:"state"`
			TurnSeat   *int              `json:"turn_seat"`
			GameEnded  bool              `json:"game_ended"`
			WinnerSeat *int              `json:"winner_seat"`
			EndReason  string            `json:"end_reason,omitempty"`
			Applied    []string          `json:"applied"`
			Legal      []legalMoveResult `json:"legal_moves"`
		}{
			BoardState: json.RawMessage(sim.BoardState),
			State:      json.RawMessage(sim.State),
			TurnSeat:   sim.TurnSeat,
			GameEnded:  sim.GameEnded,
			WinnerSeat: sim.WinnerSeat,
			EndReason:  sim.EndReason,
			Applied:    sim.Applied,
			Legal:      legalMoveResults(sim.Legal),
		}), nil
	}
}

func handleExportGame(mgr *session.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		room, err := request.RequireString("room_code")
		if err != nil || strings.TrimSpace(room) == "" {
			return mcp.NewToolResultError("room_code is required"), nil
		}
		rec, err := mgr.Export(ctx, room)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultStructuredOnly(rec), nil
	}
}

// --- training environments ------------------------------------------------

// registerEnvTools adds the in-memory, gym-style environment tools.
func registerEnvTools(s *server.MCPServer, em *envs.Manager) {
	s.AddTool(
		mcp.NewTool("env_reset",
			mcp.WithDescription("Start a new in-memory training episode (nothing is written to the database) and return the first observation. The agent plays `seat`; a built-in bot plays the other seat(s) and replies automatically inside env_step. Use opponent=\"self\" for self-play where you move for whoever is to move."),
			mcp.WithString("game_def_id", mcp.Required(), mcp.Description("Game to play (see list_game_defs).")),
			mcp.WithNumber("seat", mcp.Description("Seat the agent plays (default 1; seat 1 normally moves first).")),
			mcp.WithString("opponent", mcp.Description("Bot id (random, tactical, minimax, mcts) or \"self\" for self-play. Default \"random\".")),
			mcp.WithNumber("seed", mcp.Description("Seed for reproducible bot behaviour (0 = random).")),
			mcp.WithNumber("max_plies", mcp.Description("Truncate the episode after this many plies (default 1000).")),
			mcp.WithString("start_tps", mcp.Description("Optional starting position in TPS (Tak) instead of the standard start; useful for curricula and puzzles.")),
			mcp.WithNumber("bot_iterations", mcp.Description("For opponent=mcts: search iterations per move instead of ~1 s of wall-clock time. Much faster, and reproducible with a seed.")),
			mcp.WithArray("include", mcp.WithStringItems(), mcp.Description("Extra observation fields for model input: \"planes\" (numeric board tensor, relative to the agent; see env_spec) and/or \"action_ids\" (fixed integer ID of every legal move).")),
			mcp.WithNumber("plane_depth", mcp.Description("With planes: how many stack levels to encode (default 1, or 4 for stacking games).")),
			mcp.WithString("puzzle_id", mcp.Description("Start from a puzzle (see list_puzzles): the position, side to move and truncation are set for you; reward +1 means you found the win. game_def_id may then be omitted.")),
			mcp.WithObject("start_state", mcp.Description("Start from a position document (the `state` of any observation) instead of the standard start; with start_seat naming who moves first.")),
			mcp.WithNumber("start_seat", mcp.Description("With start_state: the seat that moves first.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := strings.TrimSpace(request.GetString("game_def_id", ""))
			startState := startStateArg(request.GetArguments()["start_state"])
			startSeat := request.GetInt("start_seat", 0)
			seat := request.GetInt("seat", 0)
			maxPlies := request.GetInt("max_plies", 0)
			if pid := strings.TrimSpace(request.GetString("puzzle_id", "")); pid != "" {
				po := envs.ResetOptions{GameDefID: id, Seat: seat, MaxPlies: maxPlies}
				if err := envs.ApplyPuzzle(&po, pid); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				id, startState, startSeat, seat, maxPlies = po.GameDefID, po.StartState, po.StartSeat, po.Seat, po.MaxPlies
			}
			if id == "" {
				return mcp.NewToolResultError("game_def_id (or puzzle_id) is required"), nil
			}
			var wantPlanes, wantIDs bool
			for _, inc := range request.GetStringSlice("include", nil) {
				switch strings.ToLower(strings.TrimSpace(inc)) {
				case "planes":
					wantPlanes = true
				case "action_ids", "action-ids":
					wantIDs = true
				default:
					return mcp.NewToolResultError(fmt.Sprintf("unknown include %q (use \"planes\" or \"action_ids\")", inc)), nil
				}
			}
			opp := strings.TrimSpace(request.GetString("opponent", "random"))
			obs, err := em.Reset(ctx, envs.ResetOptions{
				GameDefID:  id,
				Seat:       seat,
				Opponent:   opp,
				Seed:       int64(request.GetInt("seed", 0)),
				MaxPlies:   maxPlies,
				StartTPS:   strings.TrimSpace(request.GetString("start_tps", "")),
				StartState: startState,
				StartSeat:  startSeat,

				BotIterations: request.GetInt("bot_iterations", 0),
				Planes:        wantPlanes,
				ActionIDs:     wantIDs,
				PlaneDepth:    request.GetInt("plane_depth", 0),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return envObservationResult(obs), nil
		},
	)

	s.AddTool(
		mcp.NewTool("list_puzzles",
			mcp.WithDescription("List curated puzzles: positions where the side to move wins by force within a few moves (proven by exhaustive search). Start one with env_reset(puzzle_id=...) or create_game(start_state=...). Solutions are hidden unless reveal is true."),
			mcp.WithString("game_def_id", mcp.Description("Only puzzles for this game.")),
			mcp.WithNumber("max_win_in", mcp.Description("Only puzzles solvable in at most this many of your own moves.")),
			mcp.WithBoolean("reveal", mcp.Description("Include the winning first moves and a solution line.")),
		),
		func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			list, err := puzzles.Filter(strings.TrimSpace(request.GetString("game_def_id", "")), request.GetInt("max_win_in", 0))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			reveal := request.GetBool("reveal", false)
			type item struct {
				ID         string          `json:"id"`
				GameDefID  string          `json:"game_def_id"`
				Title      string          `json:"title"`
				ToMove     int             `json:"to_move"`
				WinIn      int             `json:"win_in"`
				TPS        string          `json:"tps,omitempty"`
				State      json.RawMessage `json:"state"`
				FirstMoves []string        `json:"first_moves,omitempty"`
				Solution   []string        `json:"solution,omitempty"`
			}
			items := make([]item, 0, len(list))
			for _, p := range list {
				it := item{ID: p.ID, GameDefID: p.GameDefID, Title: p.Title, ToMove: p.ToMove, WinIn: p.WinIn, TPS: p.TPS, State: p.State}
				if reveal {
					it.FirstMoves, it.Solution = p.FirstMoves, p.Solution
				}
				items = append(items, it)
			}
			return mcp.NewToolResultStructured(map[string]any{"puzzles": items}, fmt.Sprintf("%d puzzles.", len(items))), nil
		},
	)

	s.AddTool(
		mcp.NewTool("env_step",
			mcp.WithDescription("Play your move in an environment, given as notation (from legal_moves) or as a fixed action_id (see env_spec). Any bot replies are played too; the returned observation is your next decision point, with reward (+1 win / -1 loss / 0 draw) and done once the episode ends. An illegal move is an error and leaves the environment unchanged."),
			mcp.WithString("env_id", mcp.Required(), mcp.Description("Environment ID from env_reset or env_fork.")),
			mcp.WithString("move", mcp.Description("The move in notation. Give this or action_id.")),
			mcp.WithNumber("action_id", mcp.Description("The move's fixed action ID instead of notation (games with a fixed action space).")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := request.RequireString("env_id")
			if err != nil {
				return mcp.NewToolResultError("env_id is required"), nil
			}
			var obs envs.Observation
			if mv := strings.TrimSpace(request.GetString("move", "")); mv != "" {
				obs, err = em.Step(ctx, id, mv)
			} else if _, given := request.GetArguments()["action_id"]; given {
				obs, err = em.StepAction(ctx, id, request.GetInt("action_id", -1))
			} else {
				return mcp.NewToolResultError("move or action_id is required"), nil
			}
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return envObservationResult(obs), nil
		},
	)

	s.AddTool(
		mcp.NewTool("env_batch_step",
			mcp.WithDescription("Step many environments in one call (vectorised training). Each step gives env_id and either move (notation) or action_id. Steps on different environments run concurrently; a bad step reports its own error and does not affect the others. At most 128 steps."),
			mcp.WithArray("steps", mcp.Required(), mcp.Description("List of {env_id, move | action_id}."), mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"env_id":    map[string]any{"type": "string"},
					"move":      map[string]any{"type": "string"},
					"action_id": map[string]any{"type": "integer"},
				},
				"required": []string{"env_id"},
			})),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			raw, _ := request.GetArguments()["steps"].([]any)
			if len(raw) == 0 {
				return mcp.NewToolResultError("steps must be a non-empty list"), nil
			}
			if len(raw) > envs.MaxBatch {
				return mcp.NewToolResultError(fmt.Sprintf("at most %d steps per call", envs.MaxBatch)), nil
			}
			reqs := make([]envs.StepRequest, 0, len(raw))
			for i, item := range raw {
				obj, ok := item.(map[string]any)
				if !ok {
					return mcp.NewToolResultError(fmt.Sprintf("steps[%d] must be an object", i)), nil
				}
				r := envs.StepRequest{}
				r.EnvID, _ = obj["env_id"].(string)
				r.Move, _ = obj["move"].(string)
				if v, ok := obj["action_id"].(float64); ok {
					n := int(v)
					r.ActionID = &n
				}
				reqs = append(reqs, r)
			}
			outcomes := em.StepMany(ctx, reqs)
			failed := 0
			for _, o := range outcomes {
				if o.Error != "" {
					failed++
				}
			}
			text := fmt.Sprintf("%d steps, %d failed.", len(outcomes), failed)
			return mcp.NewToolResultStructured(map[string]any{"results": outcomes}, text), nil
		},
	)

	s.AddTool(
		mcp.NewTool("env_spec",
			mcp.WithDescription("Describe a game for model building: board geometry, piece types, the observation planes (channel names and shape) and the size of the fixed action space. Use it to size a network before playing; pass include_actions to also get every action's notation by ID."),
			mcp.WithString("game_def_id", mcp.Required(), mcp.Description("Game to describe (see list_game_defs).")),
			mcp.WithBoolean("include_actions", mcp.Description("Include the notation of every action by ID (large for big Tak boards).")),
			mcp.WithNumber("plane_depth", mcp.Description("Stack levels to encode in planes (default 1, or 4 for stacking games).")),
		),
		func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := request.RequireString("game_def_id")
			if err != nil || strings.TrimSpace(id) == "" {
				return mcp.NewToolResultError("game_def_id is required"), nil
			}
			spec, err := em.Spec(id, request.GetBool("include_actions", false), request.GetInt("plane_depth", 0))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			text := fmt.Sprintf("%s: %s board %dx%d, %d piece types", spec.GameDefID, spec.Board.Kind, spec.Board.Width, spec.Board.Height, len(spec.Pieces))
			if spec.Planes != nil {
				text += fmt.Sprintf(", planes %v", spec.Planes.Shape)
			}
			if spec.Actions.Fixed {
				text += fmt.Sprintf(", %d fixed actions", spec.Actions.Count)
			} else {
				text += ", dynamic action space"
			}
			return mcp.NewToolResultStructured(spec, text), nil
		},
	)

	s.AddTool(
		mcp.NewTool("env_observe",
			mcp.WithDescription("Get an environment's current observation again."),
			mcp.WithString("env_id", mcp.Required(), mcp.Description("Environment ID.")),
		),
		func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := request.RequireString("env_id")
			if err != nil {
				return mcp.NewToolResultError("env_id is required"), nil
			}
			obs, err := em.Observe(id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return envObservationResult(obs), nil
		},
	)

	s.AddTool(
		mcp.NewTool("env_fork",
			mcp.WithDescription("Clone an environment (position, history, opponent) into a new independent environment — the primitive tree search and exploration need. Returns the new environment's observation."),
			mcp.WithString("env_id", mcp.Required(), mcp.Description("Environment ID to clone.")),
		),
		func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := request.RequireString("env_id")
			if err != nil {
				return mcp.NewToolResultError("env_id is required"), nil
			}
			obs, err := em.Fork(id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return envObservationResult(obs), nil
		},
	)

	s.AddTool(
		mcp.NewTool("env_close",
			mcp.WithDescription("Discard an environment and free its memory."),
			mcp.WithString("env_id", mcp.Required(), mcp.Description("Environment ID.")),
		),
		func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := request.RequireString("env_id")
			if err != nil {
				return mcp.NewToolResultError("env_id is required"), nil
			}
			em.Close(id)
			return mcp.NewToolResultStructuredOnly(map[string]any{"closed": id}), nil
		},
	)

	s.AddTool(
		mcp.NewTool("match_bots",
			mcp.WithDescription("Play a series of games between two built-in bots and return the tally (wins, draws, average length) and, optionally, every game's moves. Seats alternate between games. Use it to baseline opponents or to generate move records for supervised training; it stops early if it runs out of its time budget."),
			mcp.WithString("game_def_id", mcp.Required(), mcp.Description("Game to play (see list_game_defs).")),
			mcp.WithString("bot_a", mcp.Required(), mcp.Description("First bot id (random, tactical, minimax, mcts).")),
			mcp.WithString("bot_b", mcp.Required(), mcp.Description("Second bot id.")),
			mcp.WithNumber("games", mcp.Description("Number of games (default 20, at most 500).")),
			mcp.WithNumber("seed", mcp.Description("Seed for reproducible play.")),
			mcp.WithNumber("max_plies", mcp.Description("Truncate games longer than this (default 400).")),
			mcp.WithBoolean("include_moves", mcp.Description("Include every game's move list in the result (default false).")),
			mcp.WithNumber("bot_iterations", mcp.Description("Limit mcts bots to this many search iterations per move (fast and reproducible) instead of ~1 s.")),
			mcp.WithBoolean("record", mcp.Description("Return every position of every game as a training example: state, move played, fixed action_id, and the result for the mover (+1/0/-1). At most 50 games.")),
			mcp.WithBoolean("record_planes", mcp.Description("With record: also include each position's mover-relative planes (see env_spec).")),
			mcp.WithNumber("plane_depth", mcp.Description("With record_planes: stack levels to encode.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := request.RequireString("game_def_id")
			if err != nil || strings.TrimSpace(id) == "" {
				return mcp.NewToolResultError("game_def_id is required"), nil
			}
			a, errA := request.RequireString("bot_a")
			b, errB := request.RequireString("bot_b")
			if errA != nil || errB != nil {
				return mcp.NewToolResultError("bot_a and bot_b are required"), nil
			}
			games := request.GetInt("games", 20)
			if games > 500 {
				games = 500
			}
			ctx, cancel := context.WithTimeout(ctx, envs.DefaultMatchBudget)
			defer cancel()
			res, err := em.PlayMatch(ctx, envs.MatchOptions{
				GameDefID: id, BotA: a, BotB: b, Games: games,
				Seed:      int64(request.GetInt("seed", 0)),
				MaxPlies:  request.GetInt("max_plies", 0),
				KeepMoves: request.GetBool("include_moves", false),

				BotIterations: request.GetInt("bot_iterations", 0),
				Record:        request.GetBool("record", false) || request.GetBool("record_planes", false),
				RecordPlanes:  request.GetBool("record_planes", false),
				PlaneDepth:    request.GetInt("plane_depth", 0),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			text := fmt.Sprintf("%s: %s vs %s over %d games — %s wins %d, %s wins %d, draws %d, unfinished %d, average %.1f plies.",
				res.GameDefID, res.BotA, res.BotB, res.Played, res.BotA, res.AWins, res.BotB, res.BWins, res.Draws, res.Unfinished, res.AvgPlies)
			if res.Truncated {
				text += " (stopped early: time budget reached)"
			}
			return mcp.NewToolResultStructured(res, text), nil
		},
	)

	s.AddTool(
		mcp.NewTool("env_list",
			mcp.WithDescription("List the IDs of live environments."),
		),
		func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultStructuredOnly(map[string]any{"env_ids": em.List()}), nil
		},
	)
}

func envObservationResult(o envs.Observation) *mcp.CallToolResult {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s — agent seat %d (%s) — ply %d\n", o.EnvID, o.GameDefID, o.AgentSeat, o.AgentColor, o.Ply)
	if len(o.LastMoves) > 0 {
		fmt.Fprintf(&b, "Moves played: %s\n", strings.Join(o.LastMoves, " "))
	}
	switch {
	case o.Truncated:
		b.WriteString("Episode truncated (max plies reached).\n")
	case o.Done:
		fmt.Fprintf(&b, "Episode over: %s (reward %+g)", orDash(o.Result), o.Reward)
		if o.EndReason != "" {
			fmt.Fprintf(&b, " by %s", o.EndReason)
		}
		b.WriteString(".\n")
	case o.YourTurn:
		b.WriteString("Your turn.\n")
	}
	b.WriteString(o.BoardText)
	b.WriteByte('\n')
	if o.TPS != "" {
		fmt.Fprintf(&b, "TPS: %s\n", o.TPS)
	}
	if o.YourTurn {
		fmt.Fprintf(&b, "Legal moves (%d): %s\n", len(o.LegalMoves), strings.Join(o.LegalMoves, " "))
	}
	return mcp.NewToolResultStructured(o, b.String())
}
