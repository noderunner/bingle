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
	if got := evalString(t, bob, `document.querySelector("[data-dismiss='`+created.RoomCode+`']").textContent`); got != "Decline" {
		t.Errorf("the button says %q", got)
	}
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

type listedGame struct {
	RoomCode       string   `json:"room_code"`
	OpenSeats      int      `json:"open_seats"`
	FriendsOnly    bool     `json:"friends_only"`
	ReservedForYou bool     `json:"reserved_for_you"`
	Tags           []string `json:"tags"`
}

func listedFor(t *testing.T, b *browser, query string) map[string]listedGame {
	t.Helper()
	var res struct {
		Games []listedGame `json:"games"`
	}
	if code := b.do("GET", "/api/games"+query, nil, &res); code != 200 {
		t.Fatalf("the list: %d", code)
	}
	out := map[string]listedGame{}
	for _, g := range res.Games {
		out[g.RoomCode] = g
	}
	return out
}

// A seat is kept for its friend: nobody else sits there, it comes with an invitation and a lobby, and
// the game is friends-only while no seat is left for anybody.
func TestFriends_ASeatKeptForAFriend(t *testing.T) {
	a, base := membersServer(t)
	ana, bob, cyd, dan := member(t, a, base, "ana"), member(t, a, base, "bob"), member(t, a, base, "cyd"), member(t, a, base, "dan")
	befriend(t, ana, bob, "bob", "ana")
	befriend(t, ana, cyd, "cyd", "ana")
	guest := newBrowser(t, base)

	create := func(body map[string]any) (int, string) {
		var created struct {
			RoomCode string `json:"room_code"`
		}
		body["game_def_id"], body["player_name"] = "tic-tac-toe", "Ana"
		return ana.do("POST", "/api/games", body, &created), created.RoomCode
	}
	// Only friends, only open seats.
	if code, _ := create(map[string]any{"reserved_seats": map[string]string{"2": "dan"}}); code != 403 {
		t.Errorf("keeping a seat for a stranger: %d, want 403", code)
	}
	if code, _ := create(map[string]any{"reserved_seats": map[string]string{"1": "bob"}}); code != 403 {
		t.Errorf("keeping her own seat: %d, want 403", code)
	}
	if code, _ := create(map[string]any{"reserved_seats": map[string]string{"7": "bob"}}); code == 200 {
		t.Errorf("keeping no seat at all: %d", code)
	}
	if code := guest.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "G", "reserved_seats": map[string]string{"2": "bob"}}, nil); code != 403 {
		t.Errorf("a guest keeps a seat: %d, want 403", code)
	}
	code, room := create(map[string]any{"reserved_seats": map[string]string{"2": "bob"}})
	if code != 200 {
		t.Fatalf("Ana keeps a seat for Bob: %d", code)
	}
	var game struct {
		Lobby        bool `json:"lobby"`
		Reservations []struct {
			Seat   int    `json:"seat"`
			Handle string `json:"handle"`
			Yours  bool   `json:"yours"`
		} `json:"reservations"`
	}
	bob.do("GET", "/api/games/"+room, nil, &game)
	if !game.Lobby || len(game.Reservations) != 1 || game.Reservations[0].Handle != "bob" || !game.Reservations[0].Yours {
		t.Errorf("as Bob sees it: %+v", game)
	}
	if f := friendsOf(t, bob); len(f.Invites) != 1 || f.Invites[0].Room != room {
		t.Errorf("Bob's invitations: %+v", f.Invites)
	}

	// Who finds it.
	for _, c := range []struct {
		name string
		b    *browser
		sees bool
	}{{"the creator", ana, true}, {"its friend", bob, true}, {"another friend", cyd, true}, {"a stranger", dan, false}, {"a guest", guest, false}} {
		g, ok := listedFor(t, c.b, "?open_seats=1")[room]
		if ok != c.sees {
			t.Errorf("%s finds it: %v, want %v", c.name, ok, c.sees)
		}
		if ok && !g.FriendsOnly {
			t.Errorf("%s: %+v should say friends only", c.name, g)
		}
	}
	if g := listedFor(t, bob, "")[room]; !g.ReservedForYou || g.OpenSeats != 1 {
		t.Errorf("for Bob: %+v", g)
	}
	if g := listedFor(t, cyd, "?open_seats=1")[room]; g.ReservedForYou || g.OpenSeats != 0 {
		t.Errorf("for Cyd, who can only watch: %+v", g)
	}

	// Who sits. Anybody can watch by the link.
	if code := dan.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Dan"}, nil); code != 403 {
		t.Errorf("a stranger sits: %d, want 403", code)
	}
	if code := dan.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Dan", "role": "spectator"}, nil); code != 200 {
		t.Errorf("a stranger watches: %d", code)
	}
	if code := bob.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Errorf("Bob sits: %d", code)
	}
}

