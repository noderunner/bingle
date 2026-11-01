// Tests of retired games (ADR 0076): a server that no longer offers a game
// ends the unfinished games of it when it starts, keeps every game of it
// readable as a record (players, result, moves), refuses anything more, and
// shows the record on the game page.

package app_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
)

// retiredGames plays games of the two retired games on a server that still
// has them (as the tests' fixtures), then reopens the same database on a
// server that does not. It returns the new server's URL and the rooms: one
// Tic-Tac-Toe game Al won, one Connect Four game still being played and one
// still waiting for its second player.
func retiredGames(t *testing.T) (base, won, playing, waiting string) {
	t.Helper()
	cfg := newTestConfig(t)
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	registerFixtures(t, a)
	srv := httptest.NewServer(a.Router())

	al, bo := newGuestKey(t, srv.URL), newGuestKey(t, srv.URL)
	create := func(game string) string {
		var created struct {
			RoomCode string `json:"room_code"`
		}
		if code := doJSONAs(t, al, "POST", srv.URL+"/api/games", map[string]any{"game_def_id": game, "player_name": "Al"}, &created); code != 200 {
			t.Fatalf("creating %s: %d", game, code)
		}
		return created.RoomCode
	}
	move := func(key, name, room, notation string) {
		if code := doJSONAs(t, key, "POST", srv.URL+"/api/games/"+room+"/moves", map[string]any{"player_name": name, "notation": notation}, nil); code != 200 {
			t.Fatalf("%s playing %s in %s: %d", name, notation, room, code)
		}
	}
	join := func(room string) {
		if code := doJSONAs(t, bo, "POST", srv.URL+"/api/games/"+room+"/join", map[string]any{"player_name": "Bo"}, nil); code != 200 {
			t.Fatalf("Bo joining %s: %d", room, code)
		}
	}

	won = create("tic-tac-toe")
	join(won)
	for i, sq := range []string{"a1", "b1", "a2", "b2", "a3"} {
		if i%2 == 0 {
			move(al, "Al", won, sq)
		} else {
			move(bo, "Bo", won, sq)
		}
	}
	playing = create("connect-four")
	join(playing)
	move(al, "Al", playing, "d1")
	waiting = create("tic-tac-toe")

	srv.Close()
	_ = a.Close()

	// The same database, on a server without the two games.
	b, err := app.New(config.Config{DBPath: cfg.DBPath, CapsFile: cfg.CapsFile})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	srv2 := httptest.NewServer(b.Router())
	t.Cleanup(srv2.Close)
	return srv2.URL, won, playing, waiting
}

type retiredGame struct {
	GameDefID  string `json:"game_def_id"`
	Status     string `json:"status"`
	Retired    bool   `json:"retired"`
	GameName   string `json:"game_name"`
	MoveSeq    int    `json:"move_seq"`
	WinnerSeat *int   `json:"winner_seat"`
	EndReason  string `json:"end_reason"`
	Seats      []struct {
		Seat int    `json:"seat"`
		Name string `json:"name"`
	} `json:"seats"`
}

