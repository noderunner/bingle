package app_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// solitaireRoom starts a game for Ana.
func solitaireRoom(t *testing.T, baseURL, def, set string) string {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
	}
	body := map[string]any{"game_def_id": def, "player_name": "Ana"}
	if set != "" {
		body["card_set"] = set
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create %s: %d", def, code)
	}
	if created.Status != "active" {
		t.Fatalf("%s: a game for one starts at once, but it is %q", def, created.Status)
	}
	return created.RoomCode
}

// The first move the server offers that has this shape, with the number of moves there are.
func legalNotations(t *testing.T, baseURL, room string) []string {
	t.Helper()
	var res struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"moves"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves/legal", nil, &res); code != 200 {
		t.Fatalf("legal: %d", code)
	}
	out := make([]string, len(res.Moves))
	for i, m := range res.Moves {
		out[i] = m.Notation
	}
	return out
}

func moveCount(t *testing.T, baseURL, room string) int {
	t.Helper()
	var g struct {
		MoveSeq int `json:"move_seq"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &g); code != 200 {
		t.Fatalf("game: %d", code)
	}
	return g.MoveSeq
}

// The stock is one click; a card with one place to go takes one click too; a card
// with several waits for where; clicking it again lets go of it.
func TestWebSolitaire_ClickingCards(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := solitaireRoom(t, baseURL, "solitaire", "")
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "solitaire", "Ana")
	waitJS(t, ctx, "the table", `document.querySelectorAll('.sol-column').length === 7`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('.sol-column .pcard.back').length`); n != 21 {
		t.Errorf("%d cards face down, want 21", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('.sol-column .pcard.face').length`); n != 7 {
		t.Errorf("%d cards face up, want 7", n)
	}
	// The stock: straight onto the waste, without a Submit.
	clickSel(t, ctx, `#sol-stock`)
	waitJS(t, ctx, "the waste", `document.querySelectorAll('.sol-waste .pcard.face').length === 1`, 10*time.Second)
	if got := moveCount(t, baseURL, room); got != 1 {
		t.Errorf("%d moves after the stock", got)
	}
	// Play the stock until a move on the table turns up, then make it with the page.
	for i := 0; i < 60; i++ {
		var one string
		for _, m := range legalNotations(t, baseURL, room) {
			if strings.HasPrefix(m, "t") && !strings.Contains(m, ".") && !strings.HasSuffix(m, "-f") {
				one = m
				break
			}
		}
		if one == "" {
			before := moveCount(t, baseURL, room)
			clickSel(t, ctx, `#sol-stock`)
			waitJS(t, ctx, "the stock's move", fmt.Sprintf(`fetch('/api/games/%s').then(r=>r.json()).then(g=>g.move_seq>%d)`, room, before), 10*time.Second)
			continue
		}
		from, to, _ := strings.Cut(one, "-")
		before := moveCount(t, baseURL, room)
		clickSel(t, ctx, fmt.Sprintf(`button[data-from="from-%s-1"]`, from))
		// One place to go: it has gone. Several: the places light up, and we click the one.
		if evalInt(t, ctx, `document.querySelectorAll('.sol-target').length`) > 0 {
			clickSel(t, ctx, fmt.Sprintf(`[data-pile="%s"] button:last-of-type`, to))
		}
		waitJS(t, ctx, "the card's move", fmt.Sprintf(`fetch('/api/games/%s').then(r=>r.json()).then(g=>g.move_seq>%d)`, room, before), 10*time.Second)
		return
	}
	t.Log("this deal offered no move on the table")
}

// A card can be dragged to where it goes; a drop anywhere else plays nothing.
func TestWebSolitaire_DraggingACard(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var room, from, to string
	// A deal with a move between columns in it from the first position.
	for i := 0; i < 40 && from == ""; i++ {
		room = solitaireRoom(t, baseURL, "solitaire", "")
		for _, m := range legalNotations(t, baseURL, room) {
			if strings.HasPrefix(m, "t") && !strings.Contains(m, ".") && !strings.HasSuffix(m, "-f") {
				from, to, _ = strings.Cut(m, "-")
				break
			}
		}
	}
	if from == "" {
		t.Skip("no deal with a move between columns")
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "solitaire", "Ana")
	waitJS(t, ctx, "the table", `document.querySelectorAll('.sol-column').length === 7`, 15*time.Second)
	drag := func(target string) {
		t.Helper()
		js := fmt.Sprintf(`(function(){
			const c = document.querySelector('button[data-from="from-%s-1"]');
			const d = document.querySelector('%s');
			const a = c.getBoundingClientRect(), b = d.getBoundingClientRect();
			const opt = (x, y) => ({bubbles:true, clientX:x, clientY:y, button:0, pointerId:1});
			c.dispatchEvent(new PointerEvent('pointerdown', opt(a.left+5, a.top+5)));
			window.dispatchEvent(new PointerEvent('pointermove', opt(a.left+30, a.top+30)));
			window.dispatchEvent(new PointerEvent('pointermove', opt(b.left+10, b.bottom-10)));
			window.dispatchEvent(new PointerEvent('pointerup', opt(b.left+10, b.bottom-10)));
			return true })()`, from, target)
		if err := chromedp.Run(ctx, chromedp.Evaluate(js, nil)); err != nil {
			t.Fatal(err)
		}
	}
	// Onto the stock: nothing happens, and the ghost has gone.
	drag(`#sol-stock`)
	waitJS(t, ctx, "the ghost gone", `!document.querySelector('.sol-ghost')`, 5*time.Second)
	if n := moveCount(t, baseURL, room); n != 0 {
		t.Errorf("a drop on the stock played %d moves", n)
	}
	// Onto its column: played.
	drag(fmt.Sprintf(`[data-pile="%s"]`, to))
	waitJS(t, ctx, "the drop's move", fmt.Sprintf(`fetch('/api/games/%s').then(r=>r.json()).then(g=>g.move_seq===1)`, room), 10*time.Second)
}

// At a themed table every card wears the set, and where colours are the rule each
// shows the red or black it counts as. Spider, where they are not, has no such mark.
func TestWebSolitaire_ThemedCardsSayTheirColour(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	room := solitaireRoom(t, baseURL, "freecell", "amphibians")
	openGameAs(t, ctx, baseURL, room, "freecell", "Ana")
	waitJS(t, ctx, "the table", `document.querySelectorAll('.sol-column .pcard.face').length === 52`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('.sol-column .pcard.face.themed[data-set="amphibians"]').length`); n != 52 {
		t.Errorf("%d cards wear the set", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('.sol-column .pcard.face .sol-colour').length`); n != 52 {
		t.Errorf("%d cards say their colour", n)
	}
	got := evalString(t, ctx, `[...new Set([...document.querySelectorAll('.sol-colour')].map(e => e.textContent))].sort().join("")`)
	if got != "♠♣♥♦" {
		t.Errorf("the marks are %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('.sol-top [data-pile^="f"] svg').length`); n != 4 {
		t.Errorf("%d foundations wear the set's suits", n)
	}
	room = solitaireRoom(t, baseURL, "spider-2", "lovecraftian")
	openGameAs(t, ctx, baseURL, room, "spider-2", "Ana")
	waitJS(t, ctx, "the table", `document.querySelectorAll('.sol-column').length === 10`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('.sol-column .pcard.face[data-set="lovecraftian"]').length`); n != 10 {
		t.Errorf("Spider: %d cards wear the set", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('.sol-colour').length`); n != 0 {
		t.Errorf("Spider: %d colour marks", n)
	}
}

// Giving up ends the game with no winner, and the page says so.
func TestWebSolitaire_GivingUp(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := solitaireRoom(t, baseURL, "freecell", "")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "freecell", "Ana")
	waitJS(t, ctx, "the table", `document.querySelector('.sol-columns')`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById('resign').textContent`); got != "Give up" {
		t.Errorf("the button says %q", got)
	}
	clickSel(t, ctx, `#resign`)
	waitJS(t, ctx, "are you sure", `document.getElementById('resign').textContent.includes("Really")`, 5*time.Second)
	clickSel(t, ctx, `#resign`)
	waitJS(t, ctx, "the ending", `document.body.textContent.includes("you gave up")`, 10*time.Second)
	if strings.Contains(evalString(t, ctx, `document.body.textContent`), "a draw") {
		t.Error("a game for one called it a draw")
	}
}

// Back takes back a move, even one that turned a card over: there is nobody to wrong.
func TestWebSolitaire_MovesCanBeTakenBack(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := solitaireRoom(t, baseURL, "solitaire", "")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "solitaire", "Ana")
	waitJS(t, ctx, "the table", `document.querySelector('#sol-stock')`, 15*time.Second)
	clickSel(t, ctx, `#sol-stock`)
	waitJS(t, ctx, "the waste", `document.querySelectorAll('.sol-waste .pcard.face').length === 1`, 10*time.Second)
	clickSel(t, ctx, `#undo-step`)
	waitJS(t, ctx, "the card back", `document.querySelectorAll('.sol-waste .pcard.face').length === 0`, 10*time.Second)
	if got := moveCount(t, baseURL, room); got != 0 {
		t.Errorf("%d moves after Back", got)
	}
}

// The tutorial plays from the first page to the spare, by clicking pins and balls.
func TestWebBowling_TheTutorial(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/tutorials", map[string]any{"game_def_id": "bowling-solitaire", "tutorial": "bowling-basics", "player_name": "Ana"}, &created); code != 200 {
		t.Fatalf("tutorial: %d", code)
	}
	room := created.RoomCode
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "bowling-solitaire", "Ana")
	waitJS(t, ctx, "the rack", `document.querySelectorAll('#sol-rack .sol-pin').length === 10`, 15*time.Second)
	next := func() {
		t.Helper()
		clickSel(t, ctx, `#tutorial-next`)
	}
	throw := func(pins []string, ball int, seq int) {
		t.Helper()
		waitJS(t, ctx, "the step's pins", fmt.Sprintf(`document.querySelector('#sol-rack button.sol-pin.sol-wanted[data-pin="%s"]') && document.querySelectorAll('#sol-rack .sol-wanted').length === %d`, pins[0], len(pins)), 10*time.Second)
		// A pin the step does not want cannot be clicked, and no ball yet.
		if n := evalInt(t, ctx, `document.querySelectorAll('#sol-rack button.sol-pin:not(.sol-wanted)').length`); n != 0 {
			t.Errorf("%d other pins can be clicked", n)
		}
		if n := evalInt(t, ctx, `document.querySelectorAll('#sol-balls button').length`); n != 0 {
			t.Errorf("%d balls can be thrown before the pins are chosen", n)
		}
		// In reverse order: the order of the clicks must not matter.
		for i := len(pins) - 1; i >= 0; i-- {
			clickSel(t, ctx, fmt.Sprintf(`#sol-rack button.sol-pin[data-pin="%s"]`, pins[i]))
		}
		sel := fmt.Sprintf(`#sol-balls button[data-throw="%d"]`, ball)
		waitJS(t, ctx, "the ball lit", fmt.Sprintf(`document.querySelector('%s')`, sel), 10*time.Second)
		clickSel(t, ctx, sel)
		waitJS(t, ctx, "the throw", fmt.Sprintf(`fetch('/api/games/%s').then(r=>r.json()).then(g=>g.move_seq===%d)`, room, seq), 10*time.Second)
	}
	next()
	next()
	throw([]string{"e"}, 1, 1)
	throw([]string{"h", "i", "j"}, 1, 2)
	throw([]string{"a", "b", "f"}, 3, 3)
	// The first ball ended by itself: the sheet says 7, and the second ball is on.
	waitJS(t, ctx, "the first ball on the sheet", `document.querySelector('#sol-sheet .sol-frame[data-frame="1"] .sol-mark').textContent === "7"`, 10*time.Second)
	next()
	throw([]string{"c", "d", "g"}, 1, 4)
	// A spare, and frame 2 on a new rack.
	waitJS(t, ctx, "the spare on the sheet", `[...document.querySelectorAll('#sol-sheet .sol-frame[data-frame="1"] .sol-mark')].map(e => e.textContent).join("") === "7/"`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#sol-rack .sol-pin:not(.down)').length`); n != 10 {
		t.Errorf("%d pins on the new rack", n)
	}
}

