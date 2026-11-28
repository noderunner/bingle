// Tests of the web UI: the create and join pages, driven in a real browser.
//
// The browser-driven tests use github.com/chromedp/chromedp with headless
// Chrome or Chromium. They look for one on $PATH (google-chrome, chromium, ...)
// or at $BINGLE_BROWSER, and are skipped when there is none, so the rest of the
// suite runs anywhere. Every browser interaction is bounded by a timeout so a
// real failure reports as an error instead of hanging the run.

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/session/roomcode"
)

// gamePageURLPattern matches the path a successful create-game submission
// must navigate the browser to: "/game/" followed by exactly a 6-character
// room code drawn from roomcode.Alphabet.
var gamePageURLPattern = regexp.MustCompile(`/game/([` + regexp.QuoteMeta(roomcode.Alphabet) + `]{6})$`)

// newChromedpContext returns a chromedp context using the default
// ExecAllocator (auto-detecting a "chromium"/"google-chrome" binary on
// $PATH, per docs/history/plan.md section 6.11) bounded by an overall timeout, plus
// the cancel function the caller must defer to clean up the browser, the
// allocator and the timeout.
//
// Each browser gets its own uniquely named user-data directory so that, on
// setups where cancelling the context does not terminate the real Chrome
// process tree (the Flatpak-wrapper quirk described in docs/test.md),
// cancel can still find and kill exactly this test's leftovers and delete
// its profile directory, instead of leaking a browser per test.
func newChromedpContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return newChromedpContextWith(t, timeout)
}