func TestRetiredGames_KeptAsARecord(t *testing.T) {
	base, won, playing, waiting := retiredGames(t)

	var g retiredGame
	if code := doJSON(t, "GET", base+"/api/games/"+won, nil, &g); code != 200 {
		t.Fatalf("GET the finished game: %d", code)
	}
	if !g.Retired || g.GameName != "Tic-Tac-Toe" || g.GameDefID != "tic-tac-toe" {
		t.Fatalf("not marked retired: %+v", g)
	}
	if g.Status != "finished" || g.WinnerSeat == nil || *g.WinnerSeat != 1 || g.MoveSeq != 5 || g.EndReason == "retired" {
		t.Fatalf("a finished game keeps its result: %+v", g)
	}
	if len(g.Seats) != 2 || g.Seats[0].Name != "Al" || g.Seats[1].Name != "Bo" {
		t.Fatalf("the players: %+v", g.Seats)
	}

	var moves []struct {
		Seat     int    `json:"seat"`
		Notation string `json:"notation"`
	}
	if code := doJSON(t, "GET", base+"/api/games/"+won+"/moves", nil, &moves); code != 200 || len(moves) != 5 {
		t.Fatalf("the moves: %d %+v", code, moves)
	}
	if moves[0].Notation != "a1" || moves[4].Notation != "a3" || moves[1].Seat != 2 {
		t.Fatalf("the moves keep their notation: %+v", moves)
	}

	// The unfinished games ended when the server started, with no result.
	for _, room := range []string{playing, waiting} {
		var u retiredGame
		if code := doJSON(t, "GET", base+"/api/games/"+room, nil, &u); code != 200 {
			t.Fatalf("GET %s: %d", room, code)
		}
		if !u.Retired || u.Status != "finished" || u.WinnerSeat != nil || u.EndReason != "retired" {
			t.Fatalf("%s should have ended with no result: %+v", room, u)
		}
	}
	var c4 retiredGame
	doJSON(t, "GET", base+"/api/games/"+playing, nil, &c4)
	if c4.GameName != "Connect Four" || c4.MoveSeq != 1 {
		t.Fatalf("the Connect Four game: %+v", c4)
	}
}

func TestRetiredGames_NothingMoreCanBeDone(t *testing.T) {
	base, won, playing, _ := retiredGames(t)
	key := newGuestKey(t, base)
	var e errorResponse
	if code := doJSONAs(t, key, "POST", base+"/api/games/"+playing+"/moves", map[string]any{"player_name": "Al", "notation": "d2"}, &e); code != 410 {
		t.Fatalf("a move in a retired game: %d %+v, want 410", code, e)
	}
	if !strings.Contains(e.Error, "Connect Four is no longer played here") {
		t.Fatalf("the refusal should say why: %q", e.Error)
	}
	if code := doJSON(t, "GET", base+"/api/games/"+won+"/state", nil, nil); code != 410 {
		t.Errorf("GET /state of a retired game: %d, want 410", code)
	}
	// Legal moves are refused already because the game is over (409).
	if code := doJSON(t, "GET", base+"/api/games/"+won+"/moves/legal", nil, nil); code < 400 {
		t.Errorf("GET /moves/legal of a retired game: %d", code)
	}
	// Nobody can start one either: the game is not offered.
	if code := doJSONAs(t, key, "POST", base+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Cy"}, nil); code != 404 {
		t.Fatalf("creating a retired game: %d, want 404", code)
	}
	var defs []gameDefSummary
	doJSON(t, "GET", base+"/api/game-defs", nil, &defs)
	for _, d := range defs {
		if d.ID == "tic-tac-toe" || d.ID == "connect-four" {
			t.Fatalf("%s is still offered", d.ID)
		}
	}
}

func TestRetiredGames_ThePageShowsTheRecord(t *testing.T) {
	base, won, playing, _ := retiredGames(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/game/"+won)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the record", `!!document.querySelector("#retired-game h2")`, 15*time.Second)
	if h := evalString(t, ctx, `document.querySelector("#retired-game h2").textContent`); h != "Tic-Tac-Toe is no longer played here" {
		t.Fatalf("heading: %q", h)
	}
	if text := evalString(t, ctx, `document.getElementById("retired-game").innerText`); !strings.Contains(text, "Played by Al and Bo. Al won.") {
		t.Fatalf("the players and the result: %q", text)
	}
	if moves := evalString(t, ctx, `[...document.querySelectorAll("#retired-game .retired-moves li")].map(li => li.textContent).join("|")`); moves != "Al: a1|Bo: b1|Al: a2|Bo: b2|Al: a3" {
		t.Fatalf("the moves: %q", moves)
	}
	if evalBool(t, ctx, `!!document.getElementById("board-error")`) {
		t.Fatal("the page shows an error as well")
	}

	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/game/"+playing)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the record", `!!document.querySelector("#retired-game h2")`, 15*time.Second)
	if text := evalString(t, ctx, `document.getElementById("retired-game").innerText`); !strings.Contains(text, "ended with no result") {
		t.Fatalf("an interrupted game: %q", text)
	}
}
