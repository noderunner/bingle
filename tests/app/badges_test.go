// Badges seen (ADR 0130): GET /api/players/{handle}/badges for accounts only,
// the room for the holder alone, and the profile page's list.

package app_test

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/web/look"
)

type badgesView struct {
	Badges []struct {
		Badge string `json:"badge"`
		Name  string `json:"name"`
		Game  string `json:"game"`
		Room  string `json:"room"`
	} `json:"badges"`
}

// winFirst plays an unrated game of Yavalath that the loser resigns, so the
// winner earns a First Win, and waits for it to be written.
func winFirst(t *testing.T, winner *browser, winnerName string, loser *browser, loserName string) string {
	t.Helper()
	var made struct {
		RoomCode string `json:"room_code"`
	}
	if code := winner.do("POST", "/api/games", map[string]any{"game_def_id": "yavalath", "player_name": winnerName}, &made); code != 200 {
		t.Fatalf("a game: %d", code)
	}
	path := "/api/games/" + made.RoomCode
	if code := loser.do("POST", path+"/join", map[string]any{"player_name": loserName}, nil); code != 200 {
		t.Fatalf("joining: %d", code)
	}
	for _, mv := range []struct {
		b    *browser
		name string
		n    string
	}{{winner, winnerName, "e5"}, {loser, loserName, "d4"}} {
		if code := mv.b.do("POST", path+"/moves", map[string]any{"player_name": mv.name, "notation": mv.n}, nil); code != 200 {
			t.Fatalf("a move: %d", code)
		}
	}
	if code := loser.do("POST", path+"/resign", map[string]any{"player_name": loserName}, nil); code != 200 {
		t.Fatalf("resigning: %d", code)
	}
	waitFor(t, "the First Win", func() bool {
		var v badgesView
		return winner.do("GET", "/api/players/"+winnerName+"/badges", nil, &v) == 200 && len(v.Badges) == 1
	})
	return made.RoomCode
}

func TestBadges_API(t *testing.T) {
	a, base := membersServer(t)
	ana := member(t, a, base, "ana")
	bea := member(t, a, base, "bea")
	guest := newBrowser(t, base)
	room := winFirst(t, ana, "ana", bea, "bea")

	var own badgesView
	if code := ana.do("GET", "/api/players/ana/badges", nil, &own); code != 200 || len(own.Badges) != 1 || own.Badges[0].Badge != "first-win" || own.Badges[0].Room != room || own.Badges[0].Game == "" {
		t.Errorf("her own badges: %d, %+v", code, own)
	}
	var theirs badgesView
	if code := bea.do("GET", "/api/players/ana/badges", nil, &theirs); code != 200 || len(theirs.Badges) != 1 || theirs.Badges[0].Room != "" {
		t.Errorf("another account reads hers: %d, %+v, want First Win without its room", code, theirs)
	}
	if code := guest.do("GET", "/api/players/ana/badges", nil, nil); code != 403 {
		t.Errorf("a guest reads hers: %d, want 403", code)
	}
	if code := bea.do("GET", "/api/players/nobody/badges", nil, nil); code != 404 {
		t.Errorf("an unknown player: %d, want 404", code)
	}
}

func TestBadges_TheProfileListsThem(t *testing.T) {
	a, base := membersServer(t)
	ana := member(t, a, base, "ana")
	bea := member(t, a, base, "bea")

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "no badges yet", `document.getElementById("badges-box").hidden === false && document.getElementById("badges-empty").hidden === false`, 15*time.Second)
	// The badges not yet earned are there too, greyed out.
	waitJS(t, ctx, "the badges to earn", `document.querySelector('#badges-list li[data-badge="first-win"][data-earned="false"].unearned') !== null`, 15*time.Second)
	// A badge is drawn with the trophy, its game having no icon of its own,
	// and is no button: there are no details to open.
	if !evalBool(t, ctx, `document.querySelector('#badges-list li[data-badge="well-met"] .badge-medal.unearned svg[data-icon="trophy"]') !== null`) {
		t.Error("a platform badge is not drawn with the trophy")
	}
	if evalBool(t, ctx, `document.querySelector('#badges-list li button, #badge-dialog') !== null`) {
		t.Error("a badge on the shelf can be clicked")
	}

	winFirst(t, ana, "ana", bea, "bea")
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the First Win", `document.querySelector('#badges-list li[data-badge="first-win"][data-earned="true"] .badge-name') !== null`, 15*time.Second)
	if got := evalString(t, ctx, `document.querySelector('#badges-list li[data-badge="first-win"] .badge-name').textContent`); got != "First Win" {
		t.Errorf("the profile names it %q", got)
	}
	if !evalBool(t, ctx, `document.getElementById("badges-empty").hidden`) {
		t.Error("the profile still says there are none")
	}
}

