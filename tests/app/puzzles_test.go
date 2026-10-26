package app_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

type puzzleResp struct {
	ID         string          `json:"id"`
	GameDefID  string          `json:"game_def_id"`
	Title      string          `json:"title"`
	ToMove     int             `json:"to_move"`
	WinIn      int             `json:"win_in"`
	TPS        string          `json:"tps"`
	State      json.RawMessage `json:"state"`
	FirstMoves []string        `json:"first_moves"`
	Solution   []string        `json:"solution"`
}

func TestPuzzlesRESTListsAndHidesSolutions(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var list []puzzleResp
	if code := doJSON(t, "GET", baseURL+"/api/puzzles", nil, &list); code != 200 {
		t.Fatalf("status %d", code)
	}
	if len(list) == 0 {
		t.Skip("no puzzles shipped")
	}
	for _, p := range list {
		if len(p.Solution) != 0 || len(p.FirstMoves) != 0 {
			t.Fatalf("%s: list leaked the solution", p.ID)
		}
		if len(p.State) == 0 || p.ToMove == 0 || p.WinIn == 0 {
			t.Fatalf("%s: incomplete %+v", p.ID, p)
		}
	}

	var filtered []puzzleResp
	doJSON(t, "GET", baseURL+"/api/puzzles?game_def_id="+list[0].GameDefID+"&max_win_in=1", nil, &filtered)
	for _, p := range filtered {
		if p.GameDefID != list[0].GameDefID || p.WinIn != 1 {
			t.Errorf("filter let %+v through", p)
		}
	}

	var hidden, revealed puzzleResp
	doJSON(t, "GET", baseURL+"/api/puzzles/"+list[0].ID, nil, &hidden)
	doJSON(t, "GET", baseURL+"/api/puzzles/"+list[0].ID+"?reveal=1", nil, &revealed)
	if len(hidden.Solution) != 0 || len(revealed.Solution) == 0 || len(revealed.FirstMoves) == 0 {
		t.Errorf("reveal handling: hidden=%v revealed=%v/%v", hidden.Solution, revealed.Solution, revealed.FirstMoves)
	}
	if code := doJSON(t, "GET", baseURL+"/api/puzzles/nope", nil, nil); code != 404 {
		t.Errorf("unknown puzzle: %d, want 404", code)
	}
}

// A puzzle is solved by playing a stored winning move: start a room from the
// puzzle exactly as the lobby does and win.
func TestPuzzleCanBeSolvedInARoom(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var list []puzzleResp
	doJSON(t, "GET", baseURL+"/api/puzzles?max_win_in=1", nil, &list)
	if len(list) == 0 {
		t.Skip("no win-in-1 puzzles shipped")
	}
	p := list[0]
	var full puzzleResp
	doJSON(t, "GET", baseURL+"/api/puzzles/"+p.ID+"?reveal=1", nil, &full)

	var created struct {
		RoomCode string `json:"room_code"`
	}
	code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{
		"game_def_id": p.GameDefID, "player_name": "Alice", "opponent": "tactical",
		"seat": p.ToMove, "start_state": p.State, "start_seat": p.ToMove,
	}, &created)
	if code != 200 {
		t.Fatalf("create from puzzle: %d", code)
	}
	var out map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/moves",
		map[string]any{"player_name": "Alice", "notation": full.FirstMoves[0]}, &out); code != 200 {
		t.Fatalf("solving move %q: %d %v", full.FirstMoves[0], code, out)
	}
	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/state", nil, &st)
	if st.Status != "finished" || st.WinnerSeat == nil || *st.WinnerSeat != p.ToMove {
		t.Errorf("after the solving move: status %q winner %v, want seat %d", st.Status, st.WinnerSeat, p.ToMove)
	}
}

