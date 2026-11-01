package app_test

// Capabilities, modes, API tokens and invitations (ADR 0071).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

func capsFile(t *testing.T, yaml string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "caps.yaml")
	if err := os.WriteFile(p, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const noGuestGames = `classes:
  guest: [play, join, chat_game]
  user:  [play, join, create_game, chat_game, run_batch]
`

const guestGames = `classes:
  guest: [play, join, create_game, chat_game]
  user:  [play, join, create_game, chat_game, run_batch]
`

func serve(t *testing.T, cfg config.Config) (*app.App, string) {
	t.Helper()
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	registerFixtures(t, a)
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return a, srv.URL
}

func createAs(t *testing.T, key, base string) int {
	t.Helper()
	return doJSONAs(t, key, "POST", base+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ann"}, nil)
}

// The file seeds a fresh database, and is not read again: the database is
// the truth. `bingle caps reset` applies it deliberately.
func TestCaps_TheFileSeedsOnceAndResetAppliesItAgain(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.CapsFile = capsFile(t, noGuestGames)
	a, base := serve(t, cfg)
	if code := createAs(t, "", base); code != http.StatusForbidden {
		t.Fatalf("a guest making a game, not allowed: %d", code)
	}
	_ = a.Close()

	// The file changes; a restart does not read it.
	if err := os.WriteFile(cfg.CapsFile, []byte(guestGames), 0o600); err != nil {
		t.Fatal(err)
	}
	a2, base2 := serve(t, cfg)
	if code := createAs(t, "", base2); code != http.StatusForbidden {
		t.Errorf("after a restart with a new file: %d, want the database's 403", code)
	}
	if err := a2.Caps().Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	if code := createAs(t, "", base2); code != 200 {
		t.Errorf("after caps reset: %d, want 200", code)
	}
}

func TestCaps_AStrictFile(t *testing.T) {
	for name, yaml := range map[string]string{
		"an unknown capability": "classes:\n  guest: [play, fly]\n",
		"an unknown class":      "classes:\n  wizard: [play]\n",
		"the mode in the file":  "mode: closed\n",
		"an unknown key":        "colours: [red]\n",
		"an unknown limit":      "limits:\n  guest: { naps: 3 }\n",
	} {
		if _, err := auth.ParseCapsSeed([]byte(yaml)); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
	if _, err := auth.ParseCapsSeed(auth.DefaultCapsYAML()); err != nil {
		t.Errorf("the built-in file: %v", err)
	}
}

// What is an account's alone (and an administrator's) is refused to a guest.
func TestCaps_AGuestIsRefusedWhatIsAnAccounts(t *testing.T) {
	base := newRESTTestServer(t)
	key := newGuestKey(t, base)
	if code := doJSONAs(t, key, "POST", base+"/api/batches", map[string]any{"base": "tic-tac-toe", "games": 1}, nil); code != http.StatusForbidden {
		t.Errorf("a guest's batch: %d, want 403", code)
	}
	if code := doJSONAs(t, key, "GET", base+"/auth/admin/caps", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("a guest at the admin page: %d, want 401", code)
	}
	// And what an account may do is listed for it.
	var me map[string]any
	doJSONAs(t, key, "GET", base+"/auth/me", nil, &me)
	if caps, _ := me["caps"].([]any); len(caps) == 0 {
		t.Errorf("a guest's capabilities: %v", me)
	}
}

// A closed server serves its members, and the sign-in to everybody else.
func TestCaps_AClosedServer(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.AuthMode = "closed"
	a, base := serve(t, cfg)
	if code := doJSON(t, "GET", base+"/api/game-defs", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("an anonymous read: %d, want 401", code)
	}
	// A page sends a newcomer to sign in; the sign-in page itself is open.
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Get(base + "/game/ABC123")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login?return=%2Fgame%2FABC123" {
		t.Errorf("a game page: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	for _, path := range []string{"/login", "/signup", "/static/style.css", "/healthz"} {
		resp, err := noFollow.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("%s on a closed server: %d", path, resp.StatusCode)
		}
	}
	// A page's first request makes no guest on a closed server.
	b := newBrowser(t, base)
	if code := b.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ann"}, nil); code != http.StatusUnauthorized || b.cookie(auth.CookieName) != "" {
		t.Errorf("a newcomer's POST: %d, cookie %q", code, b.cookie(auth.CookieName))
	}
	// A member gets in.
	pw, err := a.Accounts().Create(context.Background(), "ana", "ana@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if code := b.login("ana", pw); code != 200 {
		t.Fatalf("login: %d", code)
	}
	b.do("POST", "/auth/password", map[string]any{"old": pw, "new": anaPassword}, nil)
	if code := b.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, nil); code != 200 {
		t.Errorf("a member makes a game: %d", code)
	}
	resp, err = b.c.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.Request.URL.Path != "/" {
		t.Errorf("a member's home page sent them to %s", resp.Request.URL)
	}
}

