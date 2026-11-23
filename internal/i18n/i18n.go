// Package i18n is the languages of the interface (ADR 0141): which there are,
// which one a request reads in, and the message catalogs of the page's own
// words, one JSON file per language in catalogs/, embedded in the binary.
//
// A catalog maps a key to the words, with {name} where a value goes in. A key
// ending in "_html" holds markup, which only a template may use. A value may be
// an object of plural forms ({"one": …, "other": …}, the names of Intl's plural
// rules), for a key given a count. English is complete; any key another
// language lacks is read in English. Keys starting with "_" say something about
// the catalog itself (whether a speaker has reviewed it) and are not words.
//
// The server's own messages (the API's errors), the brand, handles, game names,
// notation and the API's words stay as they are in every language.
package i18n

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
)

//go:embed catalogs/*.json
var catalogFS embed.FS

// Language is one language of the interface.
type Language struct {
	// Code is the language's tag (BCP 47), as <html lang> and the API give it.
	Code string `json:"code"`
	// Name is the language's name in itself, as the Language setting lists it.
	Name string `json:"name"`
	// Dir is "rtl" for a language written right to left, and "ltr" otherwise.
	Dir string `json:"dir"`
}

// Default is the language of a reader whose own the interface does not have,
// and of every word a catalog lacks.
const Default = "en"

// Pseudo is a made-up language for finding layouts that break with longer
// words: English, accented and lengthened. It is not in the list a player
// chooses from; a developer sets it as the Cookie.
const Pseudo = "en-XA"

// Cookie is the cookie holding a browser's choice of language: a guest's, or
// what an account chose last in this browser.
const Cookie = "bingle_lang"

// languages are the interface's languages, in the order the setting lists them.
var languages = []Language{
	{"en", "English", "ltr"},
	{"es", "Español", "ltr"},
	{"fr", "Français", "ltr"},
	{"de", "Deutsch", "ltr"},
	{"pt-BR", "Português (Brasil)", "ltr"},
	{"zh-Hans", "中文（简体）", "ltr"},
	{"ja", "日本語", "ltr"},
	{"ko", "한국어", "ltr"},
	{"ru", "Русский", "ltr"},
	{"ar", "العربية", "rtl"},
}

// Languages are the interface's languages, in the order the setting lists them
// (Pseudo is not one).
func Languages() []Language { return slices.Clone(languages) }

// Supported says whether code is one of the interface's languages, or Pseudo.
// "" (the browser's own) is not a language.
func Supported(code string) bool {
	_, ok := catalogs[code]
	return ok
}

// Dir is "rtl" for a language written right to left, and "ltr" otherwise.
func Dir(code string) string {
	for _, l := range languages {
		if l.Code == code {
			return l.Dir
		}
	}
	return "ltr"
}

// Match is the first of the interface's languages an Accept-Language header
// asks for, by its weights, or "" when it asks for none of them. A tag matches
// the language with its code, or else the one with its primary subtag: es-MX
// is es, any Portuguese is pt-BR and Chinese is zh-Hans, except Chinese in
// traditional characters (zh-Hant, and as written in Taiwan, Hong Kong and
// Macau), which the interface does not have.
func Match(header string) string {
	type pref struct {
		tag string
		q   float64
	}
	var prefs []pref
	for part := range strings.SplitSeq(header, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		for p := range strings.SplitSeq(params, ";") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					q = f
				}
			}
		}
		if tag = strings.TrimSpace(tag); tag != "" && tag != "*" && q > 0 {
			prefs = append(prefs, pref{tag, q})
		}
	}
	sort.SliceStable(prefs, func(i, j int) bool { return prefs[i].q > prefs[j].q })
	for _, p := range prefs {
		if code := matchTag(p.tag); code != "" {
			return code
		}
	}
	return ""
}

func matchTag(tag string) string {
	lower := strings.ToLower(strings.ReplaceAll(tag, "_", "-"))
	for _, l := range languages {
		if strings.ToLower(l.Code) == lower {
			return l.Code
		}
	}
	subtags := strings.Split(lower, "-")
	switch subtags[0] {
	case "pt":
		return "pt-BR"
	case "zh":
		for _, s := range subtags[1:] {
			if s == "hant" || s == "tw" || s == "hk" || s == "mo" {
				return ""
			}
		}
		return "zh-Hans"
	}
	for _, l := range languages {
		if l.Code == subtags[0] {
			return l.Code
		}
	}
	return ""
}

// ForRequest is the language a request reads in: the one chosen (Choice),
// else the first of its Accept-Language the interface has, else Default.
func ForRequest(r *http.Request, accountChoice string) string {
	if c := Choice(r, accountChoice); c != "" {
		return c
	}
	return Browser(r)
}

