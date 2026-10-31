package app_test

// Signing in with an OpenID Connect provider (ADR 0070), against the
// in-process fake provider.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/auth/fakeoidc"
	"github.com/noderunner/bingle/internal/config"
)

// oidcServer is a server with the fake provider "fake" configured.
func oidcServer(t *testing.T, mods ...func(*config.Config)) (*app.App, string, *fakeoidc.Provider) {
	t.Helper()
	idp := fakeoidc.Start(t)
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	t.Cleanup(srv.Close)
	cfg := newTestConfig(t)
	cfg.PublicURL = srv.URL
	cfg.OIDC = []config.OIDCProvider{{ID: "fake", Issuer: idp.Issuer(), ClientID: idp.ClientID, ClientSecret: config.NewSecret(idp.ClientSecret),
		Scopes: []string{"openid", "email", "profile"}, Label: "Fake"}}
	for _, m := range mods {
		m(&cfg)
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv.Config.Handler = a.Router()
	return a, srv.URL, idp
}

// viaProvider follows a provider sign-in from start to wherever it lands,
// and returns the landing URL.
func (b *browser) viaProvider(query string) *url.URL {
	b.t.Helper()
	resp, err := b.c.Get(b.base + "/auth/oidc/fake/start" + query)
	if err != nil {
		b.t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.Request.URL
}

// toCallback follows a provider sign-in up to, not into, the callback, and
// returns the callback's URL.
func (b *browser) toCallback() string {
	b.t.Helper()
	c := *b.c
	c.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if strings.Contains(req.URL.Path, "/callback") {
			return http.ErrUseLastResponse
		}
		return nil
	}
	resp, err := c.Get(b.base + "/auth/oidc/fake/start")
	if err != nil {
		b.t.Fatal(err)
	}
	_ = resp.Body.Close()
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/auth/oidc/fake/callback") {
		b.t.Fatalf("not at the callback: %d %q", resp.StatusCode, loc)
	}
	return loc
}

