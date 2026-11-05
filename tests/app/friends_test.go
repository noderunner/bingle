package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

type friendsResp struct {
	Friends []struct {
		Name     string `json:"name"`
		Handle   string `json:"handle"`
		Address  string `json:"address"`
		Online   bool   `json:"online"`
		CanWrite bool   `json:"can_write"`
	} `json:"friends"`
	Incoming []struct {
		Handle string `json:"handle"`
	} `json:"incoming"`
	Outgoing []struct {
		Handle string `json:"handle"`
	} `json:"outgoing"`
	Invites []struct {
		Room string `json:"room"`
		Game string `json:"game"`
		From string `json:"from"`
	} `json:"invites"`
}

func friendsOf(t *testing.T, b *browser) friendsResp {
	t.Helper()
	var out friendsResp
	if code := b.do("GET", "/api/friends", nil, &out); code != 200 {
		t.Fatalf("the friends list: %d", code)
	}
	return out
}

func friendPost(b *browser, route string, body map[string]any) int {
	return b.do("POST", "/api/friends/"+route, body, nil)
}

// A request waits for an answer, and until it is accepted neither can write to or ring the other.
func TestFriends_ARequestIsAskedAndAnswered(t *testing.T) {
	a, base := membersServer(t)
	ana, bob := member(t, a, base, "ana"), member(t, a, base, "bob")
	bobAt := bob.here()
	anaAt := ana.here()

	if code := ana.do("POST", "/api/dm", map[string]any{"to": bobAt, "text": "hello"}, nil); code != 403 {
		t.Errorf("a message to a stranger: %d, want 403", code)
	}
	if code := ana.do("POST", "/api/dm/call", map[string]any{"to": bobAt, "join": true}, nil); code != 403 {
		t.Errorf("a call to a stranger: %d, want 403", code)
	}
	for _, c := range []struct {
		name   string
		handle string
		code   int
	}{{"nobody", "nobody-at-all", 404}, {"yourself", "ana", 404}, {"an @ is fine", "@bob", 200}, {"asking twice", "BOB", 409}} {
		if code := friendPost(ana, "request", map[string]any{"handle": c.handle}); code != c.code {
			t.Errorf("%s: %d, want %d", c.name, code, c.code)
		}
	}
	if f := friendsOf(t, ana); len(f.Outgoing) != 1 || f.Outgoing[0].Handle != "bob" || len(f.Friends) != 0 {
		t.Errorf("Ana's list: %+v", f)
	}
	if f := friendsOf(t, bob); len(f.Incoming) != 1 || f.Incoming[0].Handle != "ana" {
		t.Errorf("Bob's list: %+v", f)
	}
	if code := ana.do("POST", "/api/dm", map[string]any{"to": bobAt, "text": "hello"}, nil); code != 403 {
		t.Errorf("a message while the request waits: %d, want 403", code)
	}
	// Only the one asked can answer.
	if code := friendPost(ana, "answer", map[string]any{"handle": "bob", "accept": true}); code != 404 {
		t.Errorf("Ana accepts her own request: %d, want 404", code)
	}
	if code := friendPost(bob, "answer", map[string]any{"handle": "ana", "accept": true}); code != 200 {
		t.Fatalf("Bob accepts: %d", code)
	}
	f := friendsOf(t, ana)
	if len(f.Friends) != 1 || f.Friends[0].Name != "@bob" || f.Friends[0].Address != bobAt || !f.Friends[0].Online {
		t.Errorf("Ana's friends: %+v", f.Friends)
	}
	if code := ana.do("POST", "/api/dm", map[string]any{"to": bobAt, "text": "hello"}, nil); code != 200 {
		t.Errorf("a message to a friend: %d", code)
	}
	if code := bob.do("POST", "/api/dm", map[string]any{"to": anaAt, "text": "hi"}, nil); code != 200 {
		t.Errorf("the answer: %d", code)
	}
	// Removed, they are strangers again, and either may ask again.
	if code := friendPost(bob, "remove", map[string]any{"handle": "ana"}); code != 200 {
		t.Fatalf("Bob removes Ana: %d", code)
	}
	if f := friendsOf(t, ana); len(f.Friends) != 0 {
		t.Errorf("after removal: %+v", f.Friends)
	}
	if code := ana.do("POST", "/api/dm", map[string]any{"to": bobAt, "text": "hello?"}, nil); code != 403 {
		t.Errorf("a message after removal: %d, want 403", code)
	}
	if code := friendPost(ana, "request", map[string]any{"handle": "bob"}); code != 200 {
		t.Errorf("asking again: %d", code)
	}
}

