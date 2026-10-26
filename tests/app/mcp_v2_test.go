package app_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func toolText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		b.WriteString(mcp.GetTextFromContent(c))
		b.WriteByte('\n')
	}
	return b.String()
}

type envObs struct {
	EnvID      string   `json:"env_id"`
	AgentSeat  int      `json:"agent_seat"`
	ToMove     int      `json:"to_move"`
	YourTurn   bool     `json:"your_turn"`
	Ply        int      `json:"ply"`
	BoardText  string   `json:"board_text"`
	TPS        string   `json:"tps"`
	LegalMoves []string `json:"legal_moves"`
	LastMoves  []string `json:"last_moves"`
	History    []string `json:"history"`
	Done       bool     `json:"done"`
	Truncated  bool     `json:"truncated"`
	WinnerSeat *int     `json:"winner_seat"`
	EndReason  string   `json:"end_reason"`
	Reward     float64  `json:"reward"`
	Result     string   `json:"result"`

	Planes *struct {
		Shape    [3]int    `json:"shape"`
		Channels []string  `json:"channels"`
		Data     []float64 `json:"data"`
	} `json:"planes"`
	LegalActionIDs []int `json:"legal_action_ids"`
	NumActions     int   `json:"num_actions"`
}

func TestMCP_ToolListIncludesNewTools(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res, err := c.ListTools(t.Context(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	have := map[string]bool{}
	for _, tool := range res.Tools {
		have[tool.Name] = true
	}
	for _, want := range []string{
		"list_game_defs", "create_game", "join_game", "get_state", "list_legal_moves", "make_move", "get_moves",
		"get_game_info", "list_bots", "get_observation", "get_legal_moves", "undo_move", "resign", "simulate", "export_game",
		"env_reset", "env_step", "env_observe", "env_fork", "env_close", "env_list", "match_bots", "draw",
		"env_spec", "env_batch_step", "takeback", "get_game_guide", "add_bot",
		"start_game", "set_seat_cult", "send_chat", "get_chat", "get_reactions", "set_reactions",
	} {
		if !have[want] {
			t.Errorf("tool %q not registered", want)
		}
	}
}

func TestMCP_PlayTakVsBotWithObservations(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	info := callTool(t, c, "get_game_info", map[string]any{"game_def_id": "tak-5x5"})
	if info.IsError {
		t.Fatalf("get_game_info error: %s", toolText(info))
	}
	if txt := toolText(info); !strings.Contains(txt, "road") || !strings.Contains(txt, "PTN") {
		t.Errorf("get_game_info text lacks rules/notation help: %.200s", txt)
	}

	var created struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
	}
	res := callTool(t, c, "create_game", map[string]any{"game_def_id": "tak-5x5", "player_name": "Agent", "opponent": "tactical"})
	decodeToolResult(t, res, &created)
	if created.Status != "active" {
		t.Fatalf("created = %+v (%s)", created, toolText(res))
	}

	var obs struct {
		YourTurn   bool     `json:"your_turn"`
		LegalMoves []string `json:"legal_moves"`
		BoardText  string   `json:"board_text"`
		Done       bool     `json:"done"`
	}
	res = callTool(t, c, "get_observation", map[string]any{"room_code": created.RoomCode, "player_name": "Agent"})
	decodeToolResult(t, res, &obs)
	if !obs.YourTurn || len(obs.LegalMoves) != 25 || !strings.Contains(toolText(res), "Legal moves (25)") {
		t.Fatalf("observation = %+v text=%.300s", obs, toolText(res))
	}

	// Play the first few turns by always taking the first legal move.
	for i := 0; i < 4 && !obs.Done; i++ {
		mv := obs.LegalMoves[0]
		res = callTool(t, c, "make_move", map[string]any{"room_code": created.RoomCode, "player_name": "Agent", "notation": mv})
		if res.IsError {
			t.Fatalf("make_move(%q): %s", mv, toolText(res))
		}
		res = callTool(t, c, "get_observation", map[string]any{"room_code": created.RoomCode, "player_name": "Agent"})
		decodeToolResult(t, res, &obs)
		if !obs.Done && (!obs.YourTurn || len(obs.LegalMoves) == 0) {
			t.Fatalf("after move %d the agent should be to move again: %+v", i, obs)
		}
	}

	// get_legal_moves matches the observation.
	var lm struct {
		Seat  int                         `json:"seat"`
		Moves []struct{ Notation string } `json:"moves"`
	}
	res = callTool(t, c, "get_legal_moves", map[string]any{"room_code": created.RoomCode})
	decodeToolResult(t, res, &lm)
	if lm.Seat != 1 || len(lm.Moves) != len(obs.LegalMoves) {
		t.Errorf("get_legal_moves = seat %d, %d moves; observation had %d", lm.Seat, len(lm.Moves), len(obs.LegalMoves))
	}

	// Undo, then export.
	res = callTool(t, c, "undo_move", map[string]any{"room_code": created.RoomCode, "player_name": "Agent"})
	if res.IsError {
		t.Errorf("undo_move: %s", toolText(res))
	}
	res = callTool(t, c, "export_game", map[string]any{"room_code": created.RoomCode})
	var rec struct {
		Notation []string `json:"notation"`
		PTN      string   `json:"ptn"`
	}
	decodeToolResult(t, res, &rec)
	if len(rec.Notation) != 6 || !strings.Contains(rec.PTN, "[Size \"5\"]") {
		t.Errorf("export = %+v", rec)
	}

	// Placing on an occupied square is illegal: a tool-level error with a
	// useful message.
	var cur struct {
		State struct {
			Cells map[string]json.RawMessage `json:"cells"`
		} `json:"state"`
	}
	res = callTool(t, c, "get_observation", map[string]any{"room_code": created.RoomCode, "player_name": "Agent"})
	decodeToolResult(t, res, &cur)
	occupied := ""
	for sq := range cur.State.Cells {
		occupied = sq
		break
	}
	if occupied == "" {
		t.Fatal("no occupied square to test with")
	}
	res = callTool(t, c, "make_move", map[string]any{"room_code": created.RoomCode, "player_name": "Agent", "notation": occupied})
	if !res.IsError {
		t.Errorf("placing on occupied %s should be a tool-level error", occupied)
	}
	if txt := toolText(res); !strings.Contains(txt, "occupied") {
		t.Errorf("error text = %q, want it to say the square is occupied", txt)
	}
}

func TestMCP_EnvEpisodeAgainstRandomBot(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	var obs envObs
	res := callTool(t, c, "env_reset", map[string]any{"game_def_id": "tak-3x3", "opponent": "random", "seed": 7})
	if res.IsError {
		t.Fatalf("env_reset: %s", toolText(res))
	}
	decodeToolResult(t, res, &obs)
	if obs.EnvID == "" || !obs.YourTurn || len(obs.LegalMoves) != 9 || obs.AgentSeat != 1 || obs.Done {
		t.Fatalf("reset observation = %+v", obs)
	}
	id := obs.EnvID

	// Fork before moving: the fork is independent.
	var fork envObs
	res = callTool(t, c, "env_fork", map[string]any{"env_id": id})
	decodeToolResult(t, res, &fork)
	if fork.EnvID == id || fork.Ply != 0 {
		t.Fatalf("fork = %+v", fork)
	}

	steps := 0
	for !obs.Done && steps < 100 {
		mv := obs.LegalMoves[0]
		res = callTool(t, c, "env_step", map[string]any{"env_id": id, "move": mv})
		if res.IsError {
			t.Fatalf("env_step(%q): %s", mv, toolText(res))
		}
		decodeToolResult(t, res, &obs)
		steps++
		if !obs.Done && (!obs.YourTurn || len(obs.LastMoves) != 2) {
			t.Fatalf("step %d: your_turn=%v last_moves=%v (want own move + bot reply)", steps, obs.YourTurn, obs.LastMoves)
		}
	}
	if !obs.Done {
		t.Fatal("episode never finished")
	}
	if obs.Result == "" || (obs.Result == "win" && obs.Reward != 1) || (obs.Result == "loss" && obs.Reward != -1) {
		t.Errorf("terminal observation = result %q reward %v", obs.Result, obs.Reward)
	}

	// Stepping a finished episode is an error.
	res = callTool(t, c, "env_step", map[string]any{"env_id": id, "move": "a1"})
	if !res.IsError {
		t.Error("env_step after done should error")
	}

	// The fork still sits at the start.
	res = callTool(t, c, "env_observe", map[string]any{"env_id": fork.EnvID})
	decodeToolResult(t, res, &fork)
	if fork.Ply != 0 || fork.Done {
		t.Errorf("fork was affected by the original episode: %+v", fork)
	}

	callTool(t, c, "env_close", map[string]any{"env_id": id})
	res = callTool(t, c, "env_observe", map[string]any{"env_id": id})
	if !res.IsError {
		t.Error("closed env should not be observable")
	}
}

func TestMCP_EnvSelfPlayAndStartTPS(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	var obs envObs
	// White (seat 1) to move can complete a road at e3 in one move.
	res := callTool(t, c, "env_reset", map[string]any{
		"game_def_id": "tak-5x5", "opponent": "self", "start_tps": "x5/x5/1,1,1,1,x/x5/x5 1 6",
	})
	if res.IsError {
		t.Fatalf("env_reset: %s", toolText(res))
	}
	decodeToolResult(t, res, &obs)
	if obs.ToMove != 1 || !obs.YourTurn {
		t.Fatalf("obs = %+v", obs)
	}
	found := false
	for _, m := range obs.LegalMoves {
		if m == "e3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("e3 not among legal moves %v", obs.LegalMoves)
	}
	res = callTool(t, c, "env_step", map[string]any{"env_id": obs.EnvID, "move": "e3"})
	decodeToolResult(t, res, &obs)
	if !obs.Done || obs.Result != "win" || obs.Reward != 1 || obs.EndReason != "road" || obs.WinnerSeat == nil || *obs.WinnerSeat != 1 {
		t.Errorf("after road: %+v", obs)
	}
}

func TestMCP_EnvTruncatesAtMaxPlies(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	var obs envObs
	res := callTool(t, c, "env_reset", map[string]any{"game_def_id": "demo-freeplay", "opponent": "random", "seed": 1, "max_plies": 4})
	decodeToolResult(t, res, &obs)
	for i := 0; i < 10 && !obs.Done; i++ {
		res = callTool(t, c, "env_step", map[string]any{"env_id": obs.EnvID, "move": obs.LegalMoves[0]})
		if res.IsError {
			t.Fatalf("env_step: %s", toolText(res))
		}
		decodeToolResult(t, res, &obs)
	}
	if !obs.Done || !obs.Truncated || obs.Reward != 0 {
		t.Errorf("obs = done %v truncated %v reward %v, want truncated with zero reward", obs.Done, obs.Truncated, obs.Reward)
	}
}

func TestMCP_MatchBots(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	var res struct {
		Played int `json:"played"`
		AWins  int `json:"a_wins"`
		BWins  int `json:"b_wins"`
		Draws  int `json:"draws"`
		Games  []struct {
			Moves []string `json:"moves"`
		} `json:"games"`
	}
	out := callTool(t, c, "match_bots", map[string]any{
		"game_def_id": "connect-four", "bot_a": "tactical", "bot_b": "random", "games": 6, "seed": 2, "include_moves": true,
	})
	if out.IsError {
		t.Fatalf("match_bots: %s", toolText(out))
	}
	decodeToolResult(t, out, &res)
	if res.Played != 6 || res.AWins+res.BWins+res.Draws != 6 || len(res.Games) != 6 || len(res.Games[0].Moves) == 0 {
		t.Errorf("result = %+v", res)
	}
	if !strings.Contains(toolText(out), "tactical vs random") {
		t.Errorf("text summary = %q", toolText(out))
	}
}

func TestMCP_ResourcesAndPrompts(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)
	ctx := t.Context()

	res, err := c.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	foundList := false
	for _, r := range res.Resources {
		if r.URI == "bingle://games" {
			foundList = true
		}
	}
	if !foundList {
		t.Errorf("resources = %+v, want bingle://games", res.Resources)
	}

	var read mcp.ReadResourceRequest
	read.Params.URI = "bingle://games/tak-5x5/rules"
	got, err := c.ReadResource(ctx, read)
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(got.Contents) != 1 {
		t.Fatalf("contents = %d", len(got.Contents))
	}
	text := got.Contents[0].(mcp.TextResourceContents).Text
	for _, want := range []string{"Tak (5×5)", "road", "Portable Tak Notation", "Capstone"} {
		if !strings.Contains(text, want) {
			t.Errorf("rules text lacks %q:\n%.400s", want, text)
		}
	}

	var list mcp.ReadResourceRequest
	list.Params.URI = "bingle://games"
	lg, err := c.ReadResource(ctx, list)
	if err != nil || !strings.Contains(lg.Contents[0].(mcp.TextResourceContents).Text, "go-9x9") {
		t.Errorf("game list resource: %v", err)
	}

	var pz mcp.ReadResourceRequest
	pz.Params.URI = "bingle://puzzles"
	pr0, err := c.ReadResource(ctx, pz)
	if err != nil || !strings.Contains(pr0.Contents[0].(mcp.TextResourceContents).Text, "win_in") {
		t.Errorf("puzzles resource: %v", err)
	}
	if strings.Contains(pr0.Contents[0].(mcp.TextResourceContents).Text, "solution") {
		t.Error("the puzzles resource must not leak solutions")
	}

	prompts, err := c.ListPrompts(ctx, mcp.ListPromptsRequest{})
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	names := map[string]bool{}
	for _, p := range prompts.Prompts {
		names[p.Name] = true
	}
	if !names["play_game"] || !names["train_agent"] || !names["solve_puzzles"] {
		t.Errorf("prompts = %v", names)
	}
	var sp mcp.GetPromptRequest
	sp.Params.Name = "solve_puzzles"
	sp.Params.Arguments = map[string]string{"game_def_id": "tak-5x5", "max_win_in": "2"}
	spr, err := c.GetPrompt(ctx, sp)
	if err != nil {
		t.Fatalf("GetPrompt(solve_puzzles): %v", err)
	}
	if sb := spr.Messages[0].Content.(mcp.TextContent).Text; !strings.Contains(sb, `list_puzzles(game_def_id="tak-5x5", max_win_in=2)`) || !strings.Contains(sb, "puzzle_id") {
		t.Errorf("solve_puzzles prompt = %.300s", sb)
	}
	var gp mcp.GetPromptRequest
	gp.Params.Name = "play_game"
	gp.Params.Arguments = map[string]string{"game_def_id": "hex-11", "player_name": "Ada"}
	pr, err := c.GetPrompt(ctx, gp)
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	body := pr.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(body, "hex-11") || !strings.Contains(body, "Ada") || !strings.Contains(body, "legal_moves") {
		t.Errorf("prompt text = %.300s", body)
	}
}

