package app_test

// Announcements in the page (ADR 0155): the main page shows those waiting in
// the tour's card and closing it marks them all seen; Recent Changes links
// each under the release it came after, and opening one there marks nothing.

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
)

// announceAt publishes an announcement as the administrator at a given time.
func announceAt(t *testing.T, a *app.App, admin *browser, title string, at time.Time) int64 {
	t.Helper()
	a.Resolver().SetClock(func() time.Time { return at })
	defer a.Resolver().SetClock(time.Now)
	var out auth.Announcement
	if code := admin.do("POST", "/auth/admin/announcements", map[string]any{"title": title, "body": "About **" + title + "**: see [the setup](/client)."}, &out); code != http.StatusCreated {
		t.Fatalf("announce %q: %d", title, code)
	}
	return out.ID
}

func TestWebAnnouncements_ShownOnceOnTheMainPage(t *testing.T) {
	a, base, admin := announcementServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := runSteps(ctx, stepNavigate(base+"/"), stepWaitReady("#open-help")); err != nil {
		t.Fatal(err)
	}
	// The page's first request makes its guest.
	if !awaitBool(t, ctx, `fetch("/auth/tour-seen", {method: "POST", headers: {"X-Bingle-Session": "1"}}).then((r) => r.ok)`) {
		t.Fatal("no guest")
	}
	one := announce(t, a, admin, "One")
	two := announce(t, a, admin, "Two")
	if err := runSteps(ctx, stepReload()); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the announcements", `document.querySelector("#announcement-card[open]")`, 15*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll("#announcement-card .tour-dot")].map((d) => d.dataset.slide).join(" ")`); got != "announcement-"+strconv.FormatInt(one, 10)+" announcement-"+strconv.FormatInt(two, 10) {
		t.Errorf("slides %q, want one then two", got)
	}
	if got := evalString(t, ctx, `document.getElementById("tour-title").textContent + " | " + document.querySelector("#tour-text strong").textContent + " | " + document.querySelector("#tour-text a").getAttribute("href")`); got != "One | One | /client" {
		t.Errorf("the first slide: %q", got)
	}
	if evalBool(t, ctx, `!document.getElementById("tour-picture").hidden`) {
		t.Error("an announcement has an empty picture frame")
	}
	// Close, on the first slide: both were shown, and both are seen.
	clickSel(t, ctx, "#tour-skip")
	deadline := time.Now().Add(10 * time.Second)
	for !awaitBool(t, ctx, `fetch("/auth/me").then((r) => r.json()).then((me) => !me.announcements)`) {
		if time.Now().After(deadline) {
			t.Fatal("closing the card did not mark the announcements seen")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := runSteps(ctx, stepReload(), stepWaitReady("#open-help")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if evalBool(t, ctx, `!!document.getElementById("announcement-card")`) {
		t.Error("the card is shown again after it was closed")
	}
}

const announcementsChangelog = `# Changelog

## 0.2.0 - 2026-06-10

- Two.

## 0.1.0 - 2026-06-01

- One.
`

func TestWebAnnouncements_LinkedInRecentChanges(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Addr = "127.0.0.1:0"
	cfg.Changelog = announcementsChangelog
	a, base := serve(t, cfg)
	admin := newBrowser(t, base)
	admin.do("POST", "/auth/login", map[string]any{"handle": "admin", "password": auth.DevPassword}, nil)
	admin.do("POST", "/auth/password", map[string]any{"old": auth.DevPassword, "new": "the admin's own"}, nil)
	day := func(d string) time.Time {
		at, _ := time.Parse(time.DateOnly, d)
		return at.Add(12 * time.Hour)
	}
	announceAt(t, a, admin, "Before every release", day("2026-05-01"))
	between := announceAt(t, a, admin, "Between", day("2026-06-05"))
	sameDay := announceAt(t, a, admin, "On the newest's day", day("2026-06-10"))
	after := announceAt(t, a, admin, "After the newest", day("2026-06-20"))
	gone := announceAt(t, a, admin, "Withdrawn", day("2026-06-12"))
	if code := admin.do("POST", "/auth/admin/announcements/"+strconv.FormatInt(gone, 10)+"/withdraw", nil, nil); code != 200 {
		t.Fatalf("withdraw: %d", code)
	}

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := runSteps(ctx, stepNavigate(base+"/"), stepEval(`localStorage.setItem("bingle.changelog.seen", JSON.stringify("0.0.1")); true`), stepReload(), stepWaitReady("#open-help")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the changelog read", `window.performance.getEntriesByName(location.origin + "/api/changelog").length > 0`, 10*time.Second)
	time.Sleep(300 * time.Millisecond)
	clickSel(t, ctx, "#open-help")
	waitJS(t, ctx, "the links", `document.querySelectorAll(".help-announcement").length >= 3`, 10*time.Second)
	under := func(version string) string {
		return evalString(t, ctx, `[...document.querySelectorAll('.help-change[data-version="`+version+`"] .help-announcement')].map((a) => a.dataset.announcement).join(" ")`)
	}
	id := strconv.FormatInt
	if got, want := under("0.2.0"), id(after, 10)+" "+id(sameDay, 10); got != want {
		t.Errorf("under 0.2.0: %q, want %q (newest first, the withdrawn one left out)", got, want)
	}
	if got, want := under("0.1.0"), id(between, 10); got != want {
		t.Errorf("under 0.1.0: %q, want %q", got, want)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll(".help-announcement").length`); n != 3 {
		t.Errorf("%d links, want 3: one from before every release and a withdrawn one are not linked", n)
	}

	// A link shows it alone, and closing it marks nothing.
	clickSel(t, ctx, `.help-announcement[data-announcement="`+id(between, 10)+`"]`)
	waitJS(t, ctx, "the announcement", `document.querySelector("#announcement-card[open]")`, 10*time.Second)
	if got := evalString(t, ctx, `document.getElementById("tour-title").textContent`); got != "Between" {
		t.Errorf("opened %q", got)
	}
	if !evalBool(t, ctx, `document.getElementById("tour-skip") === null && document.getElementById("tour-count").hidden && document.getElementById("tour-next").textContent === "Done"`) {
		t.Error("one announcement alone has a count, a Close or no Done")
	}
	clickSel(t, ctx, "#tour-next")
	waitJS(t, ctx, "the card closed", `!document.getElementById("announcement-card") && document.getElementById("help-dialog").open`, 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	if evalBool(t, ctx, `performance.getEntriesByName(location.origin + "/auth/announcements-seen").length > 0`) {
		t.Error("opening an announcement from Recent Changes marked it seen")
	}
}

// An announcement's body is several slides, split at a line of ---, and shows
// its own pictures (#357).
func TestWebAnnouncements_SlidesAndPictures(t *testing.T) {
	a, base, admin := announcementServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := runSteps(ctx, stepNavigate(base+"/"), stepWaitReady("#open-help")); err != nil {
		t.Fatal(err)
	}
	if !awaitBool(t, ctx, `fetch("/auth/tour-seen", {method: "POST", headers: {"X-Bingle-Session": "1"}}).then((r) => r.ok)`) {
		t.Fatal("no guest")
	}
	body := "First words.\n\n![a board](board.png \"The board\")\n\n---\n\n## Second\n\nMore words.\n\n```\n---\n```\n"
	var made auth.Announcement
	if code := announceForm(t, a, admin, "News", body, []picture{{"board.png", pngOf(t)}}, &made); code != http.StatusCreated {
		t.Fatalf("announce: %d", code)
	}
	if err := runSteps(ctx, stepReload()); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the announcement", `document.querySelector("#announcement-card[open]")`, 15*time.Second)
	id := "announcement-" + strconv.FormatInt(made.ID, 10)
	if got := evalString(t, ctx, `[...document.querySelectorAll("#announcement-card .tour-dot")].map((d) => d.dataset.slide).join(" ")`); got != id+" "+id+"-2" {
		t.Errorf("slides %q, want two of one announcement", got)
	}
	if got := evalString(t, ctx, `document.getElementById("tour-title").textContent`); got != "News" {
		t.Errorf("the first slide's title %q, want the announcement's", got)
	}
	waitJS(t, ctx, "its picture", `(() => { const i = document.querySelector("#tour-text img"); return i && i.complete && i.naturalWidth === 8; })()`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#tour-text figcaption").textContent`); got != "The board" {
		t.Errorf("caption %q", got)
	}
	clickSel(t, ctx, "#tour-next")
	if got := evalString(t, ctx, `document.getElementById("tour-title").textContent + " | " + document.querySelector("#tour-text pre").textContent.trim() + " | " + !document.querySelector("#tour-text h2")`); got != "Second | --- | true" {
		t.Errorf("the second slide: %q", got)
	}
	clickSel(t, ctx, "#tour-next")
	deadline := time.Now().Add(10 * time.Second)
	for !awaitBool(t, ctx, `fetch("/auth/me").then((r) => r.json()).then((me) => !me.announcements)`) {
		if time.Now().After(deadline) {
			t.Fatal("closing the card did not mark the announcement seen")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
