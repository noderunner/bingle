package app_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestRESTEnvironmentsRoundTrip(t *testing.T) {
	baseURL := newRESTTestServer(t)

	var obs envObs
	if code := doJSON(t, "POST", baseURL+"/api/envs", map[string]any{
		"game_def_id": "tak-3x3", "opponent": "random", "seed": 3, "include": []string{"planes", "action_ids"},
	}, &obs); code != 200 {
		t.Fatalf("reset: %d", code)
	}
	if obs.EnvID == "" || !obs.YourTurn || obs.Planes == nil || len(obs.LegalActionIDs) != len(obs.LegalMoves) {
		t.Fatalf("reset observation = %+v", obs)
	}
	id := obs.EnvID

	// Step by notation, then by action id, then fork and observe.
	var stepped envObs
	if code := doJSON(t, "POST", baseURL+"/api/envs/"+id+"/step", map[string]any{"move": obs.LegalMoves[0]}, &stepped); code != 200 {
		t.Fatalf("step: %d", code)
	}
	if stepped.Ply < 2 || len(stepped.LastMoves) != 2 {
		t.Errorf("after a step: ply %d last %v", stepped.Ply, stepped.LastMoves)
	}
	if !stepped.Done {
		var again envObs
		if code := doJSON(t, "POST", baseURL+"/api/envs/"+id+"/step", map[string]any{"action_id": stepped.LegalActionIDs[0]}, &again); code != 200 {
			t.Errorf("step by action id: %d", code)
		}
	}
	var forked envObs
	if code := doJSON(t, "POST", baseURL+"/api/envs/"+id+"/fork", nil, &forked); code != 200 || forked.EnvID == id {
		t.Errorf("fork: %d %+v", code, forked.EnvID)
	}

	// Errors: illegal move 400, unknown env 404, nothing to step 400.
	var errBody map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/envs/"+forked.EnvID+"/step", map[string]any{"move": "zz9"}, &errBody); code != 400 {
		t.Errorf("illegal move: %d", code)
	}
	if code := doJSON(t, "GET", baseURL+"/api/envs/env-nope", nil, &errBody); code != 404 {
		t.Errorf("unknown env: %d", code)
	}
	if code := doJSON(t, "POST", baseURL+"/api/envs/"+forked.EnvID+"/step", map[string]any{}, &errBody); code != 400 {
		t.Errorf("empty step: %d", code)
	}

	// Batch, spec, list, close.
	var batch struct {
		Results []struct {
			EnvID string `json:"env_id"`
			Error string `json:"error"`
		} `json:"results"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/envs/batch-step", map[string]any{"steps": []map[string]any{
		{"env_id": forked.EnvID, "action_id": forked.LegalActionIDs[0]},
		{"env_id": "env-nope", "move": "a1"},
	}}, &batch); code != 200 || len(batch.Results) != 2 || batch.Results[0].Error != "" || batch.Results[1].Error == "" {
		t.Errorf("batch: %d %+v", code, batch)
	}
	var spec struct {
		Actions struct {
			Fixed bool `json:"fixed"`
			Count int  `json:"count"`
		} `json:"actions"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/env-specs/tak-3x3", nil, &spec); code != 200 || !spec.Actions.Fixed || spec.Actions.Count != obs.NumActions {
		t.Errorf("spec: %d %+v (num_actions %d)", code, spec, obs.NumActions)
	}
	if code := doJSON(t, "GET", baseURL+"/api/env-specs/nope", nil, nil); code != 404 {
		t.Errorf("unknown spec: %d", code)
	}
	var list struct {
		IDs []string `json:"env_ids"`
	}
	doJSON(t, "GET", baseURL+"/api/envs", nil, &list)
	if len(list.IDs) < 2 {
		t.Errorf("env list = %v", list.IDs)
	}
	if code := doJSON(t, "DELETE", baseURL+"/api/envs/"+id, nil, nil); code != 200 {
		t.Errorf("close: %d", code)
	}
	if code := doJSON(t, "GET", baseURL+"/api/envs/"+id, nil, &errBody); code != 404 {
		t.Errorf("closed env still observable: %d", code)
	}
}

func TestRESTMatchAndPuzzleEnvironments(t *testing.T) {
	baseURL := newRESTTestServer(t)

	var match struct {
		Played int `json:"played"`
		Games  []struct {
			Positions []struct {
				ActionID int `json:"action_id"`
				Result   int `json:"result"`
			} `json:"positions"`
		} `json:"games"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/matches", map[string]any{
		"game_def_id": "tic-tac-toe", "bot_a": "tactical", "bot_b": "random", "games": 3, "seed": 1, "record": true,
	}, &match); code != 200 || match.Played != 3 || len(match.Games) != 3 || len(match.Games[0].Positions) == 0 {
		t.Fatalf("match: %d %+v", code, match)
	}

	var list []puzzleResp
	doJSON(t, "GET", baseURL+"/api/puzzles?max_win_in=1", nil, &list)
	if len(list) == 0 {
		t.Skip("no puzzles shipped")
	}
	var obs envObs
	if code := doJSON(t, "POST", baseURL+"/api/envs", map[string]any{"puzzle_id": list[0].ID, "opponent": "tactical"}, &obs); code != 200 {
		t.Fatalf("puzzle env: %d", code)
	}
	if obs.AgentSeat != list[0].ToMove || obs.Done || !obs.YourTurn {
		t.Errorf("puzzle observation = %+v", obs)
	}
	var full puzzleResp
	doJSON(t, "GET", baseURL+"/api/puzzles/"+list[0].ID+"?reveal=1", nil, &full)
	var solved envObs
	doJSON(t, "POST", baseURL+"/api/envs/"+obs.EnvID+"/step", map[string]any{"move": full.FirstMoves[0]}, &solved)
	if !solved.Done || solved.Reward != 1 {
		t.Errorf("solving move: %+v", solved)
	}
}

// The bundled Python client (standard library only) must keep working
// against the server; skipped where python3 is not installed.
func TestPythonGymClientPlaysEpisodes(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	baseURL := newRESTTestServer(t)
	cmd := exec.Command(py, "../../scripts/bingle_env.py", baseURL, "tic-tac-toe", "--episodes", "3", "--opponent", "random")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("client failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{"tic-tac-toe: planes [5, 3, 3], 9 actions", "episode 2:", "wins "} {
		if !strings.Contains(text, want) {
			t.Errorf("client output lacks %q:\n%s", want, text)
		}
	}
}
