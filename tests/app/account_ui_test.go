package app_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/auth/fakeoidc"
)

// The pages: the profile offers a sign-in, the sign-in page signs in, and an
// account given its password changes it on the account page first.
func TestAccountUI_SignInAndChangeTheGivenPassword(t *testing.T) {
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
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(srv.URL+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the sign-in link", `document.getElementById("login-link") !== null && !document.getElementById("account-box").hidden`, 10*time.Second)
	clickSel(t, ctx, "#login-link")
	waitJS(t, ctx, "the sign-in page", `document.getElementById("login-form") !== null`, 10*time.Second)
	if err := chromedp.Run(ctx,
		chromedp.SendKeys("#login-handle", "ana", chromedp.ByID),
		chromedp.SendKeys("#login-password", pw, chromedp.ByID),
		chromedp.Click("#login-go", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	// A given password sends the account to change it.
	waitJS(t, ctx, "the account page", `location.pathname === "/account" && document.getElementById("account-who") && document.getElementById("account-who").textContent === "Signed in as @ana." && !document.getElementById("account-must-change").hidden`, 10*time.Second)
	if err := chromedp.Run(ctx,
		chromedp.SendKeys("#password-old", pw, chromedp.ByID),
		chromedp.SendKeys("#password-new", "a password of my own", chromedp.ByID),
		chromedp.Click("#password-go", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "changed", `document.getElementById("account-result").textContent.indexOf("Done") === 0 && document.getElementById("account-must-change").hidden`, 10*time.Second)
	if err := chromedp.Run(ctx, chromedp.Navigate(srv.URL+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "signed in on the profile", `document.getElementById("account-line").textContent.indexOf("Signed in as @ana.") === 0`, 10*time.Second)
	clickSel(t, ctx, "#account-link")
	// The button is there before the page's script is: wait for what the
	// script writes, so the click has a listener to reach.
	waitJS(t, ctx, "the account page, ready", `document.getElementById("logout-go") !== null && document.getElementById("account-who").textContent === "Signed in as @ana."`, 10*time.Second)
	clickSel(t, ctx, "#logout-go")
	waitJS(t, ctx, "signed out, at home", `location.pathname === "/"`, 10*time.Second)
}

// Two-step sign-in from the account page: set it up with the key the page
// shows, see the recovery codes, and sign in again with a code.
func TestAccountUI_TwoStepSignIn(t *testing.T) {
	_, base := accountServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/login")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the sign-in form", `document.getElementById("login-form") !== null`, 10*time.Second)
	if err := chromedp.Run(ctx,
		chromedp.SendKeys("#login-handle", "ana", chromedp.ByID),
		chromedp.SendKeys("#login-password", anaPassword, chromedp.ByID),
		chromedp.Click("#login-go", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "home", `location.pathname === "/"`, 10*time.Second)
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/account")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the account page, ready", `document.getElementById("tf-state").textContent.indexOf("Off") === 0`, 10*time.Second)
	if err := chromedp.Run(ctx, chromedp.SendKeys("#tf-password", anaPassword, chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#tf-setup")
	waitJS(t, ctx, "the key", `document.getElementById("tf-secret").textContent.length > 20 && !document.getElementById("tf-setting-up").hidden`, 10*time.Second)
	key := totpKey(t, evalString(t, ctx, `document.getElementById("tf-secret").textContent`))
	if err := chromedp.Run(ctx, chromedp.SendKeys("#tf-code", auth.TOTPCode(key, time.Now(), 6), chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#tf-enable")
	waitJS(t, ctx, "the recovery codes", `document.querySelectorAll("#tf-codes li").length === 10 && document.getElementById("tf-state").textContent.indexOf("On") === 0`, 10*time.Second)
	clickSel(t, ctx, "#logout-go")
	waitJS(t, ctx, "signed out", `location.pathname === "/"`, 10*time.Second)

	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/login")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the sign-in form", `document.getElementById("login-form") !== null`, 10*time.Second)
	if err := chromedp.Run(ctx,
		chromedp.SendKeys("#login-handle", "ana", chromedp.ByID),
		chromedp.SendKeys("#login-password", anaPassword, chromedp.ByID),
		chromedp.Click("#login-go", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the second step", `!document.getElementById("second-form").hidden && document.getElementById("login-form").hidden`, 10*time.Second)
	if err := chromedp.Run(ctx,
		chromedp.SendKeys("#second-code", auth.TOTPCode(key, time.Now().Add(30*time.Second), 6), chromedp.ByID),
		chromedp.Click("#second-go", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "signed in, home", `location.pathname === "/"`, 10*time.Second)
}

// The provider's button on the sign-in page, and the handle a newcomer
// chooses afterwards.
func TestAccountUI_SignInWithAProvider(t *testing.T) {
	_, base, idp := oidcServer(t)
	idp.SignInAs(fakeoidc.Identity{Subject: "s-ui", Email: "ui@example.com", EmailVerified: true})
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/login")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the provider's button", `document.getElementById("oidc-fake") && document.getElementById("oidc-fake").textContent === "Sign in with Fake"`, 10*time.Second)
	clickSel(t, ctx, "#oidc-fake")
	waitJS(t, ctx, "the handle form", `location.pathname === "/signup" && !document.getElementById("oidc-signup-form").hidden && document.getElementById("signup-form").hidden`, 10*time.Second)
	if err := chromedp.Run(ctx,
		chromedp.SendKeys("#oidc-handle", "uiuser", chromedp.ByID),
		chromedp.Click("#oidc-go", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "home", `location.pathname === "/"`, 10*time.Second)
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/account")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the account, with the provider linked and no password", `document.getElementById("account-who").textContent === "Signed in as @uiuser." && document.getElementById("password-title").textContent === "Set a password" && document.getElementById("oidc-unlink-fake") !== null`, 10*time.Second)
}

// The administration page: the matrix, and a box that changes it.
func TestAccountUI_TheAdminPage(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Addr = "127.0.0.1:0"
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	b := newBrowser(t, srv.URL)
	b.do("POST", "/auth/login", map[string]any{"handle": "admin", "password": auth.DevPassword}, nil)
	b.do("POST", "/auth/password", map[string]any{"old": auth.DevPassword, "new": "the admin's own"}, nil)

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, srv.URL, b.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.Navigate(srv.URL+"/admin")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the matrix", `document.getElementById("cap-guest-create_game") && document.getElementById("cap-guest-create_game").checked && !document.getElementById("cap-guest-run_batch").checked`, 10*time.Second)
	clickSel(t, ctx, "#cap-guest-create_game")
	waitJS(t, ctx, "saved", `document.getElementById("caps-result").textContent.indexOf("Saved") === 0 && !document.getElementById("caps-drift").hidden`, 10*time.Second)
	if code := createAs(t, newGuestKey(t, srv.URL), srv.URL); code != 403 {
		t.Errorf("a guest after the box was unticked: %d, want 403", code)
	}
	// A member who is not an administrator is turned away.
	pw, _ := a.Accounts().Create(context.Background(), "ana", "ana@example.com", false)
	nb := newBrowser(t, srv.URL)
	nb.login("ana", pw)
	ctx2, cancel2 := newChromedpContext(t, 60*time.Second)
	defer cancel2()
	signInChrome(t, ctx2, srv.URL, nb.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx2, chromedp.Navigate(srv.URL+"/admin")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx2, "refused", `!document.getElementById("admin-refused").hidden && document.getElementById("caps-card").hidden`, 10*time.Second)
}