// The host gives up a seat: it is then for anybody, and the game is public.
func TestFriends_TheHostReleasesASeat(t *testing.T) {
	a, base := membersServer(t)
	ana, bob, dan := member(t, a, base, "ana"), member(t, a, base, "bob"), member(t, a, base, "dan")
	befriend(t, ana, bob, "bob", "ana")
	var created struct {
		RoomCode string `json:"room_code"`
	}
	ana.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana", "reserved_seats": map[string]string{"2": "bob"}}, &created)
	room := created.RoomCode
	if _, ok := listedFor(t, dan, "")[room]; ok {
		t.Fatal("a stranger finds it")
	}
	if code := dan.do("POST", "/api/games/"+room+"/release", map[string]any{"player_name": "Dan", "seat": 2}, nil); code == 200 {
		t.Error("a stranger gives up the seat")
	}
	if code := ana.do("POST", "/api/games/"+room+"/release", map[string]any{"player_name": "Ana", "seat": 2}, nil); code != 200 {
		t.Fatalf("Ana gives up the seat: %d", code)
	}
	g, ok := listedFor(t, dan, "?open_seats=1")[room]
	if !ok || g.FriendsOnly || g.OpenSeats != 1 {
		t.Errorf("after the release: %v %+v", ok, g)
	}
	if code := dan.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Dan"}, nil); code != 200 {
		t.Errorf("Dan sits: %d", code)
	}
}

// A table with one seat kept and one open is still public: only the kept seat is closed.
func TestFriends_ASeatKeptAtAnOpenTable(t *testing.T) {
	a, base := membersServer(t)
	ana, bob, dan := member(t, a, base, "ana"), member(t, a, base, "bob"), member(t, a, base, "dan")
	befriend(t, ana, bob, "bob", "ana")
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := ana.do("POST", "/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Ana", "reserved_seats": map[string]string{"2": "bob"}}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	g, ok := listedFor(t, dan, "?open_seats=1")[room]
	if !ok || g.FriendsOnly || g.OpenSeats != 2 {
		t.Errorf("for a stranger: %v %+v", ok, g)
	}
	if g := listedFor(t, bob, "")[room]; g.OpenSeats != 3 || !g.ReservedForYou {
		t.Errorf("for Bob: %+v", g)
	}
	// The stranger takes a seat left open, and never the one kept.
	if code := dan.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Dan", "seat": 2}, nil); code != 403 {
		t.Errorf("Dan takes the kept seat: %d, want 403", code)
	}
	var joined struct {
		Seat int `json:"seat"`
	}
	if code := dan.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Dan"}, &joined); code != 200 || joined.Seat != 3 {
		t.Errorf("Dan sits: %d at seat %d, want seat 3", code, joined.Seat)
	}
	// Bob comes to his own, whichever seats are left.
	if code := bob.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Bob"}, &joined); code != 200 || joined.Seat != 2 {
		t.Errorf("Bob sits: %d at seat %d, want seat 2", code, joined.Seat)
	}
}

