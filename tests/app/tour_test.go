package app_test

// The welcome tour's mark (ADR 0139): a guest or an account that skipped or
// finished the tour is not offered it again, and the mark goes where the
// guest's seats go.

import (
	"net/http"
	"slices"
	"testing"

	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

func tourSeen(b *browser) bool {
	b.t.Helper()
	seen, _ := b.me()["tour_seen"].(bool)
	return seen
}

func markTourSeen(b *browser) {
	b.t.Helper()
	var out map[string]any
	if code := b.do("POST", "/auth/tour-seen", nil, &out); code != 200 || out["tour_seen"] != true {
		b.t.Fatalf("POST /auth/tour-seen: %d %v", code, out)
	}
}

// makeGuest makes b a guest the way a page does, by its first request.
func makeGuest(b *browser) {
	b.t.Helper()
	if code := b.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Somebody"}, nil); code != 200 {
		b.t.Fatalf("create: %d", code)
	}
}

// A newcomer has not seen the tour; skipping it makes the browser's guest,
// which has seen it from then on. Another guest has not.
func TestTour_AGuestIsOfferedItOnce(t *testing.T) {
	base := newRESTTestServer(t)
	b := newBrowser(t, base)
	if me := b.me(); me["kind"] != "anonymous" || tourSeen(b) {
		t.Fatalf("a newcomer: %v", me)
	}
	markTourSeen(b)
	if me := b.me(); me["kind"] != "guest" || !tourSeen(b) {
		t.Errorf("after skipping the tour: %v, want a guest who has seen it", me)
	}
	markTourSeen(b) // again: nothing changes, and nothing fails
	other := newBrowser(t, base)
	makeGuest(other)
	if tourSeen(other) {
		t.Error("another guest has seen the tour too")
	}
}

// A client with no guest and no page header has nobody to mark.
func TestTour_NobodyCannotMarkIt(t *testing.T) {
	base := newRESTTestServer(t)
	req, _ := http.NewRequest("POST", base+"/auth/tour-seen", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /auth/tour-seen from nobody: %d, want 401", resp.StatusCode)
	}
}

// An account that has seen the tour has seen it on every device.
func TestTour_AnAccountIsOfferedItOnceEverywhere(t *testing.T) {
	_, base := accountServer(t)
	laptop := newBrowser(t, base)
	laptop.login("ana", anaPassword)
	if tourSeen(laptop) {
		t.Fatal("an account made before the tour has seen it")
	}
	markTourSeen(laptop)
	phone := newBrowser(t, base)
	if code := phone.login("ana", anaPassword); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if !tourSeen(phone) {
		t.Error("the account's other device is offered the tour again")
	}
}

// Signing in brings the browser's guest along, and with it having seen the
// tour; a guest who has not seen it takes nothing away from an account that has.
func TestTour_SigningInKeepsItSeen(t *testing.T) {
	_, base := accountServer(t)
	guest := newBrowser(t, base)
	markTourSeen(guest)
	if code := guest.login("ana", anaPassword); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if !tourSeen(guest) {
		t.Error("the guest had seen the tour, the account it signed in to has not")
	}
	fresh := newBrowser(t, base)
	makeGuest(fresh)
	if code := fresh.login("ana", anaPassword); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if !tourSeen(fresh) {
		t.Error("a guest who had not seen the tour made the account forget it")
	}
}

// A guest who signs up is the account, and has still seen the tour.
func TestTour_SigningUpKeepsItSeen(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	b := newBrowser(t, e.URL)
	markTourSeen(b)
	pw := signUp(t, e, b, "ana", "ana@example.com")
	if code := b.login("ana", pw); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if me := b.me(); me["kind"] != "account" || !tourSeen(b) {
		t.Errorf("the new account: %v, want it to have seen the tour", me)
	}
}

// Taking another device's guest key keeps the tour seen, whichever of the
// two guests had seen it.
func TestTour_AdoptingAGuestKeyKeepsItSeen(t *testing.T) {
	base := newRESTTestServer(t)
	for _, seenBy := range []string{"the key's guest", "this browser's guest"} {
		phone, laptop := newBrowser(t, base), newBrowser(t, base)
		makeGuest(phone)
		makeGuest(laptop)
		if seenBy == "the key's guest" {
			markTourSeen(phone)
		} else {
			markTourSeen(laptop)
		}
		var key struct {
			Key string `json:"key"`
		}
		if code := phone.do("GET", "/api/guest/key", nil, &key); code != 200 || key.Key == "" {
			t.Fatalf("the phone's key: %d", code)
		}
		if code := laptop.do("POST", "/api/guest/adopt", map[string]any{"key": key.Key}, nil); code != 200 {
			t.Fatalf("adopt: %d", code)
		}
		if !tourSeen(laptop) {
			t.Errorf("seen by %s: the laptop, now the phone's guest, has not seen the tour", seenBy)
		}
	}
}

// A newcomer or a guest is told what an account may do here, so that the
// tour can say what signing up adds; an account is not.
func TestTour_AGuestIsToldWhatAnAccountMayDo(t *testing.T) {
	_, base := accountServer(t)
	guest := newBrowser(t, base)
	me := guest.me()
	accountCaps := stringList(me["account_caps"])
	caps := stringList(me["caps"])
	if !slices.Contains(accountCaps, "dm") || slices.Contains(caps, "dm") {
		t.Errorf("a newcomer: caps %v, account_caps %v; want dm for an account only", caps, accountCaps)
	}
	ana := newBrowser(t, base)
	ana.login("ana", anaPassword)
	if got := ana.me()["account_caps"]; got != nil {
		t.Errorf("an account is told account_caps %v", got)
	}
}

func stringList(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
