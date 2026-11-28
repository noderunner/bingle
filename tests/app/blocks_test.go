package app_test

import (
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

type blocksResp struct {
	Blocked []struct {
		Name   string `json:"name"`
		Handle string `json:"handle"`
	} `json:"blocked"`
}

func blocksOf(t *testing.T, b *browser) blocksResp {
	t.Helper()
	var out blocksResp
	if code := b.do("GET", "/api/friends/blocks", nil, &out); code != 200 {
		t.Fatalf("the blocks: %d", code)
	}
	return out
}

// A block ends a friendship and stops requests, invitations, messages and calls, and the one blocked
// gets the answers of somebody who removed them and does not respond (ADR 0156).
func TestBlocks_ABlockIsSilent(t *testing.T) {
	a, base := membersServer(t)
	ana, bob := member(t, a, base, "ana"), member(t, a, base, "bob")
	anaAt := ana.here()
	bob.here()
	befriend(t, ana, bob, "bob", "ana")
	var created struct {
		RoomCode string `json:"room_code"`
	}
	bob.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Bob"}, &created)
	if code := friendPost(bob, "invite", map[string]any{"handle": "ana", "room": created.RoomCode}); code != 200 {
		t.Fatalf("Bob invites Ana: %d", code)
	}

	for _, c := range []struct {
		name, handle string
		code         int
	}{{"nobody", "nobody-at-all", 404}, {"yourself", "ana", 404}, {"Bob", "@bob", 200}, {"Bob again", "bob", 200}} {
		if code := friendPost(ana, "block", map[string]any{"handle": c.handle}); code != c.code {
			t.Errorf("blocking %s: %d, want %d", c.name, code, c.code)
		}
	}
	if f := friendsOf(t, ana); len(f.Friends)+len(f.Incoming)+len(f.Outgoing)+len(f.Invites) != 0 {
		t.Errorf("Ana's list after the block: %+v", f)
	}
	if f := friendsOf(t, bob); len(f.Friends) != 0 {
		t.Errorf("Bob's friends after the block: %+v", f.Friends)
	}
	if b := blocksOf(t, ana); len(b.Blocked) != 1 || b.Blocked[0].Name != "@bob" || b.Blocked[0].Handle != "bob" {
		t.Errorf("Ana's blocks: %+v", b)
	}
	if b := blocksOf(t, bob); len(b.Blocked) != 0 {
		t.Errorf("Bob sees blocks: %+v", b)
	}

	// Bob gets what a removed friend gets.
	if code := bob.do("POST", "/api/dm", map[string]any{"to": anaAt, "text": "hello?"}, nil); code != 403 {
		t.Errorf("a message: %d, want 403", code)
	}
	if code := bob.do("POST", "/api/dm/call", map[string]any{"to": anaAt, "join": true}, nil); code != 403 {
		t.Errorf("a call: %d, want 403", code)
	}
	if code := friendPost(bob, "invite", map[string]any{"handle": "ana", "room": created.RoomCode}); code != 403 {
		t.Errorf("an invitation: %d, want 403", code)
	}
	// His request is taken, and waits as one nobody answers.
	if code := friendPost(bob, "request", map[string]any{"handle": "ana"}); code != 200 {
		t.Errorf("a request: %d, want 200", code)
	}
	if code := friendPost(bob, "request", map[string]any{"handle": "ana"}); code != 409 {
		t.Errorf("asking twice: %d, want 409", code)
	}
	if f := friendsOf(t, bob); len(f.Outgoing) != 1 || f.Outgoing[0].Handle != "ana" {
		t.Errorf("Bob's requests: %+v", f.Outgoing)
	}
	if f := friendsOf(t, ana); len(f.Incoming) != 0 {
		t.Errorf("Ana sees the request: %+v", f.Incoming)
	}

	// Ana herself is told why she cannot.
	if code := friendPost(ana, "request", map[string]any{"handle": "bob"}); code != 409 {
		t.Errorf("Ana asks Bob: %d, want 409", code)
	}
	if code := friendPost(ana, "answer", map[string]any{"handle": "bob", "accept": true}); code != 409 {
		t.Errorf("Ana accepts Bob: %d, want 409", code)
	}

	// Unblocked, what is still waiting shows.
	if code := friendPost(ana, "unblock", map[string]any{"handle": "bob"}); code != 200 {
		t.Fatalf("unblocking: %d", code)
	}
	if b := blocksOf(t, ana); len(b.Blocked) != 0 {
		t.Errorf("after unblocking: %+v", b)
	}
	if f := friendsOf(t, ana); len(f.Incoming) != 1 || len(f.Invites) != 1 {
		t.Errorf("Ana's list after unblocking: %+v", f)
	}
	if code := friendPost(ana, "answer", map[string]any{"handle": "bob", "accept": true}); code != 200 {
		t.Errorf("Ana accepts Bob: %d", code)
	}
}

// Somebody blocked can invite the blocker to a muster or ask to be friends as before, and nothing of it
// reaches the blocker: no line in the friends list and no mail.
func TestBlocks_NothingReachesTheBlocker(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	ana, bob := newBrowser(t, e.URL), newBrowser(t, e.URL)
	if code := ana.login("ana", signUp(t, e, ana, "ana", "ana@example.com")); code != 200 {
		t.Fatal("Ana signs in")
	}
	if code := bob.login("bob", signUp(t, e, bob, "bob", "bob@example.com")); code != 200 {
		t.Fatal("Bob signs in")
	}
	if code := ana.do("PUT", "/api/friends/settings", map[string]any{"mail_requests": true}, nil); code != 200 {
		t.Fatalf("Ana asks for mail: %d", code)
	}
	if code := friendPost(ana, "block", map[string]any{"handle": "bob"}); code != 200 {
		t.Fatalf("Ana blocks Bob: %d", code)
	}
	before := len(e.SMTP.Mails())
	if code := friendPost(bob, "request", map[string]any{"handle": "ana"}); code != 200 {
		t.Errorf("Bob asks: %d", code)
	}
	var created struct {
		RoomCode string `json:"room_code"`
	}
	muster := map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Bob", "real_time": map[string]any{}, "schedule": map[string]any{"slots": []string{inHours(6), inHours(7)}, "invite": []string{"ana"}}}
	if code := bob.do("POST", "/api/games", muster, &created); code != 200 {
		t.Fatalf("Bob's muster: %d", code)
	}
	room := created.RoomCode
	if code := bob.do("POST", "/api/games/"+room+"/schedule/slots", map[string]any{"slots": []string{inHours(8)}}, nil); code != 200 {
		t.Errorf("Bob offers new times: %d", code)
	}
	if sc := scheduleOf(t, bob, room); len(sc.People) != 1 || sc.People[0].Handle != "ana" || sc.People[0].Status != "pending" || sc.People[0].Answered {
		t.Errorf("Bob's muster: %+v", sc.People)
	}
	if f := friendsOf(t, ana); len(f.Incoming)+len(f.Invites) != 0 {
		t.Errorf("Ana's list: %+v", f)
	}
	if code := bob.do("POST", "/api/games/"+room+"/schedule/cancel", map[string]any{}, nil); code != 200 {
		t.Errorf("Bob cancels it: %d", code)
	}
	// Bob's own mail of the closing is the only one.
	e.quiet(t, before+1)
	if to := e.SMTP.Mails()[before].To; len(to) != 1 || to[0] != "bob@example.com" {
		t.Errorf("mailed %v", to)
	}
	if f := friendsOf(t, ana); len(f.Incoming)+len(f.Invites) != 0 {
		t.Errorf("Ana's list: %+v", f)
	}
	// Ana cannot invite Bob to hers.
	var made struct{}
	body := map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana", "real_time": map[string]any{}, "schedule": map[string]any{"slots": []string{inHours(6)}, "invite": []string{"bob"}}}
	if code := ana.do("POST", "/api/games", body, &made); code != 400 {
		t.Errorf("Ana invites Bob: %d, want 400", code)
	}
}

// A friend request is blocked from the friends window, and the block is listed and lifted in the profile.
func TestWebBlocks_FromTheWindowToTheProfile(t *testing.T) {
	a, base := membersServer(t)
	anaB, bobB := member(t, a, base, "ana"), member(t, a, base, "bob")
	if code := friendPost(bobB, "request", map[string]any{"handle": "ana"}); code != 200 {
		t.Fatalf("Bob asks: %d", code)
	}
	ana, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	signInChrome(t, ana, base, anaB.cookie(auth.SessionCookie))
	if err := runSteps(ana, stepNavigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "Ana's icon", `getComputedStyle(document.getElementById("open-friends")).display !== "none"`, 20*time.Second)
	clickSel(t, ana, "#open-friends")
	waitJS(t, ana, "the request", `document.querySelector("#friend-requests [data-block='bob']") !== null`, 10*time.Second)
	if err := runSteps(ana, stepEval(`window.confirm = () => true; true`)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ana, "#friend-requests [data-block='bob']")
	waitJS(t, ana, "the request gone", `document.querySelector("#friend-requests [data-accept]") === null`, 10*time.Second)

	if err := runSteps(ana, stepNavigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "the block listed", `!document.getElementById("blocked-box").hidden && document.querySelector("#blocked-list [data-unblock='bob']") !== null`, 20*time.Second)
	if got := evalString(t, ana, `document.querySelector("#blocked-list li").textContent`); got != "@bobUnblock" {
		t.Errorf("the line says %q", got)
	}
	clickSel(t, ana, "#blocked-list [data-unblock='bob']")
	waitJS(t, ana, "the list empty", `document.getElementById("blocked-box").hidden`, 10*time.Second)
	if f := friendsOf(t, anaB); len(f.Incoming) != 1 {
		t.Errorf("Bob's request after unblocking: %+v", f.Incoming)
	}
}

// A ring from somebody blocked ends as a declined one does, and they cannot ring again.
func TestBlocks_ARingEnds(t *testing.T) {
	a, base := membersServer(t)
	anaB, bobB := member(t, a, base, "ana"), member(t, a, base, "bob")
	ana := anaB.here()
	bobB.here()
	befriend(t, anaB, bobB, "bob", "ana")
	bobR := dmReader(t, base, bobB, "en")
	if code := bobB.do("POST", "/api/dm/call", map[string]any{"to": ana, "join": true}, nil); code != 200 {
		t.Fatalf("Bob rings Ana: %d", code)
	}
	if code := friendPost(anaB, "block", map[string]any{"handle": "bob"}); code != 200 {
		t.Fatalf("Ana blocks Bob: %d", code)
	}
	if ev, ok := nextCallEvent(t, bobR, 5*time.Second); !ok || ev.Kind != "end" || ev.From != ana || !ev.Declined {
		t.Errorf("Bob is told %+v (%v), want a declined end from Ana", ev, ok)
	}
	if code := bobB.do("POST", "/api/dm/signal", map[string]any{"to": ana, "kind": "offer", "data": map[string]any{"sdp": "x"}}, nil); code != 409 {
		t.Errorf("Bob signals: %d, want 409", code)
	}
	if code := bobB.do("POST", "/api/dm/call", map[string]any{"to": ana, "join": true}, nil); code != 403 {
		t.Errorf("Bob rings again: %d, want 403", code)
	}
}
