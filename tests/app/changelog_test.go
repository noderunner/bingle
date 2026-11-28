package app_test

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/i18n"
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
	Lang     string `json:"lang"`
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

// longChangelog has releases whose versions sort differently as text and as
// numbers (0.10.0 after 0.9.1), and text between them that belongs to none.
const longChangelog = `# Changelog

## 0.10.0 - 2026-11-03

### Bingle Core 0.10.0
- **Ten**: the newest.

### Fist 0.3.0
- A game's section.

## 0.9.1 - 2026-11-02

- Nine point one.

## Notes

Not a release.

## 0.9.0 - 2026-11-01

- Nine.

## 0.3.0 - 2026-09-01

- Three.
`

type changelogEntriesResp struct {
	Entries []changelogResp `json:"entries"`
}

func entryVersions(r changelogEntriesResp) string {
	var v []string
	for _, e := range r.Entries {
		v = append(v, e.Version)
	}
	return strings.Join(v, " ")
}

// GET /api/changelog/entries?since= gives every release after a version, the
// newest first, comparing versions as numbers; without since, all of them.
func TestChangelog_EntriesSinceAVersion(t *testing.T) {
	base := newServerWith(t, func(c *config.Config) { c.Changelog = longChangelog })
	for since, want := range map[string]string{
		"":       "0.10.0 0.9.1 0.9.0 0.3.0",
		"0.3.0":  "0.10.0 0.9.1 0.9.0",
		"0.9.0":  "0.10.0 0.9.1",
		"0.9.5":  "0.10.0",
		"0.10.0": "",
		"1.0.0":  "",
		"0.0.1":  "0.10.0 0.9.1 0.9.0 0.3.0",
	} {
		var got changelogEntriesResp
		if code := doJSON(t, "GET", base+"/api/changelog/entries?since="+since, nil, &got); code != 200 {
			t.Fatalf("since %q: %d", since, code)
		}
		if v := entryVersions(got); v != want {
			t.Errorf("since %q: %q, want %q", since, v, want)
		}
	}
	var got changelogEntriesResp
	doJSON(t, "GET", base+"/api/changelog/entries?since=0.9.0", nil, &got)
	if len(got.Entries) != 2 || got.Entries[1].Date != "2026-11-02" || got.Entries[1].Markdown != "- Nine point one." {
		t.Errorf("0.9.1, which a heading that is no release ends: %+v", got.Entries)
	}
	if code := doJSON(t, "GET", base+"/api/changelog/entries?since=latest", nil, nil); code != 400 {
		t.Errorf("since=latest: %d, want 400", code)
	}
	none := newRESTTestServer(t)
	if code := doJSON(t, "GET", none+"/api/changelog/entries?since=0.1.0", nil, &got); code != 200 || len(got.Entries) != 0 {
		t.Errorf("no changelog: %d %+v, want 200 and no entries", code, got)
	}
}

// The help window shows every release the player has not read, newest first,
// in a box that scrolls; the dot on its icon stays until they mark them read,
// and a browser that never read any is shown the newest alone.
func TestWebHelp_UnreadChangesUntilMarkedRead(t *testing.T) {
	base := newServerWith(t, func(c *config.Config) { c.Changelog = longChangelog })
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	open := func(what string) {
		t.Helper()
		if err := runSteps(ctx, stepReload(), stepWaitReady("#open-help")); err != nil {
			t.Fatal(err)
		}
		waitJS(t, ctx, "the changelog read", `window.performance.getEntriesByName(location.origin + "/api/changelog").length > 0`, 10*time.Second)
		time.Sleep(300 * time.Millisecond)
		clickSel(t, ctx, "#open-help")
		waitJS(t, ctx, what, `!document.getElementById("help-changes").hidden && document.querySelector("#help-changes .help-change")`, 10*time.Second)
	}
	shown := func() string {
		t.Helper()
		return evalString(t, ctx, `[...document.querySelectorAll("#help-changes .help-change")].map((e) => e.dataset.version).join(" ")`)
	}
	if err := runSteps(ctx, stepNavigate(base+"/"), stepWaitReady("#open-help")); err != nil {
		t.Fatal(err)
	}

	// Never read: the dot is on, and the window shows the newest release alone.
	waitJS(t, ctx, "the dot on the help icon", `!document.getElementById("help-badge").hidden`, 10*time.Second)
	if got := evalString(t, ctx, `document.getElementById("open-help").getAttribute("aria-label")`); got != "Help, with changes you have not seen" {
		t.Errorf("help icon label = %q", got)
	}
	open("the newest release")
	if got := shown(); got != "0.10.0" {
		t.Errorf("a browser that never read any: %q, want the newest alone", got)
	}
	if got := evalString(t, ctx, `document.querySelector("#help-changes .help-change-head").textContent`); got != "0.10.0 · 2026-11-03" {
		t.Errorf("release heading = %q", got)
	}
	// One heading per release: the Core heading that repeats it is dropped, a game's section keeps its own.
	if got := evalString(t, ctx, `[...document.querySelectorAll("#help-changes .help-change h1, #help-changes .help-change h2, #help-changes .help-change h3, #help-changes .help-change h4")].map((e) => e.textContent).join(" | ")`); got != "0.10.0 · 2026-11-03 | Fist 0.3.0" {
		t.Errorf("headings of the release = %q, want the release's own and the game's", got)
	}
	if evalBool(t, ctx, `document.getElementById("help-changes-head")`) {
		t.Error("a line of subtext is back above the changes")
	}
	if got := evalString(t, ctx, `document.querySelector("#help-changes .help-change strong").textContent`); got != "Ten" {
		t.Errorf("the Markdown was not rendered: first bold = %q", got)
	}
	if !evalBool(t, ctx, `!document.getElementById("help-mark-read").hidden`) {
		t.Error("no Mark all as read for an unread release")
	}

	// Looking is not reading: closed without marking, the dot stays.
	if err := runSteps(ctx, stepEval(`document.getElementById("help-dialog").close(); true`)); err != nil {
		t.Fatal(err)
	}
	if !evalBool(t, ctx, `!document.getElementById("help-badge").hidden`) {
		t.Error("the dot went when the window was only opened")
	}

	// Last read at 0.9.0: both releases since, newest first, in a box that scrolls.
	if err := runSteps(ctx, stepEval(`localStorage.setItem("bingle.changelog.seen", JSON.stringify("0.9.0")); true`)); err != nil {
		t.Fatal(err)
	}
	open("the unread releases")
	waitJS(t, ctx, "two releases", `document.querySelectorAll("#help-changes .help-change").length === 2`, 5*time.Second)
	if got := shown(); got != "0.10.0 0.9.1" {
		t.Errorf("unread since 0.9.0: %q, want \"0.10.0 0.9.1\"", got)
	}
	if got := evalString(t, ctx, `getComputedStyle(document.getElementById("help-changes-body")).overflowY`); got != "auto" {
		t.Errorf("the changes do not scroll: overflow-y %q", got)
	}

	// Mark all as read: the dot and the button go, and stay gone after a reload.
	clickSel(t, ctx, "#help-mark-read")
	waitJS(t, ctx, "the dot gone", `document.getElementById("help-badge").hidden && document.getElementById("help-mark-read").hidden`, 5*time.Second)
	if got := evalString(t, ctx, `JSON.parse(localStorage.getItem("bingle.changelog.seen"))`); got != "0.10.0" {
		t.Errorf("read up to %q, want 0.10.0", got)
	}
	open("the newest release, read")
	if got := shown(); got != "0.10.0" {
		t.Errorf("all read: %q, want the newest alone", got)
	}
	if !evalBool(t, ctx, `document.getElementById("help-badge").hidden && document.getElementById("help-mark-read").hidden`) {
		t.Error("the dot or the button is back after everything was read")
	}
}

