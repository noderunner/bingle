package app_test

// Identity in the interface (stage 7 of the accounts plan, ADR 0072):
// handles kept for their accounts, who holds each seat, "My games" on any
// device, and a guest key carried to another device.

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/auth"
)

// A guest cannot go by an account's handle, or by one nobody may have.
func TestIdentity_AGuestCannotTakeAHandle(t *testing.T) {
	a, base := accountServer(t) // @ana
	guest := newBrowser(t, base)
	for _, name := range []string{"ana", "@Ana", "ADMIN", "support"} {
		if code := guest.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": name}, nil); code != http.StatusConflict {
			t.Errorf("a guest called %q: %d, want 409", name, code)
		}
	}
	// A name that only looks like a handle part-way is fine.
	if code := guest.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana Lima"}, nil); code != 200 {
		t.Errorf("\"Ana Lima\": %d, want 200", code)
	}
	// The account itself may.
	ana := newBrowser(t, base)
	ana.login("ana", anaPassword)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := ana.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &created); code != 200 {
		t.Fatalf("@ana as Ana: %d", code)
	}
	// Nor can a guest speak in a game under the handle.
	if code := guest.do("POST", "/api/games/"+created.RoomCode+"/chat", map[string]any{"player_name": "ana", "text": "it is me"}, nil); code == 200 {
		t.Errorf("a spectator speaking as ana was let through")
	}
	_ = a
}

// A handle renamed away from is held for its old owner for 30 days.
func TestIdentity_ARenamedHandleIsHeld(t *testing.T) {
	a, base := accountServer(t)
	ana := newBrowser(t, base)
	ana.login("ana", anaPassword)
	var me map[string]any
	if code := ana.do("POST", "/auth/handle", map[string]any{"handle": "anabel"}, &me); code != 200 || me["handle"] != "anabel" {
		t.Fatalf("rename: %d %v", code, me)
	}
	if _, err := a.Accounts().Create(context.Background(), "ana", "other@example.com", false); err == nil {
		t.Error("somebody else took the held handle")
	}
	if code := newBrowser(t, base).do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "ana"}, nil); code != http.StatusConflict {
		t.Errorf("a guest called ana while it is held: %d, want 409", code)
	}
	// Its owner can take it back.
	if code := ana.do("POST", "/auth/handle", map[string]any{"handle": "ana"}, &me); code != 200 || me["handle"] != "ana" {
		t.Errorf("taking it back: %d %v", code, me)
	}
}

// The account's page shows its own address and nobody else's; "My games"
// follows the account to a second session.
func TestIdentity_MyGamesFollowTheAccount(t *testing.T) {
	a, base := accountServer(t)
	if _, err := a.Accounts().Create(context.Background(), "bob", "bob.secret@example.com", false); err != nil {
		t.Fatal(err)
	}
	laptop := newBrowser(t, base)
	laptop.login("ana", anaPassword)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	laptop.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &created)
	phone := newBrowser(t, base)
	phone.login("ana", anaPassword)
	var mine struct {
		Games []struct {
			RoomCode string   `json:"room_code"`
			Seats    []string `json:"seats"`
		} `json:"games"`
	}
	if code := phone.do("GET", "/api/me/games", nil, &mine); code != 200 || len(mine.Games) != 1 || mine.Games[0].RoomCode != created.RoomCode || mine.Games[0].Seats[0] != "Ana" {
		t.Fatalf("my games on the phone: %d %+v", code, mine)
	}
	me := phone.me()
	if me["email"] != "ana@example.com" {
		t.Errorf("the account's own address: %v", me["email"])
	}
	for _, path := range []string{"/auth/me", "/api/me/games", "/api/games/" + created.RoomCode} {
		resp, err := phone.c.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if strings.Contains(string(b), "bob.secret") {
			t.Errorf("%s shows another account's address", path)
		}
	}
}

// A seat says who holds it: an account by its handle, or a guest — and
// which seats are the asker's.
func TestIdentity_SeatsAndLinesSayWhoHoldsThem(t *testing.T) {
	_, base := accountServer(t)
	ana := newBrowser(t, base)
	ana.login("ana", anaPassword)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	ana.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &created)
	guest := newBrowser(t, base)
	guest.do("POST", "/api/games/"+created.RoomCode+"/join", map[string]any{"player_name": "Gus"}, nil)
	guest.do("POST", "/api/games/"+created.RoomCode+"/chat", map[string]any{"player_name": "Gus", "text": "hi"}, nil)
	var game struct {
		Seats []struct {
			Name, Handle string
			Guest, Yours bool
		} `json:"seats"`
	}
	guest.do("GET", "/api/games/"+created.RoomCode, nil, &game)
	if len(game.Seats) != 2 || game.Seats[0].Handle != "ana" || game.Seats[0].Yours || !game.Seats[1].Guest || !game.Seats[1].Yours {
		t.Errorf("seats as Gus sees them: %+v", game.Seats)
	}
	var chat []struct {
		Name   string `json:"name"`
		Guest  bool   `json:"guest"`
		Handle string `json:"handle"`
	}
	doJSON(t, "GET", base+"/api/games/"+created.RoomCode+"/chat", nil, &chat)
	if len(chat) != 1 || !chat[0].Guest || chat[0].Handle != "" {
		t.Errorf("the chat line: %+v", chat)
	}
}

