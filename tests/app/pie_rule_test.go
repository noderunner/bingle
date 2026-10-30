package app_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// pieOptionOf reports the pie-rule option a game offers: "" when it offers
// none, "on" or "off" for the state its check box starts in.
func pieOptionOf(t *testing.T, baseURL, def string) string {
	t.Helper()
	var info struct {
		Options []struct {
			ID      string `json:"id"`
			Kind    string `json:"kind"`
			Default any    `json:"default"`
		} `json:"options"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/game-defs/"+def, nil, &info); code != 200 {
		t.Fatalf("game def %s: %d", def, code)
	}
	for _, o := range info.Options {
		if o.ID != "pie-rule" {
			continue
		}
		if o.Kind != "bool" {
			t.Errorf("%s: the pie-rule option is a %q, want a bool", def, o.Kind)
		}
		if o.Default == true {
			return "on"
		}
		return "off"
	}
	return ""
}

// swapIsLegal reports whether the player to move may swap sides.
func swapIsLegal(t *testing.T, baseURL, room string) bool {
	t.Helper()
	var legal struct {
		Moves []struct {
			Notation string `json:"notation"`
			Action   string `json:"action"`
		} `json:"moves"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves/legal", nil, &legal); code != 200 {
		t.Fatalf("legal moves: %d", code)
	}
	for _, m := range legal.Moves {
		if m.Action == "swap" || m.Notation == "swap" {
			return true
		}
	}
	return false
}

// Every two-player game that can be played with the pie rule says so, with the
// box starting where the game's own practice is: on for Hex and Gomoku, off for
// the games that only offer it. A game it cannot work for offers nothing.
func TestPieRule_GamesSayWhetherTheyOfferIt(t *testing.T) {
	baseURL := newRESTTestServer(t)
	for def, want := range map[string]string{
		"hex-11":       "on",
		"hex-7":        "on", // a variant inherits it
		"gomoku":       "on",
		"yavalath":     "off",
		"havannah-8":   "off",
		"pente":        "off",
		"connect-four": "off",
		"amazons":      "off",
		"ataxx":        "off",
		"reversi":      "off",
		"go-9x9":       "off",
		"blooms":       "off", // a ruleset offers it too
		"blooms-4":     "off", // a variant inherits it
		"tic-tac-toe":  "",
		"draughts":     "",
		"breakthrough": "",
		"brandubh":     "",
		"tak-5x5":      "",
		"cthulhu-wars": "",
		"courtfall":    "",
	} {
		if got := pieOptionOf(t, baseURL, def); got != want {
			t.Errorf("%s offers the pie rule %q, want %q", def, got, want)
		}
	}
}

