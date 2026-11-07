// Tests of taking over a seat through the API, MCP and the page.
package app_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

type takeoverGame struct {
	Status string `json:"status"`
	Seats  []struct {
		Seat     int    `json:"seat"`
		Name     string `json:"name"`
		Bot      string `json:"bot"`
		Replaced *struct {
			Name    string `json:"name"`
			Reason  string `json:"reason"`
			MoveSeq int    `json:"move_seq"`
		} `json:"replaced"`
	} `json:"seats"`
	Clock *struct {
		Running int `json:"running"`
		Seats   []struct {
			Seat int `json:"seat"`
		} `json:"seats"`
	} `json:"clock"`
	RealTime *struct {
		KickVotes []struct {
			Seat   int `json:"seat"`
			Votes  int `json:"votes"`
			Needed int `json:"needed"`
		} `json:"kick_votes"`
	} `json:"real_time"`
}

func threeAtNyet(t *testing.T, base string, extra map[string]any) string {
	t.Helper()
	var created createGameResponse
	body := map[string]any{"game_def_id": "nyet-3", "player_name": "Ann"}
	for k, v := range extra {
		body[k] = v
	}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	joinRoom(t, base, created.RoomCode, "Ben")
	joinRoom(t, base, created.RoomCode, "Cal")
	return created.RoomCode
}

func TestRESTVoteKick(t *testing.T) {
	base := newRESTTestServer(t)
	room := threeAtNyet(t, base, map[string]any{"real_time": map[string]any{}})
	vote := func(name string, seat int) int {
		return doJSON(t, "POST", base+"/api/games/"+room+"/kick-vote", map[string]any{"player_name": name, "seat": seat}, nil)
	}
	if code := vote("Ann", 2); code != http.StatusConflict {
		t.Errorf("with no socket: %d, want 409", code)
	}
	for _, name := range []string{"Ann", "Ben", "Cal"} {
		dialPlaying(t, base, room, name, "player")
	}
	waitConnected(t, base, room, 1, 2, 3)
	if code := vote("Ann", 2); code != 200 {
		t.Fatalf("open: %d", code)
	}
	var g takeoverGame
	doJSON(t, "GET", base+"/api/games/"+room, nil, &g)
	if g.RealTime == nil || len(g.RealTime.KickVotes) != 1 || g.RealTime.KickVotes[0].Seat != 2 || g.RealTime.KickVotes[0].Votes != 1 || g.RealTime.KickVotes[0].Needed != 2 {
		t.Fatalf("the vote: %+v", g.RealTime)
	}
	if code := vote("Cal", 2); code != 200 {
		t.Fatalf("second vote: %d", code)
	}
	doJSON(t, "GET", base+"/api/games/"+room, nil, &g)
	s := g.Seats[1]
	if s.Bot != "tactical" || s.Replaced == nil || s.Replaced.Name != "Ben" || s.Replaced.Reason != "kicked" {
		t.Errorf("seat 2: %+v, %+v", s, s.Replaced)
	}
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/join", map[string]any{"player_name": "Ben", "role": "player"}, nil); code != http.StatusConflict {
		t.Errorf("Ben coming back: %d, want 409", code)
	}
}

// One number gives every seat its time, whatever the table.
func TestRESTClock_OneTimeForAll(t *testing.T) {
	base := newRESTTestServer(t)
	room := threeAtNyet(t, base, map[string]any{"clock_seconds": 600})
	var g takeoverGame
	doJSON(t, "GET", base+"/api/games/"+room, nil, &g)
	if g.Clock == nil || len(g.Clock.Seats) != 3 {
		t.Errorf("the clock: %+v", g.Clock)
	}
}

func TestMCPVoteKick_SaysWhyWithNoSocket(t *testing.T) {
	base := newRESTTestServer(t)
	room := threeAtNyet(t, base, map[string]any{"real_time": map[string]any{}})
	c := newMCPClient(t, base)
	res := callTool(t, c, "vote_kick", map[string]any{"room_code": room, "player_name": "Ann", "seat": 2})
	if !res.IsError {
		t.Errorf("no socket, and no refusal: %+v", res)
	}
}

// A reaction window names the reacting seat, so the clock that runs is theirs: this
// is what lets the clock go beyond two seats (real-time plan, decision 19).
func TestGameClock_RunsForTheSeatThatMustReact(t *testing.T) {
	base := newRESTTestServer(t)
	var created createGameResponse
	body := map[string]any{
		"game_def_id": councilReactionTable, "player_name": "Alice", "seat_bots": map[string]string{"2": "random", "3": "random"},
		"start_state": json.RawMessage(reactionStart(t)), "start_seat": 4, "clock_seconds": 3600,
	}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	joinRoom(t, base, room, "Bob")
	var g takeoverGame
	doJSON(t, "GET", base+"/api/games/"+room, nil, &g)
	if g.Clock == nil || g.Clock.Running != 4 {
		t.Fatalf("before the action: %+v", g.Clock)
	}
	// Bob walks into the North, where Alice's envoy stands: she is asked.
	if code, out := moveNotation(t, base, room, "Bob", "go@north"); code != 200 {
		t.Fatalf("move: %d %v", code, out)
	}
	doJSON(t, "GET", base+"/api/games/"+room, nil, &g)
	if g.Clock == nil || g.Clock.Running != 1 {
		t.Errorf("Alice is asked to react, and the clock running is seat %+v", g.Clock)
	}
}