// The guest key, copied from one browser and adopted in another, resumes
// every game there, and brings the other browser's own games along.
func TestIdentity_AGuestKeyMovesToAnotherDevice(t *testing.T) {
	base := newRESTTestServer(t)
	laptop := newBrowser(t, base)
	var g1 struct {
		RoomCode string `json:"room_code"`
	}
	laptop.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Lia"}, &g1)
	doJSONAs(t, newGuestKey(t, base), "POST", base+"/api/games/"+g1.RoomCode+"/join", map[string]any{"player_name": "Bob"}, nil)
	var got struct{ Key, Link string }
	if code := laptop.do("GET", "/api/guest/key", nil, &got); code != 200 || got.Key != laptop.cookie(auth.CookieName) || got.Link != "/#guest="+got.Key {
		t.Fatalf("the key: %d %+v", code, got)
	}
	// The phone has a game of its own already.
	phone := newBrowser(t, base)
	var g2 struct {
		RoomCode string `json:"room_code"`
	}
	phone.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Lia2"}, &g2)
	if code := phone.do("POST", "/api/guest/adopt", map[string]any{"key": got.Key}, nil); code != 200 {
		t.Fatalf("adopt: %d", code)
	}
	if code := phone.do("POST", "/api/games/"+g1.RoomCode+"/moves", map[string]any{"player_name": "Lia", "notation": "a1"}, nil); code != 200 {
		t.Errorf("the phone plays the laptop's seat: %d", code)
	}
	var mine struct {
		Games []struct {
			RoomCode string `json:"room_code"`
		} `json:"games"`
	}
	laptop.do("GET", "/api/me/games", nil, &mine)
	if len(mine.Games) != 2 {
		t.Errorf("after the phone adopted the key, the guest has %d games, want both: %+v", len(mine.Games), mine)
	}
}

// A new key leaves the guest and its seats, and retires the old key
// everywhere; starting over leaves the old seats behind.
func TestIdentity_ANewKeyAndStartingOver(t *testing.T) {
	base := newRESTTestServer(t)
	b := newBrowser(t, base)
	var g struct {
		RoomCode string `json:"room_code"`
	}
	b.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ria"}, &g)
	doJSONAs(t, newGuestKey(t, base), "POST", base+"/api/games/"+g.RoomCode+"/join", map[string]any{"player_name": "Bob"}, nil)
	old := b.cookie(auth.CookieName)
	if code := b.do("POST", "/api/guest/rotate", nil, nil); code != 200 || b.cookie(auth.CookieName) == old {
		t.Fatalf("rotate: %d", code)
	}
	if code := moveAs(t, old, base, g.RoomCode, "Ria", "a1"); code != http.StatusUnauthorized {
		t.Errorf("the old key: %d, want 401", code)
	}
	if code := b.do("POST", "/api/games/"+g.RoomCode+"/moves", map[string]any{"player_name": "Ria", "notation": "a1"}, nil); code != 200 {
		t.Errorf("the new key plays the seat: %d", code)
	}
	if code := b.do("POST", "/api/guest/discard", nil, nil); code != 200 || b.cookie(auth.CookieName) != "" {
		t.Fatalf("discard: %d", code)
	}
	if code := b.do("POST", "/api/games/"+g.RoomCode+"/undo", map[string]any{"player_name": "Ria"}, nil); code != http.StatusForbidden {
		t.Errorf("after starting over, the old seat: %d, want 403", code)
	}
}

// Adoption answers every bad key alike, and only so often.
func TestIdentity_AdoptionIsUniformAndLimited(t *testing.T) {
	base := newRESTTestServer(t)
	b := newBrowser(t, base)
	var e1, e2 map[string]string
	c1 := b.do("POST", "/api/guest/adopt", map[string]any{"key": "bgg_made-up"}, &e1)
	revoked := newBrowser(t, base)
	revoked.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Zed"}, nil)
	k := revoked.cookie(auth.CookieName)
	revoked.do("POST", "/api/guest/rotate", nil, nil)
	c2 := b.do("POST", "/api/guest/adopt", map[string]any{"key": k}, &e2)
	if c1 != http.StatusBadRequest || c1 != c2 || e1["error"] != e2["error"] {
		t.Errorf("a made-up key: %d %q; a replaced one: %d %q", c1, e1["error"], c2, e2["error"])
	}
	code := 0
	for i := 0; i < 12 && code != http.StatusTooManyRequests; i++ {
		code = b.do("POST", "/api/guest/adopt", map[string]any{"key": "bgg_guess"}, nil)
	}
	if code != http.StatusTooManyRequests {
		t.Errorf("a dozen guesses: last %d, want 429", code)
	}
}
