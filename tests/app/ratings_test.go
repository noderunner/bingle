package app_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/noderunner/bingle/internal/auth"
)

// ratedGameBody asks for a rated game of Yavalath.
func ratedGameBody(handle string) map[string]any {
	return map[string]any{"game_def_id": "yavalath", "player_name": handle, "clock_seconds": 600, "rated": true}
}

// playRatedGame plays a game through the API to a's win, and waits for its result.
func playRatedGame(t *testing.T, a *browser, aName string, b *browser, bName string) string {
	t.Helper()
	var made struct {
		RoomCode string `json:"room_code"`
	}
	if code := a.do("POST", "/api/games", ratedGameBody(aName), &made); code != 200 {
		t.Fatalf("a rated game: %d", code)
	}
	path := "/api/games/" + made.RoomCode
	if code := b.do("POST", path+"/join", map[string]any{"player_name": bName}, nil); code != 200 {
		t.Fatalf("@%s joins: %d", bName, code)
	}
	if code := a.do("POST", path+"/moves", map[string]any{"player_name": aName, "notation": "e5"}, nil); code != 200 {
		t.Fatalf("@%s moves: %d", aName, code)
	}
	if code := b.do("POST", path+"/moves", map[string]any{"player_name": bName, "notation": "d4"}, nil); code != 200 {
		t.Fatalf("@%s moves: %d", bName, code)
	}
	if code := b.do("POST", path+"/resign", map[string]any{"player_name": bName}, nil); code != 200 {
		t.Fatalf("@%s resigns: %d", bName, code)
	}
	waitForRatedResult(t, a, made.RoomCode)
	return made.RoomCode
}

type ratedGameView struct {
	Rated         bool   `json:"rated"`
	RatingStatus  string `json:"rating_status"`
	RatingChanges []struct {
		Seat   int `json:"seat"`
		Before int `json:"before"`
		After  int `json:"after"`
	} `json:"rating_changes"`
	Seats []struct {
		Seat   int `json:"seat"`
		Rating *struct {
			Rating      int  `json:"rating"`
			Provisional bool `json:"provisional"`
		} `json:"rating"`
	} `json:"seats"`
}

