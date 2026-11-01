package app_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
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
	registerFixtures(t, a)
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
	// The QR code is there and the browser could draw it: a phone's camera
	// is how most people add it.
	waitJS(t, ctx, "the QR code", `(() => { const q = document.getElementById("tf-qr"); return !q.hidden && q.complete && q.naturalWidth > 0 && getComputedStyle(q).display !== "none"; })()`, 10*time.Second)
	shot(t, ctx, "totp-setup", func(b *[]byte) error { return chromedp.Run(ctx, chromedp.FullScreenshot(b, 90)) })
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
	registerFixtures(t, a)
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
	waitJS(t, ctx, "the events", `!document.getElementById("events-card").hidden`, 10*time.Second)
	shot(t, ctx, "admin", func(b *[]byte) error { return chromedp.Run(ctx, chromedp.FullScreenshot(b, 90)) })
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

// The guest key carried to another browser by its link: the fragment is
// taken out of the address at once, and the game is there to resume.
func TestAccountUI_AGuestKeyLinkResumesTheGames(t *testing.T) {
	base := newRESTTestServer(t)
	laptop, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(laptop, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	// The laptop takes a seat in a game (as the page does: a POST from the page).
	var room string
	if err := chromedp.Run(laptop, chromedp.Evaluate(`fetch("/api/games", {method: "POST", headers: {"Content-Type": "application/json", "X-Bingle-Session": "1"}, body: JSON.stringify({game_def_id: "tic-tac-toe", player_name: "Lia"})}).then(r => r.json()).then(j => j.room_code)`, &room, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatal(err)
	}
	doJSONAs(t, newGuestKey(t, base), "POST", base+"/api/games/"+room+"/join", map[string]any{"player_name": "Bob"}, nil)
	if err := chromedp.Run(laptop, chromedp.Navigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, laptop, "the guest card", `!document.getElementById("guest-box").hidden`, 10*time.Second)
	clickSel(t, laptop, "#guest-copy")
	waitJS(t, laptop, "the link", `document.getElementById("guest-link").textContent.indexOf("#guest=bgg_") > 0`, 10*time.Second)
	link := evalString(t, laptop, `document.getElementById("guest-link").textContent`)

	phone, cancel2 := newChromedpContext(t, 60*time.Second)
	defer cancel2()
	if err := chromedp.Run(phone, chromedp.Navigate(link)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, phone, "the key out of the address", `location.hash === ""`, 10*time.Second)
	// After the reload, the game is among the phone's, and its seat is the phone's.
	if err := chromedp.Run(phone, chromedp.Navigate(base+"/game/"+room)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, phone, "the seat, as the phone's own", `document.querySelector("#seat-1 .badge") && [...document.querySelectorAll("#seat-1 .badge")].some(b => b.textContent === "you") && [...document.querySelectorAll("#seat-1 .badge")].some(b => b.textContent === "guest")`, 15*time.Second)
}

// The profile window in the top bar says who the browser is: a guest is
// offered the sign-in, an account its handle, its page and the sign-out.
func TestAccountUI_TheProfileWindowSaysWhoYouAre(t *testing.T) {
	a, base := membersServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the icon says guest", `document.getElementById("open-profile") && document.getElementById("open-profile").title === "Your profile (playing as a guest)"`, 10*time.Second)
	clickSel(t, ctx, "#open-profile")
	waitJS(t, ctx, "the sign-in in the window", `document.getElementById("profile-account-who").textContent === "You are playing as a guest." && document.getElementById("profile-signin").getAttribute("href") === "/login?return=%2F"`, 10*time.Second)
	clickSel(t, ctx, "#profile-signin")
	waitJS(t, ctx, "the sign-in page", `location.pathname === "/login" && document.getElementById("login-form") !== null`, 10*time.Second)

	ana := member(t, a, base, "ana")
	ctx2, cancel2 := newChromedpContext(t, 60*time.Second)
	defer cancel2()
	signInChrome(t, ctx2, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx2, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx2, "the icon says signed in", `document.getElementById("open-profile").title === "Your profile (signed in as @ana)"`, 10*time.Second)
	clickSel(t, ctx2, "#open-profile")
	waitJS(t, ctx2, "the account in the window", `document.getElementById("profile-account-who").textContent === "Signed in as @ana" && document.getElementById("profile-account-link") !== null && document.getElementById("profile-admin-link") === null`, 10*time.Second)
	clickSel(t, ctx2, "#profile-signout")
	waitJS(t, ctx2, "signed out", `document.getElementById("open-profile") && document.getElementById("open-profile").title === "Your profile (playing as a guest)"`, 15*time.Second)
}

// What a guest may not use is not offered to it: no direct messages, no
// batches to run, no turn mail to set up — and an account sees all three.
func TestAccountUI_AGuestIsNotOfferedWhatIsAnAccounts(t *testing.T) {
	a, base := membersServer(t)
	guest, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(guest, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, guest, "the top bar, without direct messages", `document.getElementById("open-profile") && document.getElementById("open-profile").title.indexOf("guest") > 0 && getComputedStyle(document.getElementById("open-messages")).display === "none"`, 10*time.Second)
	clickSel(t, guest, "#tab-analysis")
	waitJS(t, guest, "the Analysis tab, without the form", `document.getElementById("analysis-signin") !== null && document.getElementById("analysis-start") === null`, 15*time.Second)

	ana := member(t, a, base, "ana")
	acc, cancel2 := newChromedpContext(t, 60*time.Second)
	defer cancel2()
	signInChrome(t, acc, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(acc, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, acc, "the direct messages", `getComputedStyle(document.getElementById("open-messages")).display !== "none"`, 10*time.Second)
	clickSel(t, acc, "#tab-analysis")
	waitJS(t, acc, "the Analysis form", `document.getElementById("analysis-start") !== null && document.getElementById("analysis-signin") === null`, 15*time.Second)
}

// A guest's profile page says turn mail is for accounts instead of asking
// for an address it could not use.
func TestAccountUI_AGuestsProfileHasNoTurnMail(t *testing.T) {
	e := accountsNotifyEnv(t) // mail set up, notifications accounts' only
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(e.URL+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the note instead of the address", `!document.getElementById("notify-accounts-only").hidden && getComputedStyle(document.getElementById("profile-email-field")).display === "none" && getComputedStyle(document.getElementById("notify-box")).display === "none"`, 10*time.Second)
}

// The sign-in page asks for a username and a password, in boxes that look
// alike.
func TestAccountUI_TheSignInBoxesLookAlike(t *testing.T) {
	_, base := membersServer(t)
	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/login")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the form", `document.getElementById("login-password") !== null`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector('label[for="login-handle"]').textContent`); got != "Username" {
		t.Errorf("the first box is labelled %q, want Username", got)
	}
	style := `(function (id) { var s = getComputedStyle(document.getElementById(id)); return [s.borderTopWidth, s.borderTopStyle, s.borderTopColor, s.borderTopLeftRadius, s.paddingTop, s.paddingLeft, s.backgroundColor, s.fontSize].join("|"); })`
	user := evalString(t, ctx, style+`("login-handle")`)
	pass := evalString(t, ctx, style+`("login-password")`)
	if user != pass {
		t.Errorf("the boxes differ:\n username %s\n password %s", user, pass)
	}
}