// An invite-only game is on nobody's list, but its link works.
func TestFriends_InviteOnlyIsOnNoList(t *testing.T) {
	a, base := membersServer(t)
	ana, bob, dan := member(t, a, base, "ana"), member(t, a, base, "bob"), member(t, a, base, "dan")
	befriend(t, ana, bob, "bob", "ana")
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := ana.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana", "invite_only": true}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	for name, b := range map[string]*browser{"the creator": ana, "a friend": bob, "a stranger": dan, "a guest": newBrowser(t, base)} {
		for _, q := range []string{"", "?open_seats=1", "?family=tic-tac-toe"} {
			if _, ok := listedFor(t, b, q)[room]; ok {
				t.Errorf("%s finds it with %q", name, q)
			}
		}
	}
	// An invitation brings a friend, and a link anybody.
	if code := friendPost(ana, "invite", map[string]any{"handle": "bob", "room": room}); code != 200 {
		t.Errorf("Ana invites Bob: %d", code)
	}
	if f := friendsOf(t, bob); len(f.Invites) != 1 {
		t.Errorf("Bob's invitations: %+v", f.Invites)
	}
	var seen struct {
		InviteOnly bool `json:"invite_only"`
	}
	if code := dan.do("GET", "/api/games/"+room, nil, &seen); code != 200 || !seen.InviteOnly {
		t.Errorf("the link: %d %+v", code, seen)
	}
	if code := dan.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Dan"}, nil); code != 200 {
		t.Errorf("Dan comes by the link: %d", code)
	}
}

// Through the page: the setup keeps a seat, the lobby says so, and a friend finds the game to join.
func TestWebFriends_ReservingASeatInTheSetup(t *testing.T) {
	a, base := membersServer(t)
	anaB, bobB := member(t, a, base, "ana"), member(t, a, base, "bob")
	befriend(t, anaB, bobB, "bob", "ana")
	ana, cancelA := newChromedpContext(t, 90*time.Second)
	defer cancelA()
	bob, cancelB := newChromedpContext(t, 90*time.Second)
	defer cancelB()
	signInChrome(t, ana, base, anaB.cookie(auth.SessionCookie))
	signInChrome(t, bob, base, bobB.cookie(auth.SessionCookie))
	if err := chromedp.Run(ana, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(base+"/?game=tic-tac-toe")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "the friend to keep a seat for", `document.querySelector('#reserve-friend option[value="bob"]') !== null`, 20*time.Second)
	if !evalBool(t, ana, `document.getElementById("invite-only") !== null`) {
		t.Error("no invite-only option in the setup")
	}
	evalString(t, ana, `(function () { var s = document.getElementById("reserve-friend"); s.value = "bob"; s.dispatchEvent(new Event("change", {bubbles: true})); document.querySelector('#create-game-form button[type="submit"]').click(); return "ok"; })()`)
	waitJS(t, ana, "the lobby", `document.querySelector('#lobby-seats [data-reserved="bob"]') !== null`, 20*time.Second)
	waitJS(t, ana, "the Release button", `document.querySelector('#lobby-seats [data-act="release"]') !== null`, 10*time.Second)

	// Bob sees it in Join Game, marked as his.
	if err := chromedp.Run(bob, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, bob, "the join button", `document.getElementById("open-join") !== null`, 20*time.Second)
	clickSel(t, bob, "#open-join")
	waitJS(t, bob, "the row", `[...document.querySelectorAll("#join-open-games li .game-tag")].some(b => b.textContent === "Seat reserved for you")`, 15*time.Second)
	clickSel(t, bob, "#join-open-games button[data-join]")
	waitJS(t, bob, "Bob in the lobby", `document.querySelector('#lobby-seats .lobby-seat.mine') !== null`, 20*time.Second)
}