func (b *browser) get(u string) *url.URL {
	b.t.Helper()
	resp, err := b.c.Get(u)
	if err != nil {
		b.t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.Request.URL
}

// A provider vouches for somebody new: they choose a handle and have an
// account, and the next time the provider signs them straight in.
func TestOIDC_ASignInMakesAnAccountAndSignsInAgain(t *testing.T) {
	_, base, idp := oidcServer(t)
	idp.SignInAs(fakeoidc.Identity{Subject: "s-1", Email: "nia@example.com", EmailVerified: true})
	b := newBrowser(t, base)
	if me := b.me(); len(me["providers"].([]any)) != 1 || me["signup"] != true {
		t.Fatalf("a newcomer sees %v", me)
	}
	land := b.viaProvider("?return=/profile")
	if land.Path != "/signup" || land.Query().Get("provider") != "fake" {
		t.Fatalf("landed on %s", land)
	}
	var out struct {
		Me     map[string]any `json:"me"`
		Return string         `json:"return"`
	}
	if code := b.do("POST", "/auth/oidc/finish", map[string]any{"handle": "nia"}, &out); code != 200 || out.Me["handle"] != "nia" || out.Return != "/profile" {
		t.Fatalf("finish: %d %+v", code, out)
	}
	if me := b.me(); me["kind"] != "account" || me["password"] == true {
		t.Errorf("signed in: %v", me)
	}
	again := newBrowser(t, base)
	if land := again.viaProvider(""); land.Path != "/" {
		t.Errorf("the second sign-in landed on %s", land)
	}
	if me := again.me(); me["handle"] != "nia" {
		t.Errorf("the second sign-in is %v", me)
	}
	if idp.Exchanged != 2 {
		t.Errorf("%d codes exchanged, want 2", idp.Exchanged)
	}
}

// An address the provider does not vouch for never links to the account
// that has it; one it does vouch for does.
func TestOIDC_TheLinkingRule(t *testing.T) {
	a, base, idp := oidcServer(t)
	if _, err := a.Accounts().Create(context.Background(), "ana", "ana@example.com", false); err != nil {
		t.Fatal(err)
	}
	idp.SignInAs(fakeoidc.Identity{Subject: "s-ana", Email: "ana@example.com", EmailVerified: false})
	b := newBrowser(t, base)
	land := b.viaProvider("")
	if land.Path != "/login" || !strings.Contains(land.Query().Get("error"), "sign in with its password") {
		t.Fatalf("an unverified address landed on %s", land)
	}
	if me := b.me(); me["kind"] == "account" {
		t.Fatalf("an unverified address signed in: %v", me)
	}
	idp.SignInAs(fakeoidc.Identity{Subject: "s-ana", Email: "ANA@example.com", EmailVerified: true})
	if land := b.viaProvider(""); land.Path != "/" {
		t.Fatalf("a verified address landed on %s", land)
	}
	if me := b.me(); me["handle"] != "ana" || len(me["linked"].([]any)) != 1 {
		t.Errorf("signed in as %v", me)
	}
}

// The state must be the one the browser was given, and a callback works
// once: a tampered state, or a replayed one, signs nobody in.
func TestOIDC_ATamperedOrReplayedCallbackIsRefused(t *testing.T) {
	_, base, idp := oidcServer(t)
	idp.SignInAs(fakeoidc.Identity{Subject: "s-2", Email: "x@example.com", EmailVerified: true})

	b := newBrowser(t, base)
	cb, _ := url.Parse(b.toCallback())
	q := cb.Query()
	q.Set("state", "not-the-state")
	cb.RawQuery = q.Encode()
	if land := b.get(cb.String()); land.Path != "/login" || land.Query().Get("error") == "" {
		t.Errorf("a tampered state landed on %s", land)
	}

	// Another browser cannot finish this browser's sign-in either.
	victim, attacker := newBrowser(t, base), newBrowser(t, base)
	theirs := victim.toCallback()
	if land := attacker.get(theirs); land.Path != "/login" {
		t.Errorf("a callback from another browser landed on %s", land)
	}

	c := newBrowser(t, base)
	good := c.toCallback()
	if land := c.get(good); land.Path != "/signup" {
		t.Fatalf("the real callback landed on %s", land)
	}
	if land := c.get(good); land.Path != "/login" || land.Query().Get("error") == "" {
		t.Errorf("the callback replayed landed on %s", land)
	}
}

// An account with a second factor gives its code after the provider too.
func TestOIDC_TheSecondFactorStillAsks(t *testing.T) {
	_, base, idp := oidcServer(t)
	idp.SignInAs(fakeoidc.Identity{Subject: "s-3", Email: "kai@example.com", EmailVerified: true})
	b := newBrowser(t, base)
	b.viaProvider("")
	b.do("POST", "/auth/oidc/finish", map[string]any{"handle": "kai"}, nil)
	if code := b.do("POST", "/auth/password", map[string]any{"old": "", "new": "kai's first password"}, nil); code != 200 {
		t.Fatalf("setting a first password: %d", code)
	}
	var setup struct{ Secret string }
	b.do("POST", "/auth/totp/setup", map[string]any{"password": "kai's first password"}, &setup)
	key := totpKey(t, setup.Secret)
	if code := b.do("POST", "/auth/totp/enable", map[string]any{"code": auth.TOTPCode(key, time.Now(), 6)}, nil); code != 200 {
		t.Fatalf("enable: %d", code)
	}
	again := newBrowser(t, base)
	if land := again.viaProvider(""); land.Path != "/login" || land.Query().Get("second") != "1" {
		t.Fatalf("landed on %s", land)
	}
	if me := again.me(); me["kind"] == "account" {
		t.Fatal("signed in without the code")
	}
	if code := again.do("POST", "/auth/second", map[string]any{"code": auth.TOTPCode(key, time.Now().Add(30*time.Second), 6)}, nil); code != 200 {
		t.Errorf("the code: %d", code)
	}
}

// Linking from the account page, and unlinking — never the last way in.
func TestOIDC_LinkingAndUnlinking(t *testing.T) {
	_, base, idp := oidcServer(t)
	b := newBrowser(t, base)
	idp.SignInAs(fakeoidc.Identity{Subject: "s-4", Email: "lea@example.com", EmailVerified: true})
	b.viaProvider("")
	b.do("POST", "/auth/oidc/finish", map[string]any{"handle": "lea"}, nil)
	if code := b.do("POST", "/auth/oidc/unlink", map[string]any{"provider": "fake", "password": ""}, nil); code != http.StatusBadRequest {
		t.Errorf("unlinking the only way in: %d, want 400", code)
	}
	b.do("POST", "/auth/password", map[string]any{"old": "", "new": "lea's own password"}, nil)
	var me map[string]any
	if code := b.do("POST", "/auth/oidc/unlink", map[string]any{"provider": "fake", "password": "lea's own password"}, &me); code != 200 || me["linked"] != nil {
		t.Fatalf("unlink: %d %v", code, me)
	}
	// Linked again from the account page, while signed in.
	if land := b.viaProvider("?link=1&return=/account"); land.Path != "/account" || land.Query().Get("linked") != "fake" {
		t.Fatalf("linking landed on %s", land)
	}
	if me := b.me(); len(me["linked"].([]any)) != 1 {
		t.Errorf("after linking: %v", me)
	}
	// Somebody else's identity cannot be linked to this account.
	other := newBrowser(t, base)
	idp.SignInAs(fakeoidc.Identity{Subject: "s-5", Email: "zed@example.com", EmailVerified: true})
	other.viaProvider("")
	other.do("POST", "/auth/oidc/finish", map[string]any{"handle": "zed"}, nil)
	if land := b.viaProvider("?link=1"); land.Path != "/account" || land.Query().Get("error") == "" {
		t.Errorf("linking zed's identity to lea landed on %s", land)
	}
}

// With signing up closed a provider signs in only those who have accounts.
func TestOIDC_NoNewAccountsWhenSignupIsClosed(t *testing.T) {
	_, base, idp := oidcServer(t, func(c *config.Config) { c.Signup = "closed" })
	idp.SignInAs(fakeoidc.Identity{Subject: "s-6", Email: "new@example.com", EmailVerified: true})
	b := newBrowser(t, base)
	if land := b.viaProvider(""); land.Path != "/login" || !strings.Contains(land.Query().Get("error"), "closed") {
		t.Errorf("landed on %s", land)
	}
}
