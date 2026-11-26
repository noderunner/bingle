package app_test

// The menu on the logo, on a server that offers the bingle-3d client (ADR 0151).

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// menuState is what the menu looks like to a player: open or not, and where the keys are.
const menuState = `(() => {
	const m = document.getElementById("brand-menu"), b = document.getElementById("brand-menu-button");
	return (m.hidden ? "closed" : "open") + " " + b.getAttribute("aria-expanded") + " " + (document.activeElement.id || document.activeElement.textContent.trim());
})()`

func TestWebClientMenu_TheLogoOpensAMenuAndTheNameGoesHome(t *testing.T) {
	_, base, _ := clientServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 900), chromedp.Navigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the bar", `document.getElementById("open-help") !== null && document.getElementById("brand-menu-button") !== null`, 20*time.Second)
	if got := evalString(t, ctx, menuState); !strings.HasPrefix(got, "closed false") {
		t.Fatalf("before a click the menu is %q", got)
	}
	if got := evalString(t, ctx, `document.querySelector(".topbar .brand").textContent.trim().split(/\s+/)[0]`); got != "Bingle" {
		t.Errorf("the bar says %q", got)
	}

	// A click opens it, with the keys on its first item.
	clickSel(t, ctx, "#brand-menu-button")
	if got := evalString(t, ctx, menuState); got != "open true brand-menu-home" {
		t.Fatalf("after a click the menu is %q", got)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll('#brand-menu [role="menuitem"]')].map((e) => e.textContent.trim()).join("|")`); got != "Home|Linux|Windows|Setup file" {
		t.Errorf("the menu offers %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("brand-menu-client").textContent`); got != "3D client" {
		t.Errorf("the downloads are headed %q", got)
	}
	// It hangs under the logo, inside the window, over the page.
	if !evalBool(t, ctx, `(() => {
		const m = document.getElementById("brand-menu").getBoundingClientRect(), b = document.getElementById("brand-menu-button").getBoundingClientRect();
		const top = document.elementFromPoint(m.left + 20, m.top + 20);
		return m.top >= b.bottom && m.left >= 0 && m.right <= innerWidth && m.bottom <= innerHeight && m.height > 100 && document.getElementById("brand-menu").contains(top);
	})()`) {
		t.Errorf("the menu is not under the logo: %s", evalString(t, ctx, `JSON.stringify(document.getElementById("brand-menu").getBoundingClientRect())`))
	}

	// The keys a menu takes.
	key := func(k string) {
		t.Helper()
		if err := chromedp.Run(ctx, chromedp.KeyEvent(k)); err != nil {
			t.Fatal(err)
		}
	}
	key(kb.ArrowDown)
	if got := evalString(t, ctx, menuState); got != "open true Linux" {
		t.Errorf("after the down arrow: %q", got)
	}
	key(kb.End)
	if got := evalString(t, ctx, menuState); got != "open true brand-menu-setup" {
		t.Errorf("after End: %q", got)
	}
	key(kb.ArrowDown)
	if got := evalString(t, ctx, menuState); got != "open true brand-menu-home" {
		t.Errorf("down from the last item: %q", got)
	}
	key(kb.Escape)
	if got := evalString(t, ctx, menuState); got != "closed false brand-menu-button" {
		t.Errorf("after Escape: %q", got)
	}
	// A click elsewhere closes it too.
	clickSel(t, ctx, "#brand-menu-button")
	clickSel(t, ctx, "main")
	if got := evalString(t, ctx, menuState); !strings.HasPrefix(got, "closed false") {
		t.Errorf("after a click on the page: %q", got)
	}

	// The setup file is asked for as the page asks for anything, and saved under its name.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const make = URL.createObjectURL.bind(URL);
		URL.createObjectURL = (blob) => { blob.text().then((text) => { window.__setup = text; }); return make(blob); };
		document.addEventListener("click", (ev) => { const a = ev.target.closest("a[download]"); if (a && a.href.startsWith("blob:")) { window.__saved = a.download; ev.preventDefault(); } }, true);
		return true;
	})()`, nil)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#brand-menu-button")
	clickSel(t, ctx, "#brand-menu-setup")
	waitJS(t, ctx, "the setup file", `typeof window.__setup === "string" && window.__saved`, 10*time.Second)
	file := setupOf(t, evalString(t, ctx, `window.__setup`))
	if file["server"] != base || !strings.HasPrefix(file["guest_key"], "bgg_") || evalString(t, ctx, `window.__saved`) != "bingle-3d-setup.toml" {
		t.Errorf("the setup file: %v, saved as %q", file, evalString(t, ctx, `window.__saved`))
	}
	if got := evalString(t, ctx, menuState); !strings.HasPrefix(got, "closed false") {
		t.Errorf("after the setup file: %q", got)
	}

	// The name is the way home.
	clickSel(t, ctx, ".topbar .brand-name")
	waitJS(t, ctx, "the main page", `location.pathname === "/" && document.getElementById("create-game-form") !== null`, 15*time.Second)
}

// On a phone the bar shows the logo alone, so the menu's first item is the way home.
func TestWebClientMenu_OnAPhoneTheMenuIsTheWayHome(t *testing.T) {
	_, base, _ := clientServer(t)
	room := councilWithHistory(t, base, 2)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openOnPhone(t, ctx, base, "Alice", map[string]string{room: "test-council"}, room)
	waitJS(t, ctx, "the logo", `document.getElementById("brand-menu-button") !== null`, 10*time.Second)
	if got := evalString(t, ctx, `getComputedStyle(document.querySelector(".topbar .brand-name")).fontSize`); got != "0px" {
		t.Fatalf("the name is shown on a phone, at %s", got)
	}
	clickSel(t, ctx, "#brand-menu-button")
	if !evalBool(t, ctx, `(() => {
		const m = document.getElementById("brand-menu").getBoundingClientRect(), home = document.getElementById("brand-menu-home").getBoundingClientRect();
		return m.left >= 0 && m.right <= innerWidth && m.bottom <= innerHeight && home.height >= 28 && parseFloat(getComputedStyle(document.getElementById("brand-menu-home")).fontSize) >= 14;
	})()`) {
		t.Errorf("the menu does not fit a phone: %s", evalString(t, ctx, `JSON.stringify(document.getElementById("brand-menu").getBoundingClientRect())`))
	}
	clickSel(t, ctx, "#brand-menu-home")
	waitJS(t, ctx, "the main page", `location.pathname === "/"`, 15*time.Second)
}