func TestMCP_PuzzlesAsEnvironments(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)

	res := callTool(t, c, "list_puzzles", map[string]any{"max_win_in": 1, "reveal": true})
	if res.IsError {
		t.Fatalf("list_puzzles: %s", toolText(res))
	}
	var listed struct {
		Puzzles []puzzleResp `json:"puzzles"`
	}
	decodeToolResult(t, res, &listed)
	if len(listed.Puzzles) == 0 {
		t.Skip("no puzzles shipped")
	}
	p := listed.Puzzles[0]
	if len(p.FirstMoves) == 0 {
		t.Fatal("reveal=true should include first_moves")
	}

	res = callTool(t, c, "env_reset", map[string]any{"puzzle_id": p.ID, "opponent": "tactical"})
	if res.IsError {
		t.Fatalf("env_reset(puzzle): %s", toolText(res))
	}
	var obs envObs
	decodeToolResult(t, res, &obs)
	if !obs.YourTurn || obs.AgentSeat != p.ToMove || obs.Done {
		t.Fatalf("puzzle observation = %+v", obs)
	}
	isFirst := false
	for _, m := range obs.LegalMoves {
		if m == p.FirstMoves[0] {
			isFirst = true
		}
	}
	if !isFirst {
		t.Fatalf("solution move %q not among legal moves %v", p.FirstMoves[0], obs.LegalMoves)
	}
	res = callTool(t, c, "env_step", map[string]any{"env_id": obs.EnvID, "move": p.FirstMoves[0]})
	decodeToolResult(t, res, &obs)
	if !obs.Done || obs.Result != "win" || obs.Reward != 1 {
		t.Errorf("after the solution: %+v", obs)
	}

	// Mismatched or unknown puzzles are tool errors.
	res = callTool(t, c, "env_reset", map[string]any{"puzzle_id": "nope"})
	if !res.IsError {
		t.Error("unknown puzzle should be an error")
	}
	other := "tic-tac-toe"
	if p.GameDefID == other {
		other = "gomoku"
	}
	res = callTool(t, c, "env_reset", map[string]any{"puzzle_id": p.ID, "game_def_id": other})
	if !res.IsError || !strings.Contains(toolText(res), "is for game") {
		t.Errorf("mismatched game: %v %s", res.IsError, toolText(res))
	}
}

func TestWebLobby_PuzzleCardStartsAGameFromThePosition(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var list []puzzleResp
	doJSON(t, "GET", baseURL+"/api/puzzles", nil, &list)
	if len(list) == 0 {
		t.Skip("no puzzles shipped")
	}
	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/#puzzles"),
		chromedp.WaitVisible("#panel-puzzles", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "puzzle cards", `document.querySelectorAll('#puzzle-list .puzzle-card').length > 0`, 10*time.Second)
	waitJS(t, ctx, "puzzle board previews", `document.querySelector('#puzzle-list .puzzle-preview svg')`, 10*time.Second)
	// Needs a name.
	if err := chromedp.Run(ctx, chromedp.SetValue("#puzzle_player_name", "Alice", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#puzzle-list .puzzle-card button.primary")
	waitJS(t, ctx, "navigated to a game", `location.pathname.indexOf("/game/") === 0`, 15*time.Second)
	waitJS(t, ctx, "board", `document.getElementById("board-svg")`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .piece').length`); n == 0 {
		t.Error("the puzzle's position should be on the board")
	}
}

func TestWebGame_FailedPuzzleOffersTryAgainFromTheSamePosition(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var list []puzzleResp
	doJSON(t, "GET", baseURL+"/api/puzzles?max_win_in=1", nil, &list)
	if len(list) == 0 {
		t.Skip("no puzzles shipped")
	}
	p := list[0]
	var created struct {
		RoomCode string `json:"room_code"`
	}
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{
		"game_def_id": p.GameDefID, "player_name": "Alice", "opponent": "tactical",
		"seat": p.ToMove, "start_state": p.State, "start_seat": p.ToMove,
	}, &created)
	// Give up: resign.
	doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/resign", map[string]any{"player_name": "Alice"}, nil)

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, p.GameDefID, "Alice")
	waitJS(t, ctx, "try again", `document.getElementById("rematch") && document.getElementById("rematch").textContent.trim() === "Try again"`, 10*time.Second)
	clickSel(t, ctx, "#rematch")
	waitJS(t, ctx, "new room", `location.pathname.indexOf("/game/") === 0 && location.pathname !== "/game/`+created.RoomCode+`"`, 15*time.Second)
	var newRoom string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`location.pathname.split("/").pop()`, &newRoom)); err != nil {
		t.Fatal(err)
	}
	var a, b restState
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/state?at_seq=0", nil, &a)
	doJSON(t, "GET", baseURL+"/api/games/"+newRoom+"/state?at_seq=0", nil, &b)
	if len(a.State.Cells) != len(b.State.Cells) || a.State.Ply != b.State.Ply || len(b.State.Cells) == 0 {
		t.Errorf("retry start differs: %d/%d cells, ply %d/%d", len(a.State.Cells), len(b.State.Cells), a.State.Ply, b.State.Ply)
	}
}
