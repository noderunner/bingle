package app_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// The cards of the viewer's own hand, as the page draws them: the slots left to
// right, and the id of the card in each.
const (
	handSlotsJS = `[...document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pc-slot')]`
	handIDsJS   = handSlotsJS + `.map(s => s.querySelector("[data-card]").dataset.card)`
)

// nyetTable starts a game of the given definition (4 seats unless it says
// otherwise) that Ana holds every seat of, and plays it to the first card of
// the tricks: the board of terms is settled, the teams are made and any
// discards are done, and the player to move can play a card.
func nyetTable(t *testing.T, baseURL, def string, seats int) string {
	t.Helper()
	var also []int
	for s := 2; s <= seats; s++ {
		also = append(also, s)
	}
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": def, "player_name": "Ana", "also_seats": also}, &created); code != 200 {
		t.Fatalf("create %s: %d", def, code)
	}
	nyetPlay(t, baseURL, created.RoomCode, func(m []string) bool { return !strings.Contains(m[0], ":") })
	return created.RoomCode
}

// The cards are the four classic suits, drawn: your hand shows pictures of
// hearts, spades, diamonds and clubs with the value in two corners, and the
// cards say what they are.
func TestWebNyet_TheCardsAreTheFourClassicSuits(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := nyetTable(t, baseURL, "nyet", 4)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "nyet", "Ana")
	waitJS(t, ctx, "the hand", `document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face').length >= 10`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face svg.pc-svg').length - document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face').length`); n != 0 {
		t.Errorf("%d faces are not drawn as pictures", -n)
	}
	// Every card is named for its suit: "9 of hearts".
	got := evalString(t, ctx, `[...new Set([...document.querySelectorAll('#card-table .pcard.face')].map(c => c.title.replace(/^\d+ of /, "")))].sort().join(",")`)
	for _, name := range strings.Split(got, ",") {
		if name != "hearts" && name != "spades" && name != "diamonds" && name != "clubs" {
			t.Errorf("a card is named for %q, which is no classic suit (all: %s)", name, got)
		}
	}
	// The suits are drawn in two colours, red and black, as a deck is.
	if got := evalString(t, ctx, `[...new Set([...document.querySelectorAll('#card-table .ct-seat.me .pcard.face')].map(c => c.style.getPropertyValue("--suit").trim()))].sort().join(",")`); got != "#1f2630,#c62f2f" {
		t.Errorf("suit colours %q, want black and red", got)
	}
}

// Pointing at a card of your hand shows it twice as large, and moving away
// takes it off again.
func TestWebNyet_PointingAtACardShowsItTwiceAsLarge(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := nyetTable(t, baseURL, "nyet", 4)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "nyet", "Ana")
	waitJS(t, ctx, "the hand", `document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face').length >= 10`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll(".pc-zoom").length`); n != 0 {
		t.Fatalf("%d enlarged cards before the pointer is on one", n)
	}
	time.Sleep(1200 * time.Millisecond) // the page redraws while it loads the chat, who is present and the moves
	// Move the pointer onto the fourth card (the last one of a fan shows whole, the others by their corner).
	var rect []float64
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(function () { var e = document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face')[3]; var r = e.getBoundingClientRect(); return [r.left + 12, r.top + 20, r.width, r.height]; })()`, &rect)); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(ctx, chromedp.MouseEvent("mouseMoved", rect[0]-40, rect[1]-40), chromedp.MouseEvent("mouseMoved", rect[0], rect[1])); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the enlarged card", `document.querySelectorAll(".pc-zoom").length === 1`, 5*time.Second)
	zoom := evalString(t, ctx, `(function () { var z = document.querySelector(".pc-zoom").getBoundingClientRect(); return [Math.round(z.width), Math.round(z.height)].join("x"); })()`)
	want := fmt.Sprintf("%dx%d", int(rect[2]*2+0.5), int(rect[3]*2+0.5))
	if zoom != want {
		t.Errorf("enlarged card is %s, want twice the %.0fx%.0f card: %s", zoom, rect[2], rect[3], want)
	}
	if !evalBool(t, ctx, `document.querySelector(".pc-zoom svg.pc-svg") !== null && !document.querySelector(".pc-zoom").hasAttribute("data-card")`) {
		t.Error("the enlarged card is not a picture of the card, or it carries the card's own id")
	}
	// Away from the cards: gone.
	if err := chromedp.Run(ctx, chromedp.MouseEvent("mouseMoved", 4, 4)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the enlarged card gone", `document.querySelectorAll(".pc-zoom").length === 0`, 5*time.Second)
}

// The players are animals, shown in their seat colour wherever a player is
// named in a picture: the list of players (with the cards in each hand), and the
// cards of the trick.
func TestWebNyet_PlayersAreAnimalsWithTheirCardsCounted(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := nyetTable(t, baseURL, "nyet-5", 5)
	// One card into the trick, so that it has a caption.
	var st struct {
		TurnSeat int `json:"turn_seat"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	var legal struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"moves"`
	}
	doJSON(t, "GET", fmt.Sprintf("%s/api/games/%s/moves/legal?seat=%d", baseURL, room, st.TurnSeat), nil, &legal)
	var info struct {
		Seats []struct {
			Seat int    `json:"seat"`
			Name string `json:"name"`
		} `json:"seats"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &info)
	if code, out := moveNotation(t, baseURL, room, info.Seats[st.TurnSeat-1].Name, legal.Moves[0].Notation); code != 200 {
		t.Fatalf("play a card: %d %v", code, out)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "nyet-5", "Ana")
	waitJS(t, ctx, "five players", `document.querySelectorAll('.seat-strip').length === 5`, 15*time.Second)
	// Each player is their own animal in the colour of their seat, in the list of players.
	if got := evalString(t, ctx, `[...document.querySelectorAll('.seat-strip')].map(s => { var a = s.querySelector("svg.animal"); return a ? a.getAttribute("class").replace(/^animal /, "") : "none"; }).join(",")`); got != "animal-wolf color-red,animal-crane color-blue,animal-tiger color-green,animal-bear color-yellow,animal-eagle color-orange" {
		t.Errorf("the players' pictures: %s", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('.seat-strip .swatch').length`); n != 0 {
		t.Errorf("%d square colour swatches are left beside the names", n)
	}
	// The cards in each hand, with the icon: one has played, so 11 and 12 with 5 players hold 12.
	got := evalString(t, ctx, `[...document.querySelectorAll('.seat-strip [data-counter="cards"]')].map(e => e.textContent.trim() + (e.querySelector("svg.stat-cards") ? "" : "?")).sort().join(",")`)
	if got != "11,12,12,12,12" {
		t.Errorf("cards in hand (sorted) %q, want 11,12,12,12,12", got)
	}
	// The card in the trick has the animal of the player who played it under it.
	waitJS(t, ctx, "the card of the trick", `document.querySelectorAll('.zone-trick .pc-slot').length === 1`, 10*time.Second)
	if !evalBool(t, ctx, `document.querySelectorAll('.zone-trick .pc-slot .pc-caption.icon svg.animal').length === 1 && document.querySelector('.zone-trick .pc-caption').innerText.trim() === ""`) {
		t.Error("the played card has no animal under it (or it has words as well)")
	}
}

// The board of terms is pictures, no words: every box and every line is drawn,
// a crossed box shows the player who crossed it, the cells are of one size and
// every line is centred; on a wide screen it stands beside the trick.
func TestWebNyet_TheBoardOfTermsIsPictures(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "nyet", "player_name": "Ana", "also_seats": []int{2, 3, 4}}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	// Three boxes crossed off, by the first three players in turn.
	played := 0
	nyetPlay(t, baseURL, created.RoomCode, func([]string) bool { played++; return played > 3 })
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, created.RoomCode, "nyet", "Ana")
	waitJS(t, ctx, "the board", `document.querySelectorAll('#choice-grid .cg-line').length === 5`, 15*time.Second)
	if n := evalInt(t, ctx, `[...document.querySelectorAll('#choice-grid .cg-box, #choice-grid .cg-title')].filter(e => /[A-Za-z]/.test(e.innerText) || !e.querySelector("svg")).length`); n != 0 {
		t.Errorf("%d boxes or line heads hold letters or no picture (numerals inside a picture are fine)", n)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll('#choice-grid .cg-box.crossed')].map(b => b.dataset.by + ":" + (b.querySelector("svg.animal") ? b.querySelector("svg.animal").getAttribute("class").split(" ")[1] : "none")).join(",")`); got != "1:animal-wolf,2:animal-crane,3:animal-tiger" {
		t.Errorf("crossed boxes show %q, want the wolf, the crane and the tiger, who crossed them", got)
	}
	if got := evalString(t, ctx, `document.querySelector('#choice-grid .cg-box.crossed').title`); !strings.Contains(got, "crossed off by Wolf") {
		t.Errorf("a crossed box says %q, want who crossed it", got)
	}
	// One size for every cell, and every line centred on the same axis (Trump has one box fewer).
	if got := evalString(t, ctx, `[...new Set([...document.querySelectorAll('#choice-grid .cg-box')].map(b => Math.round(b.getBoundingClientRect().width) + "x" + Math.round(b.getBoundingClientRect().height)))].join(",")`); strings.Contains(got, ",") {
		t.Errorf("the boxes are of several sizes: %s", got)
	}
	if got := evalString(t, ctx, `[...new Set([...document.querySelectorAll('#choice-grid .cg-boxes')].map(r => { var b = r.getBoundingClientRect(); var kids = r.querySelectorAll(".cg-box"); var l = kids[0].getBoundingClientRect().left, rr = kids[kids.length - 1].getBoundingClientRect().right; return Math.round((l + rr) / 2 - b.left - b.width / 2); }))].join(",")`); got != "0" {
		t.Errorf("the lines are not centred on one axis (offsets %s)", got)
	}
	// Beside the trick, not above it.
	waitJS(t, ctx, "the trick area", `document.querySelector('#card-table .ct-center') !== null`, 5*time.Second)
	if !evalBool(t, ctx, `(function () { var g = document.getElementById("terms-board").getBoundingClientRect(), c = document.querySelector("#card-table .ct-center").getBoundingClientRect(); return g.right <= c.left + 1 && Math.abs(g.top - c.top) < 40; })()`) {
		t.Error("the board of terms is not beside the trick area on a wide screen")
	}
}

// On a phone the game is five views: the play (the trick and your hand), the
// terms, the players, the moves and the chat.
func TestWebNyet_ThePhoneHasAViewForEachPartOfTheGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := nyetTable(t, baseURL, "nyet", 4)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(390, 800),
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(`localStorage.setItem("bingle.layout", JSON.stringify("phone")); true`, nil)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "nyet", "Ana")
	waitJS(t, ctx, "the phone layout", `document.getElementById("m-shell") !== null`, 15*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll("#m-nav .m-nav-item")].map(b => b.dataset.group).join(",")`); got != "play,terms,players,log,chat" {
		t.Errorf("the views: %s", got)
	}
	for view, want := range map[string]string{
		"play":    `document.querySelectorAll("#m-page-play #card-table .zone-hand .pcard.face").length >= 10 && document.querySelector("#m-page-play #card-table .ct-center") !== null`,
		"terms":   `document.querySelectorAll("#m-page-terms #choice-grid .cg-line").length === 5 && document.querySelectorAll("#m-page-terms .cg-legend dt").length === 5`,
		"players": `document.querySelectorAll("#m-page-players .seat-strip").length === 4 && document.querySelectorAll("#m-page-players .seat-strip svg.animal").length === 4`,
		"log":     `document.querySelector("#m-page-log") !== null`,
		"chat":    `document.querySelector("#m-page-chat #chat-input") !== null`,
	} {
		if !evalBool(t, ctx, want) {
			t.Errorf("the %s view lacks its parts (%s)", view, want)
		}
		clickSel(t, ctx, fmt.Sprintf(`#m-nav [data-group="%s"]`, view))
		waitJS(t, ctx, "the "+view+" view shown", fmt.Sprintf(`document.querySelector('#m-nav [data-group="%s"]').classList.contains("on")`, view), 5*time.Second)
	}
	// The hand of the play view can be played from the phone.
	clickSel(t, ctx, `#m-nav [data-group="play"]`)
	waitJS(t, ctx, "a card to play", `document.querySelector("#m-page-play .pc-btn.playable") !== null`, 10*time.Second)
	clickSel(t, ctx, "#m-page-play .pc-btn.playable")
	waitJS(t, ctx, "the card in the trick", `document.querySelectorAll("#m-page-play .zone-trick .pc-slot").length >= 1`, 10*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// The hand is dealt in an order a player can read: the suits in the game's own
// order, and the values rising inside each suit.
func TestWebNyet_TheHandIsHeldBySuitAndValue(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := nyetTable(t, baseURL, "nyet", 4)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "nyet", "Ana")
	waitJS(t, ctx, "the hand", `document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face').length >= 10`, 15*time.Second)
	got := evalString(t, ctx, `(function () {
		var order = ["red", "blue", "yellow", "green"], ids = `+handIDsJS+`, bad = [], suit = -1, value = -1;
		for (var i = 0; i < ids.length; i++) {
			var parts = ids[i].split("-"), s = order.indexOf(parts[0]), v = parseInt(parts[1], 10);
			if (s < suit || (s === suit && v < value)) bad.push(ids[i]);
			suit = s;
			value = v;
		}
		return ids.join(",") + "|" + bad.join(",");
	})()`)
	hand, out, _ := strings.Cut(got, "|")
	if out != "" {
		t.Errorf("%s is out of order in the hand %s", out, hand)
	}
}

