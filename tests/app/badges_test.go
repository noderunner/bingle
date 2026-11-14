// Badges seen (ADR 0130): GET /api/players/{handle}/badges for accounts only,
// the room for the holder alone, and the profile page's list.

package app_test

import (
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
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

	winFirst(t, ana, "ana", bea, "bea")
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the First Win", `document.querySelector('#badges-list li[data-badge="first-win"] .badge-name') !== null`, 15*time.Second)
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
	waitJS(t, ana, "the tab", `document.getElementById("tab-chat") !== null`, 20*time.Second)
	clickSel(t, ana, "#tab-chat")
	waitJS(t, ana, "the handle", `document.querySelector('.handle-badge[data-handle="bea"]') !== null`, 15*time.Second)
	clickSel(t, ana, `.handle-badge[data-handle="bea"]`)
	waitJS(t, ana, "her badges", `document.querySelector('#pop-badges [data-badge="first-win"]') !== null`, 10*time.Second)
	if evalBool(t, ana, `document.getElementById("friend-pop-send") !== null`) {
		t.Error("a friend is offered a friend request")
	}
}
