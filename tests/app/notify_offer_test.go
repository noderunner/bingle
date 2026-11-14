// Browser notifications are on by default for an account (ADR 0130's
// announcements): an account is offered the browser's permission once, a
// guest never.

package app_test

import (
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
)

func TestNotifyOffer_AnAccountIsOfferedNotificationsOnce(t *testing.T) {
	a, base := membersServer(t)

	// A guest is not offered them.
	guest, cancelGuest := newChromedpContext(t, 60*time.Second)
	defer cancelGuest()
	if err := chromedp.Run(guest, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, guest, "the top bar", `document.getElementById("open-profile") !== null`, 10*time.Second)
	var offered bool
	if err := chromedp.Run(guest, chromedp.Sleep(500*time.Millisecond), chromedp.Evaluate(`document.getElementById("notify-offer") !== null`, &offered)); err != nil {
		t.Fatal(err)
	}
	if offered {
		t.Error("a guest was offered notifications")
	}

	ana := member(t, a, base, "ana")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	var perm string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`"Notification" in window ? Notification.permission : "none"`, &perm)); err != nil {
		t.Fatal(err)
	}
	if perm != "default" {
		t.Skipf("this browser's notification permission is %q, so nothing is offered", perm)
	}
	waitJS(t, ctx, "the offer", `document.getElementById("notify-offer") !== null`, 10*time.Second)
	clickSel(t, ctx, "#notify-offer-off")
	waitJS(t, ctx, "the offer gone and the setting off", `document.getElementById("notify-offer") === null && localStorage.getItem("bingle.notify") === "false"`, 10*time.Second)
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/"), chromedp.Sleep(800*time.Millisecond), chromedp.Evaluate(`document.getElementById("notify-offer") !== null`, &offered)); err != nil {
		t.Fatal(err)
	}
	if offered {
		t.Error("the offer came back after No thanks")
	}
}