// A game of Hex is played with the pie rule, so the second player may swap
// sides on their first turn; a table that turns it off plays a game of its own
// (hex-11~nopie) where swapping is not on offer.
func TestPieRule_HexPlaysWithItUnlessTheTableTurnsItOff(t *testing.T) {
	baseURL := newRESTTestServer(t)
	open := func(options map[string]any) (room, def string) {
		var created struct {
			RoomCode string `json:"room_code"`
		}
		body := map[string]any{"game_def_id": "hex-11", "player_name": "Ana", "also_seats": []int{2}}
		if options != nil {
			body["options"] = options
		}
		if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
			t.Fatalf("create with %v: %d", options, code)
		}
		var game struct {
			GameDefID string `json:"game_def_id"`
		}
		doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &game)
		if code, out := moveNotation(t, baseURL, created.RoomCode, "Ana", "f6"); code != 200 {
			t.Fatalf("first move: %d %v", code, out)
		}
		return created.RoomCode, game.GameDefID
	}

	// Its own practice: the plain id, and the swap is there for the reply.
	room, def := open(nil)
	if def != "hex-11" {
		t.Errorf("a game of Hex is %q, want hex-11", def)
	}
	if !swapIsLegal(t, baseURL, room) {
		t.Error("Hex does not offer the pie rule after the first move")
	}
	// Choosing the rule it plays with anyway is still the plain game.
	if _, def := open(map[string]any{"pie-rule": true}); def != "hex-11" {
		t.Errorf("Hex with the pie rule ticked is %q, want the plain hex-11", def)
	}
	// Turned off: a game of its own, with no swap on offer.
	room, def = open(map[string]any{"pie-rule": false})
	if def != "hex-11~nopie" {
		t.Fatalf("Hex without the pie rule is %q, want hex-11~nopie", def)
	}
	if swapIsLegal(t, baseURL, room) {
		t.Error("the pie rule was turned off but swapping is still on offer")
	}
	// The game plays on as Hex: the reply is an ordinary move.
	var seats struct {
		Seats []struct {
			Seat int    `json:"seat"`
			Name string `json:"name"`
		} `json:"seats"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &seats)
	if code, out := moveNotation(t, baseURL, room, seats.Seats[1].Name, "g6"); code != 200 {
		t.Fatalf("the reply of the second player: %d %v", code, out)
	}
}

// A game that only offers the pie rule is played without it, and a table that
// asks for it gets a game of its own in which swapping sides really works: the
// stone on the board changes hands and the first player is to move again.
func TestPieRule_YavalathTakesItWhenTheTableAsks(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "yavalath", "player_name": "Ana", "also_seats": []int{2}, "options": map[string]any{"pie-rule": true}}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	var game struct {
		GameDefID string `json:"game_def_id"`
		Seats     []struct {
			Seat  int    `json:"seat"`
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"seats"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if game.GameDefID != "yavalath~pie" {
		t.Fatalf("the game is %q, want yavalath~pie", game.GameDefID)
	}
	first := game.Seats[0].Name
	second := game.Seats[1].Name
	// A first placement, then the swap.
	var legal struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"moves"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves/legal", nil, &legal)
	if code, out := moveNotation(t, baseURL, room, first, legal.Moves[0].Notation); code != 200 {
		t.Fatalf("first move: %d %v", code, out)
	}
	if !swapIsLegal(t, baseURL, room) {
		t.Fatal("the table asked for the pie rule but swapping is not on offer")
	}
	if code, out := moveNotation(t, baseURL, room, second, "swap"); code != 200 {
		t.Fatalf("swap: %d %v", code, out)
	}
	var after struct {
		TurnSeat   *int            `json:"turn_seat"`
		BoardState json.RawMessage `json:"board_state"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &after)
	if after.TurnSeat == nil || *after.TurnSeat != 1 {
		t.Errorf("after the swap %v is to move, want the first player", after.TurnSeat)
	}
	// The stone that was placed is now the other colour, where it stood.
	stones := decodeOccupants(t, after.BoardState)
	if len(stones) != 1 {
		t.Fatalf("the board holds %d stones after the swap, want one", len(stones))
	}
	for pos, occ := range stones {
		if occ.Color != game.Seats[1].Color {
			t.Errorf("the stone on %s is %s, want the second player's %s", pos, occ.Color, game.Seats[1].Color)
		}
	}
	// Without the option it is the plain game, and there is no swap.
	var plain struct {
		RoomCode string `json:"room_code"`
	}
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "yavalath", "player_name": "Bo", "also_seats": []int{2}}, &plain)
	var plainGame struct {
		GameDefID string `json:"game_def_id"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+plain.RoomCode, nil, &plainGame)
	if plainGame.GameDefID != "yavalath" {
		t.Errorf("Yavalath without the option is %q", plainGame.GameDefID)
	}
	doJSON(t, "GET", baseURL+"/api/games/"+plain.RoomCode+"/moves/legal", nil, &legal)
	for _, m := range legal.Moves {
		if strings.Contains(m.Notation, "swap") {
			t.Errorf("the plain game offers %q", m.Notation)
		}
	}
}

// Blooms is a Go ruleset, so the swap is the ruleset's own move rather than
// the engine's generic action — but from outside it is the same pie rule: a
// table that asks plays blooms~pie, the second player swaps, the opening
// stone changes hands (red becomes blue) and the first player replies.
func TestPieRule_BloomsSwapsWithinItsOwnRuleset(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "blooms", "player_name": "Ana", "also_seats": []int{2}, "options": map[string]any{"pie-rule": true}}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	var game struct {
		GameDefID string `json:"game_def_id"`
		Seats     []struct {
			Seat  int    `json:"seat"`
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"seats"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if game.GameDefID != "blooms~pie" {
		t.Fatalf("the game is %q, want blooms~pie", game.GameDefID)
	}
	first, second := game.Seats[0].Name, game.Seats[1].Name
	// The opening stone, then the swap is on offer.
	if code, out := moveNotation(t, baseURL, room, first, "red@d3"); code != 200 {
		t.Fatalf("first move: %d %v", code, out)
	}
	if !swapIsLegal(t, baseURL, room) {
		t.Fatal("the table asked for the pie rule but swapping is not on offer")
	}
	if code, out := moveNotation(t, baseURL, room, second, "swap"); code != 200 {
		t.Fatalf("swap: %d %v", code, out)
	}
	var after struct {
		TurnSeat   *int            `json:"turn_seat"`
		BoardState json.RawMessage `json:"board_state"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &after)
	if after.TurnSeat == nil || *after.TurnSeat != 1 {
		t.Errorf("after the swap %v is to move, want the first player", after.TurnSeat)
	}
	stones := decodeOccupants(t, after.BoardState)
	if len(stones) != 1 {
		t.Fatalf("the board holds %d stones after the swap, want one", len(stones))
	}
	for pos, occ := range stones {
		if occ.Color != "blue" || occ.Piece != "blue" {
			t.Errorf("the stone on %s is %s/%s, want the second player's blue", pos, occ.Color, occ.Piece)
		}
	}
	if swapIsLegal(t, baseURL, room) {
		t.Error("the swap is still on offer after it was taken")
	}
}