// A handle in a chat shows the badges its account holds, even a friend's,
// which offers nothing else.
func TestBadges_AHandleShowsTheBadges(t *testing.T) {
	a, base := membersServer(t)
	anaB, beaB := member(t, a, base, "ana"), member(t, a, base, "bea")
	befriend(t, anaB, beaB, "bea", "ana")
	winFirst(t, beaB, "bea", anaB, "ana")
	if code := beaB.do("POST", "/api/chat", map[string]any{"player_name": "bea", "text": "good game"}, nil); code != 200 {
		t.Fatalf("the global chat: %d", code)
	}
	ana, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	signInChrome(t, ana, base, anaB.cookie(auth.SessionCookie))
	if err := chromedp.Run(ana, chromedp.EmulateViewport(1400, 900), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitScripts(t, ana)
	clickSel(t, ana, "#tab-chat")
	waitJS(t, ana, "the handle", `document.querySelector('.handle-badge[data-handle="bea"]') !== null`, 15*time.Second)
	clickSel(t, ana, `.handle-badge[data-handle="bea"]`)
	waitJS(t, ana, "her badges", `document.querySelector('#pop-badges [data-badge="first-win"]') !== null`, 10*time.Second)
	if evalBool(t, ana, `document.getElementById("friend-pop-send") !== null`) {
		t.Error("a friend is offered a friend request")
	}
}

// badgesGameJSON is a game of placing stones with badges of its own.
const badgesGameJSON = `{
  "id": "stones-badges",
  "name": "Stones with badges",
  "board": { "generator": { "type": "grid", "width": 4, "height": 4, "adjacency": "orthogonal4", "labels": "algebraic" } },
  "seats": [ { "seat": 1, "color": "black" }, { "seat": 2, "color": "white" } ],
  "pieces": { "black": { "color": "black" }, "white": { "color": "white" } },
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "actions": [ { "id": "place", "type": "place", "pieces": ["black", "white"], "source": "supply", "fact": "placed" } ],
  "facts": { "placed": { "description": "A stone is placed." } },
  "badges": [
    { "id": "builder", "name": "Builder", "description": "Place three stones in a game.", "when": "end",
      "count": { "fact": "placed", "at_least": 3 } },
    { "id": "bot-beater", "name": "Bot Beater", "description": "Beat the tactical bot or better.", "when": "end",
      "won": true, "bot": { "at_least": "tactical" } }
  ],
  "win_condition": { "type": "none" }
}`

type badgeCatalogView struct {
	Badges []struct {
		Badge    string `json:"badge"`
		Name     string `json:"name"`
		Platform bool   `json:"platform"`
		Family   string `json:"family"`
		Game     string `json:"game"`
		Kind     string `json:"kind"`
		Bot      string `json:"bot"`
		Bots     bool   `json:"bots"`
		When     string `json:"when"`
	} `json:"badges"`
}

// Every badge there is to earn, for anybody: the platform's first, then the
// games'.
func TestBadges_TheCatalog(t *testing.T) {
	a, base := membersServer(t)
	if err := a.Catalog().Register("stones-badges", []byte(badgesGameJSON), nil); err != nil {
		t.Fatal(err)
	}
	var v badgeCatalogView
	if code := newBrowser(t, base).do("GET", "/api/badges", nil, &v); code != 200 || len(v.Badges) == 0 {
		t.Fatalf("a guest reads the badges: %d, %+v", code, v)
	}
	first := v.Badges[0]
	if first.Badge != "first-win" || !first.Platform || first.Kind != "player" || first.When != "end" {
		t.Errorf("the first badge is %+v, want the platform's First Win", first)
	}
	kinds := map[string]string{}
	for _, b := range v.Badges {
		kinds[b.Badge] = b.Kind
		switch b.Badge {
		case "machine-breaker":
			if b.Bot == "" {
				t.Errorf("Machine Breaker names no bot: %+v", b)
			}
		case "first-win":
			// Earned both ways.
			if !b.Bots {
				t.Errorf("%s is not earned in games with bots: %+v", b.Badge, b)
			}
		case "well-met", "contender":
			if b.Bots {
				t.Errorf("%s, which counts people or rated games, can be earned against bots: %+v", b.Badge, b)
			}
		case "stones-badges.builder":
			if b.Platform || b.Family != "stones-badges" || b.Game != "Stones with badges" || !b.Bots {
				t.Errorf("a game's badge, earned both ways: %+v", b)
			}
		}
	}
	if kinds["contender"] != "rated" || kinds["machine-breaker"] != "bot" || kinds["stones-badges.bot-beater"] != "bot" || kinds["stones-badges.builder"] != "player" {
		t.Errorf("the kinds: %v", kinds)
	}
}

// Another player's page shows what they hold, not what they have yet to
// earn, and marks what the reader holds too.
func TestBadges_APlayersPage(t *testing.T) {
	a, base := membersServer(t)
	anaB, beaB := member(t, a, base, "ana"), member(t, a, base, "bea")
	winFirst(t, anaB, "ana", beaB, "bea")
	winFirst(t, beaB, "bea", anaB, "ana")

	bea, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, bea, base, beaB.cookie(auth.SessionCookie))
	if err := chromedp.Run(bea, chromedp.Navigate(base+"/players/ana")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, bea, "her badges", `document.querySelector('#player-badges li[data-badge="first-win"]') !== null`, 15*time.Second)
	if n := evalString(t, bea, `String(document.querySelectorAll('#player-badges li').length)`); n != "1" {
		t.Errorf("her page lists %s badges, want only the one she holds", n)
	}
	if !evalBool(t, bea, `document.querySelector('#player-badges li[data-badge="first-win"] .badge-also') !== null`) {
		t.Error("her page does not say the reader holds it too")
	}

	if err := chromedp.Run(bea, chromedp.Navigate(base+"/players/nobody")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, bea, "no such player", `document.getElementById("player-note").textContent.includes("no player @nobody")`, 15*time.Second)
}