func TestMCP_EnvSpecPlanesActionIDsAndBatchStep(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	// env_spec describes the game before any episode exists.
	res := callTool(t, c, "env_spec", map[string]any{"game_def_id": "tak-5x5"})
	if res.IsError {
		t.Fatalf("env_spec: %s", toolText(res))
	}
	var spec struct {
		Board struct {
			Kind          string `json:"kind"`
			Width, Height int
			Cells         []string `json:"cells"`
		} `json:"board"`
		Planes struct {
			Shape    [3]int   `json:"shape"`
			Channels []string `json:"channels"`
		} `json:"planes"`
		Actions struct {
			Fixed bool `json:"fixed"`
			Count int  `json:"count"`
		} `json:"actions"`
	}
	decodeToolResult(t, res, &spec)
	if spec.Planes.Shape[1] != 5 || spec.Planes.Shape[2] != 5 || len(spec.Planes.Channels) != spec.Planes.Shape[0] {
		t.Errorf("spec planes = %+v", spec.Planes)
	}
	if !spec.Actions.Fixed || spec.Actions.Count < 1000 {
		t.Errorf("spec actions = %+v", spec.Actions)
	}
	if len(spec.Board.Cells) != 25 || spec.Board.Cells[0] != "a5" || spec.Board.Cells[24] != "e1" {
		t.Errorf("plane cells = %v", spec.Board.Cells)
	}

	// Two environments with planes + action ids, stepped in one batch by
	// action id and by notation.
	var a, b envObs
	for i, dst := range []*envObs{&a, &b} {
		res = callTool(t, c, "env_reset", map[string]any{
			"game_def_id": "tak-5x5", "opponent": "random", "seed": i + 1,
			"include": []string{"planes", "action_ids"},
		})
		if res.IsError {
			t.Fatalf("env_reset: %s", toolText(res))
		}
		decodeToolResult(t, res, dst)
	}
	if a.Planes == nil || len(a.Planes.Data) != a.Planes.Shape[0]*25 || a.NumActions != spec.Actions.Count {
		t.Fatalf("observation encodings missing: planes=%v num_actions=%d", a.Planes != nil, a.NumActions)
	}
	if len(a.LegalActionIDs) != len(a.LegalMoves) {
		t.Fatalf("legal_action_ids %d vs legal_moves %d", len(a.LegalActionIDs), len(a.LegalMoves))
	}

	res = callTool(t, c, "env_batch_step", map[string]any{"steps": []map[string]any{
		{"env_id": a.EnvID, "action_id": a.LegalActionIDs[0]},
		{"env_id": b.EnvID, "move": b.LegalMoves[0]},
		{"env_id": b.EnvID, "move": "not-a-move"},
		{"env_id": "env-missing", "move": "a1"},
	}})
	if res.IsError {
		t.Fatalf("env_batch_step: %s", toolText(res))
	}
	var batch struct {
		Results []struct {
			EnvID       string  `json:"env_id"`
			Observation *envObs `json:"observation"`
			Error       string  `json:"error"`
		} `json:"results"`
	}
	decodeToolResult(t, res, &batch)
	if len(batch.Results) != 4 {
		t.Fatalf("results = %+v", batch.Results)
	}
	if r := batch.Results[0]; r.Error != "" || r.Observation == nil || r.Observation.History[0] != a.LegalMoves[0] {
		t.Errorf("action-id step = %+v", r)
	}
	if r := batch.Results[1]; r.Error != "" || r.Observation == nil || r.Observation.Planes == nil {
		t.Errorf("notation step = %+v", r)
	}
	if batch.Results[2].Error == "" || batch.Results[3].Error == "" {
		t.Errorf("bad steps should report errors: %+v", batch.Results[2:])
	}

	// env_step also takes an action id.
	next := batch.Results[0].Observation
	res = callTool(t, c, "env_step", map[string]any{"env_id": a.EnvID, "action_id": next.LegalActionIDs[0]})
	if res.IsError {
		t.Fatalf("env_step by action_id: %s", toolText(res))
	}
}