// A token acts as its account within its scopes, never with the admin
// group's powers, and stops when revoked.
func TestCaps_APIToken(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Addr = "127.0.0.1:0" // the seeded administrator needs no second factor here
	a, base := serve(t, cfg)
	b := newBrowser(t, base)
	b.do("POST", "/auth/login", map[string]any{"handle": "admin", "password": auth.DevPassword}, nil)
	b.do("POST", "/auth/password", map[string]any{"old": auth.DevPassword, "new": "the admin's own"}, nil)
	if code := b.do("GET", "/auth/admin/caps", nil, nil); code != 200 {
		t.Fatalf("the administrator at the admin page: %d", code)
	}
	var made struct{ Token string }
	if code := b.do("POST", "/auth/tokens", map[string]any{"name": "a script", "scopes": []string{"play", "create_game"}}, &made); code != http.StatusCreated || len(made.Token) < 20 {
		t.Fatalf("token: %d %+v", code, made)
	}
	tok := made.Token
	if code := createAs(t, tok, base); code != 200 {
		t.Errorf("the token makes a game: %d", code)
	}
	if code := doJSONAs(t, tok, "GET", base+"/auth/admin/caps", nil, nil); code != http.StatusForbidden {
		t.Errorf("an administrator's token at the admin page: %d, want 403", code)
	}
	if code := doJSONAs(t, tok, "POST", base+"/api/batches", map[string]any{"base": "tic-tac-toe", "games": 1}, nil); code != http.StatusForbidden {
		t.Errorf("a token without the scope runs a batch: %d, want 403", code)
	}
	if code := doJSONAs(t, tok, "POST", base+"/auth/tokens", map[string]any{"name": "more", "scopes": []string{"play"}}, nil); code != http.StatusUnauthorized {
		t.Errorf("a token makes a token: %d, want 401", code)
	}
	var list struct {
		Tokens []struct{ ID int64 } `json:"tokens"`
	}
	b.do("GET", "/auth/tokens", nil, &list)
	if len(list.Tokens) != 1 {
		t.Fatalf("tokens: %+v", list)
	}
	if code := b.do("DELETE", "/auth/tokens/"+strconv.FormatInt(list.Tokens[0].ID, 10), nil, nil); code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	if code := createAs(t, tok, base); code != http.StatusUnauthorized {
		t.Errorf("a revoked token: %d, want 401", code)
	}
	_ = a
}

// BINGLE_DEV's token drives a closed server, as CI does.
func TestCaps_TheDevTokenOpensAClosedServer(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Addr, cfg.Dev, cfg.AuthMode = "127.0.0.1:0", true, "closed"
	_, base := serve(t, cfg)
	if code := createAs(t, auth.DevAPIToken, base); code != 200 {
		t.Errorf("the dev token makes a game on a closed server: %d", code)
	}
}

// The admin page's matrix changes what a class may do at once, and says when
// the database has drifted from the file.
func TestCaps_TheAdminPageChangesTheMatrix(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Addr = "127.0.0.1:0"
	_, base := serve(t, cfg)
	b := newBrowser(t, base)
	b.do("POST", "/auth/login", map[string]any{"handle": "admin", "password": auth.DevPassword}, nil)
	b.do("POST", "/auth/password", map[string]any{"old": auth.DevPassword, "new": "the admin's own"}, nil)
	var m map[string]any
	b.do("GET", "/auth/admin/caps", nil, &m)
	if m["drift"] != false {
		t.Errorf("a fresh server has drifted: %v", m["drift"])
	}
	if code := b.do("PUT", "/auth/admin/caps", map[string]any{"class": "guest", "capability": "create_game", "allowed": false}, &m); code != 200 || m["drift"] != true {
		t.Fatalf("set: %d %v", code, m["drift"])
	}
	if code := createAs(t, newGuestKey(t, base), base); code != http.StatusForbidden {
		t.Errorf("a guest after the change: %d, want 403", code)
	}
	if code := b.do("PUT", "/auth/admin/caps", map[string]any{"class": "admin", "capability": "play", "allowed": false}, nil); code != http.StatusBadRequest {
		t.Errorf("the admin group's column: %d, want 400", code)
	}
}

// The built-in limits hold: three games a minute for a guest.
func TestCaps_TheLimitsHold(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.CapsFile = "" // the built-in defaults
	_, base := serve(t, cfg)
	key := newGuestKey(t, base)
	for i := 0; i < 3; i++ {
		if code := createAs(t, key, base); code != 200 {
			t.Fatalf("game %d: %d", i+1, code)
		}
	}
	if code := createAs(t, key, base); code != http.StatusTooManyRequests {
		t.Errorf("a fourth game in a minute: %d, want 429", code)
	}
}

// With BINGLE_SIGNUP=invite, signing up needs an administrator's link, once.
func TestCaps_Invitations(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{}, func(c *config.Config) { c.Signup = "invite"; c.Addr = "127.0.0.1:0" })
	b := newBrowser(t, e.URL)
	body := map[string]any{"handle": "ana", "email": "ana@example.com", "password": "correct horse battery"}
	if code := b.do("POST", "/auth/signup", body, nil); code != http.StatusForbidden {
		t.Errorf("signing up uninvited: %d, want 403", code)
	}
	admin := newBrowser(t, e.URL)
	admin.do("POST", "/auth/login", map[string]any{"handle": "admin", "password": auth.DevPassword}, nil)
	admin.do("POST", "/auth/password", map[string]any{"old": auth.DevPassword, "new": "the admin's own"}, nil)
	var inv struct{ Code, Path string }
	if code := admin.do("POST", "/auth/admin/invites", nil, &inv); code != http.StatusCreated || inv.Code == "" {
		t.Fatalf("invite: %d %+v", code, inv)
	}
	body["invite"] = inv.Code
	if code := b.do("POST", "/auth/signup", body, nil); code != http.StatusAccepted {
		t.Errorf("signing up invited: %d, want 202", code)
	}
	body["handle"], body["email"] = "bob", "bob@example.com"
	if code := b.do("POST", "/auth/signup", body, nil); code != http.StatusBadRequest {
		t.Errorf("the invitation a second time: %d, want 400", code)
	}
}
