package app_test

// The interface's languages (ADR 0141): a guest's choice is kept in the
// browser, an account's on the server, and every page says its language.

import (
	"io"
	"net/http"
	"regexp"
	"testing"
)

// page is the <html> tag of a page, fetched by b with Accept-Language accept.
func (b *browser) page(path, accept string) string {
	b.t.Helper()
	req, _ := http.NewRequest("GET", b.base+path, nil)
	if accept != "" {
		req.Header.Set("Accept-Language", accept)
	}
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return htmlTag.FindString(string(raw))
}

var htmlTag = regexp.MustCompile(`<html[^>]*>`)

func setLanguage(b *browser, language string) map[string]any {
	b.t.Helper()
	var out map[string]any
	if code := b.do("POST", "/auth/language", map[string]any{"language": language}, &out); code != 200 {
		b.t.Fatalf("POST /auth/language %q: %d %v", language, code, out)
	}
	return out
}

// A guest's choice is the browser's: a cookie, which every page and /auth/me
// follow, and which choosing the browser's own language again takes away.
func TestLanguages_AGuestsChoiceIsKeptInTheBrowser(t *testing.T) {
	base := newRESTTestServer(t)
	b := newBrowser(t, base)
	if me := b.me(); me["language"] != "en" || me["language_choice"] != nil {
		t.Fatalf("a newcomer: %v", me)
	}
	if got := b.page("/", "ko-KR,ko;q=0.9"); got != `<html lang="ko" dir="ltr">` {
		t.Errorf("the main page, for a browser asking for Korean: %s", got)
	}
	if out := setLanguage(b, "ar"); out["language"] != "ar" || out["language_choice"] != "ar" || b.cookie("bingle_lang") != "ar" {
		t.Errorf("choosing Arabic: %v, cookie %q", out, b.cookie("bingle_lang"))
	}
	if me := b.me(); me["language"] != "ar" || me["language_choice"] != "ar" {
		t.Errorf("after choosing Arabic: %v", me)
	}
	for _, path := range []string{"/", "/profile", "/login"} {
		if got := b.page(path, "ko"); got != `<html lang="ar" dir="rtl">` {
			t.Errorf("%s after choosing Arabic: %s", path, got)
		}
	}
	if out := setLanguage(b, ""); out["language"] != "en" || out["language_choice"] != nil || b.cookie("bingle_lang") != "" {
		t.Errorf("choosing the browser's own: %v, cookie %q", out, b.cookie("bingle_lang"))
	}
	if got := b.page("/profile", "ko"); got != `<html lang="ko" dir="ltr">` {
		t.Errorf("the browser's own again: %s", got)
	}
	var out map[string]any
	if code := b.do("POST", "/auth/language", map[string]any{"language": "tlh"}, &out); code != http.StatusBadRequest {
		t.Errorf("a language there is not: %d %v", code, out)
	}
}

// An account's choice follows it to every device, and stays in the browser it
// was made in after signing out.
func TestLanguages_AnAccountsChoiceFollowsItEverywhere(t *testing.T) {
	_, base := accountServer(t)
	laptop := newBrowser(t, base)
	if code := laptop.login("ana", anaPassword); code != 200 {
		t.Fatalf("login: %d", code)
	}
	setLanguage(laptop, "ja")
	phone := newBrowser(t, base)
	if code := phone.login("ana", anaPassword); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if me := phone.me(); me["language"] != "ja" || me["language_choice"] != "ja" {
		t.Errorf("the account on another device: %v", me)
	}
	if got := phone.page("/profile", "de"); got != `<html lang="ja" dir="ltr">` {
		t.Errorf("a page on another device: %s", got)
	}
	if code := phone.do("POST", "/auth/logout", nil, nil); code != 200 {
		t.Fatalf("logout: %d", code)
	}
	if got := phone.page("/profile", "de"); got != `<html lang="de" dir="ltr">` {
		t.Errorf("the other device, signed out: %s", got)
	}
	if code := laptop.do("POST", "/auth/logout", nil, nil); code != 200 {
		t.Fatalf("logout: %d", code)
	}
	if got := laptop.page("/profile", "de"); got != `<html lang="ja" dir="ltr">` {
		t.Errorf("the device it was chosen on, signed out: %s", got)
	}
}