// A handle in a chat offers a friend request, and does nothing for a friend.
func TestWebFriends_AHandleInAChat(t *testing.T) {
	a, base := membersServer(t)
	anaB, bobB, cydB := member(t, a, base, "ana"), member(t, a, base, "bob"), member(t, a, base, "cyd")
	befriend(t, anaB, cydB, "cyd", "ana")
	for _, b := range []*browser{bobB, cydB} {
		if code := b.do("POST", "/api/chat", map[string]any{"player_name": "them", "text": "anyone for a game?"}, nil); code != 200 {
			t.Fatalf("the global chat: %d", code)
		}
	}
	ana, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	signInChrome(t, ana, base, anaB.cookie(auth.SessionCookie))
	if err := chromedp.Run(ana, chromedp.EmulateViewport(1400, 900), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "the tab", `document.getElementById("tab-chat") !== null`, 20*time.Second)
	clickSel(t, ana, "#tab-chat")
	waitJS(t, ana, "the badges", `document.querySelector('.handle-badge[data-handle="bob"]') !== null && document.querySelector('.handle-badge[data-handle="cyd"]') !== null`, 15*time.Second)

	// A friend: nothing happens.
	clickSel(t, ana, `.handle-badge[data-handle="cyd"]`)
	time.Sleep(500 * time.Millisecond)
	if evalBool(t, ana, `document.getElementById("friend-pop") !== null`) {
		t.Error("a friend's badge opened a menu")
	}
	// Somebody else: an offer, which sends the request and then says so.
	clickSel(t, ana, `.handle-badge[data-handle="bob"]`)
	waitJS(t, ana, "the offer", `document.getElementById("friend-pop-send") !== null`, 10*time.Second)
	clickSel(t, ana, "#friend-pop-send")
	waitJS(t, ana, "the menu goes", `document.getElementById("friend-pop") === null`, 5*time.Second)
	waitForFunc := func() bool { return len(friendsOf(t, bobB).Incoming) == 1 }
	for i := 0; i < 50 && !waitForFunc(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if f := friendsOf(t, bobB); len(f.Incoming) != 1 || f.Incoming[0].Handle != "ana" {
		t.Errorf("Bob's requests: %+v", f.Incoming)
	}
	clickSel(t, ana, `.handle-badge[data-handle="bob"]`)
	waitJS(t, ana, "already sent", `document.getElementById("friend-pop") !== null && document.getElementById("friend-pop").textContent.includes("sent")`, 10*time.Second)
}

// In the Active Games window: its title, two columns of badges, and a short line under each game.
func TestWebFriends_TheActiveGamesWindow(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var duel, table struct {
		RoomCode string `json:"room_code"`
	}
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana", "opponent": "random"}, &duel)
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Ana", "opponent": "random"}, &table)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	games := `[{"code":"` + duel.RoomCode + `","def":"tic-tac-toe","name":"Ana","ts":` + "Date.now()" + `},{"code":"` + table.RoomCode + `","def":"cthulhu-wars","name":"Ana","ts":Date.now()}]`
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 900), chromedp.Navigate(baseURL+"/healthz"), chromedp.Evaluate(`localStorage.setItem("bingle.games", JSON.stringify(`+games+`)); true`, nil), chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the icon", `document.getElementById("open-active-games") !== null`, 20*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#active-games-dialog").getAttribute("aria-label")`); got != "Active Games" {
		t.Errorf("the window is called %q", got)
	}
	clickSel(t, ctx, "#open-active-games")
	row := func(code string) string {
		return `document.querySelector('#active-games-dialog li[data-code="` + code + `"]')`
	}
	waitJS(t, ctx, "the games", row(duel.RoomCode)+` !== null && `+row(table.RoomCode)+` !== null`, 15*time.Second)
	if got := evalString(t, ctx, row(duel.RoomCode)+`.querySelector(".meta").textContent`); !strings.HasPrefix(got, "vs ") || strings.Contains(got, "move") {
		t.Errorf("at a table of two: %q", got)
	}
	if got := evalString(t, ctx, row(table.RoomCode)+`.querySelector(".meta").textContent`); !strings.HasPrefix(got, "4 players") || strings.Contains(got, "move") || strings.Contains(got, "vs") {
		t.Errorf("at a table of four: %q", got)
	}
	if got := evalString(t, ctx, `getComputedStyle(`+row(table.RoomCode)+`.querySelector(".game-badges")).gridTemplateColumns.trim().split(/\s+/).length + ""`); got != "2" {
		t.Errorf("%s columns of badges, want 2", got)
	}
}

// The lobby of Cthulhu Wars has the short title, and pictures of what the table plays with.
func TestWebCthulhuWars_TheLobbyShowsItsPool(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var c struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Ana", "lobby": true, "options": map[string]any{"high-priests": true, "neutral-units": []string{"dimensional-shambler"}}}, &c); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 900), chromedp.Navigate(baseURL+"/game/"+c.RoomCode)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the pool", `document.querySelectorAll("#lobby-pool .lobby-pool-item").length > 0`, 15*time.Second)
	if got := evalString(t, ctx, `document.querySelector(".lobby-head h2").textContent`); got != "Cthulhu Wars" {
		t.Errorf("the title is %q", got)
	}
	got := evalString(t, ctx, `[...document.querySelectorAll("#lobby-pool .lobby-pool-item")].map(e => e.dataset.item).join(",")`)
	for _, want := range []string{"cthulhu-wars", "gc", "cc", "bg", "ys", "high-priests", "dimensional-shambler"} {
		if !strings.Contains(","+got+",", ","+want+",") {
			t.Errorf("%q not in the pool: %s", want, got)
		}
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#lobby-pool .lobby-pool-item[data-kind="faction"]').length`); n != 4 {
		t.Errorf("%d factions in the pool", n)
	}
}

