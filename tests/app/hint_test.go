package app_test

import (
	"strings"
	"testing"
	"time"
)

func TestHintOnlyAgainstBotsOrAfterTheGame(t *testing.T) {
	baseURL := newRESTTestServer(t)

	// Against a human: refused.
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	var out map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/hint", map[string]any{"player_name": "Alice"}, &out); code != 409 {
		t.Errorf("hint against a human: %d %v, want 409", code, out)
	}

	// Against a bot: a legal move comes back.
	var created struct {
		RoomCode string `json:"room_code"`
	}
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice", "opponent": "tactical"}, &created)
	var hint struct {
		Notation string `json:"notation"`
		Steps    []struct{ Kind, Value string }
		Bot      string   `json:"bot"`
		WinRate  *float64 `json:"win_rate"`
		Alts     []struct {
			Notation string  `json:"notation"`
			WinRate  float64 `json:"win_rate"`
		} `json:"alternatives"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/hint", map[string]any{"player_name": "Alice", "bot": "tactical"}, &hint); code != 200 {
		t.Fatalf("hint vs bot: %d", code)
	}
	if hint.Notation == "" || len(hint.Steps) == 0 || hint.Bot != "tactical" {
		t.Fatalf("hint = %+v", hint)
	}
	if hint.WinRate != nil {
		t.Errorf("the tactical bot does not estimate, yet win_rate = %v", *hint.WinRate)
	}
	// MCTS reports an estimate and alternatives.
	var mcts = hint
	mcts.WinRate = nil
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/hint", map[string]any{"player_name": "Alice", "bot": "mcts"}, &mcts); code != 200 {
		t.Fatalf("mcts hint: %d", code)
	}
	if mcts.WinRate == nil || *mcts.WinRate < 0 || *mcts.WinRate > 1 || len(mcts.Alts) == 0 || mcts.Alts[0].Notation != mcts.Notation {
		t.Errorf("mcts hint = %+v", mcts)
	}
	// Without a named bot, Tak (where piece counts matter) is advised by
	// minimax, which reports no win percentage; tic-tac-toe by MCTS.
	var dflt struct {
		Bot     string   `json:"bot"`
		WinRate *float64 `json:"win_rate"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/hint", map[string]any{"player_name": "Alice"}, &dflt); code != 200 || dflt.Bot != "minimax" || dflt.WinRate != nil {
		t.Errorf("default hint for Tak: %d %+v, want minimax without a win rate", code, dflt)
	}
	// The suggestion is playable.
	if code, out := moveNotation(t, baseURL, created.RoomCode, "Alice", hint.Notation); code != 200 {
		t.Errorf("hinted move %q rejected: %d %v", hint.Notation, code, out)
	}

	// A variation: hint after a what-if move.
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/hint",
		map[string]any{"player_name": "Alice", "bot": "random", "moves": []string{"e5"}}, &hint); code != 200 && code != 400 {
		t.Errorf("hint after a variation: %d", code)
	}

	// Strangers cannot ask.
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/hint", map[string]any{"player_name": "Mallory"}, &out); code != 403 {
		t.Errorf("hint for a non-player: %d, want 403", code)
	}

	// After the game (resignation) hints work for earlier positions.
	moveNotation(t, baseURL, room, "Alice", "a5")
	doJSON(t, "POST", baseURL+"/api/games/"+room+"/resign", map[string]any{"player_name": "Bob"}, nil)
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/hint", map[string]any{"player_name": "Alice", "at_seq": 0, "bot": "random"}, &hint); code != 200 {
		t.Errorf("hint for an earlier position of a finished game: %d", code)
	}
}

func TestWebGame_HintButtonSuggestsAndPlaysAMove(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice", "opponent": "tactical"}, &created)

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "tic-tac-toe", "Alice")
	waitJS(t, ctx, "my turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)

	clickSel(t, ctx, "#hint")
	waitJS(t, ctx, "hint banner", `document.getElementById("hint-move") && document.getElementById("hint-move").textContent.length > 0`, 15*time.Second)
	suggested := evalString(t, ctx, `document.getElementById("hint-move").textContent`)
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .hint-ring').length`); n != 1 {
		t.Errorf("hint rings = %d, want 1 on the suggested square", n)
	}

	clickSel(t, ctx, "#hint-use")
	waitJS(t, ctx, "submit button", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.indexOf("Submit") === 0`, 5*time.Second)
	got := evalString(t, ctx, `document.getElementById("submit-move").textContent.trim()`)
	if !strings.HasSuffix(got, suggested) {
		t.Errorf("submit button %q does not match the hint %q", got, suggested)
	}
	clickSel(t, ctx, "#submit-move")
	waitJS(t, ctx, "move played", `document.querySelector('#move-list button.mv[data-seq="1"]')`, 10*time.Second)
}

func TestWebGame_NoHintButtonAgainstAHuman(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tic-tac-toe", "Alice")
	waitJS(t, ctx, "board", `document.getElementById("show-rules")`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#hint').length`); n != 0 {
		t.Errorf("hint button shown against a human opponent")
	}
}
