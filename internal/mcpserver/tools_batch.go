package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
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
			mcp.WithDescription("Queue a batch of bot-versus-bot games that run in the background. Every game reports how each seat and bot fared, how long games ran and how they ended; a game with a practice study measures more, and its report has a section of its own. `spec` is {base: a game id like \"hex-11\", players: [{bot: random|tactical|minimax|mcts, mcts_iterations, minimax_nodes, faction}] (one per seat; faction is the study's pick for the seat, named for every seat or none), options (the study's options for the table), games, seed, rotate (shift seats each game, default advisable), max_plies, event_games, workers}. A spec over a ceiling is refused with the limit named (status 400), a full queue or a spent allowance likewise (429): GET /api/batches has the limits. Returns the batch; it is ended, keeping its games, at its time limit (limit \"time\"); poll get_batch."),
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
				return batchRefusal(err), nil
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

// batchRefusal is the answer to a start_batch that was refused: the message,
// and in its structured content the status the REST route answers with, and
// the limit the batch ran into and its value, as that route's body has them.
func batchRefusal(err error) *mcp.CallToolResult {
	res := mcp.NewToolResultError(err.Error())
	out := map[string]any{"error": err.Error()}
	var over *batch.LimitError
	var allowance *batch.AllowanceError
	switch {
	case errors.As(err, &over):
		out["status"], out["limit"], out["max"] = 400, over.Limit, over.Max
	case errors.As(err, &allowance):
		out["status"], out["limit"], out["max"] = 429, allowance.Limit, allowance.Max
	case errors.Is(err, batch.ErrNotAdmin):
		out["status"] = 403
	default:
		return res
	}
	res.StructuredContent = out
	return res
}
