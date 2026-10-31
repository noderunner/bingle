package app_test

// Accounts, sessions and password login (ADR 0068).

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

// browser is an HTTP client that keeps cookies and sends what bingle's page
// sends, like a browser on bingle's site.
type browser struct {
	t    *testing.T
	base string
	c    *http.Client
}

func newBrowser(t *testing.T, base string) *browser {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &browser{t: t, base: base, c: &http.Client{Jar: jar}}
}

// do sends a request as the page and decodes the answer into out (if any).
func (b *browser) do(method, path string, body any, out any) int {
	b.t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, b.base+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.PageHeader, "1")
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, out)
	}
	return resp.StatusCode
}

func (b *browser) cookie(name string) string {
	u, _ := url.Parse(b.base)
	for _, c := range b.c.Jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func (b *browser) me() map[string]any {
	b.t.Helper()
	var out map[string]any
	if code := b.do("GET", "/auth/me", nil, &out); code != 200 {
		b.t.Fatalf("me: %d", code)
	}
	return out
}

var accountLinkRe = regexp.MustCompile(`/account/confirm\?t=([A-Za-z0-9_-]+)`)

// signUp makes and confirms an account from b, and returns the account's
// password.
func signUp(t *testing.T, e *notifyEnv, b *browser, handle, email string) string {
	t.Helper()
	const pw = "correct horse battery"
	before := len(e.SMTP.Mails())
	if code := b.do("POST", "/auth/signup", map[string]any{"handle": handle, "email": email, "password": pw}, nil); code != http.StatusAccepted {
		t.Fatalf("signup: %d", code)
	}
	mails := e.waitMails(t, before+1)
	m := accountLinkRe.FindStringSubmatch(mails[len(mails)-1].Data)
	if m == nil {
		t.Fatalf("no confirmation link in:\n%s", mails[len(mails)-1].Data)
	}
	var out map[string]any
	if code := b.do("POST", "/auth/confirm", map[string]any{"token": m[1]}, &out); code != 200 || out["handle"] != handle {
		t.Fatalf("confirm: %d %v", code, out)
	}
	return pw
}

func (b *browser) login(handle, pw string) int {
	b.t.Helper()
	return b.do("POST", "/auth/login", map[string]any{"handle": handle, "password": pw}, nil)
}

func TestAccount_SignUpConfirmAndSignIn(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	b := newBrowser(t, e.URL)
	if me := b.me(); me["kind"] != "anonymous" || me["signup"] != true {
		t.Fatalf("a newcomer: %v", me)
	}
	pw := signUp(t, e, b, "ana", "ana@example.com")
	// Confirming does not sign in: the link may have been opened anywhere.
	if me := b.me(); me["kind"] == "account" {
		t.Fatalf("confirming signed the browser in: %v", me)
	}
	if code := b.login("Ana", pw); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if me := b.me(); me["kind"] != "account" || me["handle"] != "ana" {
		t.Errorf("signed in: %v", me)
	}
	if code := b.do("POST", "/auth/logout", nil, nil); code != 200 {
		t.Fatalf("logout: %d", code)
	}
	if me := b.me(); me["kind"] == "account" {
		t.Errorf("still signed in after signing out: %v", me)
	}
}

// A guest who signs up keeps every seat: the account takes the guest's
// principal, and the guest key stops working.
func TestAccount_AGuestWhoSignsUpKeepsItsSeats(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	b := newBrowser(t, e.URL)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := b.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	guestKey := b.cookie(auth.CookieName)
	if guestKey == "" {
		t.Fatal("the browser is no guest")
	}
	e.join(t, created.RoomCode, "Bob")
	pw := signUp(t, e, b, "ana", "ana@example.com")
	if code := b.login("ana", pw); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if b.cookie(auth.CookieName) != "" {
		t.Error("the guest cookie is still there after signing in")
	}
	if code := moveAs(t, guestKey, e.URL, created.RoomCode, "Ana", "a1"); code != http.StatusUnauthorized {
		t.Errorf("the old guest key: %d, want 401", code)
	}
	if code := b.do("POST", "/api/games/"+created.RoomCode+"/moves", map[string]any{"player_name": "Ana", "notation": "a1"}, nil); code != 200 {
		t.Errorf("the account plays the guest's seat: %d, want 200", code)
	}
}

// Signing in from another browser brings that browser's guest along: its
// seats are the account's.
func TestAccount_SigningInBringsTheBrowsersGuestAlong(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	pw := signUp(t, e, newBrowser(t, e.URL), "ana", "ana@example.com")
	phone := newBrowser(t, e.URL)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	// A guest may not go by @ana's handle (stage 7), so the phone's guest is
	// "Ana Phone".
	if code := phone.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana Phone"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	e.join(t, created.RoomCode, "Bob")
	if code := phone.login("ana", pw); code != 200 {
		t.Fatalf("login: %d", code)
	}
	laptop := newBrowser(t, e.URL)
	if code := laptop.login("ana", pw); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if code := laptop.do("POST", "/api/games/"+created.RoomCode+"/moves", map[string]any{"player_name": "Ana Phone", "notation": "a1"}, nil); code != 200 {
		t.Errorf("the account, on another device, plays the phone guest's seat: %d, want 200", code)
	}
}

func TestAccount_SignUpNeedsMail(t *testing.T) {
	base := newRESTTestServer(t)
	b := newBrowser(t, base)
	if me := b.me(); me["signup"] != false {
		t.Errorf("signup offered without mail: %v", me)
	}
	var out map[string]string
	if code := b.do("POST", "/auth/signup", map[string]any{"handle": "ana", "email": "ana@example.com", "password": "correct horse battery"}, &out); code != http.StatusForbidden || !strings.Contains(out["error"], "email") {
		t.Errorf("signup without mail: %d %v", code, out)
	}
}

func TestAccount_SignUpRefusals(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	signUp(t, e, newBrowser(t, e.URL), "ana", "ana@example.com")
	b := newBrowser(t, e.URL)
	for name, tc := range map[string]struct {
		body map[string]any
		want int
	}{
		"a reserved handle": {map[string]any{"handle": "support", "email": "x@example.com", "password": "correct horse battery"}, 400},
		"a bad handle":      {map[string]any{"handle": "a b", "email": "x@example.com", "password": "correct horse battery"}, 400},
		"a short password":  {map[string]any{"handle": "bob", "email": "x@example.com", "password": "short"}, 400},
		"a bad address":     {map[string]any{"handle": "bob", "email": "not an address", "password": "correct horse battery"}, 400},
		"a taken handle":    {map[string]any{"handle": "ANA", "email": "x@example.com", "password": "correct horse battery"}, 409},
	} {
		if code := b.do("POST", "/auth/signup", tc.body, nil); code != tc.want {
			t.Errorf("%s: %d, want %d", name, code, tc.want)
		}
	}
	// An address that is an account's already gets the answer a new one
	// gets, so the answer does not say whose it is; the mail goes to the
	// address's owner instead, and says they have an account, with no link
	// that does anything.
	before := len(e.SMTP.Mails())
	if code := b.do("POST", "/auth/signup", map[string]any{"handle": "bob", "email": "Ana@Example.com", "password": "correct horse battery"}, nil); code != http.StatusAccepted {
		t.Errorf("an address in use: %d, want 202", code)
	}
	mails := e.waitMails(t, before+1)
	got := mails[len(mails)-1]
	if !strings.Contains(got.Data, "You already have a Bingle account") || !strings.Contains(got.Data, "@ana") || accountLinkRe.MatchString(got.Data) {
		t.Errorf("the mail to the owner:\n%s", got.Data)
	}
	// Again at once: no second mail, so the form cannot flood a mailbox.
	if code := b.do("POST", "/auth/signup", map[string]any{"handle": "cid", "email": "ana@example.com", "password": "correct horse battery"}, nil); code != http.StatusAccepted {
		t.Errorf("an address in use, again: %d, want 202", code)
	}
	e.quiet(t, before+1)
}

// Every way of getting a sign-in wrong is the same answer.
func TestAccount_WrongSignInsLookAlike(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	signUp(t, e, newBrowser(t, e.URL), "ana", "ana@example.com")
	read := func(handle, pw string) (int, string) {
		b := newBrowser(t, e.URL)
		var out map[string]string
		code := b.do("POST", "/auth/login", map[string]any{"handle": handle, "password": pw}, &out)
		return code, out["error"]
	}
	c1, m1 := read("nobody", "correct horse battery")
	c2, m2 := read("ana", "wrong horse battery")
	if c1 != http.StatusUnauthorized || c1 != c2 || m1 != m2 {
		t.Errorf("no such handle: %d %q; wrong password: %d %q", c1, m1, c2, m2)
	}
}

// A changed password ends every other session, and the one that changed it
// gets a new cookie.
func TestAccount_ChangingThePasswordRotatesTheSessions(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	pw := signUp(t, e, newBrowser(t, e.URL), "ana", "ana@example.com")
	laptop, phone := newBrowser(t, e.URL), newBrowser(t, e.URL)
	laptop.login("ana", pw)
	phone.login("ana", pw)
	before := laptop.cookie(auth.SessionCookie)
	if code := laptop.do("POST", "/auth/password", map[string]any{"old": pw, "new": "a brand new password"}, nil); code != 200 {
		t.Fatalf("change: %d", code)
	}
	after := laptop.cookie(auth.SessionCookie)
	if after == "" || after == before {
		t.Errorf("the session cookie was not replaced: %q then %q", before, after)
	}
	if me := laptop.me(); me["kind"] != "account" {
		t.Errorf("the laptop is signed out: %v", me)
	}
	if me := phone.me(); me["kind"] == "account" {
		t.Errorf("the phone is still signed in: %v", me)
	}
	stale := newBrowser(t, e.URL)
	u, _ := url.Parse(e.URL)
	stale.c.Jar.SetCookies(u, []*http.Cookie{{Name: auth.SessionCookie, Value: before}})
	if me := stale.me(); me["kind"] == "account" {
		t.Errorf("the old session cookie still works: %v", me)
	}
	if code := newBrowser(t, e.URL).login("ana", pw); code != http.StatusUnauthorized {
		t.Errorf("the old password: %d, want 401", code)
	}
}

// A session cookie, like a guest cookie, only works from bingle's page.
func TestAccount_ASessionCookieNeedsThePageHeader(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	b := newBrowser(t, e.URL)
	b.login("ana", signUp(t, e, b, "ana", "ana@example.com"))
	body, _ := json.Marshal(map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"})
	req, _ := http.NewRequest("POST", e.URL+"/api/games", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a session POST without the header: %d, want 403", resp.StatusCode)
	}
}

// The first administrator of an empty server: admin/bingle123 on loopback,
// and the password must be changed before anything else.
func TestAccount_TheSeededAdministratorChangesItsPasswordFirst(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Addr = "127.0.0.1:0"
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if s := a.Seeded(); s == nil || s.Handle != "admin" || s.Password != auth.DevPassword || !s.Deterministic {
		t.Fatalf("seeded: %+v", s)
	}
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	b := newBrowser(t, srv.URL)
	var me map[string]any
	if code := b.do("POST", "/auth/login", map[string]any{"handle": "admin", "password": auth.DevPassword}, &me); code != 200 || me["must_change_password"] != true {
		t.Fatalf("login: %d %v", code, me)
	}
	if code := b.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Admin"}, nil); code != http.StatusForbidden {
		t.Errorf("playing before changing the password: %d, want 403", code)
	}
	var changed map[string]any
	if code := b.do("POST", "/auth/password", map[string]any{"old": auth.DevPassword, "new": "a proper password"}, &changed); code != 200 || changed["must_change_password"] == true {
		t.Fatalf("change: %d %v", code, changed)
	}
	if code := b.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Admin"}, nil); code != 200 {
		t.Errorf("playing after: %d, want 200", code)
	}
	// A second start finds the account and seeds nothing.
	_ = a.Close()
	again, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = again.Close() }()
	if again.Seeded() != nil {
		t.Error("an administrator was seeded on a database that has one")
	}
}