func TestMCP_CreateGameFromTPSPosition(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "create_game", map[string]any{
		"game_def_id": "tak-5x5", "player_name": "Agent", "opponent": "random",
		"start_tps": "x5/x5/1,1,1,1,x/x5/x5 1 6",
	})
	if res.IsError {
		t.Fatalf("create_game: %s", toolText(res))
	}
	var created struct {
		RoomCode string `json:"room_code"`
	}
	decodeToolResult(t, res, &created)

	res = callTool(t, c, "get_observation", map[string]any{"room_code": created.RoomCode, "player_name": "Agent"})
	if res.IsError {
		t.Fatalf("get_observation: %s", toolText(res))
	}
	var obs struct {
		YourTurn   bool     `json:"your_turn"`
		LegalMoves []string `json:"legal_moves"`
		Ply        int      `json:"ply"`
	}
	decodeToolResult(t, res, &obs)
	if !obs.YourTurn || obs.Ply != 10 {
		t.Fatalf("observation = %+v, want my turn at ply 10", obs)
	}
	winning := false
	for _, m := range obs.LegalMoves {
		if m == "e3" {
			winning = true
		}
	}
	if !winning {
		t.Errorf("e3 (the road-completing move) missing from %v", obs.LegalMoves)
	}

	// An invalid start is a tool error, not a half-created room.
	res = callTool(t, c, "create_game", map[string]any{"game_def_id": "tak-5x5", "player_name": "Agent", "start_tps": "nonsense"})
	if !res.IsError {
		t.Error("an invalid start_tps should be a tool error")
	}
}

