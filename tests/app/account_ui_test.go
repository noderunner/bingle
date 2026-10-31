package app_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
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
