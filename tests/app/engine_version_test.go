package app_test

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/version"
)

// The bingle engine has a version of its own (the games have theirs): it
// starts at 0.3.0, health reports it, and every page ends with it in small type.
func TestEngineVersion_IsSemverAndShownInTheFooterOfEveryPage(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(version.Engine) {
		t.Fatalf("engine version %q is not major.minor.patch", version.Engine)
	}
	baseURL := newRESTTestServer(t)
	var health struct {
		Version string `json:"version"`
	}
	if code := doJSON(t, "GET", baseURL+"/healthz", nil, &health); code != 200 || health.Version != version.Engine {
		t.Errorf("healthz: %d, version %q", code, health.Version)
	}
	room := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	for _, path := range []string{"/", "/profile", "/game/" + room, "/batches"} {
		resp, err := http.Get(baseURL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		want := fmt.Sprintf(`<footer class="site-footer"><sub title="The version of the bingle engine">bingle v%s · MIT License · <a href="https://github.com/noderunner/bingle" target="_blank" rel="noopener noreferrer">GitHub</a></sub></footer>`, version.Engine)
		if !regexp.MustCompile(regexp.QuoteMeta(want)).Match(body) {
			t.Errorf("%s: no footer with the engine version", path)
		}
	}
	// Drawn small, at the bottom of the page.
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/"), chromedp.WaitVisible(".site-footer", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, `document.querySelector(".site-footer").textContent`); got != "bingle v"+version.Engine+" · MIT License · GitHub" {
		t.Errorf("footer = %q", got)
	}
	if !evalBool(t, ctx, `parseFloat(getComputedStyle(document.querySelector(".site-footer")).fontSize) < 13 && document.querySelector(".site-footer").getBoundingClientRect().top >= document.querySelector("main").getBoundingClientRect().bottom - 1`) {
		t.Error("the footer is not small and below the page's content")
	}
}

// Every page with the top bar has a help button: the engine's version at the top,
// a link to the project and a button that goes to its issues.
func TestHelp_TopBarHasTheVersionAndTheProjectLinks(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	for _, path := range []string{"/", "/profile", "/game/" + room} {
		if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+path), chromedp.WaitVisible("#open-help", chromedp.ByQuery)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		clickSel(t, ctx, "#open-help")
		waitJS(t, ctx, path+": the help dialog", `document.getElementById("help-dialog").open`, 5*time.Second)
		if got := evalString(t, ctx, `document.querySelector("#help-dialog h2 + .help-version, #help-dialog .help-version").textContent`); got != "Bingle v"+version.Engine {
			t.Errorf("%s: help shows %q", path, got)
		}
		if !evalBool(t, ctx, `document.querySelector("#help-dialog .help-version").getBoundingClientRect().top < document.getElementById("help-github").getBoundingClientRect().top`) {
			t.Errorf("%s: the version is not at the top of the help", path)
		}
		if got := evalString(t, ctx, `document.getElementById("help-github").href`); got != "https://github.com/noderunner/bingle" {
			t.Errorf("%s: GitHub link %q", path, got)
		}
		if got := evalString(t, ctx, `document.getElementById("help-issues").href + "|" + document.getElementById("help-issues").textContent`); got != "https://github.com/noderunner/bingle/issues|Report Issues" {
			t.Errorf("%s: issues link %q", path, got)
		}
	}
}
