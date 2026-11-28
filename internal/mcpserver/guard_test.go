package mcpserver

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"github.com/noderunner/bingle/internal/auth"
)

// Every tool has a REST twin to take its capability from, and the capability
// each asks for is the one its twin's route rule does (the sweep of #376).
func TestToolsAskForTheirTwinsCapability(t *testing.T) {
	s := server.NewMCPServer("t", "0")
	registerTools(s, nil)
	registerPlayTools(s, nil)
	registerLobbyTools(s, nil)
	registerEnvTools(s, nil)
	registerBatchTools(s, nil)
	for name := range s.ListTools() {
		if _, ok := twins[name]; !ok {
			t.Errorf("the tool %q has no REST twin in twins: say which route does what it does", name)
		}
	}
	for name := range twins {
		if s.GetTool(name) == nil {
			t.Errorf("twins names %q, which is not a tool", name)
		}
	}
	want := map[string]string{
		"create_game": auth.CapCreateGame, "import_game": auth.CapCreateGame,
		"join_game": auth.CapJoin, "send_chat": auth.CapChatGame,
		"enter_tournament": auth.CapEnterTournament, "check_in_tournament": auth.CapEnterTournament,
		"start_batch": auth.CapRunBatch, "cancel_batch": auth.CapRunBatch,
		"make_move": auth.CapPlay, "start_game": auth.CapPlay, "add_bot": auth.CapPlay, "env_reset": auth.CapPlay,
		"match_bots": auth.CapPlay, "set_reactions": auth.CapPlay,
		"get_batch": "", "list_batches": "", "get_chat": "", "list_tournaments": "", "env_close": "",
	}
	for tool, capability := range want {
		tw := twins[tool]
		if got := auth.Need(tw.method, tw.path); got != capability {
			t.Errorf("%s asks for %q, want %q", tool, got, capability)
		}
	}
}
