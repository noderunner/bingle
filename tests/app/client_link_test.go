package app_test

// A link that opens the 3D client at a game (ADR 0163): the page asks for a
// code good once and for a minute, and the client trades it for a sign-in of
// its own as the same player.

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
)

// linkCode is the form of every code a link may carry.
var linkCode = regexp.MustCompile(`^bg[cj]_[A-Za-z0-9_-]{43}$`)

// askCode asks for a link's code as b's page does.
func askCode(t *testing.T, b *browser) (int, string) {
	t.Helper()
	var out struct{ Code, Error string }
	status := b.do("POST", "/api/client/code", nil, &out)
	return status, out.Code
}

// clientSignIn is POST /auth/client's answer.
type clientSignIn struct {
	sessionToken
	GuestKey string `json:"guest_key"`
}

// trade spends a link's code as a client does, bringing bearer if it is set.
func trade(t *testing.T, base, bearer, code string) (int, clientSignIn) {
	t.Helper()
	var out clientSignIn
	status, _ := native(t, base, bearer, "POST", "/auth/client", map[string]any{"code": code}, &out)
	return status, out
}

func TestClientLink_AnAccountsCodeSignsTheClientInOnceWithinAMinute(t *testing.T) {
	a, base, ana := clientServer(t)
	status, code := askCode(t, ana)
	if status != 200 || !linkCode.MatchString(code) || !strings.HasPrefix(code, auth.SignInCodePrefix) {
		t.Fatalf("asking for a code: %d %q", status, code)
	}
	// A GET makes none.
	var got map[string]any
	if c := ana.do("GET", "/api/client/code", nil, &got); c == 200 || got["code"] != nil {
		t.Errorf("a GET for a code: %d %v", c, got)
	}

	status, out := trade(t, base, "", code)
	if status != 200 || !strings.HasPrefix(out.Token, auth.SessionTokenPrefix) || out.Me["handle"] != "ana" || out.GuestKey != "" {
		t.Fatalf("trading the code: %d %+v", status, out)
	}
	// The client's own session, not the browser's: listed beside it under the client's name.
	if strings.TrimPrefix(out.Token, auth.SessionTokenPrefix) == ana.cookie(auth.SessionCookie) {
		t.Error("the client was handed the browser's session")
	}
	var me map[string]any
	if c, _ := native(t, base, out.Token, "GET", "/auth/me", nil, &me); c != 200 || me["handle"] != "ana" {
		t.Errorf("the client's session: %d %v", c, me)
	}
	var list struct {
		Sessions []map[string]any `json:"sessions"`
	}
	ana.do("GET", "/auth/sessions", nil, &list)
	agents := map[any]bool{}
	for _, s := range list.Sessions {
		agents[s["agent"]] = true
	}
	if len(list.Sessions) != 2 || !agents[nativeAgent] {
		t.Errorf("the sessions: %v", list.Sessions)
	}

	// Once.
	if status, out := trade(t, base, "", code); status != http.StatusUnauthorized || out.Token != "" {
		t.Errorf("trading the code again: %d %+v", status, out)
	}
	// For a minute.
	_, late := askCode(t, ana)
	a.Accounts().SetClock(func() time.Time { return time.Now().Add(time.Minute + time.Second) })
	if status, _ := trade(t, base, "", late); status != http.StatusUnauthorized {
		t.Errorf("a code a minute old: %d, want 401", status)
	}
	a.Accounts().SetClock(time.Now)
	// And not past signing out everywhere else.
	_, unspent := askCode(t, ana)
	if c := ana.do("POST", "/auth/sessions/end-others", nil, nil); c != 200 {
		t.Fatalf("ending the other sessions: %d", c)
	}
	if status, _ := trade(t, base, "", unspent); status != http.StatusUnauthorized {
		t.Errorf("a code after the other sessions were ended: %d, want 401", status)
	}
	for _, bad := range []string{"", "nothing", auth.GuestCodePrefix + "nothing", ana.cookie(auth.SessionCookie)} {
		if status, out := trade(t, base, "", bad); status != http.StatusUnauthorized || out.Token != "" || out.GuestKey != "" {
			t.Errorf("a code that is not one (%q): %d", bad, status)
		}
	}
}

