package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/i18n"
)

var (
	// t("key" and tParts("key" in the page's modules, and {{t .Lang "key" in its templates.
	jsKey       = regexp.MustCompile(`\b(?:t|tr|tParts)\(\s*"([^"]+)"`)
	templateKey = regexp.MustCompile(`[{(]t \.Lang "([^"]+)"`)
	// Any string that looks like a key, as a t(…) of a choice between keys has them.
	keyLike = regexp.MustCompile(`"([a-z]+)\.([a-z_]+)"`)
)

// Every key the page and the templates use is in the English catalog, which
// every other falls back to (ADR 0141).
func TestCatalog_HasEveryKeyThePageUses(t *testing.T) {
	en, err := i18n.Raw(i18n.Default)
	if err != nil {
		t.Fatal(err)
	}
	spaces := map[string]bool{}
	for k := range en {
		spaces[strings.SplitN(k, ".", 2)[0]] = true
	}
	used := 0
	check := func(file string, src []byte, re *regexp.Regexp) {
		for _, m := range re.FindAllSubmatch(src, -1) {
			used++
			if _, ok := en[string(m[1])]; !ok {
				t.Errorf("%s uses %q, which en.json lacks", file, m[1])
			}
		}
	}
	err = fs.WalkDir(staticFS, "static/js", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".js") {
			return err
		}
		src, err := staticFS.ReadFile(path)
		if err != nil {
			return err
		}
		check(path, src, jsKey)
		if !strings.Contains(string(src), `from "./i18n.js"`) {
			return nil
		}
		for _, m := range keyLike.FindAllSubmatch(src, -1) {
			if key := string(m[1]) + "." + string(m[2]); spaces[string(m[1])] {
				if _, ok := en[key]; !ok {
					t.Errorf("%s names %q, which en.json lacks", path, key)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		src, err := templateFS.ReadFile("templates/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		check(e.Name(), src, templateKey)
	}
	if used < 50 {
		t.Errorf("only %d uses of a key found: is the pattern still right?", used)
	}
}

func get(t *testing.T, h http.Handler, path string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Every page says its language and direction: the account's choice, else the
// browser's cookie, else Accept-Language, else English.
func TestPages_SayTheirLanguageAndDirection(t *testing.T) {
	account := ""
	h := NewRouter(nil, config.Brand{}, func(*http.Request) string { return account })
	for _, tc := range []struct {
		name, account, cookie, accept, want, dir, chosen string
	}{
		{"nothing asked", "", "", "", "en", "ltr", ""},
		{"the browser's", "", "", "fr-CA,fr;q=0.9", "fr", "ltr", ""},
		{"the cookie's", "", "ar", "fr", "ar", "rtl", "ar"},
		{"the account's", "ja", "ar", "fr", "ja", "ltr", "ja"},
	} {
		account = tc.account
		header := http.Header{}
		if tc.accept != "" {
			header.Set("Accept-Language", tc.accept)
		}
		if tc.cookie != "" {
			header.Set("Cookie", i18n.Cookie+"="+tc.cookie)
		}
		// The main page lists the games, which needs a manager: tests/app sees to it.
		for _, path := range []string{"/game/ABCD", "/profile", "/players/ana", "/tournament/1", "/login", "/admin", "/notify/stop", "/batches"} {
			rec := get(t, h, path, header)
			body := rec.Body.String()
			if rec.Code != 200 || !strings.Contains(body, `<html lang="`+tc.want+`" dir="`+tc.dir+`">`) || !strings.Contains(body, `<meta name="bingle-language" content="`+tc.chosen+`"`) {
				head, _, _ := strings.Cut(body, "<body>")
				t.Errorf("%s, %s: %d\n%s", tc.name, path, rec.Code, head)
			}
		}
	}
	account = ""
	rec := get(t, h, "/profile", http.Header{"Accept-Language": {"de"}})
	if body := rec.Body.String(); !strings.Contains(body, "<h1>Alle Einstellungen</h1>") || !strings.Contains(body, `data-browser="de"`) {
		t.Errorf("the profile page in German:\n%s", body)
	}
}

// The account pages name themselves by a key of the catalog, which their titles show in the page's language.
func TestAccountPages_TitlesAreTranslated(t *testing.T) {
	h := NewRouter(nil, config.Brand{}, nil)
	for _, path := range []string{"/login", "/signup", "/account", "/account/confirm", "/forgot", "/account/reset"} {
		for _, lang := range []string{"en", "fr"} {
			body := get(t, h, path, http.Header{"Accept-Language": {lang}}).Body.String()
			_, title, _ := strings.Cut(body, "<title>")
			title, _, _ = strings.Cut(title, "</title>")
			if strings.Contains(title, "account.") || strings.Contains(title, "common.") {
				t.Errorf("%s in %s: the title is a key: %q", path, lang, title)
			}
			if lang == "fr" && path == "/login" && !strings.Contains(title, "Se connecter") {
				t.Errorf("/login in French: %q", title)
			}
		}
	}
}

func TestCatalog_IsServedWithAnETag(t *testing.T) {
	h := NewRouter(nil, config.Brand{}, nil)
	// The metrics listener's Health page loads js/i18n.js too.
	if rec := get(t, StaticHandler(), "/static/i18n/fr.json", nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"language.label":"Langue"`) {
		t.Errorf("the catalog from StaticHandler: %d", rec.Code)
	}
	if rec := get(t, StaticHandler(), "/static/style.css", nil); rec.Code != 200 {
		t.Errorf("the stylesheet from StaticHandler: %d", rec.Code)
	}
	rec := get(t, h, "/static/i18n/zh-Hans.json", nil)
	etag := rec.Header().Get("ETag")
	if rec.Code != 200 || etag == "" || !strings.Contains(rec.Body.String(), `"language.label":"语言"`) {
		t.Fatalf("GET the Chinese catalog: %d %q %.200s", rec.Code, etag, rec.Body.String())
	}
	if rec := get(t, h, "/static/i18n/zh-Hans.json", http.Header{"If-None-Match": {etag}}); rec.Code != http.StatusNotModified {
		t.Errorf("asked again: %d", rec.Code)
	}
	if rec := get(t, h, "/static/i18n/xx.json", nil); rec.Code != http.StatusNotFound {
		t.Errorf("no such language: %d", rec.Code)
	}
}

var (
	// mirrorless are the rules that may say left or right: what stays left to right
	// in every language (a board, cards, the move list, pictures), what is centred,
	// what fills the window, and a phone's safe area, which is the screen's.
	mirrorless = regexp.MustCompile(`\.pc-|\.layout-|\.zone|\.ct-|\.sol-|\.pile-count|\.medal-|#board|\.move-list|\.victory|\.m-turn-note|\.options-window|\.m-landscape-map|\.m-page\b|\.m-nav\b|\.m-mode \.topbar|\.sr-only|\.tour-picture|\.toast|#toast|\.piece-tip|\.sol-pin-name`)
)

// The stylesheet mirrors for a language written right to left (ADR 0141): its
// rules say inline-start and inline-end, not left and right, outside what stays
// left to right in every language.
func TestStylesheet_SaysStartAndEndNotLeftAndRight(t *testing.T) {
	css, err := staticFS.ReadFile("static/style.css")
	if err != nil {
		t.Fatal(err)
	}
	src := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(css), "")
	for _, rule := range strings.Split(src, "}") {
		selector, body, ok := strings.Cut(rule, "{")
		if !ok {
			continue
		}
		if i := strings.LastIndex(body, "{"); i >= 0 { // a rule inside @media
			selector, body = body[:i], body[i+1:]
		}
		selector = strings.TrimSpace(selector)
		if mirrorless.MatchString(selector) || strings.Contains(body, "safe-area") {
			continue
		}
		for _, decl := range strings.Split(body, ";") {
			name, value, _ := strings.Cut(strings.TrimSpace(decl), ":")
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			switch {
			case name == "left" && value == "50%", name == "left" && value == "0" && strings.Contains(body, "right: 0"):
				// centred, or from edge to edge: the same either way
			case name == "right" && value == "0" && strings.Contains(body, "left: 0"):
			case strings.HasSuffix(name, "-left") || strings.HasSuffix(name, "-right") || name == "left" || name == "right" || name == "float",
				name == "text-align" && (value == "left" || value == "right"):
				t.Errorf("%s { %s: %s }: say inline-start or inline-end", selector, name, value)
			}
		}
	}
}
