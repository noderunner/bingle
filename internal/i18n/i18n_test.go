package i18n

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestMatch_TakesTheFirstLanguageAskedForThatTheInterfaceHas(t *testing.T) {
	for header, want := range map[string]string{
		"":                               "",
		"en-US,en;q=0.9":                 "en",
		"es-MX,es;q=0.9,en;q=0.8":        "es",
		"pt-PT":                          "pt-BR",
		"pt-br":                          "pt-BR",
		"zh-CN,zh;q=0.9":                 "zh-Hans",
		"zh-Hans-SG":                     "zh-Hans",
		"zh-TW,zh-Hant;q=0.9,ja;q=0.8":   "ja",
		"zh-HK":                          "",
		"nl-NL,de;q=0.7,fr;q=0.8":        "fr",
		"*":                              "",
		"ar-EG":                          "ar",
		"ko_KR":                          "ko",
		"de;q=0, ru":                     "ru",
		"fr;q=0.2,  en;q=0.3":            "en",
		"tlh, ja-JP;q=0.5":               "ja",
		"en-GB;q=0.8;extra=1, sv;q=0.99": "en",
	} {
		if got := Match(header); got != want {
			t.Errorf("Match(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestForRequest_TheAccountsChoiceThenTheCookieThenTheBrowser(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := ForRequest(r, ""); got != Default {
		t.Errorf("nothing asked for: %q", got)
	}
	r.Header.Set("Accept-Language", "de-DE,de;q=0.9")
	if got := ForRequest(r, ""); got != "de" {
		t.Errorf("the browser asks for German: %q", got)
	}
	r.Header.Set("Cookie", Cookie+"=ar")
	if got, choice := ForRequest(r, ""), Choice(r, ""); got != "ar" || choice != "ar" {
		t.Errorf("the browser chose Arabic: %q %q", got, choice)
	}
	if got, choice := ForRequest(r, "ja"), Choice(r, "ja"); got != "ja" || choice != "ja" {
		t.Errorf("the account chose Japanese: %q %q", got, choice)
	}
	r.Header.Set("Cookie", Cookie+"=xx")
	if got, choice := ForRequest(r, "nope"), Choice(r, "nope"); got != "de" || choice != "" {
		t.Errorf("no language chosen that there is: %q %q", got, choice)
	}
}

func TestLanguages_EachHasACatalogAndOnlyArabicIsRightToLeft(t *testing.T) {
	codes := map[string]bool{}
	for _, l := range Languages() {
		if codes[l.Code] {
			t.Errorf("%s twice", l.Code)
		}
		codes[l.Code] = true
		if body, _ := Page(l.Code); !Supported(l.Code) || body == nil {
			t.Errorf("%s has no catalog", l.Code)
		}
		if want := map[bool]string{true: "rtl", false: "ltr"}[l.Code == "ar"]; Dir(l.Code) != want || l.Dir != want {
			t.Errorf("%s is written %s", l.Code, l.Dir)
		}
	}
	if len(codes) != 10 || Languages()[0].Code != Default {
		t.Errorf("the languages: %v", Languages())
	}
	entries, err := catalogFS.ReadDir("catalogs")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if code := strings.TrimSuffix(e.Name(), ".json"); !codes[code] {
			t.Errorf("catalogs/%s is no language's", e.Name())
		}
	}
	if !Supported(Pseudo) || slices.ContainsFunc(Languages(), func(l Language) bool { return l.Code == Pseudo }) {
		t.Error("the pseudo-locale can be set, but is not listed")
	}
	if Supported("") {
		t.Error(`"" is the browser's own, not a language`)
	}
}

var (
	placeholder = regexp.MustCompile(`\{[a-z_]+\}`)
	tag         = regexp.MustCompile(`</?[a-z]+`)
	pluralForms = []string{"zero", "one", "two", "few", "many", "other"}
)

// found is what of re a value has, sorted, its plural forms' together.
func found(re *regexp.Regexp, v any) []string {
	var out []string
	switch v := v.(type) {
	case string:
		out = re.FindAllString(v, -1)
	case map[string]any:
		seen := map[string]bool{}
		for _, s := range v {
			if s, ok := s.(string); ok {
				for _, m := range re.FindAllString(s, -1) {
					seen[m] = true
				}
			}
		}
		for m := range seen {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// Every catalog says the same things as English's: no key English lacks, the
// same values put in, the same markup, and plural forms where English has them.
func TestCatalogs_MatchEnglish(t *testing.T) {
	en, err := Raw(Default)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range en {
		switch v := v.(type) {
		case string:
		case map[string]any:
			if _, ok := v["other"].(string); !ok {
				t.Errorf("en %s: a plural without an \"other\" form", k)
			}
		default:
			t.Errorf("en %s: neither words nor plural forms: %v", k, v)
		}
	}
	for _, l := range Languages() {
		own, err := Raw(l.Code)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range own {
			want, ok := en[k]
			if !ok {
				t.Errorf("%s %s: English has no such key", l.Code, k)
				continue
			}
			if _, plural := want.(map[string]any); plural {
				forms, ok := v.(map[string]any)
				if !ok || forms["other"] == nil {
					t.Errorf("%s %s: English's is a plural, this is not, or has no \"other\" form", l.Code, k)
				}
				for f := range forms {
					if !slices.Contains(pluralForms, f) {
						t.Errorf("%s %s: no plural form is called %q", l.Code, k, f)
					}
				}
			} else if _, ok := v.(string); !ok {
				t.Errorf("%s %s: English's are words, this is not", l.Code, k)
			}
			if got, want := found(placeholder, v), found(placeholder, want); !slices.Equal(got, want) {
				t.Errorf("%s %s puts in %v, English %v", l.Code, k, got, want)
			}
			if strings.HasSuffix(k, "_html") {
				if got, want := found(tag, v), found(tag, want); !slices.Equal(got, want) {
					t.Errorf("%s %s has the markup %v, English %v", l.Code, k, got, want)
				}
			} else if tag.MatchString(asJSON(v)) {
				t.Errorf("%s %s has markup, but is not an _html key", l.Code, k)
			}
		}
	}
}

func asJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// TestCatalogs_Report is the report of what each catalog lacks: run it with -v
// to see the keys still to translate. It fails on nothing.
func TestCatalogs_Report(t *testing.T) {
	en, _ := Raw(Default)
	for _, l := range Languages()[1:] {
		own, _ := Raw(l.Code)
		var missing []string
		for k := range en {
			if _, ok := own[k]; !ok {
				missing = append(missing, k)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Logf("%s lacks %d of %d: %s", l.Code, len(missing), len(en), strings.Join(missing, ", "))
		}
	}
}

func TestT_PutsValuesInAndFallsBackToEnglish(t *testing.T) {
	if got := T("fr", "profile.write_to", "name", "Ana"); got != "Écrire à Ana" {
		t.Errorf("French: %q", got)
	}
	if got := T("nope", "profile.write_to", "name", "Ana"); got != "Write to Ana" {
		t.Errorf("no such language: %q", got)
	}
	if got := T("de", "no.such.key"); got != "no.such.key" {
		t.Errorf("no such key: %q", got)
	}
	catalogs["de"]["test.only_english"] = "English"
	defer delete(catalogs["de"], "test.only_english")
	if got := T("de", "test.only_english"); got != "English" {
		t.Errorf("a key German lacks: %q", got)
	}
}

func TestHTML_EscapesTextAndTheValuesPutIntoMarkup(t *testing.T) {
	if got := string(HTML("en", "settings.email_note", "brand", "<b>")); !strings.Contains(got, "&lt;b&gt; mails a link") {
		t.Errorf("text: %s", got)
	}
	got := string(HTML("en", "settings.accounts_only_html", "brand", "A&B"))
	if !strings.Contains(got, `<a href="/login?return=%2Fprofile">sign in</a>`) || !strings.Contains(got, "A&amp;B mail you") {
		t.Errorf("markup: %s", got)
	}
}

func TestPseudo_IsLongerAccentedAndKeepsWhatIsPutIn(t *testing.T) {
	got := T(Pseudo, "profile.write_to", "name", "Ana")
	if !strings.HasPrefix(got, "[Ŵŕíţé ţö Ana ") || !strings.HasSuffix(got, "]") || len([]rune(got)) < len("Write to Ana")*13/10 {
		t.Errorf("pseudo: %q", got)
	}
	html := T(Pseudo, "settings.accounts_only_html")
	if !strings.Contains(html, `<a href="/login?return=%2Fprofile">`) || !strings.Contains(html, "{brand}") {
		t.Errorf("pseudo markup: %q", html)
	}
}

func TestPage_IsTheLanguagesAndTheCatalogWithEnglishFilledIn(t *testing.T) {
	body, etag := Page("ar")
	var page struct {
		Languages []Language     `json:"languages"`
		Messages  map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	en, _ := Raw(Default)
	if len(page.Languages) != 10 || len(page.Messages) != len(en) || page.Messages["language.label"] != "اللغة" || etag == "" {
		t.Errorf("Arabic's page: %d languages, %d of %d messages, %v, %q", len(page.Languages), len(page.Messages), len(en), page.Messages["language.label"], etag)
	}
	if _, other := Page("de"); other == etag {
		t.Error("two languages' pages have the same ETag")
	}
	if body, _ := Page("xx"); body != nil {
		t.Error("a page for no language")
	}
}