func TestClientLink_AGuestsCodeIsANewKeyOfTheSameGuest(t *testing.T) {
	a, base, _ := clientServer(t)
	gil := newBrowser(t, base)
	var created createGameResponse
	if c := gil.do("POST", "/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Gil"}, &created); c != 200 {
		t.Fatalf("a game: %d", c)
	}
	status, code := askCode(t, gil)
	if status != 200 || !linkCode.MatchString(code) || !strings.HasPrefix(code, auth.GuestCodePrefix) {
		t.Fatalf("a guest asking for a code: %d %q", status, code)
	}
	status, out := trade(t, base, "", code)
	if status != 200 || !strings.HasPrefix(out.GuestKey, auth.GuestKeyPrefix) || out.Token != "" {
		t.Fatalf("trading a guest's code: %d %+v", status, out)
	}
	if out.GuestKey == gil.cookie(auth.CookieName) {
		t.Error("the client was handed the browser's own key")
	}
	// The same guest: the client may come back to the browser's seat, and another guest may not.
	sit := func(key string) int {
		c, _ := native(t, base, key, "POST", "/api/games/"+created.RoomCode+"/join", map[string]any{"player_name": "Gil", "role": "player"}, nil)
		return c
	}
	if c := sit(out.GuestKey); c != 200 {
		t.Errorf("the client in the browser's seat: %d", c)
	}
	var other auth.GuestKey
	if c := doJSON(t, "POST", base+"/api/guest", nil, &other); c != http.StatusCreated || other.Key == "" {
		t.Fatalf("another guest: %d", c)
	}
	if c := sit(other.Key); c != http.StatusForbidden {
		t.Errorf("another guest in the browser's seat: %d, want 403", c)
	}
	if status, _ := trade(t, base, "", code); status != http.StatusUnauthorized {
		t.Errorf("trading a guest's code again: %d, want 401", status)
	}

	// A guest the client brings goes with it, as on signing in.
	_, code = askCode(t, gil)
	if status, out := trade(t, base, other.Key, code); status != 200 || out.GuestKey == "" {
		t.Fatalf("trading with the client's own guest: %d %+v", status, out)
	}
	if c, _ := native(t, base, other.Key, "GET", "/auth/me", nil, nil); c != http.StatusUnauthorized {
		t.Errorf("the client's old guest still works: %d", c)
	}

	// For a minute.
	_, code = askCode(t, gil)
	a.Resolver().SetClock(func() time.Time { return time.Now().Add(time.Minute + time.Second) })
	status, _ = trade(t, base, "", code)
	a.Resolver().SetClock(time.Now)
	if status != http.StatusUnauthorized {
		t.Errorf("a guest's code a minute old: %d, want 401", status)
	}
	// A new key for the browser ends the codes it has not traded; so does giving the guest up.
	_, code = askCode(t, gil)
	if c := gil.do("POST", "/api/guest/rotate", nil, nil); c != 200 {
		t.Fatalf("a new key: %d", c)
	}
	if status, _ := trade(t, base, "", code); status != http.StatusUnauthorized {
		t.Errorf("a code after a new key: %d, want 401", status)
	}
	_, code = askCode(t, gil)
	if c := gil.do("POST", "/api/guest/discard", nil, nil); c != 200 {
		t.Fatalf("giving the guest up: %d", c)
	}
	if status, _ := trade(t, base, "", code); status != http.StatusUnauthorized {
		t.Errorf("a code after the guest was given up: %d, want 401", status)
	}
}

func TestClientLink_NoCodeForAnAdministratorOrAToken(t *testing.T) {
	a, base, ana := clientServer(t)
	boss, err := a.Accounts().Create(context.Background(), "boss", "boss@example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	admin := newBrowser(t, base)
	admin.login("boss", boss)
	if c := admin.do("POST", "/auth/password", map[string]any{"old": boss, "new": "the boss's own password"}, nil); c != 200 {
		t.Fatalf("the administrator's password: %d", c)
	}
	if status, code := askCode(t, admin); status != http.StatusForbidden || code != "" {
		t.Errorf("an administrator's code: %d %q, want 403", status, code)
	}
	var made struct{ Token string }
	if c := ana.do("POST", "/auth/tokens", map[string]any{"name": "a script", "scopes": []string{"play"}}, &made); c != http.StatusCreated {
		t.Fatalf("a token: %d", c)
	}
	if c, _ := native(t, base, made.Token, "POST", "/api/client/code", nil, nil); c != http.StatusForbidden {
		t.Errorf("an API token's code: %d, want 403", c)
	}
	// A request proved by the cookie must come from the page.
	req, _ := http.NewRequest("POST", base+"/api/client/code", nil)
	resp, err := ana.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a code asked for from another site: %d, want 403", resp.StatusCode)
	}

	// Each player is given only so many an hour.
	limited := 0
	for range 32 {
		if status, _ := askCode(t, ana); status == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited != 2 {
		t.Errorf("of 32 codes in an hour, %d were refused, want 2", limited)
	}
}

func TestClientLink_WithoutBuildsThereIsNoCode(t *testing.T) {
	base := newRESTTestServer(t)
	b := newBrowser(t, base)
	if status, code := askCode(t, b); status != http.StatusNotFound || code != "" {
		t.Errorf("a code from a server that offers no client: %d %q, want 404", status, code)
	}
	if status, _ := trade(t, base, "", auth.SignInCodePrefix+strings.Repeat("A", 43)); status != http.StatusUnauthorized {
		t.Errorf("trading there: %d, want 401", status)
	}
}

// lockedBuffer is a log both the server's goroutines and the test write and read.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestClientLink_TheCodeIsNeitherLoggedNorCounted(t *testing.T) {
	var logs lockedBuffer
	a, base, ana := clientServer(t, app.WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	gil := newBrowser(t, base)
	var codes []string
	for _, b := range []*browser{ana, gil} {
		_, code := askCode(t, b)
		if code == "" {
			t.Fatal("no code")
		}
		codes = append(codes, code)
		// Twice: once traded, once refused.
		trade(t, base, "", code)
		trade(t, base, "", code)
	}
	rec := httptest.NewRecorder()
	a.MetricsHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	metrics, _ := io.ReadAll(rec.Body)
	if !strings.Contains(logs.String(), "/auth/client") {
		t.Fatalf("the trades are not in the log at all:\n%s", logs.String())
	}
	for _, code := range codes {
		secret := code[len("bgc_"):]
		if strings.Contains(logs.String(), secret) || strings.Contains(string(metrics), secret) {
			t.Errorf("the code %s is in the log or the metrics", code)
		}
	}
}
