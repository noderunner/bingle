package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// administrator makes an account in the admin group, signed in.
func administrator(t *testing.T, a *app.App, base, handle string) *browser {
	t.Helper()
	pw, err := a.Accounts().Create(context.Background(), handle, handle+"@example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	b := newBrowser(t, base)
	if code := b.login(handle, pw); code != 200 {
		t.Fatalf("@%s signs in: %d", handle, code)
	}
	if code := b.do("POST", "/auth/password", map[string]any{"old": pw, "new": handle + "'s own password"}, nil); code != 200 {
		t.Fatalf("@%s sets a password: %d", handle, code)
	}
	return b
}

func cupBody() map[string]any {
	return map[string]any{"name": "The cup", "game_def_id": "yavalath", "format": "swiss", "clock_seconds": 86400}
}

// Through the API: who may do what, by the capabilities.
func TestTournaments_API(t *testing.T) {
	a, base := membersServer(t)
	org := administrator(t, a, base, "org")
	ana := member(t, a, base, "ana")
	bea := member(t, a, base, "bea")
	guest := newBrowser(t, base)

	if code := guest.do("POST", "/api/tournaments", cupBody(), nil); code != http.StatusForbidden {
		t.Errorf("a guest makes one: %d, want 403", code)
	}
	if code := ana.do("POST", "/api/tournaments", cupBody(), nil); code != http.StatusForbidden {
		t.Errorf("a member makes one: %d, want 403", code)
	}
	var made session.TournamentDetail
	if code := org.do("POST", "/api/tournaments", cupBody(), &made); code != http.StatusCreated {
		t.Fatalf("an administrator makes one: %d", code)
	}
	path := fmt.Sprintf("/api/tournaments/%d", made.ID)
	if made.Status != "draft" || made.GamesPerPairing != 2 || !made.Rated || !made.PieRule {
		t.Errorf("the defaults: %+v", made.TournamentSummary)
	}
	if code := ana.do("GET", path, nil, nil); code != http.StatusNotFound {
		t.Errorf("a member reads a draft: %d, want 404", code)
	}
	if code := guest.do("GET", "/api/tournaments/nonsense", nil, nil); code != http.StatusNotFound {
		t.Errorf("an id that is not one: %d, want 404", code)
	}
	body := cupBody()
	body["clock_seconds"] = 0
	if code := org.do("PUT", path, body, nil); code != http.StatusBadRequest {
		t.Errorf("no clock: %d, want 400", code)
	}
	if code := org.do("POST", path+"/start", nil, nil); code != http.StatusConflict {
		t.Errorf("starting a draft: %d, want 409", code)
	}
	if code := org.do("POST", path+"/open", nil, nil); code != 200 {
		t.Fatalf("opening: %d", code)
	}
	if code := guest.do("POST", path+"/enter", nil, nil); code != http.StatusForbidden {
		t.Errorf("a guest enters: %d, want 403", code)
	}
	if code := ana.do("POST", path+"/start", nil, nil); code != http.StatusForbidden {
		t.Errorf("a member starts it: %d, want 403", code)
	}
	if code := org.do("POST", path+"/start", nil, nil); code != http.StatusConflict {
		t.Errorf("starting with nobody: %d, want 409", code)
	}
	for _, p := range []*browser{ana, bea} {
		if code := p.do("POST", path+"/enter", nil, nil); code != 200 {
			t.Fatalf("entering: %d", code)
		}
	}
	var running session.TournamentDetail
	if code := org.do("POST", path+"/start", nil, &running); code != 200 || running.Status != "running" {
		t.Fatalf("starting: %d %+v", code, running.TournamentSummary)
	}
	room := running.Games[0].Games[0].RoomCode

	// Anybody can read a running tournament and its standings.
	var read session.TournamentDetail
	if code := guest.do("GET", path, nil, &read); code != 200 || len(read.Standings) != 2 {
		t.Errorf("a guest reads it: %d %+v", code, read.Standings)
	}
	var list struct {
		Tournaments []session.TournamentSummary `json:"tournaments"`
	}
	if code := guest.do("GET", "/api/tournaments", nil, &list); code != 200 || len(list.Tournaments) != 1 {
		t.Errorf("a guest lists them: %d %+v", code, list)
	}

	// The game says it is part of the tournament, and is the players' own.
	var game struct {
		Tournament struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"tournament"`
	}
	if code := guest.do("GET", "/api/games/"+room, nil, &game); code != 200 || game.Tournament.ID != made.ID || game.Tournament.Name != "The cup" {
		t.Errorf("the game's tournament: %d %+v", code, game.Tournament)
	}
	if code := guest.do("POST", "/api/games/"+room+"/resign", map[string]any{"player_name": "ana"}, nil); code != http.StatusForbidden {
		t.Errorf("a stranger resigns for a player: %d, want 403", code)
	}

	// An administrator can open making them up to all accounts.
	if err := a.Caps().Set(context.Background(), auth.ClassUser, auth.CapCreateTournament, true); err != nil {
		t.Fatal(err)
	}
	if code := ana.do("POST", "/api/tournaments", cupBody(), nil); code != http.StatusCreated {
		t.Errorf("a member, once allowed: %d", code)
	}

	// Only the organiser removes a player, and withdrawing ends this one.
	if code := ana.do("POST", path+"/leave", map[string]any{"handle": "bea"}, nil); code != http.StatusForbidden {
		t.Errorf("removing another player: %d, want 403", code)
	}
	if code := ana.do("POST", path+"/leave", nil, nil); code != 200 {
		t.Errorf("withdrawing: %d", code)
	}
	if code := org.do("POST", path+"/abandon", nil, nil); code != http.StatusConflict {
		t.Errorf("abandoning a finished tournament: %d, want 409", code)
	}
}

// A database made before the capabilities existed gets them as the file says, and
// nothing the admin page changed is touched.
func TestTournaments_AnOlderDatabaseGetsTheCapabilities(t *testing.T) {
	cfg := newTestConfig(t)
	first, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Caps().Set(context.Background(), auth.ClassUser, auth.CapRunBatch, false); err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec(`DELETE FROM capabilities WHERE capability IN (?, ?)`, auth.CapEnterTournament, auth.CapCreateTournament); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	second, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	c := second.Caps()
	if !c.Allowed(auth.ClassUser, auth.CapEnterTournament) {
		t.Error("an account may not enter tournaments")
	}
	if c.Allowed(auth.ClassGuest, auth.CapEnterTournament) || c.Allowed(auth.ClassUser, auth.CapCreateTournament) {
		t.Error("the new capabilities went too far")
	}
	if c.Allowed(auth.ClassUser, auth.CapRunBatch) {
		t.Error("the admin's change was undone")
	}
}

// An account with a confirmed address gets its turn mail without asking.
func TestTournaments_TurnMailGoesToTheAccount(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{}, func(c *config.Config) { c.CapsFile = testCapsFile })
	org := administrator(t, e.App, e.URL, "org")
	ana := member(t, e.App, e.URL, "ana")
	bea := member(t, e.App, e.URL, "bea")
	var made session.TournamentDetail
	body := cupBody()
	body["games_per_pairing"] = 1
	if code := org.do("POST", "/api/tournaments", body, &made); code != http.StatusCreated {
		t.Fatalf("making it: %d", code)
	}
	path := fmt.Sprintf("/api/tournaments/%d", made.ID)
	org.do("POST", path+"/open", nil, nil)
	ana.do("POST", path+"/enter", nil, nil)
	bea.do("POST", path+"/enter", nil, nil)
	n := len(e.SMTP.Mails())
	var running session.TournamentDetail
	if code := org.do("POST", path+"/start", nil, &running); code != 200 {
		t.Fatalf("starting it: %d", code)
	}
	mails := e.waitMails(t, n+1)
	g := running.Games[0].Games[0]
	last := mails[len(mails)-1]
	if !strings.Contains(strings.Join(last.To, ","), g.First+"@example.com") || !strings.Contains(last.Data, g.RoomCode) {
		t.Errorf("the mail went to %v, about %q; want %s about %s", last.To, last.Data, g.First, g.RoomCode)
	}
	if strings.Contains(last.Data, "The cup") {
		t.Error("a name an organiser typed was mailed: only fixed text is")
	}
}

// The pages: the tab, the organiser's form, entering, and a game to its result.
func TestTournaments_UI(t *testing.T) {
	a, base := membersServer(t)
	org := administrator(t, a, base, "org")
	ana := member(t, a, base, "ana")
	bea := member(t, a, base, "bea")

	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, org.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the tab", `document.getElementById("tab-tournaments") !== null`, 20*time.Second)
	clickSel(t, ctx, "#tab-tournaments")
	waitJS(t, ctx, "the list", `document.getElementById("tournament-empty") !== null`, 10*time.Second)
	clickSel(t, ctx, "#tournament-new")
	waitJS(t, ctx, "the form", `document.getElementById("t-game") && document.getElementById("t-game").options.length > 1`, 10*time.Second)
	// Only games of two are offered, and a long clock is the default.
	if evalBool(t, ctx, `[...document.getElementById("t-game").options].some((o) => o.value === "nyet")`) {
		t.Error("a game of four is offered")
	}
	if got := evalString(t, ctx, `document.getElementById("t-clock").value`); got != "259200" {
		t.Errorf("the clock starts at %q, want three days", got)
	}
	evalString(t, ctx, `(function () {
		var set = function (id, v) { var s = document.getElementById(id); s.value = v; s.dispatchEvent(new Event("change", { bubbles: true })); };
		set("t-game", "blooms-6");
		set("t-format", "round_robin");
		set("t-pairing", "1");
		document.getElementById("t-name").value = "<b>The cup</b>";
		document.getElementById("t-description").value = "Friends only.\nBe kind.";
		return "ok";
	})()`)
	waitJS(t, ctx, "no rounds for a round robin", `document.getElementById("t-rounds").closest(".field").hidden`, 5*time.Second)
	evalString(t, ctx, `(function () { var s = document.getElementById("t-clock"); s.value = "minutes"; s.dispatchEvent(new Event("change", { bubbles: true })); return "ok"; })()`)
	waitJS(t, ctx, "the game's own time", `document.getElementById("t-minutes").hidden === false && document.getElementById("t-minutes").value === "15"`, 5*time.Second)
	clickSel(t, ctx, "#t-save")
	loc := pollLocation(t, ctx, regexp.MustCompile(`/tournament/[0-9]+$`))
	id := loc[strings.LastIndex(loc, "/")+1:]
	waitJS(t, ctx, "the draft", `document.getElementById("tournament-status") && document.getElementById("tournament-status").textContent === "Draft"`, 15*time.Second)
	if evalBool(t, ctx, `/\bnull\b/.test(document.getElementById("tournament-page").innerText)`) {
		t.Errorf("an empty part of the page reads null: %s", evalString(t, ctx, `document.getElementById("tournament-page").innerText`))
	}
	if got := evalString(t, ctx, `document.getElementById("tournament-name").textContent`); got != "<b>The cup</b>" {
		t.Errorf("the name reads %q: it should be shown as typed, never as markup", got)
	}
	if evalBool(t, ctx, `document.querySelector("#tournament-name b") !== null`) {
		t.Error("the name was taken for markup")
	}

	clickSel(t, ctx, "#t-edit")
	waitJS(t, ctx, "the form again", `document.getElementById("t-save") !== null && document.getElementById("t-minutes").value === "15"`, 10*time.Second)
	evalString(t, ctx, `document.getElementById("t-name").value = "The cup"; "ok"`)
	clickSel(t, ctx, "#t-save")
	waitJS(t, ctx, "the new name", `document.getElementById("tournament-name") && document.getElementById("tournament-name").textContent === "The cup"`, 10*time.Second)
	clickSel(t, ctx, "#t-open")
	waitJS(t, ctx, "open for entries", `document.getElementById("tournament-status").textContent === "Open for entries" && document.getElementById("t-enter") !== null`, 10*time.Second)
	clickSel(t, ctx, "#t-enter")
	waitJS(t, ctx, "the organiser's entry", `document.querySelector('#tournament-entries li[data-handle="org"]') !== null && document.getElementById("t-leave") !== null`, 10*time.Second)

	path := "/api/tournaments/" + id
	if code := ana.do("POST", path+"/enter", nil, nil); code != 200 {
		t.Fatalf("Ana enters: %d", code)
	}
	if code := ana.do("POST", path+"/leave", nil, nil); code != 200 {
		t.Fatalf("Ana leaves: %d", code)
	}
	if code := bea.do("POST", path+"/enter", nil, nil); code != 200 {
		t.Fatalf("Bea enters: %d", code)
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(loc)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the players", `document.querySelectorAll("#tournament-entries li").length === 2 && document.getElementById("t-start") !== null`, 15*time.Second)
	// The confirm box says yes in a test.
	evalString(t, ctx, `window.confirm = function () { return true; }; "ok"`)
	clickSel(t, ctx, "#t-start")
	waitJS(t, ctx, "the first game", `document.querySelector('.tournament-game a[href^="/game/"]') !== null && document.getElementById("tournament-round").textContent === "Round 1 of 1"`, 15*time.Second)

	// The game links back to the tournament.
	clickSel(t, ctx, `.tournament-game a[href^="/game/"]`)
	loc2 := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	room := loc2[strings.LastIndex(loc2, "/")+1:]
	waitJS(t, ctx, "the link back", `document.getElementById("tournament-badge") !== null && document.getElementById("tournament-badge").textContent === "The cup"`, 15*time.Second)
	if code := bea.do("POST", "/api/games/"+room+"/resign", map[string]any{"player_name": "bea"}, nil); code != 200 {
		t.Fatalf("Bea resigns: %d", code)
	}
	clickSel(t, ctx, "#tournament-badge")
	pollLocation(t, ctx, regexp.MustCompile(`/tournament/[0-9]+$`))
	waitJS(t, ctx, "the final standings", `document.getElementById("tournament-status") && document.getElementById("tournament-status").textContent === "Finished" && document.querySelector('#tournament-standings tbody tr').dataset.handle === "org"`, 15*time.Second)

	// A guest can read it, and is not offered anything.
	gctx, gcancel := newChromedpContext(t, 60*time.Second)
	defer gcancel()
	if err := chromedp.Run(gctx, chromedp.EmulateViewport(390, 800), chromedp.Navigate(loc)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, gctx, "the standings", `document.getElementById("tournament-standings") !== null`, 20*time.Second)
	if got := evalString(t, gctx, `String(document.querySelectorAll("#tournament-actions button").length)`); got != "0" {
		t.Errorf("a guest is offered %s actions", got)
	}
	if evalBool(t, gctx, `document.documentElement.scrollWidth > document.documentElement.clientWidth + 1`) {
		t.Error("the page scrolls sideways on a phone")
	}
}

// Nobody can act through a stale page: the server says no, and the page says why.
func TestTournaments_UIShowsARefusal(t *testing.T) {
	a, base := membersServer(t)
	org := administrator(t, a, base, "org")
	ana := member(t, a, base, "ana")
	var made session.TournamentDetail
	if code := org.do("POST", "/api/tournaments", cupBody(), &made); code != http.StatusCreated {
		t.Fatalf("making it: %d", code)
	}
	path := fmt.Sprintf("/api/tournaments/%d", made.ID)
	org.do("POST", path+"/open", nil, nil)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(fmt.Sprintf("%s/tournament/%d", base, made.ID))); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the button", `document.getElementById("t-enter") !== null`, 20*time.Second)
	if code := org.do("POST", path+"/abandon", nil, nil); code != 200 {
		t.Fatalf("abandoning: %d", code)
	}
	clickSel(t, ctx, "#t-enter")
	waitJS(t, ctx, "the refusal", `document.getElementById("tournament-error") && document.getElementById("tournament-error").textContent !== ""`, 10*time.Second)
}

// The rule on entering holds however the request arrives (MCP reaches the session
// layer through one route), and somebody it has shut out can still leave.
func TestTournaments_WhoMayNotEnterCannotAndCanStillLeave(t *testing.T) {
	a, base := membersServer(t)
	org := administrator(t, a, base, "org")
	ana := member(t, a, base, "ana")
	var made session.TournamentDetail
	if code := org.do("POST", "/api/tournaments", cupBody(), &made); code != http.StatusCreated {
		t.Fatalf("making it: %d", code)
	}
	path := fmt.Sprintf("/api/tournaments/%d", made.ID)
	org.do("POST", path+"/open", nil, nil)
	if code := ana.do("POST", path+"/enter", nil, nil); code != 200 {
		t.Fatalf("entering: %d", code)
	}
	ctx := context.Background()
	if err := a.Caps().Set(ctx, auth.ClassUser, auth.CapEnterTournament, false); err != nil {
		t.Fatal(err)
	}
	if code := ana.do("POST", path+"/leave", nil, nil); code != 200 {
		t.Errorf("leaving: %d", code)
	}
	if code := ana.do("POST", path+"/enter", nil, nil); code != http.StatusForbidden {
		t.Errorf("entering over REST: %d, want 403", code)
	}
	member := auth.WithPrincipal(ctx, auth.Principal{ID: "p", Kind: store.KindAccount, Account: &store.Account{ID: 1, Handle: "any"}})
	if _, err := a.Manager().EnterTournament(member, made.ID); !errors.Is(err, session.ErrMayNotEnter) {
		t.Errorf("entering past the route: %v, want it refused", err)
	}
}

// Turn mail that came by itself can be turned off where the changelog says it can:
// in the game, from a browser that never set up an address of its own.
func TestTournaments_TurnMailCanBeTurnedOffInTheGame(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{}, func(c *config.Config) { c.CapsFile = testCapsFile })
	org := administrator(t, e.App, e.URL, "org")
	players := map[string]*browser{"ana": member(t, e.App, e.URL, "ana"), "bea": member(t, e.App, e.URL, "bea")}
	var made session.TournamentDetail
	body := cupBody()
	body["games_per_pairing"] = 1
	if code := org.do("POST", "/api/tournaments", body, &made); code != http.StatusCreated {
		t.Fatalf("making it: %d", code)
	}
	path := fmt.Sprintf("/api/tournaments/%d", made.ID)
	org.do("POST", path+"/open", nil, nil)
	for _, p := range players {
		p.do("POST", path+"/enter", nil, nil)
	}
	var running session.TournamentDetail
	if code := org.do("POST", path+"/start", nil, &running); code != 200 {
		t.Fatalf("starting it: %d", code)
	}
	g := running.Games[0].Games[0]
	first, second := players[g.First], players[g.Second]
	e.waitMails(t, 1)

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, e.URL, first.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(e.URL+"/game/"+g.RoomCode), chromedp.WaitReady("#board-svg", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#open-options")
	waitJS(t, ctx, "the switch, on and usable", `document.getElementById("notify-on") && !document.getElementById("notify-on").disabled && document.getElementById("notify-on").checked`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("notify-hint").textContent`); !strings.Contains(got, "@example.com") {
		t.Errorf("the page should say where the mail goes: %q", got)
	}
	clickSel(t, ctx, "#notify-on")
	waitJS(t, ctx, "saved", `document.getElementById("notify-hint").textContent.indexOf("Saved") === 0 && !document.getElementById("notify-on").checked`, 10*time.Second)

	// The turn comes back to the first player, and no mail comes with it.
	move := func(b *browser, name, notation string) {
		t.Helper()
		if code := b.do("POST", "/api/games/"+g.RoomCode+"/moves", map[string]any{"player_name": name, "notation": notation}, nil); code != 200 {
			t.Fatalf("%s plays %s: %d", name, notation, code)
		}
	}
	move(first, g.First, "e5")
	n := len(e.waitMails(t, 2)) // the second player's turn
	move(second, g.Second, "d4")
	e.quiet(t, n)

	// And it can be turned on again, with no address set up here either.
	clickSel(t, ctx, "#notify-on")
	waitJS(t, ctx, "saved on", `document.getElementById("notify-hint").textContent.indexOf("Saved") === 0 && document.getElementById("notify-on").checked`, 10*time.Second)
}

// The games that had no Submit and no test of playing with it: a whole run of turns
// through the page, pressing Submit whenever it is asked for.
func TestWebSubmit_TheOtherGamesPlayWithIt(t *testing.T) {
	for game, step := range map[string]string{"fist": autoStepJS, "fendo": fendoStepJS, "nyet": nyetStepJS, "mycelium": autoStepJS} {
		t.Run(game, func(t *testing.T) {
			baseURL := newRESTTestServer(t)
			var created struct {
				RoomCode string `json:"room_code"`
			}
			if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": game, "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
				t.Fatalf("create: %d", code)
			}
			ctx, cancel := newChromedpContext(t, 90*time.Second)
			defer cancel()
			openGameAs(t, ctx, baseURL, created.RoomCode, game, "Alice")
			if err := chromedp.Run(ctx,
				chromedp.Evaluate(`localStorage.setItem("bingle.confirm", "true"); location.reload(); true`, nil),
				chromedp.Sleep(300*time.Millisecond),
				chromedp.WaitReady("#board-svg, #card-table", chromedp.ByQuery),
			); err != nil {
				t.Fatal(err)
			}
			withSubmit := `(function () {
				var submit = document.getElementById("submit-move");
				if (submit && !submit.disabled) { submit.click(); return "submit"; }
				return ` + step + `;
			})()`
			progress := func() (st struct {
				MoveSeq int    `json:"move_seq"`
				Status  string `json:"status"`
			}) {
				t.Helper()
				doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &st)
				return st
			}
			var steps []string
			submits, stuck := 0, 0
			for i := 0; i < 300 && stuck < 60; i++ {
				var what string
				if err := chromedp.Run(ctx, chromedp.Evaluate(withSubmit, &what)); err != nil {
					t.Fatalf("step %d: %v", i, err)
				}
				if what != "wait" {
					steps = append(steps, what)
				}
				switch what {
				case "submit":
					submits++
					stuck = 0
				case "wait", "stuck":
					stuck++
				default:
					stuck = 0
				}
				time.Sleep(70 * time.Millisecond)
				if st := progress(); st.MoveSeq >= 8 || st.Status == "finished" {
					break
				}
			}
			st := progress()
			if st.MoveSeq < 4 && st.Status != "finished" {
				t.Errorf("the page should have played turns: %d moves", st.MoveSeq)
			}
			// Whatever the position, a player to move must have something to press:
			// a forced pass used to leave them nothing, until their clock ran out.
			if stuck >= 60 && st.Status != "finished" {
				t.Errorf("the page offered nothing to press after %d moves; steps: %v", st.MoveSeq, steps)
			}
		})
	}
}

