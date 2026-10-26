package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/noderunner/bingle/internal/batch"
)

// registerBatchTools adds the bot practice batch tools: run many bot games in
// the background and read the aggregated metrics.
func registerBatchTools(s *server.MCPServer, bm *batch.Manager) {
	s.AddTool(
		mcp.NewTool("start_batch",
			mcp.WithDescription("Queue a batch of bot-versus-bot Cthulhu Wars games that run in the background and are measured (units built, actions, gates, spellbooks, areas held, doom by round). `spec` is {base: layout id like \"cthulhu-wars\" or \"cthulhu-wars-5p\", high_priests, players: [{faction: gc|cc|bg|ys|ww|sl, bot: random|tactical|minimax|mcts, mcts_iterations, minimax_nodes}] (one per seat), games, seed, rotate (shift seats each game, default advisable), max_plies, event_games}. Returns the batch; poll get_batch."),
			mcp.WithObject("spec", mcp.Required(), mcp.Description("The batch specification (see above).")),
			mcp.WithString("name", mcp.Description("A name for the batch.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			raw, err := json.Marshal(request.GetArguments()["spec"])
			if err != nil {
				return mcp.NewToolResultError("spec must be an object"), nil
			}
			var spec batch.Spec
			if err := json.Unmarshal(raw, &spec); err != nil {
				return mcp.NewToolResultError("bad spec: " + err.Error()), nil
			}
			id, err := bm.Start(ctx, request.GetString("name", ""), spec)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			info, _ := bm.Get(ctx, id)
			return mcp.NewToolResultStructured(info, fmt.Sprintf("Batch %d queued: %d games.", id, info.Games)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_batch",
			mcp.WithDescription("Read a practice batch: its status and progress, and (report=true, the default) the aggregated report of the games finished so far — per cult win rate with confidence interval, doom/gates by round, spellbook timing, units built, actions used and areas held, each with the win rate when the cult did more than its average of it."),
			mcp.WithNumber("batch_id", mcp.Required(), mcp.Description("Batch id from start_batch or list_batches.")),
			mcp.WithBoolean("report", mcp.Description("Include the aggregated report (default true).")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := int64(request.GetInt("batch_id", 0))
			info, err := bm.Get(ctx, id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			out := map[string]any{"batch": info}
			if request.GetBool("report", true) {
				rep, err := bm.Report(ctx, id)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				out["report"] = rep
			}
			return mcp.NewToolResultStructured(out, fmt.Sprintf("Batch %d: %s, %d/%d games.", id, info.Status, info.Done, info.Games)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("list_batches",
			mcp.WithDescription("List the practice batches, newest first."),
		),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			list, err := bm.List(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructured(map[string]any{"batches": list}, fmt.Sprintf("%d batches.", len(list))), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cancel_batch",
			mcp.WithDescription("Stop a running or queued practice batch (the games already played are kept)."),
			mcp.WithNumber("batch_id", mcp.Required(), mcp.Description("Batch id.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := int64(request.GetInt("batch_id", 0))
			if err := bm.Cancel(ctx, id); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructuredOnly(map[string]any{"cancelled": id}), nil
		},
	)
}