// The rules window of a game with badges has a tab of them.
func TestBadges_TheRulesWindowListsTheGamesBadges(t *testing.T) {
	a, base := membersServer(t)
	if err := a.Catalog().Register("stones-badges", []byte(badgesGameJSON), nil); err != nil {
		t.Fatal(err)
	}
	ana := member(t, a, base, "ana")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/?game=stones-badges")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the setup", `!document.getElementById("game-setup").hidden`, 15*time.Second)
	clickSel(t, ctx, "#game-rules-button")
	waitJS(t, ctx, "the Badges tab", `!!document.querySelector('#rules-dialog [data-tab="badges"]')`, 10*time.Second)
	clickSel(t, ctx, `#rules-dialog [data-tab="badges"]`)
	waitJS(t, ctx, "the game's badges", `document.querySelector('#rules-badges li[data-badge="stones-badges.builder"].unearned') !== null`, 10*time.Second)
	if evalBool(t, ctx, `document.querySelector('#rules-badges li[data-badge="first-win"]') !== null`) {
		t.Error("the rules window lists the platform's badges")
	}
}

// quickWinJSON is a game the first seat wins by placing on the top row, with
// a badge for winning.
const quickWinJSON = `{
  "id": "quick-win",
  "name": "Quick win",
  "board": { "generator": { "type": "grid", "width": 3, "height": 2, "adjacency": "orthogonal4", "labels": "algebraic" } },
  "seats": [ { "seat": 1, "color": "black" }, { "seat": 2, "color": "white" } ],
  "pieces": { "stone": { "shape": "disc" } },
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "actions": [ { "id": "place", "type": "place", "source": "supply" } ],
  "badges": [
    { "id": "quick", "name": "Quick", "description": "Win a quick game.", "when": "end",
      "won": true }
  ],
  "win_condition": { "type": "rules", "rules": [ { "id": "first", "type": "reach", "seat": 1, "region": "top", "reason": "first" } ] }
}`