// The setup of a two-player game offers the pie rule where the game can be
// played with it: Hex starts with the box ticked and a game made that way is
// the plain game, while unticking it opens the game without the rule. Yavalath
// starts unticked, and a game with no pie rule shows no box at all.
func TestWebPieRule_TheSetupOffersIt(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	open := func(game string) {
		t.Helper()
		if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game="+game)); err != nil {
			t.Fatal(err)
		}
		waitJS(t, ctx, "the setup of "+game, `document.getElementById("create-game-form") !== null && !document.getElementById("game-setup").hidden`, 20*time.Second)
	}
	box := `document.getElementById("option-pie-rule")`

	open("hex-11")
	waitJS(t, ctx, "the pie-rule box", box+` !== null && !document.getElementById("setup-options").hidden`, 10*time.Second)
	if !evalBool(t, ctx, box+`.checked`) {
		t.Error("Hex starts with the pie rule unticked")
	}
	if got := evalString(t, ctx, `document.querySelector('label[for="option-pie-rule"], #setup-options .option-flag').textContent`); !strings.Contains(got, "swap sides") {
		t.Errorf("the box reads %q", got)
	}

	// A game that has no pie rule offers no box.
	open("tic-tac-toe")
	waitJS(t, ctx, "no pie-rule box", box+` === null && document.getElementById("setup-options").hidden`, 10*time.Second)

	// Yavalath offers it, off.
	open("yavalath")
	waitJS(t, ctx, "the pie-rule box", box+` !== null`, 10*time.Second)
	if evalBool(t, ctx, box+`.checked`) {
		t.Error("Yavalath starts with the pie rule ticked")
	}

	// Untick it for Hex and make the game: it is the game without the rule.
	open("hex-11")
	waitJS(t, ctx, "the pie-rule box", box+` !== null`, 10*time.Second)
	evalString(t, ctx, box+`.click(); "ok"`)
	if evalBool(t, ctx, box+`.checked`) {
		t.Fatal("the box is still ticked after a click")
	}
	evalString(t, ctx, `document.getElementById("create_player_name").value = "Ana"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	room := loc[strings.LastIndex(loc, "/")+1:]
	var info struct {
		GameDefID string `json:"game_def_id"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &info)
	if info.GameDefID != "hex-11~nopie" {
		t.Errorf("the game is %q, want hex-11~nopie", info.GameDefID)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
