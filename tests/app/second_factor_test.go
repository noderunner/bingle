package app_test

// The second factor (ADR 0069): TOTP, recovery codes, the backoff, the
// administrator's second factor and resetting a forgotten password.

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
	"github.com/noderunner/bingle/internal/transport"
)

const anaPassword = "ana's own password"

// accountServer is a server with the account @ana, whose password is
// anaPassword (changed from the given one already).
func accountServer(t *testing.T, opts ...app.Option) (*app.App, string) {
	t.Helper()
	a, err := app.New(newTestConfig(t), opts...)
	if err != nil {
		t.Fatal(err)
	}
	registerFixtures(t, a)
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	given, err := a.Accounts().Create(context.Background(), "ana", "ana@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	b := newBrowser(t, srv.URL)
	if code := b.login("ana", given); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if code := b.do("POST", "/auth/password", map[string]any{"old": given, "new": anaPassword}, nil); code != 200 {
		t.Fatalf("password: %d", code)
	}
	return a, srv.URL
}

func totpKey(t *testing.T, secret string) []byte {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// enableTOTP turns ana's second factor on from b, and returns the key and
// the recovery codes.
func enableTOTP(t *testing.T, b *browser) ([]byte, []string) {
	t.Helper()
	var setup struct {
		Secret, URI string
		QR          string `json:"qr_svg"`
	}
	if code := b.do("POST", "/auth/totp/setup", map[string]any{"password": anaPassword}, &setup); code != 200 || setup.Secret == "" {
		t.Fatalf("setup: %d %+v", code, setup)
	}
	// The link comes as a QR code too, for a phone's camera.
	if !strings.HasPrefix(setup.QR, "<svg ") || !strings.HasSuffix(setup.QR, "</svg>") {
		t.Errorf("no QR code with the setup: %.80q", setup.QR)
	}
	key := totpKey(t, setup.Secret)
	var on struct {
		Codes []string       `json:"recovery_codes"`
		Me    map[string]any `json:"me"`
	}
	if code := b.do("POST", "/auth/totp/enable", map[string]any{"code": auth.TOTPCode(key, time.Now(), 6)}, &on); code != 200 || len(on.Codes) != 10 || on.Me["second_factor"] != true {
		t.Fatalf("enable: %d %+v", code, on)
	}
	return key, on.Codes
}

// signInWith signs a fresh browser in to ana with the password and then
// typed (a code or a recovery code), and returns the browser and the status
// of the second step.
func signInWith(t *testing.T, base, typed string) (*browser, int) {
	t.Helper()
	b := newBrowser(t, base)
	var first map[string]any
	if code := b.do("POST", "/auth/login", map[string]any{"handle": "ana", "password": anaPassword}, &first); code != 200 || first["second_factor"] != true {
		t.Fatalf("the password step: %d %v", code, first)
	}
	if me := b.me(); me["kind"] == "account" {
		t.Fatalf("signed in on the password alone: %v", me)
	}
	body := map[string]any{"code": typed}
	if strings.Contains(typed, "-") {
		body = map[string]any{"recovery_code": typed}
	}
	return b, b.do("POST", "/auth/second", body, nil)
}

func TestSecondFactor_ACodeSignsInOnce(t *testing.T) {
	_, base := accountServer(t)
	b := newBrowser(t, base)
	b.login("ana", anaPassword)
	key, _ := enableTOTP(t, b)
	// Turning it on signs the other browsers out, and keeps this one in.
	if me := b.me(); me["kind"] != "account" || me["second_factor"] != true || me["recovery_codes_left"] != float64(10) {
		t.Errorf("after turning it on: %v", me)
	}
	// The code that turned it on is spent: the next step's code signs in.
	next := auth.TOTPCode(key, time.Now().Add(30*time.Second), 6)
	b2, code := signInWith(t, base, next)
	if code != 200 || b2.me()["kind"] != "account" {
		t.Fatalf("the second step: %d", code)
	}
	if _, code := signInWith(t, base, next); code != http.StatusUnauthorized {
		t.Errorf("the same code again: %d, want 401", code)
	}
	if _, code := signInWith(t, base, "000000"); code != http.StatusUnauthorized {
		t.Errorf("a wrong code: %d, want 401", code)
	}
}

func TestSecondFactor_ARecoveryCodeWorksOnce(t *testing.T) {
	_, base := accountServer(t)
	b := newBrowser(t, base)
	b.login("ana", anaPassword)
	_, codes := enableTOTP(t, b)
	if _, code := signInWith(t, base, strings.ToUpper(codes[3])); code != 200 {
		t.Fatalf("a recovery code: %d", code)
	}
	if _, code := signInWith(t, base, codes[3]); code != http.StatusUnauthorized {
		t.Errorf("the recovery code again: %d, want 401", code)
	}
	if me := b.me(); me["recovery_codes_left"] != float64(9) {
		t.Errorf("left after one: %v", me["recovery_codes_left"])
	}
}

// The second step belongs to the browser that gave the password.
func TestSecondFactor_TheCodeAloneIsNotASignIn(t *testing.T) {
	_, base := accountServer(t)
	b := newBrowser(t, base)
	b.login("ana", anaPassword)
	key, _ := enableTOTP(t, b)
	other := newBrowser(t, base)
	if code := other.do("POST", "/auth/second", map[string]any{"code": auth.TOTPCode(key, time.Now().Add(30*time.Second), 6)}, nil); code != http.StatusUnauthorized {
		t.Errorf("a code with no password before it: %d, want 401", code)
	}
}

func TestSecondFactor_TurningItOffNeedsThePasswordAndACode(t *testing.T) {
	_, base := accountServer(t)
	b := newBrowser(t, base)
	b.login("ana", anaPassword)
	key, _ := enableTOTP(t, b)
	next := auth.TOTPCode(key, time.Now().Add(30*time.Second), 6)
	if code := b.do("POST", "/auth/totp/disable", map[string]any{"password": "wrong password!", "code": next}, nil); code != http.StatusUnauthorized {
		t.Errorf("off with a wrong password: %d, want 401", code)
	}
	if code := b.do("POST", "/auth/totp/disable", map[string]any{"password": anaPassword, "code": "000000"}, nil); code != http.StatusUnauthorized {
		t.Errorf("off with a wrong code: %d, want 401", code)
	}
	var me map[string]any
	if code := b.do("POST", "/auth/totp/disable", map[string]any{"password": anaPassword, "code": next}, &me); code != 200 || me["second_factor"] == true {
		t.Fatalf("off: %d %v", code, me)
	}
	if code := newBrowser(t, base).login("ana", anaPassword); code != 200 {
		t.Errorf("the password alone again: %d", code)
	}
}

// Five wrong passwords lock the handle for a while, the right one included,
// and an unknown handle locks the same way.
func TestSecondFactor_RepeatedFailuresBackOff(t *testing.T) {
	for _, handle := range []string{"ana", "nobody"} {
		// A server each: one address may try ten times a minute in all.
		_, base := accountServer(t)
		for i := 0; i < 5; i++ {
			if code := newBrowser(t, base).login(handle, "a wrong password"); code != http.StatusUnauthorized {
				t.Fatalf("%s, wrong password %d: %d", handle, i+1, code)
			}
		}
		if code := newBrowser(t, base).login(handle, anaPassword); code != http.StatusTooManyRequests {
			t.Errorf("%s after five failures: %d, want 429", handle, code)
		}
	}
}

// proxied is a client behind a trusted proxy: it says the connection was
// https and keeps the (Secure) cookies itself.
type proxied struct {
	t       *testing.T
	base    string
	cookies map[string]string
	last    string // the body of the latest answer
}

func (p *proxied) do(method, path string, body any) int {
	p.t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, p.base+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.PageHeader, "1")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	for k, v := range p.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ = io.ReadAll(resp.Body)
	p.last = string(raw)
	for _, c := range resp.Cookies() {
		if c.MaxAge < 0 {
			delete(p.cookies, c.Name)
		} else {
			p.cookies[c.Name] = c.Value
		}
	}
	return resp.StatusCode
}

// An administrator off this machine must set up a second factor before
// anything else — and behind a trusted proxy nothing is "this machine".
func TestSecondFactor_AnAdministratorNeedsOneOffLoopback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		proxy   bool
		blocked bool
	}{{"on loopback", false, false}, {"behind a trusted proxy", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			var opts []app.Option
			if tc.proxy {
				opts = append(opts, app.WithTransport(transport.Policy{TrustProxy: true}))
			}
			a, err := app.New(newTestConfig(t), opts...)
			if err != nil {
				t.Fatal(err)
			}
			registerFixtures(t, a)
			t.Cleanup(func() { _ = a.Close() })
			srv := httptest.NewServer(a.Router())
			t.Cleanup(srv.Close)
			given, err := a.Accounts().Create(context.Background(), "ops", "ops@example.com", true)
			if err != nil {
				t.Fatal(err)
			}
			p := &proxied{t: t, base: srv.URL, cookies: map[string]string{}}
			if code := p.do("POST", "/auth/login", map[string]any{"handle": "ops", "password": given}); code != 200 {
				t.Fatalf("login: %d", code)
			}
			if code := p.do("POST", "/auth/password", map[string]any{"old": given, "new": "the operator's own"}); code != 200 {
				t.Fatalf("password: %d", code)
			}
			code := p.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ops"})
			if tc.blocked && code != http.StatusForbidden {
				t.Errorf("an administrator without a second factor plays: %d, want 403", code)
			}
			// The admin page says what is missing, not that they are no administrator.
			admin := p.do("GET", "/auth/admin/caps", nil)
			if tc.blocked && (admin != http.StatusForbidden || !strings.Contains(p.last, "need a second factor") || !strings.Contains(p.last, "account page")) {
				t.Errorf("the admin page off loopback: %d %s", admin, p.last)
			}
			if !tc.blocked && admin != 200 {
				t.Errorf("the admin page on loopback: %d %s", admin, p.last)
			}
			if !tc.blocked && code != 200 {
				t.Errorf("an administrator on loopback plays: %d, want 200", code)
			}
		})
	}
}