// A card of your own hand can be dragged to another place in it; the drag does
// not play it, and the hand keeps the new order when the page is drawn again
// and when the game is opened afresh.
func TestWebNyet_ACardOfYourHandCanBeDraggedElsewhere(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := nyetTable(t, baseURL, "nyet", 4)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "nyet", "Ana")
	waitJS(t, ctx, "the hand", `document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face').length >= 10`, 15*time.Second)
	time.Sleep(1200 * time.Millisecond) // the page redraws while it loads the chat, who is present and the moves
	before := strings.Split(evalString(t, ctx, handIDsJS+`.join(",")`), ",")
	if len(before) < 6 {
		t.Fatalf("the hand holds %d cards: %v", len(before), before)
	}
	// Take the first card and drop it between the fourth and the fifth.
	var at []float64
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(function () {
		var slots = `+handSlotsJS+`;
		var mid = function (i) { var r = slots[i].getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; };
		var a = mid(0), b = mid(3), c = mid(4);
		return [a[0], a[1], (b[0] + c[0]) / 2, b[1]];
	})()`, &at)); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchMouseEvent(input.MousePressed, at[0], at[1]).WithButton(input.Left).WithClickCount(1).Do(c)
		}),
		chromedp.ActionFunc(func(c context.Context) error {
			for i := 1; i <= 6; i++ {
				x := at[0] + (at[2]-at[0])*float64(i)/6
				if err := input.DispatchMouseEvent(input.MouseMoved, x, at[3]).WithButton(input.Left).Do(c); err != nil {
					return err
				}
				time.Sleep(30 * time.Millisecond)
			}
			return nil
		}),
		chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchMouseEvent(input.MouseReleased, at[2], at[3]).WithButton(input.Left).WithClickCount(1).Do(c)
		}),
	); err != nil {
		t.Fatal(err)
	}
	want := strings.Join(append([]string{before[1], before[2], before[3], before[0]}, before[4:]...), ",")
	waitJS(t, ctx, "the card in its new place", fmt.Sprintf(handIDsJS+`.join(",") === %q`, want), 5*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('.zone-trick .pc-slot').length`); n != 0 {
		t.Errorf("%d cards went into the trick: the drag played a card", n)
	}
	if !evalBool(t, ctx, fmt.Sprintf(`Object.values(JSON.parse(localStorage.getItem("bingle.handOrder")) || {}).some(ids => ids.join(",") === %q)`, want)) {
		t.Error("the order the cards were dragged into was not remembered")
	}
	// Opened again, the hand is in the order this browser left it in.
	openGameAs(t, ctx, baseURL, room, "nyet", "Ana")
	waitJS(t, ctx, "the hand again", `document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face').length >= 10`, 15*time.Second)
	if got := evalString(t, ctx, handIDsJS+`.join(",")`); got != want {
		t.Errorf("the hand reads %s after the page was opened again, want %s", got, want)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
