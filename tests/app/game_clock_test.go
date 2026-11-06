package app_test

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

type clockResp struct {
	Running int `json:"running"`
	Seats   []struct {
		Seat    int     `json:"seat"`
		Initial int     `json:"initial_seconds"`
		Left    float64 `json:"seconds_left"`
	} `json:"seats"`
}

type gameWithClock struct {
	Status          string     `json:"status"`
	ImpulseDeadline string     `json:"impulse_deadline"`
	Clock           *clockResp `json:"clock"`
	Seats           []struct {
		Seat int    `json:"seat"`
		Name string `json:"name"`
	} `json:"seats"`
}

// A game clock is asked for when the game is made: one time for both players,
// or one for each when they want odds. The game then reports what each side
// has left and whose clock is running.
func TestGameClock_OverREST(t *testing.T) {
	baseURL := newRESTTestServer(t)
	open := func(body map[string]any) (string, int) {
		var created struct {
			RoomCode string `json:"room_code"`
		}
		code := doJSON(t, "POST", baseURL+"/api/games", body, &created)
		return created.RoomCode, code
	}

	// One number: both players get it.
	room, code := open(map[string]any{"game_def_id": "tak-3x3", "player_name": "Ana", "also_seats": []int{2}, "clock_seconds": 300})
	if code != 200 {
		t.Fatalf("create with a clock: %d", code)
	}
	var game gameWithClock
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if game.Clock == nil {
		t.Fatal("the game reports no clock")
	}
	if len(game.Clock.Seats) != 2 || game.Clock.Seats[0].Initial != 300 || game.Clock.Seats[1].Initial != 300 {
		t.Errorf("the clock = %+v, want five minutes each", game.Clock.Seats)
	}
	if game.Clock.Running != 1 || game.ImpulseDeadline == "" {
		t.Errorf("running = %d, deadline %q: seat 1's clock should be ticking", game.Clock.Running, game.ImpulseDeadline)
	}

	// A move takes the mover's time off their own clock, and starts the other.
	if code, out := moveNotation(t, baseURL, room, game.Seats[0].Name, "a1"); code != 200 {
		t.Fatalf("a move: %d %v", code, out)
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if game.Clock.Running != 2 {
		t.Errorf("after the move seat %d's clock runs, want seat 2's", game.Clock.Running)
	}
	if game.Clock.Seats[0].Left > 300 || game.Clock.Seats[1].Left != 300 {
		t.Errorf("after the move the clocks are %+v, want the mover's charged and the other whole", game.Clock.Seats)
	}

	// One time each: handicap play.
	room, code = open(map[string]any{"game_def_id": "tak-3x3", "player_name": "Bo", "also_seats": []int{2}, "clock_seconds": []int{120, 600}})
	if code != 200 {
		t.Fatalf("create with two times: %d", code)
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if game.Clock == nil || game.Clock.Seats[0].Initial != 120 || game.Clock.Seats[1].Initial != 600 {
		t.Errorf("the clock = %+v, want two minutes against ten", game.Clock)
	}

	// What cannot be played with is refused.
	for name, body := range map[string]map[string]any{
		"a clock and a time per move": {"game_def_id": "tak-3x3", "player_name": "X", "clock_seconds": 300, "move_time_seconds": 60},
		"three times for four seats":  {"game_def_id": "nyet", "player_name": "X", "clock_seconds": []int{300, 300, 300}},
		"a clock for one player":      {"game_def_id": "freecell", "player_name": "X", "clock_seconds": 300},
		"a clock of no time":          {"game_def_id": "tak-3x3", "player_name": "X", "clock_seconds": 0},
		"one time for two seats":      {"game_def_id": "tak-3x3", "player_name": "X", "clock_seconds": []int{300}},
		"a clock that is not a time":  {"game_def_id": "tak-3x3", "player_name": "X", "clock_seconds": "five minutes"},
	} {
		if _, code := open(body); code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}

	// An increment comes with the clock, and needs one.
	room, code = open(map[string]any{"game_def_id": "tak-3x3", "player_name": "Cy", "also_seats": []int{2}, "clock_seconds": 300, "clock_increment_seconds": 5})
	if code != 200 {
		t.Fatalf("create with an increment: %d", code)
	}
	var withInc struct {
		Clock struct {
			Increment int `json:"increment_seconds"`
		} `json:"clock"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &withInc)
	if withInc.Clock.Increment != 5 {
		t.Errorf("the game reports an increment of %d, want 5", withInc.Clock.Increment)
	}
	if _, code := open(map[string]any{"game_def_id": "tak-3x3", "player_name": "Di", "clock_increment_seconds": 5}); code != 400 {
		t.Errorf("an increment with no clock: %d, want 400", code)
	}

	// A game made without one has no clock at all. (A fresh value: the field
	// is left out of the answer, so an old one would stay behind.)
	room, _ = open(map[string]any{"game_def_id": "tak-3x3", "player_name": "Cy", "also_seats": []int{2}})
	var plain gameWithClock
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &plain)
	if plain.Status == "" {
		t.Fatal("the game without a clock was not created")
	}
	if plain.Clock != nil {
		t.Errorf("a game without a clock reports %+v", plain.Clock)
	}
}

// The setup offers the clock for two seats or more (not for a game for one),
// the two sides of a game of two can be given different times, and the game page shows every
// player's clock with the running one counting down.
func TestWebGameClock_SetupAndTheSeatStrips(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000)); err != nil {
		t.Fatal(err)
	}
	open := func(game string) {
		t.Helper()
		if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game="+game)); err != nil {
			t.Fatal(err)
		}
		waitJS(t, ctx, "the setup of "+game, `document.getElementById("create-game-form") !== null && !document.getElementById("game-setup").hidden`, 20*time.Second)
	}

	// A game for one has no clock (ADR 0091). The seat count is set when the field is drawn, so
	// this cannot pass on the page as it was before.
	open("solitaire")
	waitJS(t, ctx, "no clock field", `document.getElementById("clock-field").dataset.seats === "1" && document.getElementById("clock-field").hidden === true`, 10*time.Second)

	// A table of four has one, the same for everybody: different times are for a game of two.
	open("nyet")
	waitJS(t, ctx, "the clock field at four", `document.getElementById("clock-field").hidden === false`, 10*time.Second)
	if !evalBool(t, ctx, `document.getElementById("clock-handicap").parentElement.hidden`) {
		t.Error("a table of four offers different times")
	}

	// A game of two does, off by default.
	open("tak-5x5")
	waitJS(t, ctx, "the clock field", `document.getElementById("clock-field").hidden === false`, 10*time.Second)
	if got := evalString(t, ctx, `document.getElementById("clock_seconds").value`); got != "0" {
		t.Errorf("the clock starts at %q, want no clock", got)
	}
	if !evalBool(t, ctx, `document.getElementById("clock-handicap-row").hidden`) {
		t.Error("the handicap times show before they are asked for")
	}
	// The increment appears with a clock, and goes when the clock does.
	if !evalBool(t, ctx, `document.getElementById("clock-increment-row").hidden`) {
		t.Error("the increment shows with no clock chosen")
	}
	setClock := `(function (v) { var s = document.getElementById("clock_seconds"); s.value = v; s.dispatchEvent(new Event("change", { bubbles: true })); return "ok"; })`
	evalString(t, ctx, setClock+`("minutes")`)
	waitJS(t, ctx, "the increment", `document.getElementById("clock-increment-row").hidden === false`, 5*time.Second)
	evalString(t, ctx, setClock+`("0")`)
	waitJS(t, ctx, "the increment to go", `document.getElementById("clock-increment-row").hidden === true`, 5*time.Second)

	// Different times for the two players, and a game made with them.
	evalString(t, ctx, `document.getElementById("clock-handicap").click(); "ok"`)
	waitJS(t, ctx, "the handicap times", `document.getElementById("clock-handicap-row").hidden === false`, 5*time.Second)
	evalString(t, ctx, `(function () {
		var set = function (id, v) { var s = document.getElementById(id); s.value = v; s.dispatchEvent(new Event("change", { bubbles: true })); };
		set("clock_minutes_1", "3");
		set("clock_minutes_2", "10");
		set("clock_increment", "5");
		document.getElementById("move_time_seconds").value = "0";
		localStorage.setItem("bingle.name", JSON.stringify("Ana"));
		return "ok";
	})()`)
	// Play both seats, so the page can watch its own clock run.
	evalString(t, ctx, `(function () {
		var sel = document.querySelector('#opponent-choices input[value="@self"], #opponent-choices input[value="self"]');
		if (sel) { sel.checked = true; sel.dispatchEvent(new Event("change", { bubbles: true })); }
		return sel ? "self" : "friend";
	})()`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	room := loc[strings.LastIndex(loc, "/")+1:]
	var game gameWithClock
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if game.Clock == nil || game.Clock.Seats[0].Initial != 180 || game.Clock.Seats[1].Initial != 600 {
		t.Fatalf("the game was made with %+v, want three minutes against ten", game.Clock)
	}
	var inc struct {
		Clock struct {
			Increment int `json:"increment_seconds"`
		} `json:"clock"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &inc)
	if inc.Clock.Increment != 5 {
		t.Errorf("the page asked for an increment of %d, want 5", inc.Clock.Increment)
	}

	// Both clocks are on the page; only the one to move counts down.
	waitJS(t, ctx, "two clocks", `document.querySelectorAll('.seat-strip .clock[data-clock]').length === 2`, 15*time.Second)
	// (The seats are listed from the viewer's own, so they are sorted here.)
	if got := evalString(t, ctx, `[...document.querySelectorAll('.seat-strip .clock')].map(c => c.dataset.clock + ":" + c.classList.contains("running")).sort().join(",")`); got != "1:true,2:false" {
		t.Errorf("the clocks read %q, want seat 1's running", got)
	}
	if got := evalString(t, ctx, `document.querySelector('.seat-strip[data-seat="2"] .clock').textContent`); !strings.Contains(got, "10:00") {
		t.Errorf("seat 2's clock reads %q, want ten minutes", got)
	}
	first := evalString(t, ctx, `document.querySelector('.seat-strip[data-seat="1"] .clock').textContent`)
	waitJS(t, ctx, "the running clock to tick", fmt.Sprintf(`document.querySelector('.seat-strip[data-seat="1"] .clock').textContent !== %q`, first), 5*time.Second)
	if got := evalString(t, ctx, `document.querySelector('.seat-strip[data-seat="2"] .clock').textContent`); !strings.Contains(got, "10:00") {
		t.Errorf("seat 2's clock moved to %q while it was not their turn", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// The setup takes any whole number of minutes from 1 to 60, suggests the time
// the game's definition names (a bigger board its own), and keeps a time the
// player typed when they look at another size. A clock stays off until chosen.
func TestWebClockMinutesAndTheGamesDefault(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	open := func(game string) {
		t.Helper()
		if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(baseURL+"/?game="+game)); err != nil {
			t.Fatal(err)
		}
		waitJS(t, ctx, "the setup of "+game, `document.getElementById("clock-field").hidden === false && !document.getElementById("game-setup").hidden`, 20*time.Second)
	}
	minutes := func() string {
		t.Helper()
		return evalString(t, ctx, `document.getElementById("clock_minutes").value`)
	}
	for game, want := range map[string]string{"blooms": "10", "blooms-4": "5", "blooms-6": "15", "tak-5x5": "10"} {
		open(game)
		waitJS(t, ctx, game+"'s time", fmt.Sprintf(`document.getElementById("clock_minutes").value === %q`, want), 5*time.Second)
		if got := evalString(t, ctx, `document.getElementById("clock_seconds").value`); got != "0" {
			t.Errorf("%s starts with the clock at %q, want no clock", game, got)
		}
		if !evalBool(t, ctx, `document.getElementById("clock_minutes").hidden`) {
			t.Errorf("%s shows the minutes with no clock chosen", game)
		}
	}

	open("blooms-6")
	evalString(t, ctx, `(function () {
		var s = document.getElementById("clock_seconds");
		s.value = "minutes";
		s.dispatchEvent(new Event("change", { bubbles: true }));
		return "ok";
	})()`)
	waitJS(t, ctx, "the minutes", `document.getElementById("clock_minutes").hidden === false`, 5*time.Second)
	if got := minutes(); got != "15" {
		t.Errorf("choosing a clock on the big board suggests %s minutes, want 15", got)
	}
	// What is typed stays within the range, in whole minutes.
	const typeMinutes = `(function (v) {
		var b = document.getElementById("clock_minutes");
		b.value = v;
		b.dispatchEvent(new Event("input", { bubbles: true }));
		b.dispatchEvent(new Event("change", { bubbles: true }));
		return b.value;
	})`
	for _, c := range []struct{ typed, want string }{{"90", "60"}, {"0", "1"}, {"-3", "1"}, {"7.4", "7"}, {"", "15"}, {"17", "17"}} {
		if got := evalString(t, ctx, typeMinutes+fmt.Sprintf(`(%q)`, c.typed)); got != c.want {
			t.Errorf("typing %q leaves %q, want %q", c.typed, got, c.want)
		}
	}
	evalString(t, ctx, `localStorage.setItem("bingle.name", JSON.stringify("Ana")); "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	var game gameWithClock
	doJSON(t, "GET", baseURL+"/api/games/"+loc[strings.LastIndex(loc, "/")+1:], nil, &game)
	if game.Clock == nil || game.Clock.Seats[0].Initial != 17*60 || game.Clock.Seats[1].Initial != 17*60 {
		t.Fatalf("the game was made with %+v, want 17 minutes each", game.Clock)
	}
}

// A longer time is still on offer, for a game played over days.
func TestWebClockKeepsTheLongTimes(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(baseURL+"/?game=tak-5x5")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the setup", `document.getElementById("clock-field").hidden === false && !document.getElementById("game-setup").hidden`, 20*time.Second)
	evalString(t, ctx, `(function () {
		var s = document.getElementById("clock_seconds");
		s.value = "86400";
		s.dispatchEvent(new Event("change", { bubbles: true }));
		localStorage.setItem("bingle.name", JSON.stringify("Ana"));
		return "ok";
	})()`)
	waitJS(t, ctx, "no minutes for a day", `document.getElementById("clock_minutes").hidden === true`, 5*time.Second)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	var game gameWithClock
	doJSON(t, "GET", baseURL+"/api/games/"+loc[strings.LastIndex(loc, "/")+1:], nil, &game)
	if game.Clock == nil || game.Clock.Seats[0].Initial != 86400 {
		t.Fatalf("the game was made with %+v, want a day each", game.Clock)
	}
}
