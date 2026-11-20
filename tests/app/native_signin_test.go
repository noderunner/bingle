package app_test

// A native client's sign-in, answered with a session token (ADR 0138).

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/noderunner/bingle/internal/auth"
)

// nativeAgent is the User-Agent of the native client in these tests.
const nativeAgent = "bingle-3d/0.6 (test)"

// native sends what a client that is not a browser sends: no cookies, no
// X-Bingle-Session, and its credential (if any) as a bearer.
func native(t *testing.T, base, bearer, method, path string, body any, out any) (int, http.Header) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, base+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", nativeAgent)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, out)
	}
	return resp.StatusCode, resp.Header
}

type sessionToken struct {
	Token   string         `json:"token"`
	Expires time.Time      `json:"expires"`
	Me      map[string]any `json:"me"`
	Error   string         `json:"error"`
}

// nativeSignIn signs ana in as a native client, with the guest key given (or
// none), and returns the token.
func nativeSignIn(t *testing.T, base, guestKey string) string {
	t.Helper()
	var out sessionToken
	code, h := native(t, base, guestKey, "POST", "/auth/login", map[string]any{"handle": "ana", "password": anaPassword, "token": true}, &out)
	if code != 200 || !strings.HasPrefix(out.Token, auth.SessionTokenPrefix) || out.Me["handle"] != "ana" || out.Expires.Before(time.Now()) {
		t.Fatalf("a native sign-in: %d %+v", code, out)
	}
	if c := h.Get("Set-Cookie"); strings.Contains(c, auth.SessionCookie+"=") {
		t.Errorf("a native sign-in set the session cookie: %s", c)
	}
	return out.Token
}

func TestNativeSignIn_ATokenIsTheAccountsSession(t *testing.T) {
	_, base := accountServer(t)
	tok := nativeSignIn(t, base, "")

	var me map[string]any
	if code, _ := native(t, base, tok, "GET", "/auth/me", nil, &me); code != 200 || me["kind"] != "account" || me["handle"] != "ana" {
		t.Fatalf("GET /auth/me with the token: %d %v", code, me)
	}
	// It is the whole account, with no page header: making a game, friends, direct messages.
	if code, _ := native(t, base, tok, "POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, nil); code != 200 {
		t.Errorf("making a game with the token: %d", code)
	}
	if code, _ := native(t, base, tok, "GET", "/api/friends", nil, nil); code != 200 {
		t.Errorf("the friends list with the token: %d", code)
	}
	if code, _ := native(t, base, tok, "POST", "/api/dm/threads", map[string]any{}, nil); code != 200 {
		t.Errorf("the direct messages with the token: %d", code)
	}

	// It is listed with the browser's session, under the client's name, as this one.
	var list struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if code, _ := native(t, base, tok, "GET", "/auth/sessions", nil, &list); code != 200 || len(list.Sessions) != 2 {
		t.Fatalf("the sessions: %d %v", code, list)
	}
	found := false
	for _, s := range list.Sessions {
		if s["current"] == true {
			found = true
			if s["agent"] != nativeAgent {
				t.Errorf("the token's session is not named after its client: %v", s)
			}
		}
	}
	if !found {
		t.Errorf("no session is the token's: %v", list.Sessions)
	}

	// /ws-dm takes it in the Authorization header.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(base, "/ws-dm"), &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + tok}}})
	if err != nil {
		t.Fatalf("/ws-dm with the token: %v", err)
	}
	var ev struct{ Type string }
	if err := wsjson.Read(ctx, conn, &ev); err != nil || ev.Type != "ready" {
		t.Errorf("/ws-dm's first event: %v %+v", err, ev)
	}
	_ = conn.CloseNow()

	// Signing out ends it.
	if code, _ := native(t, base, tok, "POST", "/auth/logout", nil, nil); code != 200 {
		t.Fatalf("logout: %d", code)
	}
	var gone sessionToken
	if code, _ := native(t, base, tok, "GET", "/auth/me", nil, &gone); code != http.StatusUnauthorized || gone.Error == "" {
		t.Errorf("the token after signing out: %d %+v, want 401", code, gone)
	}
}