func waitForRatedResult(t *testing.T, b *browser, room string) ratedGameView {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var view ratedGameView
		if code := b.do("GET", "/api/games/"+room, nil, &view); code != 200 {
			t.Fatalf("reading the game: %d", code)
		}
		if view.RatingStatus != "" && view.RatingStatus != "pending" {
			return view
		}
		if time.Now().After(deadline) {
			t.Fatalf("the result was never written: %+v", view)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Through the API: who may play one, what it refuses, and what it does.
func TestRatedGames_API(t *testing.T) {
	a, base := membersServer(t)
	ana := member(t, a, base, "ana")
	bea := member(t, a, base, "bea")
	guest := newBrowser(t, base)

	if code := guest.do("POST", "/api/games", map[string]any{"game_def_id": "yavalath", "player_name": "Gus", "clock_seconds": 600, "rated": true}, nil); code != 400 {
		t.Errorf("a guest's rated game: %d, want 400", code)
	}
	for name, body := range map[string]map[string]any{
		"no clock":    {"game_def_id": "yavalath", "player_name": "ana", "rated": true},
		"a bot":       {"game_def_id": "yavalath", "player_name": "ana", "clock_seconds": 600, "rated": true, "opponent": "random"},
		"a handicap":  {"game_def_id": "yavalath", "player_name": "ana", "clock_seconds": []int{60, 600}, "rated": true},
		"a big table": {"game_def_id": "nyet", "player_name": "ana", "rated": true},
	} {
		if code := ana.do("POST", "/api/games", body, nil); code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}

	var made struct {
		RoomCode string `json:"room_code"`
	}
	if code := ana.do("POST", "/api/games", ratedGameBody("ana"), &made); code != 200 {
		t.Fatalf("a rated game: %d", code)
	}
	path := "/api/games/" + made.RoomCode
	if code := guest.do("POST", path+"/join", map[string]any{"player_name": "Gus"}, nil); code != 400 {
		t.Errorf("a guest joins a rated game: %d, want 400", code)
	}
	if code := bea.do("POST", path+"/join", map[string]any{"player_name": "bea"}, nil); code != 200 {
		t.Fatalf("a second account joins: %d", code)
	}
	var view ratedGameView
	ana.do("GET", path, nil, &view)
	if !view.Rated || view.Seats[0].Rating == nil || view.Seats[0].Rating.Rating != 1500 || !view.Seats[0].Rating.Provisional {
		t.Errorf("the game in play: %+v", view)
	}
	if code := ana.do("POST", path+"/moves", map[string]any{"player_name": "ana", "notation": "e5"}, nil); code != 200 {
		t.Fatalf("a move: %d", code)
	}
	if code := ana.do("POST", path+"/undo", map[string]any{"player_name": "ana"}, nil); code != 409 {
		t.Errorf("undo: %d, want 409", code)
	}
	if code := ana.do("POST", path+"/takeback", map[string]any{"player_name": "ana", "action": "request"}, nil); code != 409 {
		t.Errorf("take-back: %d, want 409", code)
	}
	if code := bea.do("POST", path+"/moves", map[string]any{"player_name": "bea", "notation": "d4"}, nil); code != 200 {
		t.Fatalf("a move: %d", code)
	}
	if code := bea.do("POST", path+"/resign", map[string]any{"player_name": "bea"}, nil); code != 200 {
		t.Fatalf("resigning: %d", code)
	}
	view = waitForRatedResult(t, ana, made.RoomCode)
	if view.RatingStatus != "rated" || len(view.RatingChanges) != 2 || view.RatingChanges[0].After <= 1500 || view.RatingChanges[1].After >= 1500 {
		t.Errorf("after the game: %+v", view)
	}

	// Her own ratings she can read at once; nobody else can until she has played
	// enough, and no leaderboard has her.
	var own struct {
		Ratings []struct {
			Family string `json:"family"`
			Name   string `json:"name"`
			Won    int    `json:"won"`
			Listed bool   `json:"listed"`
		} `json:"ratings"`
	}
	if code := ana.do("GET", "/api/players/ana/ratings", nil, &own); code != 200 || len(own.Ratings) != 1 || own.Ratings[0].Name != "Yavalath" || own.Ratings[0].Won != 1 || own.Ratings[0].Listed {
		t.Errorf("her own ratings: %d, %+v", code, own)
	}
	var theirs struct {
		Ratings []any `json:"ratings"`
	}
	if code := guest.do("GET", "/api/players/ana/ratings", nil, &theirs); code != 200 || len(theirs.Ratings) != 0 {
		t.Errorf("a guest reads hers: %d, %+v", code, theirs)
	}
	if code := guest.do("GET", "/api/players/nobody/ratings", nil, nil); code != 404 {
		t.Errorf("an unknown player: %d, want 404", code)
	}
	var games struct {
		Games []struct {
			Family string `json:"family"`
		} `json:"games"`
	}
	if code := guest.do("GET", "/api/ratings", nil, &games); code != 200 || len(games.Games) != 0 {
		t.Errorf("the games with a board: %d, %+v", code, games)
	}
}

// The page: the setup offers it to an account and not a guest, the game shows it
// and takes away what it does not allow, and the result reaches the seats, the
// Ratings tab and the profile.
func TestRatedGames_UI(t *testing.T) {
	a, base := membersServer(t)
	ana := member(t, a, base, "ana")
	bea := member(t, a, base, "bea")

	// A guest is not offered it.
	guestCtx, cancelGuest := newChromedpContext(t, 60*time.Second)
	defer cancelGuest()
	if err := chromedp.Run(guestCtx, chromedp.Navigate(base+"/?game=yavalath")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, guestCtx, "the setup", `document.getElementById("clock-field").hidden === false`, 20*time.Second)
	time.Sleep(300 * time.Millisecond)
	if !evalBool(t, guestCtx, `document.getElementById("rated-field").hidden`) {
		t.Error("a guest is offered a rated game")
	}

	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(base+"/?game=nyet")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the setup of a big table", `document.getElementById("clock-field").hidden === true`, 20*time.Second)
	time.Sleep(300 * time.Millisecond)
	if !evalBool(t, ctx, `document.getElementById("rated-field").hidden`) {
		t.Error("a table of four is offered a rated game")
	}

	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/?game=yavalath")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the choice", `document.getElementById("rated-field").hidden === false`, 20*time.Second)
	evalString(t, ctx, `document.getElementById("rated").click(); "ok"`)
	waitJS(t, ctx, "a clock chosen for it", `document.getElementById("clock_seconds").value === "600"`, 5*time.Second)
	if !evalBool(t, ctx, `[...document.querySelectorAll('#opponent-choices input[name="opponent"]')].every((r) => r.disabled === (r.value !== ""))`) {
		t.Error("the bots can still be chosen for a rated game")
	}
	// Taking the clock away takes the choice away.
	evalString(t, ctx, `(function () { var c = document.getElementById("clock_seconds"); c.value = "0"; c.dispatchEvent(new Event("change", { bubbles: true })); return "ok"; })()`)
	waitJS(t, ctx, "the choice given up", `document.getElementById("rated").checked === false`, 5*time.Second)
	if evalBool(t, ctx, `[...document.querySelectorAll('#opponent-choices input[name="opponent"]')].some((r) => r.disabled)`) {
		t.Error("the opponents stay shut after giving the choice up")
	}
	evalString(t, ctx, `document.getElementById("rated").click(); "ok"`)
	evalString(t, ctx, `document.getElementById("create_player_name").value = "ana"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	room := loc[strings.LastIndex(loc, "/")+1:]

	waitJS(t, ctx, "the badge", `document.getElementById("rated-badge") !== null`, 15*time.Second)
	path := "/api/games/" + room
	if code := bea.do("POST", path+"/join", map[string]any{"player_name": "bea"}, nil); code != 200 {
		t.Fatalf("a second account joins: %d", code)
	}
	waitJS(t, ctx, "both seats rated", `document.querySelectorAll('.seat-strip .badge.rating').length === 2`, 15*time.Second)
	if code := ana.do("POST", path+"/moves", map[string]any{"player_name": "ana", "notation": "e5"}, nil); code != 200 {
		t.Fatalf("a move: %d", code)
	}
	waitJS(t, ctx, "the move", `document.querySelector('#move-list button.mv') !== null`, 15*time.Second)
	if evalBool(t, ctx, `document.getElementById("undo-move") !== null`) {
		t.Error("a rated game offers an undo")
	}
	if code := bea.do("POST", path+"/moves", map[string]any{"player_name": "bea", "notation": "d4"}, nil); code != 200 {
		t.Fatalf("a move: %d", code)
	}
	if code := bea.do("POST", path+"/resign", map[string]any{"player_name": "bea"}, nil); code != 200 {
		t.Fatalf("resigning: %d", code)
	}
	waitJS(t, ctx, "the change at the seat", `[...document.querySelectorAll('.seat-strip .badge.rating')].some((b) => /\(\+\d+\)/.test(b.textContent))`, 15*time.Second)

	// Enough games against enough people for a place on the board.
	others := []*browser{bea, member(t, a, base, "cyd"), member(t, a, base, "dia")}
	names := []string{"bea", "cyd", "dia"}
	for i := 0; i < 4; i++ {
		playRatedGame(t, ana, "ana", others[i%3], names[i%3])
	}

	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/#ratings")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the leaderboard", `document.querySelector('#leaderboard tr[data-handle="ana"]') !== null`, 15*time.Second)
	if got := evalString(t, ctx, `document.querySelector('#leaderboard tr[data-handle="ana"]').children[3].textContent`); got != "5" {
		t.Errorf("the leaderboard says %s games", got)
	}
	if evalBool(t, ctx, `document.querySelector('#leaderboard tr[data-handle="bea"]') !== null`) {
		t.Error("somebody with one opponent is on the board")
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the profile's ratings", `document.getElementById("ratings-box").hidden === false`, 15*time.Second)
	if got := evalString(t, ctx, `document.querySelector('#ratings-rows tr').children[0].textContent`); got != "Yavalath" {
		t.Errorf("the profile lists %q", got)
	}
}