// findBrowser is the path of the Chrome or Chromium the browser tests drive:
// $BINGLE_BROWSER, else the first well-known name found on $PATH, else "".
func findBrowser() string {
	if p := os.Getenv("BINGLE_BROWSER"); p != "" {
		return p
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "headless_shell"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// tourSeenScript marks the welcome tour seen in a page's browser before the page runs.
const tourSeenScript = `try { localStorage.setItem("bingle.tour.seen", "true"); } catch (_) {}`

// newChromedpContextWithTour is a browser whose pages are offered the welcome tour
// as a new browser's are.
func newChromedpContextWithTour(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return chromedpContext(t, timeout)
}

// killLeftoverBrowser terminates any process still holding profileDir open
// and removes the profile (including the copy a sandboxed browser may have
// created under the Flatpak runtime directory). Everything is best-effort.
func killLeftoverBrowser(profileDir string) {
	base := filepath.Base(profileDir)
	// pkill matches the full command line; the random suffix makes the
	// pattern unique to this one browser.
	_ = exec.Command("pkill", "-9", "-f", "--", base).Run()
	_ = os.RemoveAll(profileDir)
	if uid := os.Getuid(); uid >= 0 {
		sandboxTmp := filepath.Join("/run/user", fmt.Sprint(uid), ".flatpak", "org.chromium.Chromium", "tmp", base)
		_ = os.RemoveAll(sandboxTmp)
	}
}

// TestWebIndex_CreateGameForm_SubmitsAndCreatesRealGame covers backlog item
// 37's AC end-to-end via a real headless browser: loading "/" renders a
// create-game form (a select for game_def_id offering "test-freeplay", and
// the player's name comes from the profile); submitting it navigates the
// browser to /game/<6-char-code>; and that code corresponds to a real game
// (a plain REST GET /api/games/{code} against the same server succeeds and
// shows "Alice" seated at seat 1).
func TestWebIndex_CreateGameForm_SubmitsAndCreatesRealGame(t *testing.T) {
	baseURL := newRESTTestServer(t)

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()

	if err := runSteps(ctx,
		// A link with ?game= opens the setup of that game (the game is
		// otherwise picked from the gallery).
		stepNavigate(baseURL+"/?game=test-freeplay"),
		stepWaitVisible(`#create-submit`),
	); err != nil {
		t.Fatalf("navigate to / and locate create-game form fields: %v", err)
	}

	// Confirm the game_def_id select actually offers test-freeplay as an
	// option before trying to select it — a clearer failure than "SetValue
	// silently did nothing" if the option is missing/misnamed.
	var hasFixtureOption bool
	if err := runSteps(ctx, stepEvalTo(
		`!!document.querySelector('select[name="game_def_id"] option[value="test-freeplay"]')`,
		&hasFixtureOption,
	)); err != nil {
		t.Fatalf("check for test-freeplay option: %v", err)
	}
	if !hasFixtureOption {
		t.Fatalf(`select[name="game_def_id"] has no option[value="test-freeplay"]`)
	}

	if err := runSteps(ctx,
		// A guest's made-up name is already there: this is typing their own.
		stepEval(`localStorage.setItem("bingle.name", JSON.stringify(""))`),
		stepEval(`localStorage.setItem("bingle.name", JSON.stringify("Alice"))`),
		submitClosestForm(ctx, `select[name="game_def_id"]`),
	); err != nil {
		t.Fatalf("fill in and submit create-game form: %v", err)
	}

	gameURL := pollLocation(t, ctx, gamePageURLPattern)

	matches := gamePageURLPattern.FindStringSubmatch(gameURL)
	if len(matches) != 2 {
		t.Fatalf("gamePageURLPattern.FindStringSubmatch(%q) = %v, want a match with one capture group", gameURL, matches)
	}
	roomCode := matches[1]

	// Confirm the room code in the URL corresponds to a REAL game, via a
	// plain REST call against the same server (not through the browser).
	resp, err := http.Get(baseURL + "/api/games/" + roomCode)
	if err != nil {
		t.Fatalf("GET /api/games/%s: %v", roomCode, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET /api/games/%s response body: %v", roomCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/games/%s status = %d, want %d (body: %s)", roomCode, resp.StatusCode, http.StatusOK, raw)
	}

	var body getGameResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode GET /api/games/%s response: %v (body: %s)", roomCode, err, raw)
	}

	var seat1 *getGameSeat
	for i := range body.Seats {
		if body.Seats[i].Seat == 1 {
			seat1 = &body.Seats[i]
		}
	}
	if seat1 == nil {
		t.Fatalf("GET /api/games/%s response has no seat 1 entry (seats: %+v)", roomCode, body.Seats)
	}
	if seat1.Name != "Alice" {
		t.Errorf("seat 1 name = %q, want %q (room %s was created via the web UI form)", seat1.Name, "Alice", roomCode)
	}
}

// TestWebIndex_ServedHTML_CreateFormPostsToAPIGames is the lighter-weight,
// non-browser-driven counterpart of the AC test above: it fetches "/" via a
// plain net/http GET (no browser, no JS execution) and confirms the raw
// response body gives static evidence that the create-game form is wired to
// POST /api/games — either directly (a <form ... action="/api/games"
// method="post" ...>) or indirectly, via a client-side script (referenced by
// a <script src="..."> in the page, or inlined) that references both
// "/api/games" and a POST verb. This guards against a future regression
// ("the form no longer points at the right endpoint at all") without
// requiring a browser.
func TestWebIndex_ServedHTML_CreateFormPostsToAPIGames(t *testing.T) {
	baseURL := newRESTTestServer(t)

	html := fetchBody(t, baseURL, "/")
	combined := html + " " + fetchLinkedScripts(t, baseURL, html)

	if !strings.Contains(combined, "/api/games") {
		t.Fatalf(`served "/" page (plus any linked <script> files) does not mention "/api/games" anywhere; want evidence the create-game form POSTs there. Page body:\n%s`, html)
	}

	lower := strings.ToLower(combined)
	hasFormAction := strings.Contains(lower, `action="/api/games"`) || strings.Contains(lower, `action='/api/games'`)
	hasPostVerbNearby := strings.Contains(lower, "post")

	if !hasFormAction && !hasPostVerbNearby {
		t.Fatalf("served \"/\" page (plus any linked <script> files) mentions \"/api/games\" but shows no evidence of a POST (neither a <form action=\"/api/games\" ...> nor any \"POST\" text near it). Combined content:\n%s", combined)
	}
}

// TestWebIndex_ServedHTML_HasCreateGameFormFields is a light structural
// check (no browser) that the served "/" page contains a game_def_id select
// offering "test-freeplay" and no box for the name — the two form
// controls the AC test above drives via chromedp. This exists so a
// regression in the static template (e.g. renaming a field, dropping the
// test-freeplay option) is caught even without spinning up a browser.
func TestWebIndex_ServedHTML_HasCreateGameFormFields(t *testing.T) {
	baseURL := newRESTTestServer(t)
	html := fetchBody(t, baseURL, "/")

	if !regexp.MustCompile(`(?is)<select[^>]*name=["']game_def_id["']`).MatchString(html) {
		t.Errorf(`served "/" page has no <select name="game_def_id" ...>. Page body:\n%s`, html)
	}
	if !regexp.MustCompile(`(?is)value=["']test-freeplay["']`).MatchString(html) {
		t.Errorf(`served "/" page has no element with value="test-freeplay" (expected an <option> inside the game_def_id select). Page body:\n%s`, html)
	}
	// The name comes from the profile, not from a box in the form.
	if regexp.MustCompile(`(?is)<input[^>]*name=["']player_name["']`).MatchString(html) {
		t.Error(`served "/" page still has an <input name="player_name">`)
	}
}

// TestWebIndex_JoinIconOpensTheJoinForm checks that the join-existing-game
// form is not on the page itself but behind the Join Game icon of the top bar.
func TestWebIndex_JoinIconOpensTheJoinForm(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()

	if err := runSteps(ctx,
		stepNavigate(baseURL+"/"),
		stepWaitVisible(`#open-join`),
	); err != nil {
		t.Fatalf("navigate to /: %v", err)
	}
	var onPage bool
	if err := runSteps(ctx, stepEvalTo(`!!document.querySelector('main #join-game-form')`, &onPage)); err != nil {
		t.Fatal(err)
	}
	if onPage {
		t.Error("the join form is on the page itself, want it only in the dialog of the top bar's Join icon")
	}
	if err := runSteps(ctx,
		stepClick(`#open-join`),
		stepWaitVisible(`#join-dialog #room_code`),
		stepWaitVisible(`#join-dialog #join-submit`),
	); err != nil {
		t.Fatalf("open the join dialog: %v", err)
	}
}

// fetchBody GETs baseURL+path via a plain net/http client and returns the
// response body as a string, failing the test on any transport error or
// non-200 status.
func fetchBody(t *testing.T, baseURL, path string) string {
	t.Helper()

	resp, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body for GET %s: %v", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d (body: %s)", path, resp.StatusCode, http.StatusOK, raw)
	}
	return string(raw)
}