// A badge earned in a game with bots and in one of people only is one medal,
// with a bot at its lower left and a gold star at its lower right.
func TestBadges_TheWaysABadgeWasEarned(t *testing.T) {
	a, base := membersServer(t)
	if err := a.Catalog().Register("quick-win", []byte(quickWinJSON), nil); err != nil {
		t.Fatal(err)
	}
	anaB, beaB := member(t, a, base, "ana"), member(t, a, base, "bea")
	ways := func() []string {
		var v struct {
			Badges []struct{ Badge, Against string } `json:"badges"`
		}
		anaB.do("GET", "/api/players/ana/badges", nil, &v)
		out := []string{}
		for _, b := range v.Badges {
			if b.Badge == "quick-win.quick" {
				out = append(out, b.Against)
			}
		}
		return out
	}
	var made struct {
		RoomCode string `json:"room_code"`
	}
	if code := anaB.do("POST", "/api/games", map[string]any{"game_def_id": "quick-win", "player_name": "ana", "opponent": "tactical"}, &made); code != 200 {
		t.Fatalf("a game against the tactical bot: %d", code)
	}
	winAt := func(b *browser, room string) {
		t.Helper()
		// The bottom row, then, once the other seat has moved, the top.
		if code := b.do("POST", "/api/games/"+room+"/moves", map[string]any{"player_name": "ana", "notation": "a1"}, nil); code != 200 {
			t.Fatalf("the first stone: %d", code)
		}
		won := false
		waitFor(t, "the winning stone", func() bool {
			for _, at := range []string{"a2", "b2", "c2"} {
				if b.do("POST", "/api/games/"+room+"/moves", map[string]any{"player_name": "ana", "notation": at}, nil) == 200 {
					won = true
					return true
				}
			}
			return false
		})
		if !won {
			t.Fatal("no winning stone")
		}
	}
	winAt(anaB, made.RoomCode)
	waitFor(t, "the badge against bots", func() bool { return len(ways()) == 1 })

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, anaB.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	item := `#badges-list li[data-badge="quick-win.quick"]`
	waitJS(t, ctx, "earned against bots", `document.querySelector('`+item+`[data-ways="bots"] .medal-bot') !== null`, 15*time.Second)
	if evalBool(t, ctx, `document.querySelector('`+item+` .medal-people') !== null`) {
		t.Error("a badge earned against bots only has the gold star")
	}

	if code := anaB.do("POST", "/api/games", map[string]any{"game_def_id": "quick-win", "player_name": "ana"}, &made); code != 200 {
		t.Fatalf("a game: %d", code)
	}
	if code := beaB.do("POST", "/api/games/"+made.RoomCode+"/join", map[string]any{"player_name": "bea"}, nil); code != 200 {
		t.Fatalf("joining: %d", code)
	}
	if code := anaB.do("POST", "/api/games/"+made.RoomCode+"/moves", map[string]any{"player_name": "ana", "notation": "a1"}, nil); code != 200 {
		t.Fatalf("the first stone: %d", code)
	}
	if code := beaB.do("POST", "/api/games/"+made.RoomCode+"/moves", map[string]any{"player_name": "bea", "notation": "b1"}, nil); code != 200 {
		t.Fatalf("bea's stone: %d", code)
	}
	if code := anaB.do("POST", "/api/games/"+made.RoomCode+"/moves", map[string]any{"player_name": "ana", "notation": "a2"}, nil); code != 200 {
		t.Fatalf("the winning stone: %d", code)
	}
	waitFor(t, "the badge against people", func() bool { return len(ways()) == 2 })
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "earned both ways", `document.querySelectorAll('`+item+`').length === 1 && document.querySelector('`+item+`[data-ways="people bots"] .medal-bot') !== null && document.querySelector('`+item+` .medal-people') !== null`, 15*time.Second)
}

// A game draws its own badges with the icons its web.yaml names among its
// files, the rest with the trophy, and a medal's marks sit on that icon.
func TestBadges_AGameDrawsItsBadgesIcons(t *testing.T) {
	a, base := membersServer(t)
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle cx="5" cy="5" r="4" fill="green"/></svg>`
	if err := a.Catalog().RegisterModule(catalog.Module{
		ID:         "stones-badges",
		Definition: []byte(badgesGameJSON),
		Assets:     fstest.MapFS{"icons/builder.svg": {Data: []byte(svg)}},
		Web:        []byte(`{"badges": {"builder": {"icon": "icons/builder.svg"}}}`),
	}); err != nil {
		t.Fatal(err)
	}
	web := []byte(`{"badges": {"builder": {"icon": "icons/builder.svg"}}}`)
	cg, _ := a.Catalog().Get("stones-badges")
	if err := look.Check(web, "stones-badges", map[string]*catalog.CompiledGame{"stones-badges": cg}); err != nil {
		t.Fatal(err)
	}
	ana := member(t, a, base, "ana")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile#badges")); err != nil {
		t.Fatal(err)
	}
	builder := `#badges-list li[data-badge="stones-badges.builder"] .badge-medal`
	waitJS(t, ctx, "the game's icon", `document.querySelector('`+builder+` img.badge-icon[src="/api/game-defs/stones-badges/assets/icons/builder.svg"]') !== null`, 15*time.Second)
	waitJS(t, ctx, "it loads", `document.querySelector('`+builder+` img').complete && document.querySelector('`+builder+` img').naturalWidth > 0`, 10*time.Second)
	if !evalBool(t, ctx, `document.querySelector('#badges-list li[data-badge="stones-badges.bot-beater"] .badge-medal svg[data-icon="trophy"]') !== null`) {
		t.Error("a game's badge without an icon is not drawn with the trophy")
	}
}