// Off loopback the seeded password is random, and never the well-known one.
func TestAccount_TheSeededPasswordIsRandomOffLoopback(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Addr = ":8080"
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	if s := a.Seeded(); s == nil || s.Deterministic || s.Password == auth.DevPassword || len(s.Password) != 16 {
		t.Errorf("seeded: %+v", s)
	}
	cfg = newTestConfig(t)
	cfg.Addr, cfg.TrustProxy = "127.0.0.1:0", true
	b, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	if s := b.Seeded(); s == nil || s.Deterministic {
		t.Errorf("behind a trusted proxy a loopback bind is not local: %+v", s)
	}
}

// The session cookie is Secure on a secure connection and only omits it on
// loopback HTTP, where a browser would not send it back otherwise.
func TestAccount_TheCookieIsSecureOffLoopback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tls    bool
		secure bool
	}{{"loopback HTTP", false, false}, {"TLS", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfig(t)
			a, err := app.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = a.Close() })
			pw, err := a.Accounts().Create(context.Background(), "ana", "ana@example.com", false)
			if err != nil {
				t.Fatal(err)
			}
			var srv *httptest.Server
			if tc.tls {
				srv = httptest.NewTLSServer(a.Router())
			} else {
				srv = httptest.NewServer(a.Router())
			}
			t.Cleanup(srv.Close)
			body, _ := json.Marshal(map[string]any{"handle": "ana", "password": pw})
			req, _ := http.NewRequest("POST", srv.URL+"/auth/login", bytes.NewReader(body))
			req.Header.Set(auth.PageHeader, "1")
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			found := false
			for _, c := range resp.Cookies() {
				if c.Name == auth.SessionCookie {
					found = true
					if c.Secure != tc.secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
						t.Errorf("cookie %+v, want Secure=%v, HttpOnly, SameSite=Lax", c, tc.secure)
					}
				}
			}
			if !found {
				t.Fatalf("no session cookie (%d)", resp.StatusCode)
			}
		})
	}
}

