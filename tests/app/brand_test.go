package app_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

const testIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" fill="#a33"/></svg>`

// A server under another name, with its own icon.
func brandedServer(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "icon.svg")
	if err := os.WriteFile(path, []byte(testIcon), 0o600); err != nil {
		t.Fatal(err)
	}
	brand, err := config.LoadBrand("Night Club", path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig(t)
	cfg.Brand = brand
	cfg.Changelog = "## 1.0.0 - 2026-11-05\n\n### Bingle Core 1.0.0\n- Bingle is better.\n"
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	registerFixtures(t, a)
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return srv.URL
}

func fetch(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

// What an operator can get wrong stops the start.
func TestBrand_WhatCannotBeUsed(t *testing.T) {
	dir := t.TempDir()
	notPNG := filepath.Join(dir, "icon.png")
	good := filepath.Join(dir, "icon.svg")
	huge := filepath.Join(dir, "huge.svg")
	exe := filepath.Join(dir, "icon.exe")
	notSVG := filepath.Join(dir, "plain.svg")
	_ = os.WriteFile(notPNG, []byte("not a picture"), 0o600)
	_ = os.WriteFile(good, []byte(testIcon), 0o600)
	_ = os.WriteFile(huge, []byte("<svg>"+strings.Repeat(" ", 600*1024)), 0o600)
	_ = os.WriteFile(exe, []byte(testIcon), 0o600)
	_ = os.WriteFile(notSVG, []byte("hello"), 0o600)
	for _, c := range []struct {
		name, brand, icon string
		ok                bool
	}{
		{"nothing set", "", "", true},
		{"a name", "Night Club", "", true},
		{"a name with an apostrophe", "Ana's Games", "", true},
		{"letters of any alphabet", "Ночной клуб", "", true},
		{"an icon", "", good, true},
		{"markup in the name", "<b>Night</b>", "", false},
		{"a line break", "Night\r\nBcc: x@example.com", "", false},
		{"too long", strings.Repeat("a", 41), "", false},
		{"no such file", "", filepath.Join(dir, "none.svg"), false},
		{"a kind of file not allowed", "", exe, false},
		{"not what it says it is", "", notPNG, false},
		{"not an svg", "", notSVG, false},
		{"too big", "", huge, false},
	} {
		if _, err := config.LoadBrand(c.brand, c.icon); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if got := (config.Brand{}).DisplayName(); got != "Bingle" {
		t.Errorf("the default is %q", got)
	}
}

// Every page and what the server sends carry the name; the icon is served, and inert.
func TestBrand_PagesAndWhatTheServerSends(t *testing.T) {
	base := brandedServer(t)
	for _, path := range []string{"/", "/profile", "/login", "/signup", "/forgot", "/account", "/admin", "/batches", "/notify/stop?t=x", "/notify/confirm?t=x", "/tournament/1", "/game/ABC123"} {
		resp, body := fetch(t, base+path)
		if resp.StatusCode != 200 {
			t.Errorf("%s: %d", path, resp.StatusCode)
			continue
		}
		if strings.Contains(body, "Bingle") {
			at := strings.Index(body, "Bingle")
			t.Errorf("%s still says Bingle: …%s…", path, body[max(0, at-60):min(len(body), at+40)])
		}
		for _, want := range []string{"Night Club", `href="/brand/icon"`, `class="logo custom"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %s", path, want)
			}
		}
	}
	resp, body := fetch(t, base+"/brand/icon")
	if resp.StatusCode != 200 || body != testIcon || resp.Header.Get("Content-Type") != "image/svg+xml" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("the icon: %d %q %v", resp.StatusCode, body, resp.Header)
	}
	if _, log := fetch(t, base+"/api/changelog"); strings.Contains(log, "Bingle") || !strings.Contains(log, "Night Club Core") {
		t.Errorf("the changelog: %s", log)
	}
	if _, guide := fetch(t, base+"/api/game-defs/cthulhu-wars/guide"); strings.Contains(guide, "Bingle") || !strings.Contains(guide, "Night Club") {
		t.Errorf("the rules still say Bingle, or have no name at all")
	}
}

// Without any of it, a server is Bingle, as ever, with the default icon and no icon route.
func TestBrand_TheDefault(t *testing.T) {
	base := newRESTTestServer(t)
	_, body := fetch(t, base+"/")
	if !strings.Contains(body, "<title>Bingle</title>") || !strings.Contains(body, `href="/static/favicon.svg"`) || strings.Contains(body, "logo custom") {
		t.Errorf("the default page: %s", body[:min(len(body), 700)])
	}
}

// Mail goes out under the name: subject, sender, words and the calendar file.
func TestBrand_Mail(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{}, func(c *config.Config) { c.Brand = config.Brand{Name: "Night Club"} })
	ana := newBrowser(t, e.URL)
	before := len(e.SMTP.Mails())
	signUp(t, e, ana, "ana", "ana@example.com")
	got := e.waitMails(t, before+1)[before].Data
	if strings.Contains(got, "Bingle") || !strings.Contains(got, "Night Club") {
		t.Errorf("the mail: %s", got)
	}
}

// In a browser: the bar, the title, the help window and the tab's turn dot on its own icon.
func TestWebBrand_ThePage(t *testing.T) {
	base := brandedServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 900), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the bar", `document.getElementById("open-help") !== null || document.querySelector(".topbar .brand") !== null`, 20*time.Second)
	if got := evalString(t, ctx, `document.querySelector(".topbar .brand").textContent.trim()`); got != "Night Club" {
		t.Errorf("the bar says %q", got)
	}
	if got := evalString(t, ctx, `document.title`); !strings.Contains(got, "Night Club") {
		t.Errorf("the title says %q", got)
	}
	waitJS(t, ctx, "the help text", `document.getElementById("help-version") !== null`, 10*time.Second)
	if got := evalString(t, ctx, `document.getElementById("help-version").textContent`); !strings.HasPrefix(got, "Night Club") {
		t.Errorf("the help window says %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("help-github").textContent`); got != "Source on GitHub" {
		t.Errorf("the source link says %q", got)
	}
	if evalBool(t, ctx, `document.getElementById("help-issues") !== null`) {
		t.Error("another instance sends its players to this project's issues")
	}
	if !evalBool(t, ctx, `getComputedStyle(document.querySelector(".topbar .brand .logo")).backgroundImage.includes("/brand/icon")`) {
		t.Error("the bar has the default icon")
	}
	if got := evalString(t, ctx, `document.body.innerText`); strings.Contains(got, "Bingle") {
		at := strings.Index(got, "Bingle")
		t.Errorf("the page says Bingle: %q", got[max(0, at-50):min(len(got), at+30)])
	}
}
