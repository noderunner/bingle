package app_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/session"
)

// The page (ADR 0157): the admin page's Ratings card lists a player's rated
// games and takes one out of the ratings, and the tournament's standings then
// say so, with the moderator's handle, to anybody.
func TestWebRatingRemoval_TheAdminCardAndTheStandings(t *testing.T) {
	a, base := membersServer(t)
	kim := administrator(t, a, base, "kim")
	ana := member(t, a, base, "ana")
	bea := member(t, a, base, "bea")
	who := map[string]*browser{"ana": ana, "bea": bea}

	body := cupBody()
	body["format"], body["games_per_pairing"], body["pie_rule"] = "round_robin", 1, false
	var made session.TournamentDetail
	if code := kim.do("POST", "/api/tournaments", body, &made); code != 201 {
		t.Fatalf("a tournament: %d", code)
	}
	path := fmt.Sprintf("/api/tournaments/%d", made.ID)
	kim.do("POST", path+"/open", nil, nil)
	ana.do("POST", path+"/enter", nil, nil)
	bea.do("POST", path+"/enter", nil, nil)
	var running session.TournamentDetail
	if code := kim.do("POST", path+"/start", nil, &running); code != 200 {
		t.Fatalf("starting: %d", code)
	}
	g := running.Games[0].Games[0]
	first, second := who[g.First], who[g.Second]
	game := "/api/games/" + g.RoomCode
	first.do("POST", game+"/moves", map[string]any{"player_name": g.First, "notation": "e5"}, nil)
	second.do("POST", game+"/moves", map[string]any{"player_name": g.Second, "notation": "d4"}, nil)
	if code := second.do("POST", game+"/resign", map[string]any{"player_name": g.Second}, nil); code != 200 {
		t.Fatalf("resigning: %d", code)
	}
	waitForRatedResult(t, kim, g.RoomCode)

	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, kim.cookie(auth.SessionCookie))
	if err := runSteps(ctx, stepViewport(1100, 900), stepNavigate(base+"/admin")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the card", `!document.getElementById("ratings-card").hidden`, 10*time.Second)
	if err := runSteps(ctx, stepSetValue("#ratings-player", "ana")); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#ratings-go")
	waitJS(t, ctx, "Ana's game", `document.querySelector('#ratings-rows tr[data-room="`+g.RoomCode+`"] button') !== null`, 10*time.Second)
	shot(t, ctx, "rating-removal-admin", func(b *[]byte) error { return runSteps(ctx, stepScreenshot(b)) })
	// The page asks first; this answers yes.
	if err := runSteps(ctx, stepEval(`window.confirm = () => true; true`)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, `#ratings-rows tr[data-room="`+g.RoomCode+`"] button`)
	waitJS(t, ctx, "the game removed", `document.getElementById("ratings-result").textContent === "Ratings recomputed" &&
		document.querySelector('#ratings-rows tr[data-room="`+g.RoomCode+`"]').textContent.includes("Removed by @kim")`, 10*time.Second)
	shot(t, ctx, "rating-removal-admin-removed", func(b *[]byte) error { return runSteps(ctx, stepScreenshot(b)) })
	if shooting() {
		dark := stepColorScheme("dark")
		shot(t, ctx, "rating-removal-admin-dark", func(b *[]byte) error {
			return runSteps(ctx, dark, stepSleep(200*time.Millisecond), stepScreenshot(b))
		})
		shot(t, ctx, "rating-removal-admin-phone", func(b *[]byte) error {
			return runSteps(ctx, stepColorScheme("light"),
				stepMobileViewport(390, 844), stepSleep(300*time.Millisecond), stepScreenshot(b))
		})
	}

	// Anybody sees the standings say it: a guest's browser.
	guest, cancelGuest := newChromedpContext(t, 60*time.Second)
	defer cancelGuest()
	if err := runSteps(guest, stepViewport(1100, 900), stepNavigate(fmt.Sprintf("%s/tournament/%d", base, made.ID))); err != nil {
		t.Fatal(err)
	}
	waitJS(t, guest, "the annotation", `document.querySelectorAll("#tournament-standings .rating-removed").length === 2 &&
		[...document.querySelectorAll("#tournament-standings .rating-removed")].every((e) => /^Rating change [+−-]\d+ removed by @kim$/.test(e.textContent))`, 10*time.Second)
	shot(t, guest, "rating-removal-standings", func(b *[]byte) error { return runSteps(guest, stepScreenshot(b)) })
}

// shooting says the screenshots are wanted (BINGLE_SHOTS).
func shooting() bool { return os.Getenv("BINGLE_SHOTS") != "" }