var resetLinkRe = regexp.MustCompile(`/account/reset\?t=([A-Za-z0-9_-]+)`)

// A forgotten password: the link resets it, once, and an account with a
// second factor gives its code as well.
func TestSecondFactor_AForgottenPasswordIsResetByMail(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	b := newBrowser(t, e.URL)
	old := signUp(t, e, b, "ana", "ana@example.com")
	b.login("ana", old)
	before := len(e.SMTP.Mails())
	var out map[string]string
	if code := newBrowser(t, e.URL).do("POST", "/auth/forgot", map[string]any{"who": "nobody"}, &out); code != http.StatusAccepted {
		t.Errorf("forgot for nobody: %d, want 202", code)
	}
	if code := newBrowser(t, e.URL).do("POST", "/auth/forgot", map[string]any{"who": "ANA@example.com"}, nil); code != http.StatusAccepted {
		t.Fatalf("forgot: %d", code)
	}
	mails := e.waitMails(t, before+1)
	if len(mails) != before+1 {
		t.Fatalf("%d mails for one request (and one for nobody)", len(mails)-before)
	}
	token := resetLinkRe.FindStringSubmatch(mails[len(mails)-1].Data)[1]
	r := newBrowser(t, e.URL)
	if code := r.do("POST", "/auth/reset", map[string]any{"token": token, "password": "short"}, nil); code != http.StatusBadRequest {
		t.Errorf("a short password: %d, want 400", code)
	}
	if code := r.do("POST", "/auth/reset", map[string]any{"token": token, "password": "a remembered one"}, nil); code != 200 {
		t.Fatalf("reset: %d", code)
	}
	if code := r.do("POST", "/auth/reset", map[string]any{"token": token, "password": "yet another one"}, nil); code != http.StatusBadRequest {
		t.Errorf("the link again: %d, want 400", code)
	}
	if me := b.me(); me["kind"] == "account" {
		t.Error("the old session survived the reset")
	}
	if code := newBrowser(t, e.URL).login("ana", "a remembered one"); code != 200 {
		t.Errorf("the new password: %d", code)
	}
}