// Asking somebody who has already asked you makes you friends. A refusal holds against the same
// request again, and taking a request back is no way round it; the one who refused may still ask.
func TestFriends_BothAskingAndARefusal(t *testing.T) {
	a, base := membersServer(t)
	ana, bob, cyd := member(t, a, base, "ana"), member(t, a, base, "bob"), member(t, a, base, "cyd")

	friendPost(ana, "request", map[string]any{"handle": "bob"})
	if code := friendPost(bob, "request", map[string]any{"handle": "ana"}); code != 200 {
		t.Fatalf("Bob asks too: %d", code)
	}
	if f := friendsOf(t, ana); len(f.Friends) != 1 {
		t.Errorf("both asked: %+v", f)
	}

	friendPost(cyd, "request", map[string]any{"handle": "ana"})
	if code := friendPost(ana, "answer", map[string]any{"handle": "cyd", "accept": false}); code != 200 {
		t.Fatalf("Ana declines: %d", code)
	}
	if f := friendsOf(t, cyd); len(f.Outgoing) != 0 || len(f.Friends) != 0 {
		t.Errorf("Cyd's list after the refusal: %+v", f)
	}
	if code := friendPost(cyd, "request", map[string]any{"handle": "ana"}); code != 429 {
		t.Errorf("asking again at once: %d, want 429", code)
	}
	if code := friendPost(cyd, "remove", map[string]any{"handle": "ana"}); code != 200 {
		t.Errorf("removing a refusal: %d", code)
	}
	if code := friendPost(cyd, "request", map[string]any{"handle": "ana"}); code != 429 {
		t.Errorf("asking again after removing: %d, want 429", code)
	}
	if code := friendPost(ana, "request", map[string]any{"handle": "cyd"}); code != 200 {
		t.Errorf("the one who refused asks: %d", code)
	}
}

// Friends are for accounts: a guest has no list and cannot ask.
func TestFriends_AreNotForGuests(t *testing.T) {
	a, base := membersServer(t)
	member(t, a, base, "bob")
	guest := newBrowser(t, base)
	if code := guest.do("GET", "/api/friends", nil, nil); code != 401 && code != 403 {
		t.Errorf("a guest's list: %d", code)
	}
	if code := friendPost(guest, "request", map[string]any{"handle": "bob"}); code != 401 && code != 403 {
		t.Errorf("a guest asks: %d", code)
	}
}

// An invitation waits for a friend who is away, comes only from somebody in the game and goes when
// it has been dismissed.
func TestFriends_InvitedToAGame(t *testing.T) {
	a, base := membersServer(t)
	ana, bob, cyd := member(t, a, base, "ana"), member(t, a, base, "bob"), member(t, a, base, "cyd")
	befriend(t, ana, bob, "bob", "ana")
	befriend(t, cyd, bob, "bob", "cyd")
	var created struct {
		RoomCode string `json:"room_code"`
	}
	ana.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &created)
	room := created.RoomCode

	if code := friendPost(ana, "invite", map[string]any{"handle": "cyd", "room": room}); code != 403 {
		t.Errorf("inviting a stranger: %d, want 403", code)
	}
	if code := friendPost(cyd, "invite", map[string]any{"handle": "bob", "room": room}); code != 403 {
		t.Errorf("inviting to somebody else's game: %d, want 403", code)
	}
	if code := friendPost(ana, "invite", map[string]any{"handle": "bob", "room": "NOSUCH"}); code != 404 {
		t.Errorf("inviting to no game: %d, want 404", code)
	}
	if code := friendPost(ana, "invite", map[string]any{"handle": "bob", "room": room}); code != 200 {
		t.Fatalf("Ana invites Bob: %d", code)
	}
	f := friendsOf(t, bob)
	if len(f.Invites) != 1 || f.Invites[0].Room != room || f.Invites[0].From != "@ana" || f.Invites[0].Game == "" {
		t.Fatalf("Bob's invitations: %+v", f.Invites)
	}
	if code := friendPost(bob, "dismiss", map[string]any{"room": room}); code != 200 {
		t.Errorf("dismissing: %d", code)
	}
	if f := friendsOf(t, bob); len(f.Invites) != 0 {
		t.Errorf("after dismissing: %+v", f.Invites)
	}
}

