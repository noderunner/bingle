// Tests of the setup page's layout: the tabs, the sections of Cthulhu Wars' setup, its
// sand-timer spellbooks as cards of their own, and no box for the name.

package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestWebSetup_HasNoNameBoxAndPlaysUnderTheProfileName(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=gomoku")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the setup", `!document.getElementById("game-setup").hidden`, 20*time.Second)
	if evalBool(t, ctx, `document.getElementById("create_player_name") !== null || document.getElementById("puzzle_player_name") !== null`) {
		t.Error("a name box is still on the page")
	}
	if got := evalString(t, ctx, `document.getElementById("setup-tab-new").textContent`); got != "Create Game" {
		t.Errorf("the tab says %q", got)
	}
	name := evalString(t, ctx, `JSON.parse(localStorage.getItem("bingle.name"))`)
	clickSel(t, ctx, "#create-submit")
	waitJS(t, ctx, "the game", `location.pathname.startsWith("/game/")`, 15*time.Second)
	waitJS(t, ctx, "the seat under that name", `document.querySelector("#seat-1 .name") && document.querySelector("#seat-1 .name").textContent === `+jsQuote(name), 15*time.Second)
}

func jsQuote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

func TestWebCthulhuWarsSetup_TheLayout(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1300, 1000), chromedp.Navigate(baseURL+"/?game=cthulhu-wars")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the picker", `document.getElementById("cw-panel") !== null && document.querySelector("#slots-field .slot-select") !== null`, 20*time.Second)

	// The sections, in order, each with its title.
	if got := evalString(t, ctx, `[...document.querySelectorAll("#create-game-form .section-head h3")].filter(h => h.offsetParent).map(h => h.textContent).join("|")`); got != "Expansions|Seats|Other Options" {
		t.Errorf("the sections are %q", got)
	}
	// The setup's buttons share the title's row, to its right.
	if !evalBool(t, ctx, `document.getElementById("cw-profile").closest(".section-head") !== null && document.getElementById("cw-shuffle").closest(".section-head") !== null`) {
		t.Error("the buttons are not beside the Expansions title")
	}
	for _, id := range []string{"slots-all"} {
		if evalBool(t, ctx, `document.getElementById("`+id+`") !== null`) {
			t.Errorf("#%s is still there", id)
		}
	}
	if got := evalString(t, ctx, `document.getElementById("slots-field").innerText`); strings.Contains(got, "named after") {
		t.Errorf("the note on bots' names is still there: %q", got)
	}

	// The other options are cards in two columns.
	if !evalBool(t, ctx, `(function () { var c = [...document.querySelectorAll("#other-options .field")].filter(f => f.offsetParent).map(f => f.getBoundingClientRect()); return c.length >= 2 && new Set(c.map(r => Math.round(r.left))).size === 2 && getComputedStyle(document.querySelector("#other-options .field")).borderTopStyle === "solid"; })()`) {
		t.Error("the other options are not cards in two columns")
	}

	// The maps tab shows the pictures of the maps.
	if got := evalString(t, ctx, `document.querySelector("#cw-panel .cw-map-thumb").getAttribute("src")`); !strings.HasSuffix(got, ".webp") {
		t.Errorf("a map's thumbnail is %q", got)
	}
	waitJS(t, ctx, "the pictures to load", `[...document.querySelectorAll("#cw-panel .cw-map-thumb")].some(i => i.complete && i.naturalWidth > 0)`, 10*time.Second)

	// The Factions tab says how open seats choose.
	clickSel(t, ctx, `#cw-setup .cw-tab[data-tab="factions"]`)
	if got := evalString(t, ctx, `document.getElementById("cw-factions-note").textContent`); !strings.HasPrefix(got, "Open seats will only be allowed") {
		t.Errorf("the note says %q", got)
	}

	// The sand-timer spellbooks are not in the Other tab, and have a card each.
	clickSel(t, ctx, `#cw-setup .cw-tab[data-tab="other"]`)
	if evalBool(t, ctx, `document.querySelector("#cw-panel #book-swaps, #cw-panel .swap-select") !== null`) {
		t.Error("the spellbook swaps are still in the Other tab")
	}
	card := func(cult string) string {
		return `document.querySelector('#other-options [data-swap-card="` + cult + `"]')`
	}
	if !evalBool(t, ctx, card("bg")+` !== null && `+card("cc")+` !== null`) {
		t.Fatal("no cards for Ghroth and The Thousand Forms")
	}
	if !evalBool(t, ctx, card("bg")+`.getBoundingClientRect().left !== `+card("cc")+`.getBoundingClientRect().left`) {
		t.Error("the spellbooks' cards are not side by side")
	}
	if got := evalString(t, ctx, card("bg")+`.innerText`); !strings.Contains(got, "sand-timer") || !strings.Contains(got, "correspondence") {
		t.Errorf("the card says %q", got)
	}
	options := func(cult string) string {
		return evalString(t, ctx, `[...document.querySelector('select[data-swap="`+cult+`"]').options].map(o => o.textContent).join("|")`)
	}
	if got := options("bg"); !strings.Contains(got, "Undimensioned (Recommended)") || strings.Contains(got, "Replace with") || strings.Contains(got, "Stars Are Right (Recommended)") {
		t.Errorf("Ghroth's choices are %q", got)
	}
	if got := options("cc"); !strings.Contains(got, "Stars Are Right (Recommended)") || strings.Contains(got, "Undimensioned (Recommended)") {
		t.Errorf("The Thousand Forms' choices are %q", got)
	}

	// A cult that leaves the table takes its card, and only its card.
	clickSel(t, ctx, `#cw-setup .cw-tab[data-tab="factions"]`)
	clickSel(t, ctx, `#cw-panel [data-item="cc"]`)
	waitJS(t, ctx, "the card gone", card("cc")+` === null && `+card("bg")+` !== null`, 5*time.Second)

	// Another game of the page has no such cards, and no Seats section.
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=gomoku")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the setup", `!document.getElementById("game-setup").hidden && document.getElementById("create-submit") !== null`, 20*time.Second)
	if evalBool(t, ctx, `document.getElementById("other-options-head").offsetParent !== null || document.querySelector("[data-swap-card]") !== null`) {
		t.Error("a game of two has Cthulhu Wars' sections")
	}
}