// A server with no changelog shows no section and no dot.
func TestWebHelp_NoChangelogNoSection(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := runSteps(ctx, stepNavigate(base+"/"), stepWaitReady("#open-help")); err != nil {
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

// Each language of the interface has its changelog (docs/changelog/<code>.md, ADR 0141), whose
// newest entry is the engine's version too, so the help window has news in every language. Each
// entry it has is a release of CHANGELOG.md, with the same sections (the core's, each game's).
func TestChangelog_EachLanguageHasTheNewestEntry(t *testing.T) {
	raw, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	heading := regexp.MustCompile(`(?m)^## \[?(\d+\.\d+\.\d+)\]?(?: - (\d{4}-\d{2}-\d{2}))?\s*$`)
	sections := func(text string) map[string]string {
		out := map[string]string{}
		parts := heading.Split(text, -1)
		for i, m := range heading.FindAllStringSubmatch(text, -1) {
			out[m[1]] = strings.Join(regexp.MustCompile(`(?m)^### .*$`).FindAllString(parts[i+1], -1), "|")
		}
		return out
	}
	english := sections(string(raw))
	for _, l := range i18n.Languages() {
		if l.Code == i18n.Default {
			continue
		}
		text, err := os.ReadFile("../../docs/changelog/" + l.Code + ".md")
		if err != nil {
			t.Errorf("%s: no changelog: %v", l.Code, err)
			continue
		}
		m := heading.FindStringSubmatch(string(text))
		if m == nil || m[1] != version.Engine {
			t.Errorf("%s: the newest entry is not %s, the engine's version: add it to docs/changelog/%s.md", l.Code, version.Engine, l.Code)
		}
		for v, own := range sections(string(text)) {
			if en, ok := english[v]; !ok {
				t.Errorf("%s: %s is no release of CHANGELOG.md", l.Code, v)
			} else if own != en {
				t.Errorf("%s %s: sections %q, English's %q", l.Code, v, own, en)
			}
		}
	}
}

// A release is told in the reader's language when its translation has it, and else in English,
// which the answer says.
func TestChangelog_InTheReadersLanguage(t *testing.T) {
	fr := "# Nouveautés\n\n## 0.10.0 - 2026-11-03\n\n### Bingle Core 0.10.0\n- **Dix** : la plus récente.\n"
	base := newServerWith(t, func(c *config.Config) {
		c.Changelog = longChangelog
		c.Changelogs = map[string]string{"fr": fr}
	})
	get := func(path, lang string, out any) {
		t.Helper()
		req, _ := http.NewRequest("GET", base+path, nil)
		req.Header.Set("Accept-Language", lang)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	var newest changelogResp
	get("/api/changelog", "fr-FR,fr;q=0.9", &newest)
	if newest.Version != "0.10.0" || newest.Lang != "fr" || !strings.Contains(newest.Markdown, "**Dix**") {
		t.Errorf("the newest in French: %+v", newest)
	}
	var since changelogEntriesResp
	get("/api/changelog/entries?since=0.9.0", "fr", &since)
	if len(since.Entries) != 2 || since.Entries[0].Lang != "fr" || since.Entries[1].Lang != "en" || since.Entries[1].Markdown != "- Nine point one." {
		t.Errorf("since 0.9.0 in French: %+v", since.Entries)
	}
	get("/api/changelog", "de", &newest)
	if newest.Lang != "en" || !strings.Contains(newest.Markdown, "**Ten**") {
		t.Errorf("the newest in German, which has no changelog here: %+v", newest)
	}
}