// Somebody can choose to take no invitations, and an answer is kept: a decline is not asked again.
func TestFriends_AnsweringInvitationsAndOptingOut(t *testing.T) {
	a, base := membersServer(t)
	ana, bob := member(t, a, base, "ana"), member(t, a, base, "bob")
	befriend(t, ana, bob, "bob", "ana")
	var one, two struct {
		RoomCode string `json:"room_code"`
	}
	ana.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &one)
	ana.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &two)

	friendPost(ana, "invite", map[string]any{"handle": "bob", "room": one.RoomCode})
	if code := friendPost(ana, "invite-answer", map[string]any{"room": one.RoomCode, "accept": true}); code != 404 {
		t.Errorf("answering somebody else's invitation: %d, want 404", code)
	}
	if code := friendPost(bob, "invite-answer", map[string]any{"room": one.RoomCode, "accept": false}); code != 200 {
		t.Fatalf("Bob declines: %d", code)
	}
	if f := friendsOf(t, bob); len(f.Invites) != 0 {
		t.Errorf("after declining: %+v", f.Invites)
	}
	friendPost(ana, "invite", map[string]any{"handle": "bob", "room": one.RoomCode})
	if f := friendsOf(t, bob); len(f.Invites) != 0 {
		t.Errorf("asked again after declining: %+v", f.Invites)
	}
	// Accepted, it leaves the list too.
	friendPost(ana, "invite", map[string]any{"handle": "bob", "room": two.RoomCode})
	if code := friendPost(bob, "invite-answer", map[string]any{"room": two.RoomCode, "accept": true}); code != 200 {
		t.Fatalf("Bob accepts: %d", code)
	}
	if f := friendsOf(t, bob); len(f.Invites) != 0 {
		t.Errorf("after accepting: %+v", f.Invites)
	}

	// The opt-out: on by default, and it refuses.
	var settings struct {
		AcceptInvites bool `json:"accept_invites"`
	}
	if code := bob.do("GET", "/api/friends/settings", nil, &settings); code != 200 || !settings.AcceptInvites {
		t.Fatalf("the default: %d %+v", code, settings)
	}
	if code := bob.do("PUT", "/api/friends/settings", map[string]any{"accept_invites": false}, nil); code != 200 {
		t.Fatalf("Bob opts out: %d", code)
	}
	var three struct {
		RoomCode string `json:"room_code"`
	}
	ana.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &three)
	if code := friendPost(ana, "invite", map[string]any{"handle": "bob", "room": three.RoomCode}); code != 403 {
		t.Errorf("inviting somebody who opted out: %d, want 403", code)
	}
}