// A confirmation link works once.
func TestAccount_AConfirmationLinkWorksOnce(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	b := newBrowser(t, e.URL)
	if code := b.do("POST", "/auth/signup", map[string]any{"handle": "ana", "email": "ana@example.com", "password": "correct horse battery"}, nil); code != http.StatusAccepted {
		t.Fatalf("signup: %d", code)
	}
	mails := e.waitMails(t, 1)
	token := accountLinkRe.FindStringSubmatch(mails[0].Data)[1]
	// Before it is confirmed, the account cannot sign in.
	if code := b.login("ana", "correct horse battery"); code != http.StatusForbidden {
		t.Errorf("signing in unconfirmed: %d, want 403", code)
	}
	if code := b.do("POST", "/auth/confirm", map[string]any{"token": token}, nil); code != 200 {
		t.Fatalf("confirm: %d", code)
	}
	if code := b.do("POST", "/auth/confirm", map[string]any{"token": token}, nil); code != http.StatusBadRequest {
		t.Errorf("the link a second time: %d, want 400", code)
	}
	if code := b.do("POST", "/auth/confirm", map[string]any{"token": "made-up"}, nil); code != http.StatusBadRequest {
		t.Errorf("a made-up link: %d, want 400", code)
	}
}

// The command line's reset: a new password, to be changed, and every session
// signed out.
func TestAccount_AResetSignsOutEverywhere(t *testing.T) {
	cfg := newTestConfig(t)
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	pw, err := a.Accounts().Create(context.Background(), "ana", "ana@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	b := newBrowser(t, srv.URL)
	if code := b.login("ana", pw); code != 200 {
		t.Fatalf("login: %d", code)
	}
	fresh, err := a.Accounts().ResetPassword(context.Background(), "ana")
	if err != nil || fresh == pw {
		t.Fatalf("reset: %q %v", fresh, err)
	}
	if me := b.me(); me["kind"] == "account" {
		t.Errorf("still signed in after a reset: %v", me)
	}
	if code := newBrowser(t, srv.URL).login("ana", pw); code != http.StatusUnauthorized {
		t.Errorf("the old password: %d", code)
	}
	var me map[string]any
	if code := newBrowser(t, srv.URL).do("POST", "/auth/login", map[string]any{"handle": "ana", "password": fresh}, &me); code != 200 || me["must_change_password"] != true {
		t.Errorf("the new password: %d %v", code, me)
	}
}