// The setup offers a clock to a table of three, with one time for all.
func TestWebClock_TheSetupOfALargeTable(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/?game=nyet-3"), chromedp.WaitVisible(`#create-submit`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the clock is offered", `document.getElementById("clock-field").hidden === false`, 10*time.Second)
	if !evalBool(t, ctx, `document.getElementById("clock-handicap").parentElement.hidden`) {
		t.Error("different times are offered for a table of three")
	}
	evalString(t, ctx, `(() => { const c = document.getElementById("clock_seconds"); c.value = "7200"; c.dispatchEvent(new Event("change")); return "ok"; })()`)
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`localStorage.setItem("bingle.name", JSON.stringify("Ann"))`, nil),
		submitClosestForm(ctx, `select[name="game_def_id"]`),
	); err != nil {
		t.Fatal(err)
	}
	url := pollLocation(t, ctx, gamePageURLPattern)
	room := gamePageURLPattern.FindStringSubmatch(url)[1]
	var g takeoverGame
	doJSON(t, "GET", base+"/api/games/"+room, nil, &g)
	if g.Clock == nil || len(g.Clock.Seats) != 3 {
		t.Errorf("the clock made: %+v", g.Clock)
	}
}

// The whole of it on the page: the button, the vote painted in, the seat taken, and
// the player left watching.
func TestWebVoteKick(t *testing.T) {
	base := newRESTTestServer(t)
	room := threeAtNyet(t, base, map[string]any{"real_time": map[string]any{}})

	// Ann and Ben each move first, so that the record has something of Ben's.
	for _, name := range []string{"Ann", "Ben"} {
		if code, out := moveNotation(t, base, room, name, legalNotations(t, base, room)[0]); code != 200 {
			t.Fatalf("%s's move: %d %v", name, code, out)
		}
	}

	annCtx, cancelA := newChromedpContext(t, 120*time.Second)
	defer cancelA()
	openGameAs(t, annCtx, base, room, "nyet-3", "Ann")
	benCtx, cancelB := newChromedpContext(t, 120*time.Second)
	defer cancelB()
	openGameAs(t, benCtx, base, room, "nyet-3", "Ben")
	calCtx, cancelC := newChromedpContext(t, 120*time.Second)
	defer cancelC()
	openGameAs(t, calCtx, base, room, "nyet-3", "Cal")

	button := `document.querySelector('#seat-2 .seat-kick')`
	waitJS(t, annCtx, "Ann can start a vote", button+` && !`+button+`.hidden`, 15*time.Second)
	evalString(t, annCtx, `document.querySelector("#seat-1").dataset.same = "yes"; window.confirm = () => true; `+button+`.click(); "ok"`)

	waitJS(t, calCtx, "Cal is asked", `document.querySelector('#kick-vote .kick-vote-yes') !== null`, 15*time.Second)
	waitJS(t, benCtx, "Ben is told", `document.getElementById("kick-vote").textContent.includes("take your seat")`, 15*time.Second)
	waitJS(t, annCtx, "Ann sees her vote counted", `document.getElementById("kick-vote").textContent.includes("1 of 2")`, 15*time.Second)
	if evalBool(t, annCtx, `document.querySelector('#kick-vote .kick-vote-yes') !== null`) {
		t.Error("Ann is asked to vote again")
	}
	if evalBool(t, annCtx, `!document.querySelector('#seat-2 .seat-kick').hidden`) {
		t.Error("a second vote is on offer with one open")
	}
	if got := evalString(t, annCtx, `document.querySelector("#seat-1").dataset.same || ""`); got != "yes" {
		t.Error("the page was re-rendered for a vote")
	}

	evalString(t, calCtx, `document.querySelector('#kick-vote .kick-vote-yes').click(); "ok"`)
	waitJS(t, annCtx, "the seat shows who it replaced", `document.querySelector('#seat-2 .seat-replaced') !== null && document.querySelector('#seat-2 .seat-replaced').textContent.includes("Ben")`, 15*time.Second)
	waitJS(t, annCtx, "the vote is gone", `document.getElementById("kick-vote").hidden`, 15*time.Second)
	evalString(t, annCtx, `document.getElementById("history-notation").click(); "ok"`)
	waitJS(t, annCtx, "the move list still says Ben played the move before", `document.querySelector('#move-list [title*="by Ben"]') !== null`, 15*time.Second)
	waitJS(t, benCtx, "Ben no longer has a seat", `document.querySelector('#seat-2 .badge.bot') !== null && ![...document.querySelectorAll('.seat-strip .badge')].some((b) => b.textContent === "you")`, 15*time.Second)
}
