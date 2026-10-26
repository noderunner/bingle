package app_test

import (
	"strings"
	"testing"
)

// The setup options of Cthulhu Wars over REST: a derived table has its own id,
// seats in the colours of the chosen cults, a lobby that waits for its host,
// bots named after the cults, and is refused when the choice does not fit.
func TestCthulhuWarsOptions_DerivedTableOverREST(t *testing.T) {
	baseURL := newRESTTestServer(t)

	// The options are described by the game, and derived ids stay out of the list.
	var def struct {
		Options []struct {
			ID      string `json:"id"`
			Kind    string `json:"kind"`
			Choices []struct {
				ID    string `json:"id"`
				Color string `json:"color"`
			} `json:"choices"`
		} `json:"options"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/game-defs/cthulhu-wars-5p", nil, &def); code != 200 {
		t.Fatalf("game def: %d", code)
	}
	kinds := map[string]string{}
	for _, o := range def.Options {
		kinds[o.ID] = o.Kind
	}
	if kinds["factions"] != "seat-choice" || kinds["high-priests"] != "bool" {
		t.Errorf("options = %v", kinds)
	}
	var list []struct {
		ID string `json:"id"`
	}
	doJSON(t, "GET", baseURL+"/api/game-defs", nil, &list)
	for _, d := range list {
		if strings.Contains(d.ID, "~") {
			t.Errorf("a derived id is listed: %q", d.ID)
		}
	}

	// Create a waiting table: Sleeper, Windwalker, Great Cthulhu, Yellow Sign, Black Goat.
	var created struct {
		RoomCode string `json:"room_code"`
		Seat     int    `json:"seat"`
		Color    string `json:"color"`
		Status   string `json:"status"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars-5p", "player_name": "Alice", "lobby": true, "seat": 2,
		"options": map[string]any{"factions": []string{"sl", "ww", "gc", "ys", "bg"}, "high-priests": true},
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if created.Seat != 2 || created.Color != "white" || created.Status != "waiting" {
		t.Errorf("created = %+v", created)
	}
	var info struct {
		GameDefID string `json:"game_def_id"`
		Status    string `json:"status"`
		Seats     []struct {
			Seat  int    `json:"seat"`
			Color string `json:"color"`
			Label string `json:"label"`
			Name  string `json:"name"`
			Bot   string `json:"bot"`
		} `json:"seats"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &info); code != 200 {
		t.Fatalf("get: %d", code)
	}
	if info.GameDefID != "cthulhu-wars-5p~sl-ww-gc-ys-bg-hp" {
		t.Errorf("game = %q", info.GameDefID)
	}
	wantLabels := []string{"Sleeper", "Windwalker", "Great Cthulhu", "Yellow Sign", "Black Goat"}
	wantColors := []string{"orange", "white", "green", "yellow", "red"}
	if len(info.Seats) != 5 {
		t.Fatalf("%d seats", len(info.Seats))
	}
	for i, s := range info.Seats {
		if s.Label != wantLabels[i] || s.Color != wantColors[i] {
			t.Errorf("seat %d = %s/%s, want %s/%s", i+1, s.Label, s.Color, wantLabels[i], wantColors[i])
		}
	}
	// Fill the open seats with bots: each is named after its cult.
	for _, seat := range []int{1, 3, 4, 5} {
		var added struct {
			Name string `json:"name"`
		}
		if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/bots", map[string]any{"player_name": "Alice", "bot": "random", "seat": seat}, &added); code != 200 {
			t.Fatalf("add bot seat %d: %d", seat, code)
		}
		if !strings.HasPrefix(added.Name, wantLabels[seat-1]) {
			t.Errorf("bot of seat %d is named %q", seat, added.Name)
		}
	}
	var started struct {
		Status string `json:"status"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/start", map[string]any{"player_name": "Alice"}, &started); code != 200 || started.Status != "active" {
		t.Fatalf("start: %d %+v", code, started)
	}
	// The Windwalker (the human, seat 2) begins: two sigil areas to choose from.
	var legal struct {
		Seat  int `json:"seat"`
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"moves"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/moves/legal", nil, &legal); code != 200 {
		t.Fatalf("legal: %d", code)
	}
	var got []string
	for _, m := range legal.Moves {
		got = append(got, m.Notation)
	}
	if legal.Seat != 2 || len(got) != 2 || got[0] != "start@antarctica" && got[0] != "start@arctic-ocean" {
		t.Errorf("legal moves of seat %d: %v", legal.Seat, got)
	}

	// Choices that do not fit are refused.
	for name, opts := range map[string]map[string]any{
		"twice":   {"factions": []string{"gc", "gc", "bg", "ys", "ww"}},
		"count":   {"factions": []string{"gc", "cc", "bg"}},
		"unknown": {"factions": []string{"gc", "cc", "bg", "ys", "xx"}},
		"option":  {"colour": "red"},
	} {
		code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars-5p", "player_name": "Bob", "options": opts}, nil)
		if code != 400 {
			t.Errorf("%s: status %d, want 400", name, code)
		}
	}
	// A game without options refuses them too.
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Bob", "options": map[string]any{"factions": []string{"a"}}}, nil); code != 400 {
		t.Errorf("options for a game without any: status %d, want 400", code)
	}
}

// A Windwalker played by a bot begins by itself when the game is created.
func TestCthulhuWarsOptions_BotWindwalkerBeginsByItself(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
		Seat     int    `json:"seat"`
		Status   string `json:"status"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars-5p", "player_name": "Alice", "seat": 3, "opponent": "tactical",
		"options": map[string]any{"factions": []string{"ww", "sl", "gc", "cc", "bg"}},
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if created.Status != "active" || created.Seat != 3 {
		t.Fatalf("created = %+v", created)
	}
	// The Windwalker (seat 1) played the first move: it chose a sigil area.
	var moves []struct {
		Notation string `json:"notation"`
		Seat     int    `json:"seat"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/moves", nil, &moves); code != 200 {
		t.Fatalf("moves: %d", code)
	}
	if len(moves) == 0 || moves[0].Seat != 1 || !strings.HasPrefix(moves[0].Notation, "start@") {
		t.Errorf("the Windwalker bot did not begin: %+v", moves)
	}
}