func TestNativeSignIn_EndedByTheOtherSessionsAndANewPassword(t *testing.T) {
	_, base := accountServer(t)
	b := newBrowser(t, base)
	b.login("ana", anaPassword)

	// A browser's "end the others" ends the token.
	tok := nativeSignIn(t, base, "")
	if code := b.do("POST", "/auth/sessions/end-others", nil, nil); code != 200 {
		t.Fatalf("end others: %d", code)
	}
	if code, _ := native(t, base, tok, "GET", "/auth/me", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("the token after the others were ended: %d, want 401", code)
	}

	// A password changed with a token answers with the new one, and ends the rest.
	tok = nativeSignIn(t, base, "")
	other := nativeSignIn(t, base, "")
	var changed sessionToken
	if code, _ := native(t, base, tok, "POST", "/auth/password", map[string]any{"old": anaPassword, "new": "a brand new password"}, &changed); code != 200 || !strings.HasPrefix(changed.Token, auth.SessionTokenPrefix) || changed.Token == tok {
		t.Fatalf("a new password with the token: %d %+v", code, changed)
	}
	for name, old := range map[string]string{"the token that changed it": tok, "another token": other} {
		if code, _ := native(t, base, old, "GET", "/auth/me", nil, nil); code != http.StatusUnauthorized {
			t.Errorf("%s, after a new password: %d, want 401", name, code)
		}
	}
	var me map[string]any
	if code, _ := native(t, base, changed.Token, "GET", "/auth/me", nil, &me); code != 200 || me["kind"] != "account" {
		t.Errorf("the new token: %d %v", code, me)
	}
	if me := b.me(); me["kind"] == "account" {
		t.Errorf("the browser is still signed in after a new password: %v", me)
	}
}

func TestNativeSignIn_TheSecondFactorComesInTheBody(t *testing.T) {
	_, base := accountServer(t)
	b := newBrowser(t, base)
	b.login("ana", anaPassword)
	key, recovery := enableTOTP(t, b)

	var first struct {
		SecondFactor bool   `json:"second_factor"`
		Pending      string `json:"pending"`
		Token        string `json:"token"`
	}
	if code, _ := native(t, base, "", "POST", "/auth/login", map[string]any{"handle": "ana", "password": anaPassword, "token": true}, &first); code != 200 || !first.SecondFactor || first.Pending == "" || first.Token != "" {
		t.Fatalf("the password step: %d %+v", code, first)
	}
	if code, _ := native(t, base, "", "POST", "/auth/second", map[string]any{"pending": "not-a-sign-in", "code": "000000"}, nil); code != http.StatusUnauthorized {
		t.Errorf("a sign-in nobody began: %d, want 401", code)
	}
	next := auth.TOTPCode(key, time.Now().Add(30*time.Second), 6)
	var out sessionToken
	if code, _ := native(t, base, "", "POST", "/auth/second", map[string]any{"pending": first.Pending, "code": next}, &out); code != 200 || !strings.HasPrefix(out.Token, auth.SessionTokenPrefix) || out.Me["handle"] != "ana" {
		t.Fatalf("the code step: %d %+v", code, out)
	}
	if code, _ := native(t, base, out.Token, "GET", "/auth/me", nil, nil); code != 200 {
		t.Errorf("the token after the code: %d", code)
	}
	// A recovery code works the same way.
	native(t, base, "", "POST", "/auth/login", map[string]any{"handle": "ana", "password": anaPassword, "token": true}, &first)
	if code, _ := native(t, base, "", "POST", "/auth/second", map[string]any{"pending": first.Pending, "recovery_code": recovery[0]}, &out); code != 200 || out.Token == "" {
		t.Errorf("a recovery code: %d %+v", code, out)
	}
}

// A guest key sent with the sign-in brings the guest's seats into the
// account, as a browser's guest cookie does, and stops working.
func TestNativeSignIn_TheGuestSentAlongBecomesTheAccounts(t *testing.T) {
	_, base := accountServer(t)
	var guest struct {
		Key string `json:"key"`
	}
	if code, _ := native(t, base, "", "POST", "/api/guest", nil, &guest); code != http.StatusCreated {
		t.Fatalf("a guest: %d", code)
	}
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code, _ := native(t, base, guest.Key, "POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana Guest"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if code, _ := native(t, base, newGuestKey(t, base), "POST", "/api/games/"+created.RoomCode+"/join", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	tok := nativeSignIn(t, base, guest.Key)
	if code, _ := native(t, base, tok, "POST", "/api/games/"+created.RoomCode+"/moves", map[string]any{"player_name": "Ana Guest", "notation": "a1"}, nil); code != 200 {
		t.Errorf("the token plays the guest's seat: %d, want 200", code)
	}
	if code, _ := native(t, base, guest.Key, "GET", "/auth/me", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("the guest key after signing in: %d, want 401", code)
	}
}