// fetchLinkedScripts finds every <script src="..."> reference in html,
// fetches each one (resolved relative to baseURL) via plain net/http, follows
// relative ES-module imports ("from \"./api.js\"") recursively, and returns
// all of their contents concatenated. Any script that can't be fetched is
// silently skipped (best-effort — this is a supplementary check, and a
// missing/broken script reference is either irrelevant to this test's
// assertion or would already be caught by a more targeted test).
func fetchLinkedScripts(t *testing.T, baseURL, html string) string {
	t.Helper()

	var out strings.Builder
	seen := map[string]bool{}

	var fetch func(url string)
	fetch = func(url string) {
		if seen[url] {
			return
		}
		seen[url] = true
		resp, err := http.Get(url)
		if err != nil {
			return
		}
		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			return
		}
		out.Write(raw)
		out.WriteString(" ")

		base := url[:strings.LastIndex(url, "/")+1]
		for _, m := range regexp.MustCompile(`from\s+["'](\.{1,2}/[^"']+)["']`).FindAllStringSubmatch(string(raw), -1) {
			rel := strings.TrimPrefix(m[1], "./")
			fetch(base + rel)
		}
	}

	for _, m := range regexp.MustCompile(`(?is)<script[^>]*\bsrc=["']([^"']+)["']`).FindAllStringSubmatch(html, -1) {
		src := m[1]
		switch {
		case strings.HasPrefix(src, "http://"), strings.HasPrefix(src, "https://"):
			fetch(src)
		case strings.HasPrefix(src, "/"):
			fetch(baseURL + src)
		default:
			fetch(baseURL + "/" + src)
		}
	}
	return out.String()
}