func TestMCP_GameGuideAndAddBot(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "get_game_guide", map[string]any{"game_def_id": "cthulhu-wars"})
	if res.IsError {
		t.Fatalf("get_game_guide: %s", toolText(res))
	}
	txt := toolText(res)
	for _, want := range []string{"Instant Death", "Ritual of Annihilation", "Great Cthulhu", "| Spellbook |"} {
		if !strings.Contains(txt, want) {
			t.Errorf("guide lacks %q", want)
		}
	}
	one := callTool(t, c, "get_game_guide", map[string]any{"game_def_id": "cthulhu-wars", "tab": "battle"})
	if one.IsError || !strings.Contains(toolText(one), "Kill") || strings.Contains(toolText(one), "Instant Death") {
		t.Errorf("a single page: %s", toolText(one))
	}
	if bad := callTool(t, c, "get_game_guide", map[string]any{"game_def_id": "cthulhu-wars", "tab": "nope"}); !bad.IsError {
		t.Error("an unknown page should be an error")
	}
	gen := callTool(t, c, "get_game_guide", map[string]any{"game_def_id": "demo-freeplay"})
	if gen.IsError || !strings.Contains(toolText(gen), "Move notation") {
		t.Errorf("generated guide: %s", toolText(gen))
	}
	tak := callTool(t, c, "get_game_guide", map[string]any{"game_def_id": "tak-5x5"})
	if tak.IsError || !strings.Contains(toolText(tak), "## Roads") {
		t.Errorf("Tak's own guide: %s", toolText(tak))
	}

	var created struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
	}
	decodeToolResult(t, callTool(t, c, "create_game", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Agent"}), &created)
	if created.Status != "waiting" {
		t.Fatalf("created %+v", created)
	}
	for i := 0; i < 3; i++ {
		r := callTool(t, c, "add_bot", map[string]any{"room_code": created.RoomCode, "player_name": "Agent", "bot": "random"})
		if r.IsError {
			t.Fatalf("add_bot %d: %s", i, toolText(r))
		}
	}
	if r := callTool(t, c, "add_bot", map[string]any{"room_code": created.RoomCode, "player_name": "Agent", "bot": "random"}); !r.IsError {
		t.Error("a full table has no open seat")
	}
}

func TestMCP_LobbyChatAndReactions(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	var created struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
	}
	decodeToolResult(t, callTool(t, c, "create_game", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Agent", "lobby": true}), &created)
	if created.Status != "waiting" {
		t.Fatalf("created %+v", created)
	}
	// Bots are named after the faction of their seat and their kind.
	var bot struct {
		Name string `json:"name"`
		Seat int    `json:"seat"`
	}
	decodeToolResult(t, callTool(t, c, "add_bot", map[string]any{"room_code": created.RoomCode, "player_name": "Agent", "bot": "mcts", "seat": 4}), &bot)
	if bot.Name != "Yellow Sign (MCTS)" || bot.Seat != 4 {
		t.Fatalf("bot %+v", bot)
	}
	if r := callTool(t, c, "start_game", map[string]any{"room_code": created.RoomCode, "player_name": "Agent"}); !r.IsError {
		t.Error("starting with open seats must fail")
	}
	for _, seat := range []int{2, 3} {
		if r := callTool(t, c, "add_bot", map[string]any{"room_code": created.RoomCode, "player_name": "Agent", "bot": "random", "seat": seat}); r.IsError {
			t.Fatalf("add_bot: %s", toolText(r))
		}
	}
	var started struct {
		Status string `json:"status"`
	}
	decodeToolResult(t, callTool(t, c, "start_game", map[string]any{"room_code": created.RoomCode, "player_name": "Agent"}), &started)
	if started.Status != "active" {
		t.Fatalf("started %+v", started)
	}

	if r := callTool(t, c, "send_chat", map[string]any{"room_code": created.RoomCode, "player_name": "Agent", "text": "good luck"}); r.IsError {
		t.Fatalf("send_chat: %s", toolText(r))
	}
	var chat struct {
		Messages []struct {
			Name string `json:"name"`
			Text string `json:"text"`
		} `json:"messages"`
	}
	decodeToolResult(t, callTool(t, c, "get_chat", map[string]any{"room_code": created.RoomCode}), &chat)
	if len(chat.Messages) != 1 || chat.Messages[0].Name != "Agent" || chat.Messages[0].Text != "good luck" {
		t.Fatalf("chat %+v", chat)
	}

	var set struct {
		Abilities []any `json:"abilities"`
		Targeted  bool  `json:"targeted"`
		AskAll    bool  `json:"ask_all"`
	}
	// An AI client is asked about every reaction (as a bot is): nothing is
	// passed for it unless it chooses so.
	decodeToolResult(t, callTool(t, c, "get_reactions", map[string]any{"room_code": created.RoomCode, "player_name": "Agent"}), &set)
	if !set.AskAll || set.Abilities == nil {
		t.Fatalf("settings %+v", set)
	}
	decodeToolResult(t, callTool(t, c, "set_reactions", map[string]any{"room_code": created.RoomCode, "player_name": "Agent", "ask_all": false, "targeted": true}), &set)
	if set.AskAll || !set.Targeted {
		t.Fatalf("settings %+v", set)
	}
	decodeToolResult(t, callTool(t, c, "get_reactions", map[string]any{"room_code": created.RoomCode, "player_name": "Agent"}), &set)
	if set.AskAll || !set.Targeted {
		t.Fatalf("settings %+v", set)
	}
}