// At a seat, a player's @handle opens the menu of their badges and the way to
// their page, and your own the way to yours.
func TestBadges_AHandleAtASeatOpensItsMenu(t *testing.T) {
	a, base := membersServer(t)
	anaB, beaB := member(t, a, base, "ana"), member(t, a, base, "bea")
	room := winFirst(t, beaB, "bea", anaB, "ana")

	ana, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ana, base, anaB.cookie(auth.SessionCookie))
	if err := chromedp.Run(ana, chromedp.EmulateViewport(1400, 900), chromedp.Navigate(base+"/game/"+room)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "bea's seat", `document.querySelector('button.badge.account[data-handle="bea"]') !== null`, 20*time.Second)
	clickSel(t, ana, `button.badge.account[data-handle="bea"]`)
	waitJS(t, ana, "her menu", `document.querySelector('#friend-pop #pop-badges [data-badge="first-win"]') !== null && document.querySelector('#friend-pop #pop-player[href="/players/bea"]') !== null`, 10*time.Second)
	if err := chromedp.Run(ana, chromedp.KeyEvent("\x1b")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "the menu closed", `document.getElementById("friend-pop") === null`, 5*time.Second)
	clickSel(t, ana, `button.badge.account[data-handle="ana"]`)
	waitJS(t, ana, "her own menu", `document.querySelector('#friend-pop #pop-player[href="/players/ana"]')?.textContent === "Your page"`, 10*time.Second)
	if evalBool(t, ana, `document.getElementById("friend-pop-send") !== null`) {
		t.Error("her own handle offers a friend request")
	}

	// A guest watching: the way to the page, and nothing else.
	guest, cancelGuest := newChromedpContext(t, 60*time.Second)
	defer cancelGuest()
	if err := chromedp.Run(guest, chromedp.EmulateViewport(1400, 900), chromedp.Navigate(base+"/game/"+room)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, guest, "bea's seat", `document.querySelector('button.badge.account[data-handle="bea"]') !== null`, 20*time.Second)
	clickSel(t, guest, `button.badge.account[data-handle="bea"]`)
	waitJS(t, guest, "the way to her page", `document.querySelector('#friend-pop #pop-player[href="/players/bea"]') !== null`, 10*time.Second)
	if evalBool(t, guest, `document.querySelector('#friend-pop [data-badge]') !== null`) {
		t.Error("a guest is shown badges")
	}
}

// The settings page has a tab each for the settings, the badges (an
// account's) and the finished games, and its address can name one.
func TestBadges_TheProfileHasTabs(t *testing.T) {
	a, base := membersServer(t)
	ana := member(t, a, base, "ana")

	guest, cancelGuest := newChromedpContext(t, 60*time.Second)
	defer cancelGuest()
	if err := chromedp.Run(guest, chromedp.Navigate(base+"/profile#badges")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, guest, "the settings", `document.getElementById("panel-settings").hidden === false`, 10*time.Second)
	if !evalBool(t, guest, `document.getElementById("tab-badges").hidden && document.getElementById("panel-badges").hidden`) {
		t.Error("a guest has a Badges tab")
	}
	clickSel(t, guest, "#tab-finished")
	waitJS(t, guest, "the finished games", `!document.getElementById("panel-finished").hidden && document.getElementById("panel-settings").hidden && location.hash === "#finished"`, 5*time.Second)

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile#badges")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the Badges tab, open", `!document.getElementById("tab-badges").hidden && !document.getElementById("panel-badges").hidden && document.getElementById("panel-settings").hidden`, 15*time.Second)
	// The tab opens before the shelf, which fetches the badges' descriptions, is drawn.
	waitJS(t, ctx, "the badges on the tab", `document.querySelector('#panel-badges #badges-list li[data-badge="first-win"]') !== null`, 10*time.Second)
	clickSel(t, ctx, "#tab-settings")
	waitJS(t, ctx, "the settings", `!document.getElementById("panel-settings").hidden && document.getElementById("panel-badges").hidden && location.hash === ""`, 5*time.Second)
}
