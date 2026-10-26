// Package mcpserver exposes bingle's session.Manager over the Model Context
// Protocol (MCP), via github.com/mark3labs/mcp-go's Streamable HTTP
// transport, so an AI client can play games through the same
// session.Manager/store.Store the REST API and WebSocket hub already share.
// See docs/history/plan.md section 6.10 for the per-tool JSON shapes this package
// implements.
package mcpserver

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"

	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/envs"
	"github.com/noderunner/bingle/internal/session"
)

// serverName/serverVersion identify this MCP server during the initialize
// handshake; the version is a fixed placeholder (bingle has no separate MCP
// server versioning scheme yet).
const (
	serverName    = "bingle"
	serverVersion = "0.1.0"
)

// NewServer builds an MCP server backed by mgr, with every tool registered,
// and returns it wrapped as a Streamable HTTP http.Handler suitable for
// mounting directly onto a router (e.g. at "/mcp", a sibling of "/api" and
// "/ws/{code}").
func NewServer(mgr *session.Manager, envMgr *envs.Manager, batchMgr *batch.Manager) http.Handler {
	mcpServer := server.NewMCPServer(serverName, serverVersion,
		server.WithInstructions(instructions),
		server.WithResourceCapabilities(false, false),
		server.WithPromptCapabilities(false),
	)
	registerTools(mcpServer, mgr)
	registerPlayTools(mcpServer, mgr)
	registerLobbyTools(mcpServer, mgr)
	registerEnvTools(mcpServer, envMgr)
	registerBatchTools(mcpServer, batchMgr)
	registerResourcesAndPrompts(mcpServer, mgr)
	return server.NewStreamableHTTPServer(mcpServer)
}

// instructions is shown to MCP clients on initialize.
const instructions = `Bingle hosts turn-based abstract strategy games (Tak and others) for human and AI players.

To play: list_game_defs, get_game_info (rules + move notation), create_game (optionally with opponent="mcts"), then loop get_observation -> make_move using a string from legal_moves.
To train or evaluate an agent at scale use the env_* tools: env_reset starts an in-memory episode against a built-in bot (or self-play), env_step plays a move and returns the next observation with reward, env_fork clones a position for search. Environments never touch the database and are fast.`