// With a second factor, a mailbox alone does not reset the password.
func TestSecondFactor_AResetAsksForTheCodeToo(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	b := newBrowser(t, e.URL)
	pw := signUp(t, e, b, "ana", "ana@example.com")
	b.login("ana", pw)
	var setup struct{ Secret string }
	b.do("POST", "/auth/totp/setup", map[string]any{"password": pw}, &setup)
	key := totpKey(t, setup.Secret)
	if code := b.do("POST", "/auth/totp/enable", map[string]any{"code": auth.TOTPCode(key, time.Now(), 6)}, nil); code != 200 {
		t.Fatalf("enable: %d", code)
	}
	before := len(e.SMTP.Mails())
	newBrowser(t, e.URL).do("POST", "/auth/forgot", map[string]any{"who": "ana"}, nil)
	mails := e.waitMails(t, before+1)
	token := resetLinkRe.FindStringSubmatch(mails[len(mails)-1].Data)[1]
	r := newBrowser(t, e.URL)
	if code := r.do("POST", "/auth/reset", map[string]any{"token": token, "password": "a remembered one"}, nil); code != http.StatusUnauthorized {
		t.Errorf("without the code: %d, want 401", code)
	}
	// The link survives a missing code, and works with the right one.
	if code := r.do("POST", "/auth/reset", map[string]any{"token": token, "password": "a remembered one", "code": auth.TOTPCode(key, time.Now().Add(30*time.Second), 6)}, nil); code != 200 {
		t.Errorf("with the code: %d, want 200", code)
	}
}

// The command line's reset takes the second factor away with the password.
func TestSecondFactor_TheCommandLineResetRemovesIt(t *testing.T) {
	a, base := accountServer(t)
	b := newBrowser(t, base)
	b.login("ana", anaPassword)
	enableTOTP(t, b)
	pw, err := a.Accounts().ResetPassword(context.Background(), "ana")
	if err != nil {
		t.Fatal(err)
	}
	var me map[string]any
	if code := newBrowser(t, base).do("POST", "/auth/login", map[string]any{"handle": "ana", "password": pw}, &me); code != 200 || me["second_factor"] == true || me["kind"] != "account" {
		t.Errorf("after a reset: %d %v", code, me)
	}
}
