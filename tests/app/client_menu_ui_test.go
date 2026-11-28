package app_test

// The menu on the logo, on a server that offers the bingle-3d client (ADR 0151).

import (
	"strings"
	"testing"
	"time"
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
	if err := runSteps(ctx, stepViewport(1400, 900), stepNavigate(base+"/profile")); err != nil {
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
	name := evalString(t, ctx, `JSON.parse(localStorage.getItem("bingle.name") || "\"\"")`)
	if name == "" {
		t.Fatal("the guest has no name")
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll('#brand-menu [role="menuitem"]')].map((e) => e.textContent.trim()).join("|")`); got != "Home|0.0.2|0.0.3|"+name+" Profile Configs|Client Quick Setup Guide" {
		t.Errorf("the menu offers %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("brand-menu-client").textContent`); got != "Download Game Client" {
		t.Errorf("the downloads are headed %q", got)
	}
	// A build is a download button: the platform's icon and the version on one line, named for a screen reader.
	if got := evalString(t, ctx, `[...document.querySelectorAll("#brand-menu a[download]")].map((a) => a.getAttribute("aria-label") + ":" + a.title + ":" + (a.querySelector("svg.platform-icon") ? "icon" : "none") + ":" + (a.getClientRects().length === 1 && a.querySelector("svg").getBoundingClientRect().bottom > a.getBoundingClientRect().top && Math.abs(a.querySelector("svg").getBoundingClientRect().top - a.getBoundingClientRect().top) < a.getBoundingClientRect().height / 2 ? "line" : "wrapped")).join("|")`); got != "Linux 0.0.2:0.0.2:icon:line|Windows 0.0.3:0.0.3:icon:line" {
		t.Errorf("the builds: %q", got)
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
		if err := runSteps(ctx, stepKey(k)); err != nil {
			t.Fatal(err)
		}
	}
	key(keyArrowDown)
	if got := evalString(t, ctx, menuState); got != "open true 0.0.2" {
		t.Errorf("after the down arrow: %q", got)
	}
	key(keyEnd)
	if got := evalString(t, ctx, menuState); got != "open true brand-menu-quick" {
		t.Errorf("after End: %q", got)
	}
	key(keyArrowUp)
	if got := evalString(t, ctx, menuState); got != "open true brand-menu-setup" {
		t.Errorf("up from the last item: %q", got)
	}
	key(keyEnd)
	key(keyArrowDown)
	if got := evalString(t, ctx, menuState); got != "open true brand-menu-home" {
		t.Errorf("down from the last item: %q", got)
	}
	key(keyEscape)
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
	if err := runSteps(ctx, stepEval(`(() => {
		const make = URL.createObjectURL.bind(URL);
		URL.createObjectURL = (blob) => { blob.text().then((text) => { window.__setup = text; }); return make(blob); };
		document.addEventListener("click", (ev) => { const a = ev.target.closest("a[download]"); if (a && a.href.startsWith("blob:")) { window.__saved = a.download; ev.preventDefault(); } }, true);
		return true;
	})()`)); err != nil {
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

	// The last item opens the quick setup page, which makes the same file from its own button.
	clickSel(t, ctx, "#brand-menu-button")
	clickSel(t, ctx, "#brand-menu-quick")
	waitJS(t, ctx, "the quick setup page", `location.pathname === "/client" && document.querySelector(".quick-platform.current .quick-setup-file") !== null`, 15*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll("#quick-setup .quick-platform")].map((s) => s.dataset.platform + ":" + s.querySelector(".quick-setup-file").textContent.trim()).join("|")`); got != "linux:"+name+" Profile Configs|windows:"+name+" Profile Configs" {
		t.Errorf("the page's sections, the computer's own first, with their profile configs buttons: %q", got)
	}
	if err := runSteps(ctx, stepEval(`(() => {
		const make = URL.createObjectURL.bind(URL);
		window.__setup = undefined;
		URL.createObjectURL = (blob) => { blob.text().then((text) => { window.__setup = text; }); return make(blob); };
		document.addEventListener("click", (ev) => { const a = ev.target.closest("a[download]"); if (a && a.href.startsWith("blob:")) { window.__saved = a.download; ev.preventDefault(); } }, true);
		return true;
	})()`)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, ".quick-platform.current .quick-setup-file")
	waitJS(t, ctx, "the setup file from the page", `typeof window.__setup === "string" && window.__saved === "bingle-3d-setup.toml"`, 10*time.Second)

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