// Mail about a request is off until asked for, and says only who asked.
func TestFriends_MailAboutARequestIsOptIn(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	bob := newBrowser(t, e.URL)
	bobPw := signUp(t, e, bob, "bob", "bob@example.com")
	if code := bob.login("bob", bobPw); code != 200 {
		t.Fatalf("Bob signs in: %d", code)
	}
	ana := newBrowser(t, e.URL)
	anaPw := signUp(t, e, ana, "ana", "ana@example.com")
	if code := ana.login("ana", anaPw); code != 200 {
		t.Fatalf("Ana signs in: %d", code)
	}
	var settings struct {
		MailRequests bool `json:"mail_requests"`
	}
	if code := bob.do("GET", "/api/friends/settings", nil, &settings); code != 200 || settings.MailRequests {
		t.Fatalf("the default: %d %+v", code, settings)
	}
	before := len(e.SMTP.Mails())
	if code := friendPost(ana, "request", map[string]any{"handle": "bob"}); code != 200 {
		t.Fatalf("Ana asks: %d", code)
	}
	e.quiet(t, before)
	friendPost(ana, "remove", map[string]any{"handle": "bob"})

	if code := bob.do("PUT", "/api/friends/settings", map[string]any{"mail_requests": true}, nil); code != 200 {
		t.Fatalf("Bob asks for mail: %d", code)
	}
	if code := friendPost(ana, "request", map[string]any{"handle": "bob"}); code != 200 {
		t.Fatalf("Ana asks again: %d", code)
	}
	got := e.waitMails(t, before+1)[before]
	if !strings.Contains(got.Data, "@ana") || !strings.Contains(got.Data, "friend request") || !strings.Contains(got.Data, "/profile") {
		t.Errorf("the mail: %s", got.Data)
	}
	if !strings.Contains(strings.Join(got.To, ","), "bob@example.com") {
		t.Errorf("sent to %v", got.To)
	}
}

