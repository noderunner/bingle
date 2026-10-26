package app_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/version"
)

const sampleChangelog = `# Changelog

Intro text that belongs to no release.

## [0.4.0] - 2026-10-26

### Core
- Added **Recent Changes** to the help window.
- Something else.

### Fist
- The designer is listed.

## 0.3.0 — 2026-09-01

- An older change.
`

type changelogResp struct {
	Version  string `json:"version"`
	Date     string `json:"date"`
	Markdown string `json:"markdown"`
}

// The server reports the newest release of the changelog it was built with,
// and only that one.
func TestChangelog_ReportsTheNewestRelease(t *testing.T) {
	base := newServerWith(t, func(c *config.Config) { c.Changelog = sampleChangelog })
	var got changelogResp
	if code := doJSON(t, "GET", base+"/api/changelog", nil, &got); code != 200 {
		t.Fatalf("GET /api/changelog: %d", code)
	}
	if got.Version != "0.4.0" || got.Date != "2026-10-26" {
		t.Errorf("release = %q dated %q, want 0.4.0 dated 2026-10-26", got.Version, got.Date)
	}
	for _, want := range []string{"### Core", "Added **Recent Changes**", "### Fist"} {
		if !strings.Contains(got.Markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, got.Markdown)
		}
	}
	for _, not := range []string{"Intro text", "An older change", "0.3.0", "\n## ", "# Changelog"} {
		if strings.Contains(got.Markdown, not) {
			t.Errorf("markdown holds %q, which is not part of the newest release:\n%s", not, got.Markdown)
		}
	}
}

// A heading with no brackets or no date is still a release; text with no
// release at all is no changelog.
func TestChangelog_HeadingFormsAndNone(t *testing.T) {
	base := newServerWith(t, func(c *config.Config) { c.Changelog = "## 1.2.3\n\n- one\n" })
	var got changelogResp
	if code := doJSON(t, "GET", base+"/api/changelog", nil, &got); code != 200 || got.Version != "1.2.3" || got.Date != "" || got.Markdown != "- one" {
		t.Errorf("heading without date: %d %+v", code, got)
	}
	for name, text := range map[string]string{"empty": "", "no release": "# Changelog\n\nNothing yet.\n", "not a version": "## Unreleased\n\n- x\n"} {
		none := newServerWith(t, func(c *config.Config) { c.Changelog = text })
		if code := doJSON(t, "GET", none+"/api/changelog", nil, &got); code != 404 {
			t.Errorf("%s: GET /api/changelog = %d, want 404", name, code)
		}
	}
}

// The help window shows the newest changes, and a dot on its icon says there
// is something the player has not seen until they have opened the window.
func TestWebHelp_RecentChangesAndTheUnseenDot(t *testing.T) {
	base := newServerWith(t, func(c *config.Config) { c.Changelog = sampleChangelog })
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/"), chromedp.WaitReady("#open-help", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	// Never seen: the dot is on.
	waitJS(t, ctx, "the dot on the help icon", `document.getElementById("help-badge") && !document.getElementById("help-badge").hidden`, 10*time.Second)
	if got := evalString(t, ctx, `document.getElementById("open-help").getAttribute("aria-label")`); got != "Help, with changes you have not seen" {
		t.Errorf("help icon label = %q", got)
	}
	clickSel(t, ctx, "#open-help")
	waitJS(t, ctx, "the recent changes", `!document.getElementById("help-changes").hidden && document.querySelectorAll("#help-changes li").length === 3`, 10*time.Second)
	if got := evalString(t, ctx, `document.getElementById("help-changes-head").textContent`); got != "Bingle 0.4.0 · 2026-10-26" {
		t.Errorf("entry heading = %q", got)
	}
	if got := evalString(t, ctx, `document.querySelector("#help-changes li strong").textContent`); got != "Recent Changes" {
		t.Errorf("the Markdown was not rendered: first bold = %q", got)
	}
	// Seen: the dot goes, and it stays gone after a reload.
	waitJS(t, ctx, "the dot gone", `document.getElementById("help-badge").hidden`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Reload(), chromedp.WaitReady("#open-help", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the changelog read", `window.performance.getEntriesByName(location.origin + "/api/changelog").length > 0`, 10*time.Second)
	time.Sleep(300 * time.Millisecond)
	if !evalBool(t, ctx, `document.getElementById("help-badge").hidden`) {
		t.Error("the dot came back for an entry already seen")
	}
	// An older version seen: this entry is news again.
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`localStorage.setItem("bingle.changelog.seen", JSON.stringify("0.3.0")); true`, nil),
		chromedp.Reload(), chromedp.WaitReady("#open-help", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the dot for a newer entry", `!document.getElementById("help-badge").hidden`, 10*time.Second)
}

// A server with no changelog shows no section and no dot.
func TestWebHelp_NoChangelogNoSection(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/"), chromedp.WaitReady("#open-help", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#open-help")
	waitJS(t, ctx, "the help window", `document.getElementById("help-dialog").open`, 5*time.Second)
	time.Sleep(400 * time.Millisecond)
	if !evalBool(t, ctx, `document.getElementById("help-changes").hidden && document.getElementById("help-badge").hidden`) {
		t.Error("a section or dot is shown though the server has no changelog")
	}
}

// The help window shows the newest entry of CHANGELOG.md, so that entry is
// the one for the version of the engine the binary is: the heading of a release
// is the engine's version (CONTRIBUTING.md says to start a new entry when it is raised).
func TestChangelog_NewestEntryIsTheEnginesVersion(t *testing.T) {
	raw, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^## \[?(\d+\.\d+\.\d+)\]?(?: - (\d{4}-\d{2}-\d{2}))?\s*$`).FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatal("CHANGELOG.md has no release heading such as \"## 0.4.0 - 2026-10-26\"")
	}
	if m[1] != version.Engine {
		t.Errorf("the newest entry of CHANGELOG.md is for %s, but the engine is %s: add an entry for %s (CONTRIBUTING.md)", m[1], version.Engine, version.Engine)
	}
	if m[2] == "" {
		t.Error("the newest entry has no date")
	}
	// What the server would show is that entry's text, with its sections.
	base := newServerWith(t, func(c *config.Config) { c.Changelog = string(raw) })
	var got changelogResp
	if code := doJSON(t, "GET", base+"/api/changelog", nil, &got); code != 200 || got.Version != version.Engine || !strings.Contains(got.Markdown, "### Bingle Core") {
		t.Errorf("GET /api/changelog: %d %+v", code, got)
	}
}