// Tutorials have a tab of their own, offered by a game that has any.
func TestWebSetup_TutorialsHaveATab(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=blooms")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the tab", `!document.getElementById("setup-tab-tutorials").hidden`, 20*time.Second)
	if !evalBool(t, ctx, `document.getElementById("setup-tutorials-panel").hidden`) {
		t.Error("the tutorials show before their tab is chosen")
	}
	clickSel(t, ctx, "#setup-tab-tutorials")
	waitJS(t, ctx, "the tutorials", `!document.getElementById("setup-tutorials-panel").hidden && document.querySelector('#setup-tutorials button[data-tutorial="blooms-basics"]') !== null && document.getElementById("setup-layout").hidden`, 5*time.Second)
	if got := evalString(t, ctx, `document.getElementById("setup-tutorials").innerText`); strings.Contains(got, "Your name") {
		t.Errorf("the tutorials ask for a name: %q", got)
	}
	// A game with none gives way to the form.
	evalBool(t, ctx, `(document.getElementById("setup-back").click(), true)`)
	waitJS(t, ctx, "the list", `!document.getElementById("gallery-view").hidden`, 5*time.Second)
	clickSel(t, ctx, `.game-card[data-family="gomoku"]`)
	waitJS(t, ctx, "gomoku's setup", `document.getElementById("setup-tab-tutorials").hidden && !document.getElementById("setup-layout").hidden`, 10*time.Second)
}
