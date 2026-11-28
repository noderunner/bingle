package app_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
)

const testDiscord = "https://discord.gg/nightclub"

// serverWithDiscord is a server with a link to its Discord, under the default name or another.
func serverWithDiscord(t *testing.T, name string) string {
	t.Helper()
	brand, err := config.LoadBrand(name, "")
	if err != nil {
		t.Fatal(err)
	}
	if brand.DiscordURL, err = config.ParseDiscordURL(testDiscord); err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig(t)
	cfg.Brand = brand
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

func TestDiscord_WhatTheSettingAccepts(t *testing.T) {
	for in, ok := range map[string]bool{
		"":                                 true,
		"https://discord.gg/abc":           true,
		"https://discord.com/invite/abc":   true,
		"https://www.discord.com/invite/a": true,
		"http://discord.gg/abc":            false,
		"https://example.com/discord.gg":   false,
		"https://discord.gg.evil.com/abc":  false,
		"https://evildiscord.com/abc":      false,
		"https://user@discord.gg/abc":      false,
		"https://discord.gg:8443/abc":      false,
		"javascript:alert(1)":              false,
		"discord.gg/abc":                   false,
	} {
		if _, err := config.ParseDiscordURL(in); (err == nil) != ok {
			t.Errorf("%q: err = %v, want ok %v", in, err, ok)
		}
	}
}

// The footer, and the page's meta tag, with the setting and without it, under any name.
func TestDiscord_FooterAndMeta(t *testing.T) {
	link := `<a href="` + testDiscord + `" target="_blank" rel="noopener noreferrer">Discord</a></sub></footer>`
	paths := []string{"/", "/profile", "/account", "/admin", "/batches", "/notify/stop?t=x", "/tournament/1", "/game/ABC123"}
	for _, name := range []string{"", "Night Club"} {
		base := serverWithDiscord(t, name)
		for _, p := range paths {
			if _, body := fetch(t, base+p); !strings.Contains(body, "GitHub</a> · "+link) || !strings.Contains(body, `data-discord="`+testDiscord+`"`) {
				t.Errorf("%q %s: no Discord link in the footer and the meta tag", name, p)
			}
		}
	}
	base := newRESTTestServer(t)
	for _, p := range paths {
		if _, body := fetch(t, base+p); strings.Contains(strings.ToLower(body), "discord") {
			t.Errorf("%s: says Discord without a setting", p)
		}
	}
}

// Every page with a footer has the link: the quick setup page is not served without builds, so the
// templates are read.
func TestDiscord_EveryFooterHasIt(t *testing.T) {
	files, _ := filepath.Glob("../../internal/web/templates/*.html.tmpl")
	n := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "site-footer") {
			n++
			if !strings.Contains(string(b), `{{with discordURL}} · <a href="{{.}}"`) {
				t.Errorf("%s: no Discord link in the footer", f)
			}
		}
	}
	if n == 0 {
		t.Error("no template has a footer")
	}
}

// In a browser: the button is after GitHub and before the report button, under Bingle's name and another.
func TestWebDiscord_HelpButton(t *testing.T) {
	for _, name := range []string{"", "Night Club"} {
		base := serverWithDiscord(t, name)
		ctx, cancel := newChromedpContext(t, 60*time.Second)
		if err := chromedp.Run(ctx, chromedp.Navigate(base+"/"), chromedp.WaitVisible("#open-help", chromedp.ByQuery)); err != nil {
			cancel()
			t.Fatal(err)
		}
		clickSel(t, ctx, "#open-help")
		waitJS(t, ctx, "the help dialog", `document.getElementById("help-dialog").open`, 5*time.Second)
		if got := evalString(t, ctx, `(() => { const a = document.getElementById("help-discord"); return a.href + "|" + a.textContent + "|" + a.target + "|" + a.rel + "|" + a.previousElementSibling.id })()`); got != testDiscord+"|Discord|_blank|noopener noreferrer|help-github" {
			t.Errorf("%q: the button is %q", name, got)
		}
		if name == "" && !evalBool(t, ctx, `document.getElementById("help-discord").nextElementSibling.id === "help-issues"`) {
			t.Error("the button is not before the report button")
		}
		cancel()
	}
	// Without the setting there is no button.
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(newRESTTestServer(t)+"/"), chromedp.WaitVisible("#open-help", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#open-help")
	waitJS(t, ctx, "the help dialog", `document.getElementById("help-dialog").open`, 5*time.Second)
	if evalBool(t, ctx, `document.getElementById("help-discord") !== null`) {
		t.Error("a Discord button without a setting")
	}
}
