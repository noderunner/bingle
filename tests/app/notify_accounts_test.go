package app_test

// Email notifications are for accounts (ADR 0073), with the real defaults:
// a guest makes no contact, a confirmed one keeps its mail, and a confirmed
// contact becomes an account without a second confirmation.

import (
	"context"
	"net/http"
	"testing"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

// accountsNotifyEnv is a notify env with the defaults: notifications are an
// account's.
func accountsNotifyEnv(t *testing.T) *notifyEnv {
	t.Helper()
	return newNotifyEnv(t, fakesmtp.Options{}, func(c *config.Config) { c.CapsFile = testCapsFile })
}

// confirmAs saves and confirms address as the browser b.
func (e *notifyEnv) confirmAs(t *testing.T, b *browser, address string) contactResp {
	t.Helper()
	n := len(e.SMTP.Mails())
	var c contactResp
	if code := b.do("POST", "/api/notify/contact", map[string]any{"address": address}, &c); code != 200 || c.Secret == "" {
		t.Fatalf("save: %d %+v", code, c)
	}
	m := e.waitMails(t, n+1)
	if st, body := post(t, e.URL+link(t, m[len(m)-1], "confirm")); st != 200 {
		t.Fatalf("confirm: %d %s", st, body)
	}
	return c
}

// setGameNotifyAs switches a seat's turn mail on as the browser b.
func setGameNotifyAs(t *testing.T, b *browser, room, name string, c contactResp) int {
	t.Helper()
	return b.do("PUT", "/api/games/"+room+"/notify", map[string]any{"player_name": name, "contact_id": c.ID, "contact_secret": c.Secret, "enabled": true}, nil)
}

func TestNotifyAccounts_AGuestMakesNoContact(t *testing.T) {
	e := accountsNotifyEnv(t)
	if code := newBrowser(t, e.URL).do("POST", "/api/notify/contact", map[string]any{"address": "gus@example.com"}, nil); code != http.StatusForbidden {
		t.Errorf("a guest's contact: %d, want 403", code)
	}
}

// An account sets up its turn mail and gets it.
func TestNotifyAccounts_AnAccountGetsItsTurnMail(t *testing.T) {
	e := accountsNotifyEnv(t)
	alice := member(t, e.App, e.URL, "alice")
	var g struct {
		RoomCode string `json:"room_code"`
	}
	alice.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice"}, &g)
	bob := newBrowser(t, e.URL)
	bob.do("POST", "/api/games/"+g.RoomCode+"/join", map[string]any{"player_name": "Bob"}, nil)
	c := e.confirmAs(t, alice, "alice@example.com")
	if code := setGameNotifyAs(t, alice, g.RoomCode, "Alice", c); code != 200 {
		t.Fatalf("switch on: %d", code)
	}
	n := len(e.SMTP.Mails())
	alice.do("POST", "/api/games/"+g.RoomCode+"/moves", map[string]any{"player_name": "Alice", "notation": "a1"}, nil)
	bob.do("POST", "/api/games/"+g.RoomCode+"/moves", map[string]any{"player_name": "Bob", "notation": "b1"}, nil)
	e.waitMails(t, n+1) // Alice's turn again
}

// grandfather makes a guest with a confirmed contact and its turn mail on,
// as a guest could before notifications were an account's, and then takes
// notifications away from guests. It returns the guest, the room and the
// contact.
func grandfather(t *testing.T, e *notifyEnv) (*browser, string, contactResp) {
	t.Helper()
	ctx := context.Background()
	if err := e.App.Caps().Set(ctx, auth.ClassGuest, auth.CapNotify, true); err != nil {
		t.Fatal(err)
	}
	guest := newBrowser(t, e.URL)
	var g struct {
		RoomCode string `json:"room_code"`
	}
	guest.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice"}, &g)
	c := e.confirmAs(t, guest, "alice@example.com")
	if code := setGameNotifyAs(t, guest, g.RoomCode, "Alice", c); code != 200 {
		t.Fatalf("switch on: %d", code)
	}
	if err := e.App.Caps().Set(ctx, auth.ClassGuest, auth.CapNotify, false); err != nil {
		t.Fatal(err)
	}
	return guest, g.RoomCode, c
}

// A guest's confirmed contact keeps its turn mail; the guest just cannot
// change it.
func TestNotifyAccounts_AConfirmedContactIsGrandfathered(t *testing.T) {
	e := accountsNotifyEnv(t)
	guest, room, c := grandfather(t, e)
	bob := newBrowser(t, e.URL)
	bob.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Bob"}, nil)
	n := len(e.SMTP.Mails())
	guest.do("POST", "/api/games/"+room+"/moves", map[string]any{"player_name": "Alice", "notation": "a1"}, nil)
	bob.do("POST", "/api/games/"+room+"/moves", map[string]any{"player_name": "Bob", "notation": "b1"}, nil)
	e.waitMails(t, n+1)
	if code := setGameNotifyAs(t, guest, room, "Alice", c); code != http.StatusForbidden {
		t.Errorf("the guest changes the setting: %d, want 403", code)
	}
}

// A confirmed contact becomes an account with a handle and a password: no
// second confirmation, the guest's games come along, and the mail goes on.
func TestNotifyAccounts_AConfirmedContactBecomesAnAccount(t *testing.T) {
	e := accountsNotifyEnv(t)
	guest, room, c := grandfather(t, e)
	n := len(e.SMTP.Mails())
	var me map[string]any
	if code := guest.do("POST", "/auth/signup", map[string]any{"contact_id": c.ID, "contact_secret": c.Secret, "handle": "alice", "password": "alice's own password"}, &me); code != 200 {
		t.Fatalf("upgrade: %d %v", code, me)
	}
	if me["kind"] != "account" || me["handle"] != "alice" || me["email"] != "alice@example.com" {
		t.Errorf("the account: %v", me)
	}
	e.quiet(t, n) // no confirmation mail: the address was confirmed already
	// The guest's seat is the account's, and the account may change its mail.
	if code := setGameNotifyAs(t, guest, room, "Alice", c); code != 200 {
		t.Errorf("the account changes its seat's setting: %d", code)
	}
	// Somebody else's contact secret is no way in.
	other := newBrowser(t, e.URL)
	if code := other.do("POST", "/auth/signup", map[string]any{"contact_id": c.ID, "contact_secret": "guess", "handle": "mallory", "password": "mallory's password"}, nil); code != http.StatusForbidden {
		t.Errorf("a wrong contact secret: %d, want 403", code)
	}
}