// The window: a request, its acceptance, an invitation to a game and taking a friend away. A guest has no icon.
func TestWebFriends_TheWindow(t *testing.T) {
	a, base := membersServer(t)
	guest, cancelG := newChromedpContext(t, 90*time.Second)
	defer cancelG()
	if err := chromedp.Run(guest, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, guest, "the top bar", `document.getElementById("open-profile") !== null`, 20*time.Second)
	if !evalBool(t, guest, `getComputedStyle(document.getElementById("open-friends")).display === "none"`) {
		t.Error("a guest has the friends icon")
	}

	anaB, bobB := member(t, a, base, "ana"), member(t, a, base, "bob")
	var created struct {
		RoomCode string `json:"room_code"`
	}
	anaB.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &created)
	ana, cancelA := newChromedpContext(t, 90*time.Second)
	defer cancelA()
	bob, cancelB := newChromedpContext(t, 90*time.Second)
	defer cancelB()
	signInChrome(t, ana, base, anaB.cookie(auth.SessionCookie))
	signInChrome(t, bob, base, bobB.cookie(auth.SessionCookie))
	if err := chromedp.Run(ana, chromedp.Navigate(base+"/healthz"), chromedp.Evaluate(`localStorage.setItem("bingle.games", JSON.stringify([{code: "`+created.RoomCode+`", def: "tic-tac-toe", name: "Ana", ts: Date.now()}])); true`, nil), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(bob, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "Ana's icon", `getComputedStyle(document.getElementById("open-friends")).display !== "none"`, 20*time.Second)
	waitJS(t, bob, "Bob's icon", `getComputedStyle(document.getElementById("open-friends")).display !== "none"`, 20*time.Second)

	// Ana asks by handle. Bob's badge counts it while his window is shut.
	clickSel(t, ana, "#open-friends")
	if err := chromedp.Run(ana, chromedp.Evaluate(`document.getElementById("friend-handle").value = "bob"; document.getElementById("friend-add-go").click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "waiting for an answer", `document.querySelector("#friend-outgoing [data-cancel='bob']") !== null`, 10*time.Second)
	waitJS(t, bob, "Bob's badge", `document.getElementById("dm-badge").textContent === "1" && !document.getElementById("dm-badge").hidden`, 15*time.Second)
	clickSel(t, bob, "#open-friends")
	waitJS(t, bob, "the request", `document.querySelector("[data-accept='ana']") !== null`, 10*time.Second)
	clickSel(t, bob, "[data-accept='ana']")
	waitJS(t, ana, "Bob among Ana's friends", `document.querySelector("#friends-list .dm-person") !== null && document.querySelector("#friends-list .friend-dot.online") !== null`, 20*time.Second)

	// She invites him to her game. It arrives without his doing anything.
	clickSel(t, ana, "#friends-list .dm-person")
	waitJS(t, ana, "the buttons", `!document.getElementById("friend-invite").hidden && !document.getElementById("friend-remove").hidden`, 10*time.Second)
	clickSel(t, ana, "#friend-invite")
	waitJS(t, ana, "the game to invite to", `document.getElementById("friend-invite-game").value === "`+created.RoomCode+`"`, 10*time.Second)
	clickSel(t, ana, "#friend-invite-send")
	waitJS(t, bob, "the invitation", `document.querySelector("[data-join-invite='`+created.RoomCode+`']") !== null`, 15*time.Second)
	clickSel(t, bob, "[data-dismiss='"+created.RoomCode+"']")
	waitJS(t, bob, "the invitation gone", `document.querySelector("[data-join-invite]") === null`, 10*time.Second)

	// Ana removes him, and it goes from both lists.
	if err := chromedp.Run(ana, chromedp.Evaluate(`window.confirm = () => true; true`, nil)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ana, "#friend-remove")
	waitJS(t, ana, "Bob gone from the list", `document.querySelector("#friends-list .dm-person") === null`, 10*time.Second)
	waitJS(t, bob, "Ana gone from Bob's list", `document.querySelector("#friends-list .dm-person") === null`, 15*time.Second)
}

// Someone who hides looks exactly like someone who has gone, until they speak first.
func TestFriends_HidingLooksLikeBeingGone(t *testing.T) {
	a, base := membersServer(t)
	ana, bob := member(t, a, base, "ana"), member(t, a, base, "bob")
	befriend(t, ana, bob, "bob", "ana")
	anaAt, bobAt := ana.here(), bob.here()
	seen := func(want bool) {
		t.Helper()
		f := friendsOf(t, ana)
		if len(f.Friends) != 1 || f.Friends[0].Online != want || f.Friends[0].CanWrite != want {
			t.Fatalf("Ana sees %+v, want online %v", f.Friends, want)
		}
		var people struct {
			People []struct {
				Address string `json:"address"`
			} `json:"people"`
		}
		if code := ana.do("GET", "/api/here", nil, &people); code != 200 {
			t.Fatalf("who is here: %d", code)
		}
		listed := false
		for _, p := range people.People {
			listed = listed || p.Address == bobAt
		}
		if listed != want {
			t.Fatalf("Bob in Here now: %v, want %v (%+v)", listed, want, people)
		}
	}
	var settings struct {
		ShowPresence bool `json:"show_presence"`
		MailRequests bool `json:"mail_requests"`
	}
	if code := bob.do("GET", "/api/friends/settings", nil, &settings); code != 200 || !settings.ShowPresence {
		t.Fatalf("the default: %d %+v", code, settings)
	}
	seen(true)
	if code := ana.do("POST", "/api/dm", map[string]any{"to": bobAt, "text": "before"}, nil); code != 200 {
		t.Fatalf("a message while seen: %d", code)
	}

	// Bob hides. It holds at once, and over the next minute's mark.
	if code := bob.do("PUT", "/api/friends/settings", map[string]any{"show_presence": false}, nil); code != 200 {
		t.Fatalf("Bob hides: %d", code)
	}
	if code := bob.do("GET", "/api/friends/settings", nil, &settings); code != 200 || settings.ShowPresence {
		t.Fatalf("after hiding: %d %+v", code, settings)
	}
	seen(false)
	if bob.here() == "" {
		t.Fatal("Bob's beat")
	}
	seen(false)
	var hiddenBody, goneBody map[string]any
	hiddenCode := ana.do("POST", "/api/dm", map[string]any{"to": bobAt, "text": "there?"}, &hiddenBody)
	if hiddenCode != 404 {
		t.Errorf("a message to somebody hidden: %d, want 404", hiddenCode)
	}
	if code := ana.do("POST", "/api/dm/call", map[string]any{"to": bobAt, "join": true}, nil); code != 404 {
		t.Errorf("a call to somebody hidden: %d, want 404", code)
	}
	// The same words as for somebody who has gone.
	cyd := member(t, a, base, "cyd")
	befriend(t, ana, cyd, "cyd", "ana")
	var cydAddr string
	for _, f := range friendsOf(t, ana).Friends {
		if f.Handle == "cyd" {
			cydAddr = f.Address
		}
	}
	if code := ana.do("POST", "/api/dm", map[string]any{"to": cydAddr, "text": "there?"}, &goneBody); code != 404 {
		t.Fatalf("a message to somebody gone: %d", code)
	}
	if hiddenBody["error"] != goneBody["error"] {
		t.Errorf("the answers differ: %v and %v", hiddenBody, goneBody)
	}

	// Bob wrote nothing yet. Once he speaks, Ana can answer, though he still looks offline.
	if code := bob.do("POST", "/api/dm", map[string]any{"to": anaAt, "text": "only me"}, nil); code != 200 {
		t.Fatalf("Bob writes while hidden: %d", code)
	}
	for _, f := range friendsOf(t, ana).Friends {
		if f.Handle == "bob" && (f.Online || !f.CanWrite) {
			t.Errorf("after Bob wrote: %+v", f)
		}
	}
	if code := ana.do("POST", "/api/dm", map[string]any{"to": bobAt, "text": "hello"}, nil); code != 200 {
		t.Errorf("Ana's answer: %d", code)
	}

	// He comes back into view.
	if code := bob.do("PUT", "/api/friends/settings", map[string]any{"show_presence": true}, nil); code != 200 {
		t.Fatalf("Bob shows himself: %d", code)
	}
	for _, f := range friendsOf(t, ana).Friends {
		if f.Handle == "bob" && !f.Online {
			t.Errorf("after showing: %+v", f)
		}
	}
}