// Choice is the language chosen for a request: the account's when it chose
// one, else the browser's (its Cookie), else "" for the browser's own.
func Choice(r *http.Request, accountChoice string) string {
	if Supported(accountChoice) {
		return accountChoice
	}
	if c, err := r.Cookie(Cookie); err == nil && Supported(c.Value) {
		return c.Value
	}
	return ""
}

// Browser is the language the request's Accept-Language asks for, or Default.
func Browser(r *http.Request) string {
	if m := Match(r.Header.Get("Accept-Language")); m != "" {
		return m
	}
	return Default
}

// catalogs are each language's words, English's filled in where it lacks one;
// pages are what a page loads for each, and etags their ETags.
var (
	catalogs = map[string]map[string]any{}
	pages    = map[string][]byte{}
	etags    = map[string]string{}
)

func init() {
	en, err := load(Default)
	if err != nil {
		panic(err)
	}
	for _, l := range languages {
		own := en
		if l.Code != Default {
			if own, err = load(l.Code); err != nil {
				panic(err)
			}
		}
		merged := make(map[string]any, len(en))
		maps.Copy(merged, en)
		maps.Copy(merged, own)
		catalogs[l.Code] = merged
	}
	catalogs[Pseudo] = pseudo(en)
	for code, m := range catalogs {
		page, err := json.Marshal(struct {
			Languages []Language     `json:"languages"`
			Messages  map[string]any `json:"messages"`
		}{languages, m})
		if err != nil {
			panic(err)
		}
		sum := sha256.Sum256(page)
		pages[code], etags[code] = page, `"`+hex.EncodeToString(sum[:8])+`"`
	}
}

// Raw is a language's own catalog as its file has it, without English's
// filled in: for the tests, and the report of what a catalog lacks.
func Raw(code string) (map[string]any, error) { return load(code) }

func load(code string) (map[string]any, error) {
	b, err := catalogFS.ReadFile("catalogs/" + code + ".json")
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("i18n: catalogs/%s.json: %w", code, err)
	}
	for k := range m {
		if strings.HasPrefix(k, "_") {
			delete(m, k)
		}
	}
	return m, nil
}

// Page is what a page loads for a language, and its ETag: the interface's
// languages, and the catalog with English's words filled in where it lacks
// its own. It is nil for no such language.
func Page(code string) (body []byte, etag string) { return pages[code], etags[code] }

// T is the words of key in a language, with params ("name", value, …) put in
// for {name}. A plural key takes the form for "n", when it is given. A key no
// catalog has is given back as itself, so that it shows.
func T(code, key string, params ...any) string {
	return fill(lookup(code, key, params), params, false)
}

// HTML is T for a template: the words of a "_html" key are markup, the values
// put into them escaped unless they are template.HTML (a field the template
// put inside the sentence); any other key's are text, which the template escapes.
func HTML(code, key string, params ...any) template.HTML {
	if !strings.HasSuffix(key, "_html") {
		return template.HTML(html.EscapeString(T(code, key, params...)))
	}
	return template.HTML(fill(lookup(code, key, params), params, true)) //nolint:gosec // the catalogs are the repository's own, and each value is escaped
}

func lookup(code, key string, params []any) string {
	m := catalogs[code]
	if m == nil {
		m = catalogs[Default]
	}
	switch v := m[key].(type) {
	case string:
		return v
	case map[string]any:
		form := "other"
		for i := 0; i+1 < len(params); i += 2 {
			if params[i] == "n" {
				form = pluralForm(code, params[i+1])
			}
		}
		if s, ok := v[form].(string); ok {
			return s
		}
		s, _ := v["other"].(string)
		return s
	}
	return key
}

// pluralForm is the plural form of n in a language. The mail is the server's
// one use of plurals, and its rules are settled with it (ADR 0141); until then
// this is English's, and Russian's and Arabic's catalogs give their counts
// as "other".
func pluralForm(_ string, n any) string {
	if f, ok := toFloat(n); ok && f == 1 {
		return "one"
	}
	return "other"
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func fill(s string, params []any, escape bool) string {
	for i := 0; i+1 < len(params); i += 2 {
		name, _ := params[i].(string)
		val := fmt.Sprint(params[i+1])
		// Markup the template made (a field inside a sentence) goes in as it is.
		if _, markup := params[i+1].(template.HTML); escape && !markup {
			val = html.EscapeString(val)
		}
		s = strings.ReplaceAll(s, "{"+name+"}", val)
	}
	return s
}