// The whole of a real-time tournament in the browser: the organiser's form, checking
// in, the player's game, the first-move deadline, and being told of the next game.
func TestTournaments_RealTimeUI(t *testing.T) {
	a, base := membersServer(t)
	var offset atomic.Int64
	offset.Store(int64(-time.Hour))
	a.Manager().SetClock(func() time.Time { return time.Now().Add(time.Duration(offset.Load())) })
	org := administrator(t, a, base, "org")
	ana := member(t, a, base, "ana")
	bea := member(t, a, base, "bea")

	// The organiser's form shows what a real-time tournament needs, and sends it.
	orgCtx, cancelOrg := newChromedpContext(t, 120*time.Second)
	defer cancelOrg()
	signInChrome(t, orgCtx, base, org.cookie(auth.SessionCookie))
	if err := chromedp.Run(orgCtx, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, orgCtx, "the tab", `document.getElementById("tab-tournaments") !== null`, 20*time.Second)
	clickSel(t, orgCtx, "#tab-tournaments")
	waitJS(t, orgCtx, "the button", `document.getElementById("tournament-new") !== null`, 10*time.Second)
	clickSel(t, orgCtx, "#tournament-new")
	waitJS(t, orgCtx, "the form", `document.getElementById("t-game") && document.getElementById("t-game").options.length > 1`, 10*time.Second)
	if !evalBool(t, orgCtx, `document.getElementById("t-real-time-fields").hidden`) {
		t.Error("the real-time fields show before it is chosen")
	}
	starts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	evalString(t, orgCtx, `(function () {
		document.getElementById("t-name").value = "The live cup";
		var s = document.getElementById("t-game"); s.value = "yavalath"; s.dispatchEvent(new Event("change", { bubbles: true }));
		var c = document.getElementById("t-clock"); c.value = "minutes"; c.dispatchEvent(new Event("change", { bubbles: true }));
		document.getElementById("t-real-time").click();
		return "ok";
	})()`)
	if evalBool(t, orgCtx, `document.getElementById("t-real-time-fields").hidden`) {
		t.Error("the real-time fields are hidden once it is chosen")
	}
	if got := evalString(t, orgCtx, `(function () {
		var s = document.getElementById("t-starts");
		var d = new Date(`+"`"+starts+"`"+`);
		var pad = function (n) { return String(n).padStart(2, "0"); };
		s.value = d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) + "T" + pad(d.getHours()) + ":" + pad(d.getMinutes());
		return s.value ? "ok" : "empty";
	})()`); got != "ok" {
		t.Fatalf("setting the start: %s", got)
	}
	clickSel(t, orgCtx, "#t-save")
	loc := pollLocation(t, orgCtx, regexp.MustCompile(`/tournament/[0-9]+$`))
	id := loc[strings.LastIndex(loc, "/")+1:]
	path := "/api/tournaments/" + id
	waitJS(t, orgCtx, "the draft", `document.getElementById("t-open") !== null`, 15*time.Second)
	clickSel(t, orgCtx, "#t-open")
	waitJS(t, orgCtx, "open", `document.getElementById("tournament-status").textContent === "Open for entries"`, 10*time.Second)
	if evalBool(t, orgCtx, `document.getElementById("t-start") !== null`) {
		t.Error("the organiser is offered a Start button")
	}
	for _, p := range []*browser{ana, bea} {
		if code := p.do("POST", path+"/enter", nil, nil); code != 200 {
			t.Fatalf("entering: %d", code)
		}
	}

	// Check-in opens, and Ana checks in on the page.
	offset.Store(int64(-10 * time.Minute))
	a.Manager().AdvanceRealTimeTournaments(context.Background())
	anaCtx, cancelAna := newChromedpContext(t, 120*time.Second)
	defer cancelAna()
	signInChrome(t, anaCtx, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(anaCtx, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(base+"/tournament/"+id)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, anaCtx, "the check-in", `document.getElementById("t-check-in") !== null`, 15*time.Second)
	clickSel(t, anaCtx, "#t-check-in")
	waitJS(t, anaCtx, "checked in", `document.getElementById("t-check-in") === null && document.getElementById("tournament-check-in").textContent.includes("You are checked in")`, 10*time.Second)
	if code := bea.do("POST", path+"/check-in", nil, nil); code != 200 {
		t.Fatalf("Bea checks in: %d", code)
	}

	// It starts, and the page takes her to her game.
	offset.Store(0)
	time.Sleep(1100 * time.Millisecond) // past the start, which has no seconds on the form
	a.Manager().AdvanceRealTimeTournaments(context.Background())
	waitJS(t, anaCtx, "her game", `document.querySelector("#tournament-your-game a") !== null`, 15*time.Second)
	room := evalString(t, anaCtx, `document.querySelector("#tournament-your-game a").getAttribute("href").split("/").pop()`)

	clickSel(t, anaCtx, "#tournament-your-game a")
	waitJS(t, anaCtx, "the game", `document.getElementById("real-time-badge") !== null`, 15*time.Second)
	var game struct {
		RealTime struct {
			FirstMove *struct {
				Seat int `json:"seat"`
			} `json:"first_move"`
		} `json:"real_time"`
		Seats []struct {
			Seat int    `json:"seat"`
			Name string `json:"name"`
		} `json:"seats"`
	}
	if code := ana.do("GET", "/api/games/"+room, nil, &game); code != 200 || game.RealTime.FirstMove == nil {
		t.Fatalf("the game: %d %+v", code, game)
	}
	mine := evalBool(t, anaCtx, `document.getElementById("first-move") !== null`)
	anaFirst := false
	for _, s := range game.Seats {
		anaFirst = anaFirst || (s.Name == "ana" && s.Seat == game.RealTime.FirstMove.Seat)
	}
	if mine != anaFirst {
		t.Errorf("the first-move notice shows: %v; Ana moves first: %v", mine, anaFirst)
	}

	// The game ends, the second of the pairing starts, and she hears of it on this page.
	if code := bea.do("POST", "/api/games/"+room+"/resign", map[string]any{"player_name": "bea"}, nil); code != 200 {
		t.Fatalf("Bea resigns: %d", code)
	}
	waitJS(t, anaCtx, "the next game", `document.querySelector("#next-game a") !== null`, 15*time.Second)
	if got := evalString(t, anaCtx, `document.querySelector("#next-game a").getAttribute("href")`); strings.HasSuffix(got, room) {
		t.Errorf("sent back to the same game: %s", got)
	}
}