// Outside a tutorial, any pin that can be part of a play can be chosen and let go of,
// a ball lights only for the pins chosen, and the score sheet has its ten frames.
func TestWebBowling_ChoosingPinsAndABall(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := solitaireRoom(t, baseURL, "bowling-solitaire", "ancient")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "bowling-solitaire", "Ana")
	waitJS(t, ctx, "the rack", `document.querySelectorAll('#sol-rack .sol-pin').length === 10`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#sol-sheet .sol-frame').length`); n != 10 {
		t.Errorf("%d frames on the sheet", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#sol-balls .pcard.face').length`); n != 3 {
		t.Errorf("%d balls showing", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#sol-rack .pcard[data-set="ancient"]').length`); n != 10 {
		t.Errorf("%d pins wear the set", n)
	}
	// The back row cannot be reached by the first play.
	for _, p := range "abcd" {
		if n := evalInt(t, ctx, fmt.Sprintf(`document.querySelectorAll('#sol-rack button.sol-pin[data-pin="%c"]').length`, p)); n != 0 {
			t.Errorf("pin %c can be chosen on the first play", p)
		}
	}
	var pins, ball string
	for _, m := range legalNotations(t, baseURL, room) {
		if strings.Contains(m, "@") {
			pins, ball, _ = strings.Cut(m, "@")
			break
		}
	}
	if pins == "" {
		t.Fatal("no play on this deal")
	}
	letters := strings.Split(pins, "+")
	for _, p := range letters {
		clickSel(t, ctx, fmt.Sprintf(`#sol-rack button.sol-pin[data-pin="%s"]`, p))
	}
	sel := fmt.Sprintf(`#sol-balls button[data-throw="%s"]`, ball)
	waitJS(t, ctx, "the ball lit", fmt.Sprintf(`document.querySelector('%s')`, sel), 10*time.Second)
	// Letting go of a pin takes the ball's light away.
	clickSel(t, ctx, fmt.Sprintf(`#sol-rack button.sol-pin[data-pin="%s"]`, letters[0]))
	waitJS(t, ctx, "the ball dark", fmt.Sprintf(`!document.querySelector('%s')`, sel), 10*time.Second)
	clickSel(t, ctx, fmt.Sprintf(`#sol-rack button.sol-pin[data-pin="%s"]`, letters[0]))
	clickSel(t, ctx, sel)
	waitJS(t, ctx, "the throw", fmt.Sprintf(`fetch('/api/games/%s').then(r=>r.json()).then(g=>g.move_seq===1)`, room), 10*time.Second)
	waitJS(t, ctx, "the pins down", fmt.Sprintf(`document.querySelectorAll('#sol-rack .sol-pin.down').length === %d`, len(letters)), 10*time.Second)
}

// The game is on the home page, for one player, with its variants; the form asks for
// no opponent and offers the sets of cards.
func TestWebSolitaire_SettingUp(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the card", `document.querySelector('.game-card[data-family="solitaire"]')`, 15*time.Second)
	if got := evalString(t, ctx, `document.querySelector('.game-card[data-family="solitaire"] .game-card-players').textContent`); got != "1 player" {
		t.Errorf("the card says %q", got)
	}
	clickSel(t, ctx, `.game-card[data-family="solitaire"]`)
	waitJS(t, ctx, "the setup", `document.querySelectorAll('#setup-variants .chip[data-id]').length === 7`, 10*time.Second)
	waitJS(t, ctx, "no opponent to choose", `document.getElementById('opponent-choices').closest('.field').hidden`, 10*time.Second)
	waitJS(t, ctx, "the sets of cards", `!document.getElementById('card-set-field').hidden`, 10*time.Second)
	clickSel(t, ctx, `#setup-variants .chip[data-id="bowling-solitaire"]`)
	waitJS(t, ctx, "the tutorial offered", `document.body.textContent.includes("How to play Bowling Solitaire")`, 10*time.Second)
	if err := chromedp.Run(ctx, chromedp.SetValue(`#create_player_name`, "Ana", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	waitJS(t, ctx, "the game, started", `document.querySelectorAll('#sol-rack .sol-pin').length === 10`, 15*time.Second)
}
