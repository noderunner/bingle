package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/noderunner/bingle/internal/puzzles"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/wire"
)

// rulesText renders a game description as plain markdown-ish text: rules,
// how it ends, the pieces and the move notation. It is what get_game_info and
// the bingle://games/{id}/rules resource show a language model.
func rulesText(info session.GameInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\n", info.Name, info.ID)
	if info.Description != "" {
		fmt.Fprintf(&b, "%s\n", info.Description)
	}
	for _, r := range info.Rules {
		fmt.Fprintf(&b, "\n%s\n", r)
	}
	if len(info.WinRules) > 0 {
		b.WriteString("\nHow the game ends:\n")
		for _, r := range info.WinRules {
			fmt.Fprintf(&b, "- %s\n", r)
		}
	}
	if len(info.Pieces) > 0 {
		b.WriteString("\nPieces:\n")
		for _, p := range info.Pieces {
			code := ""
			if p.Code != "" {
				code = fmt.Sprintf(" [%s]", p.Code)
			}
			extra := ""
			if !p.Coverable {
				extra = " (cannot be covered)"
			}
			fmt.Fprintf(&b, "- %s%s%s\n", p.Label, code, extra)
		}
	}
	if len(info.NotationDoc) > 0 {
		b.WriteString("\nMove notation:\n")
		for _, n := range info.NotationDoc {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	return b.String()
}

// registerResourcesAndPrompts exposes game rules as MCP resources and adds
// ready-made prompts that tell a model how to play or train.
func registerResourcesAndPrompts(s *server.MCPServer, mgr *session.Manager) {
	s.AddResource(
		mcp.NewResource("bingle://games", "Available games",
			mcp.WithResourceDescription("Every game this server can host, with a one-line description."),
			mcp.WithMIMEType("application/json"),
		),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			defs := mgr.ListGameDefs(ctx)
			data, err := json.MarshalIndent(wire.GameDefs(defs), "", "  ")
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{mcp.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)}}, nil
		},
	)

	s.AddResource(
		mcp.NewResource("bingle://puzzles", "Puzzles",
			mcp.WithResourceDescription("The proven \"win in N\" puzzles (without solutions). Start one with env_reset(puzzle_id=...)."),
			mcp.WithMIMEType("application/json"),
		),
		func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			list, err := puzzles.All()
			if err != nil {
				return nil, err
			}
			type item struct {
				ID        string `json:"id"`
				GameDefID string `json:"game_def_id"`
				Title     string `json:"title"`
				ToMove    int    `json:"to_move"`
				WinIn     int    `json:"win_in"`
			}
			out := make([]item, len(list))
			for i, p := range list {
				out[i] = item{ID: p.ID, GameDefID: p.GameDefID, Title: p.Title, ToMove: p.ToMove, WinIn: p.WinIn}
			}
			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{mcp.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)}}, nil
		},
	)

	s.AddResourceTemplate(
		mcp.NewResourceTemplate("bingle://games/{id}/rules", "Game rules",
			mcp.WithTemplateDescription("The rules, pieces, win conditions and move notation of one game."),
			mcp.WithTemplateMIMEType("text/plain"),
		),
		func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			uri := req.Params.URI
			id := strings.TrimSuffix(strings.TrimPrefix(uri, "bingle://games/"), "/rules")
			info, err := mgr.GameInfo(id)
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{mcp.TextResourceContents{URI: uri, MIMEType: "text/plain", Text: rulesText(info)}}, nil
		},
	)

	s.AddPrompt(
		mcp.NewPrompt("play_game",
			mcp.WithPromptDescription("Instructions for playing a complete game of a Bingle game through the MCP tools."),
			mcp.WithArgument("game_def_id", mcp.RequiredArgument(), mcp.ArgumentDescription("The game to play, e.g. tak-5x5 (see list_game_defs).")),
			mcp.WithArgument("player_name", mcp.ArgumentDescription("The name to play under (default: Claude).")),
			mcp.WithArgument("opponent", mcp.ArgumentDescription("A built-in bot to play against: random, tactical, minimax or mcts (default: tactical).")),
		),
		func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			args := req.Params.Arguments
			game := args["game_def_id"]
			if game == "" {
				return nil, fmt.Errorf("game_def_id is required")
			}
			name := args["player_name"]
			if name == "" {
				name = "Claude"
			}
			opp := args["opponent"]
			if opp == "" {
				opp = "tactical"
			}
			text := fmt.Sprintf(`Play a complete game of %[1]s on the Bingle server, as player %[2]q against the %[3]q bot.

1. Call get_game_info with game_def_id=%[1]q and read the rules and the move notation carefully before moving.
2. Call create_game with game_def_id=%[1]q, player_name=%[2]q, opponent=%[3]q. Remember the room_code it returns.
3. Repeat until the game is over:
   a. Call get_observation(room_code, player_name=%[2]q). Read the board_text and the legal_moves list.
   b. Think about what your opponent threatens and what you threaten. Check for an immediate win for either side first.
   c. Call make_move with notation set to EXACTLY one string from legal_moves. If the call is rejected, read the error, fix the move and try again.
4. When done is true, report the result and a short summary of how the game went.

Never invent a move that is not in legal_moves, and do not undo or resign unless asked.`, game, name, opp)
			return mcp.NewGetPromptResult(
				fmt.Sprintf("Play %s against a bot", game),
				[]mcp.PromptMessage{mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text))},
			), nil
		},
	)

	s.AddPrompt(
		mcp.NewPrompt("train_agent",
			mcp.WithPromptDescription("A procedure for evaluating or training a policy on a game using the in-memory env_* tools."),
			mcp.WithArgument("game_def_id", mcp.RequiredArgument(), mcp.ArgumentDescription("The game to train on, e.g. tak-5x5.")),
			mcp.WithArgument("episodes", mcp.ArgumentDescription("How many episodes to run (default 20).")),
		),
		func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			args := req.Params.Arguments
			game := args["game_def_id"]
			if game == "" {
				return nil, fmt.Errorf("game_def_id is required")
			}
			episodes := args["episodes"]
			if episodes == "" {
				episodes = "20"
			}
			text := fmt.Sprintf(`Evaluate a policy on %[1]s for %[2]s episodes using Bingle's environments.

For each episode:
1. env_reset(game_def_id=%[1]q, opponent="tactical", seed=<episode number>, bot_iterations=50 if you use mcts) and keep the env_id.
2. While the observation's done is false: choose a move from legal_moves (the observation's board_text and tps describe the position) and call env_step(env_id, move). The opponent's reply is already included in the next observation.
3. Record result/reward (+1 win, -1 loss, 0 draw), the number of plies and the game's history.
4. Call env_close(env_id).

Use env_fork(env_id) before a risky move if you want to try alternatives from the same position. Finish with the win/draw/loss tally and the average game length. To establish baselines first, call match_bots between random, tactical, minimax and mcts.`, game, episodes)
			return mcp.NewGetPromptResult(
				fmt.Sprintf("Evaluate a policy on %s", game),
				[]mcp.PromptMessage{mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text))},
			), nil
		},
	)

	s.AddPrompt(
		mcp.NewPrompt("solve_puzzles",
			mcp.WithPromptDescription("Work through Bingle's proven puzzles (positions with a forced win) and report a solve rate — a quick reasoning benchmark."),
			mcp.WithArgument("game_def_id", mcp.ArgumentDescription("Only puzzles for this game, e.g. tak-5x5 (default: all).")),
			mcp.WithArgument("max_win_in", mcp.ArgumentDescription("Only puzzles solvable in at most this many of your moves (default: all).")),
		),
		func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			args := req.Params.Arguments
			filter := "list_puzzles()"
			if g := args["game_def_id"]; g != "" {
				filter = fmt.Sprintf("list_puzzles(game_def_id=%q", g)
				if w := args["max_win_in"]; w != "" {
					filter += ", max_win_in=" + w
				}
				filter += ")"
			} else if w := args["max_win_in"]; w != "" {
				filter = "list_puzzles(max_win_in=" + w + ")"
			}
			text := fmt.Sprintf(`Solve Bingle puzzles and report how many you got right.

1. Call %s to get the puzzle list (each entry names the side to move and how many moves the win takes).
2. For each puzzle: call env_reset(puzzle_id=<id>, opponent="tactical"). The observation shows the position (board_text) and legal_moves; you move first and have a forced win. Search for it: look for moves that win at once, then moves after which every defence loses.
3. Play your move with env_step(env_id, move). If the opponent replies, find your next move. The puzzle is solved when the observation says done with result "win" (reward +1); it is failed on a loss, a draw or truncated.
4. Call env_close(env_id), and never peek at solutions (do not call list_puzzles with reveal=true).

Finish with a table: puzzle id, win_in, solved or not, and the overall solve rate.`, filter)
			return mcp.NewGetPromptResult(
				"Solve proven puzzles",
				[]mcp.PromptMessage{mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text))},
			), nil
		},
	)
}
