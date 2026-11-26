package app_test

// The bingle-3d client for download, and its setup file (ADR 0151).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/transport"
)

const (
	windowsBuild = "bingle-3d-windows-x86_64.zip"
	linuxBuild   = "bingle-3d-linux-x86_64.tar.gz"
)

// clientBuildsDir is a directory of builds as an operator keeps one: two
// versions, the newer without a Linux build.
func clientBuildsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"0.0.2/" + windowsBuild: "windows 0.0.2", "0.0.2/" + linuxBuild: "linux 0.0.2",
		"0.0.3/" + windowsBuild: "windows 0.0.3, a longer archive",
	} {
		at := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(at, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// clientApp is an app that offers the client, with the account ana.
func clientApp(t *testing.T, opts ...app.Option) *app.App {
	t.Helper()
	cfg := newTestConfig(t)
	cfg.ClientBuilds = clientBuildsDir(t)
	a, err := app.New(cfg, opts...)
	if err != nil {
		t.Fatal(err)
	}
	registerFixtures(t, a)
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// clientServer serves clientApp, and returns a browser signed in as ana.
func clientServer(t *testing.T, opts ...app.Option) (*app.App, string, *browser) {
	t.Helper()
	a := clientApp(t, opts...)
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	given, err := a.Accounts().Create(context.Background(), "ana", "ana@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	b := newBrowser(t, srv.URL)
	if code := b.login("ana", given); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if code := b.do("POST", "/auth/password", map[string]any{"old": given, "new": anaPassword}, nil); code != 200 {
		t.Fatalf("password: %d", code)
	}
	return a, srv.URL, b
}

var setupLine = regexp.MustCompile(`^([a-z0-9_]+) = (?:"([^"\\]*)"|(\d+))$`)

// setupOf reads a setup file: its keys and their values.
func setupOf(t *testing.T, body string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		m := setupLine.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("a setup file's line is not a key and a value: %q", line)
		}
		out[m[1]] = m[2] + m[3]
	}
	return out
}

// askSetup asks for a setup file with c, as the page does when page is set.
func askSetup(t *testing.T, c *http.Client, base string, page bool, bearer string) (*http.Response, map[string]string) {
	t.Helper()
	req, _ := http.NewRequest("POST", base+"/api/client/setup", nil)
	if page {
		req.Header.Set(auth.PageHeader, "1")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return resp, nil
	}
	return resp, setupOf(t, string(raw))
}

// A server that was not given a directory of builds is as it always was.
func TestClientDownloads_WithoutADirectoryNothingChanges(t *testing.T) {
	base := newRESTTestServer(t)
	for _, c := range []struct{ method, path string }{{"GET", "/api/client"}, {"GET", "/api/client/files/" + windowsBuild}, {"POST", "/api/client/setup"}} {
		req, _ := http.NewRequest(c.method, base+c.path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404", c.method, c.path, resp.StatusCode)
		}
	}
	for _, path := range []string{"/", "/game/ABC123", "/login"} {
		_, body := fetch(t, base+path)
		if !strings.Contains(body, `<a class="brand" href="/"><span class="logo"></span>Bingle</a>`) || strings.Contains(body, "brand-menu") {
			t.Errorf("%s: the logo is not the plain link home", path)
		}
	}
}

func TestClientDownloads_TheNewestBuildOfEachPlatformIsListedAndServed(t *testing.T) {
	_, base, _ := clientServer(t)
	var list struct {
		Builds []struct {
			Platform, Version, File, SHA256 string
			Size                            int64
		}
	}
	resp, body := fetch(t, base+"/api/client")
	if err := json.Unmarshal([]byte(body), &list); err != nil || resp.StatusCode != 200 || len(list.Builds) != 2 {
		t.Fatalf("GET /api/client: %d %s", resp.StatusCode, body)
	}
	linux, windows := list.Builds[0], list.Builds[1]
	if linux.Platform != "linux-x86_64" || linux.Version != "0.0.2" || linux.File != linuxBuild {
		t.Errorf("the Linux build: %+v", linux)
	}
	sum := sha256.Sum256([]byte("windows 0.0.3, a longer archive"))
	if windows.Platform != "windows-x86_64" || windows.Version != "0.0.3" || windows.File != windowsBuild || windows.Size != 31 || windows.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("the Windows build: %+v", windows)
	}

	resp, body = fetch(t, base+"/api/client/files/"+windowsBuild)
	h := resp.Header
	if resp.StatusCode != 200 || body != "windows 0.0.3, a longer archive" || h.Get("Content-Type") != "application/zip" || h.Get("ETag") != `"`+windows.SHA256+`"` ||
		!strings.Contains(h.Get("Content-Disposition"), "attachment") || !strings.Contains(h.Get("Content-Disposition"), windowsBuild) ||
		h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("the Windows archive: %d %q %v", resp.StatusCode, body, h)
	}
	if resp, _ := fetch(t, base+"/api/client/files/"+linuxBuild); resp.Header.Get("Content-Type") != "application/gzip" {
		t.Errorf("the Linux archive is sent as %s", resp.Header.Get("Content-Type"))
	}
	get := func(header, value string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", base+"/api/client/files/"+windowsBuild, nil)
		req.Header.Set(header, value)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		raw, _ := io.ReadAll(resp.Body)
		return resp, string(raw)
	}
	if resp, body := get("If-None-Match", `"`+windows.SHA256+`"`); resp.StatusCode != http.StatusNotModified || body != "" {
		t.Errorf("the archive it has already: %d %q", resp.StatusCode, body)
	}
	if resp, body := get("Range", "bytes=8-12"); resp.StatusCode != http.StatusPartialContent || body != "0.0.3" {
		t.Errorf("a range of the archive: %d %q", resp.StatusCode, body)
	}
	for _, name := range []string{"bingle-3d-steam-frame.tar.gz", "release.json", "..%2F0.0.2%2F" + linuxBuild} {
		if resp, _ := fetch(t, base+"/api/client/files/"+name); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET /api/client/files/%s: %d, want 404", name, resp.StatusCode)
		}
	}
}

func TestClientDownloads_EveryPageHasTheMenuOnItsLogo(t *testing.T) {
	_, base, _ := clientServer(t)
	for _, path := range []string{"/", "/profile", "/login", "/signup", "/forgot", "/account", "/admin", "/batches", "/notify/stop?t=x", "/tournament/1", "/game/ABC123", "/players/ana"} {
		resp, body := fetch(t, base+path)
		if resp.StatusCode != 200 {
			t.Errorf("%s: %d", path, resp.StatusCode)
			continue
		}
		for _, want := range []string{
			`id="brand-menu-button" aria-haspopup="menu" aria-expanded="false"`,
			`<a class="brand-name" href="/">Bingle</a>`,
			`<a role="menuitem" tabindex="-1" id="brand-menu-home" href="/">Home</a>`,
			`>3D client</span>`,
			`href="/api/client/files/` + linuxBuild + `" download title="0.0.2">Linux</a>`,
			`href="/api/client/files/` + windowsBuild + `" download title="0.0.3">Windows</a>`,
			`id="brand-menu-setup">Setup file</button>`,
			`src="/static/js/brand-menu.js"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %s", path, want)
			}
		}
		if strings.Contains(body, `<a class="brand" href="/">`) {
			t.Errorf("%s: the logo is still a link", path)
		}
	}
	// In another language the menu's words are that language's, and the platforms keep their names.
	req, _ := http.NewRequest("GET", base+"/", nil)
	req.Header.Set("Accept-Language", "de")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	for _, want := range []string{`>Startseite</a>`, `>3D-Client</span>`, `>Einrichtungsdatei</button>`, `>Windows</a>`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the German page lacks %s", want)
		}
	}
}

func TestClientSetup_SaysTheServerAndWhoAsked(t *testing.T) {
	_, base, ana := clientServer(t)

	// Nobody: the server's address and nothing else. It is never kept by a cache.
	resp, file := askSetup(t, http.DefaultClient, base, false, "")
	if resp.StatusCode != 200 || len(file) != 2 || file["format"] != "1" || file["server"] != base {
		t.Fatalf("nobody's setup file: %d %v", resp.StatusCode, file)
	}
	h := resp.Header
	if h.Get("Content-Type") != "application/toml" || h.Get("Cache-Control") != "no-store" || !strings.Contains(h.Get("Content-Disposition"), `filename=bingle-3d-setup.toml`) {
		t.Errorf("the file's headers: %v", h)
	}

	// A script's guest: its key.
	var made struct{ Key string }
	if code := doJSON(t, "POST", base+"/api/guest", nil, &made); code != http.StatusCreated || made.Key == "" {
		t.Fatalf("a guest: %d", code)
	}
	if _, file := askSetup(t, http.DefaultClient, base, false, made.Key); len(file) != 3 || file["guest_key"] != made.Key {
		t.Errorf("a guest's setup file: %v", file)
	}

	// A browser's guest: the key its cookie holds, so the client sits in the browser's seats.
	guest := newBrowser(t, base)
	if code := guest.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Gil"}, nil); code != 200 {
		t.Fatalf("a guest's game: %d", code)
	}
	key := guest.cookie(auth.CookieName)
	if _, file := askSetup(t, guest.c, base, true, ""); key == "" || len(file) != 3 || file["guest_key"] != key {
		t.Errorf("a browser guest's setup file: %v, its cookie %q", file, key)
	}
	// The cookie alone does not get it: a page on another site cannot ask.
	if resp, _ := askSetup(t, guest.c, base, false, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a setup file asked for from another site: %d, want 403", resp.StatusCode)
	}

	// A browser that is nobody yet becomes a guest by asking, and the file has the key its cookie now holds.
	fresh := newBrowser(t, base)
	_, file = askSetup(t, fresh.c, base, true, "")
	if key := fresh.cookie(auth.CookieName); key == "" || file["guest_key"] != key {
		t.Errorf("a new browser's setup file: %v, its cookie %q", file, key)
	}

	// An account: its name and a sign-in code, and no session.
	_, file = askSetup(t, ana.c, base, true, "")
	if len(file) != 4 || file["name"] != "ana" || !strings.HasPrefix(file["sign_in_code"], auth.SignInCodePrefix) || file["guest_key"] != "" {
		t.Errorf("an account's setup file: %v", file)
	}
	for _, v := range file {
		if strings.Contains(v, auth.SessionTokenPrefix) || strings.Contains(v, ana.cookie(auth.SessionCookie)) {
			t.Errorf("the file carries a session: %v", file)
		}
	}
}

// spend signs a native client in with a setup file's code.
func spend(t *testing.T, base, code string) (int, sessionToken) {
	t.Helper()
	var out sessionToken
	status, _ := native(t, base, "", "POST", "/auth/login", map[string]any{"code": code}, &out)
	return status, out
}

func TestClientSetup_TheCodeSignsAClientInOnce(t *testing.T) {
	_, base, ana := clientServer(t)
	_, file := askSetup(t, ana.c, base, true, "")
	code := file["sign_in_code"]

	status, out := spend(t, base, code)
	if status != 200 || !strings.HasPrefix(out.Token, auth.SessionTokenPrefix) || out.Me["handle"] != "ana" {
		t.Fatalf("spending the code: %d %+v", status, out)
	}
	var me map[string]any
	if c, _ := native(t, base, out.Token, "GET", "/auth/me", nil, &me); c != 200 || me["handle"] != "ana" {
		t.Errorf("the client's session: %d %v", c, me)
	}
	// The session is the client's own: listed under its name, beside the browser's.
	var list struct {
		Sessions []map[string]any `json:"sessions"`
	}
	ana.do("GET", "/auth/sessions", nil, &list)
	names := map[any]bool{}
	for _, s := range list.Sessions {
		names[s["agent"]] = true
	}
	if len(list.Sessions) != 2 || !names[nativeAgent] {
		t.Errorf("the sessions: %v", list.Sessions)
	}
	// Signing the client out leaves the browser signed in.
	native(t, base, out.Token, "POST", "/auth/logout", nil, nil)
	if ana.me()["handle"] != "ana" {
		t.Error("the browser was signed out with the client")
	}

	// Once.
	if status, out := spend(t, base, code); status != http.StatusUnauthorized || out.Token != "" || !strings.Contains(out.Error, "sign-in code") {
		t.Errorf("spending the code again: %d %+v", status, out)
	}
	for _, bad := range []string{auth.SignInCodePrefix + "nothing", "nothing", ana.cookie(auth.SessionCookie)} {
		if status, out := spend(t, base, bad); status != http.StatusUnauthorized || out.Token != "" {
			t.Errorf("a code that is not one (%q): %d", bad, status)
		}
	}
}

func TestClientSetup_ACodeEndsAfterADayAndWithTheAccountsSessions(t *testing.T) {
	a, base, ana := clientServer(t)
	code := func() string {
		_, file := askSetup(t, ana.c, base, true, "")
		if file["sign_in_code"] == "" {
			t.Fatalf("no code: %v", file)
		}
		return file["sign_in_code"]
	}

	// A day later.
	old := code()
	a.Accounts().SetClock(func() time.Time { return time.Now().Add(24*time.Hour + time.Minute) })
	if status, _ := spend(t, base, old); status != http.StatusUnauthorized {
		t.Errorf("a code a day old: %d, want 401", status)
	}
	a.Accounts().SetClock(time.Now)

	// Signing out everywhere else.
	elsewhere := code()
	if c := ana.do("POST", "/auth/sessions/end-others", nil, nil); c != 200 {
		t.Fatalf("ending the other sessions: %d", c)
	}
	if status, _ := spend(t, base, elsewhere); status != http.StatusUnauthorized {
		t.Errorf("a code after the other sessions were ended: %d, want 401", status)
	}

	// A new password.
	before := code()
	if c := ana.do("POST", "/auth/password", map[string]any{"old": anaPassword, "new": "another password of ana's"}, nil); c != 200 {
		t.Fatalf("a new password: %d", c)
	}
	if status, _ := spend(t, base, before); status != http.StatusUnauthorized {
		t.Errorf("a code after a new password: %d, want 401", status)
	}
	// One made since works.
	if status, out := spend(t, base, code()); status != 200 || out.Me["handle"] != "ana" {
		t.Errorf("a code made after the new password: %d", status)
	}
}

func TestClientSetup_IsBehindASignInsGates(t *testing.T) {
	// Behind a proxy no request is this machine's, so the administrator needs a second factor.
	a, base, ana := clientServer(t, app.WithTransport(transport.Policy{TrustProxy: true}))

	// A password that must be changed.
	given, err := a.Accounts().Create(context.Background(), "bea", "bea@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	bea := newBrowser(t, base)
	if c := bea.login("bea", given); c != 200 {
		t.Fatalf("login: %d", c)
	}
	if resp, _ := askSetup(t, bea.c, base, true, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a setup file before the password is changed: %d, want 403", resp.StatusCode)
	}

	// An administrator with no second factor yet.
	boss, err := a.Accounts().Create(context.Background(), "boss", "boss@example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	admin := newBrowser(t, base)
	admin.login("boss", boss)
	if c := admin.do("POST", "/auth/password", map[string]any{"old": boss, "new": "the boss's own password"}, nil); c != 200 {
		t.Fatalf("the administrator's password: %d", c)
	}
	if resp, _ := askSetup(t, admin.c, base, true, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("an administrator's setup file without a second factor: %d, want 403", resp.StatusCode)
	}

	// An API token acts within its scopes; the file would be the whole account.
	var made struct{ Token string }
	if c := ana.do("POST", "/auth/tokens", map[string]any{"name": "a script", "scopes": []string{"play"}}, &made); c != http.StatusCreated {
		t.Fatalf("a token: %d", c)
	}
	if resp, _ := askSetup(t, http.DefaultClient, base, false, made.Token); resp.StatusCode != http.StatusForbidden {
		t.Errorf("an API token's setup file: %d, want 403", resp.StatusCode)
	}

	// The proxy says how the browser reached it, and the file says the same.
	req, _ := http.NewRequest("POST", base+"/api/client/setup", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Host = "games.example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if file := setupOf(t, string(raw)); file["server"] != "https://games.example.com" || file["certificate_sha256"] != "" {
		t.Errorf("behind a proxy: %v", file)
	}

	// Each account is given only so many an hour.
	limited := 0
	for range 12 {
		if resp, _ := askSetup(t, ana.c, base, true, ""); resp.StatusCode == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited != 2 {
		t.Errorf("of 12 setup files in an hour, %d were refused, want 2", limited)
	}
}

func TestClientSetup_CarriesTheFingerprintOfTheServersOwnCertificate(t *testing.T) {
	const fingerprint = "AB:CD:EF:01"
	a := clientApp(t, app.WithCertificate(func() string { return fingerprint }))
	secure := httptest.NewTLSServer(a.Router())
	t.Cleanup(secure.Close)
	if _, file := askSetup(t, secure.Client(), secure.URL, false, ""); file["certificate_sha256"] != fingerprint || file["server"] != secure.URL {
		t.Errorf("over the server's own certificate: %v", file)
	}
	// Not over a connection that certificate did not make.
	plain := httptest.NewServer(a.Router())
	t.Cleanup(plain.Close)
	if _, file := askSetup(t, http.DefaultClient, plain.URL, false, ""); file["certificate_sha256"] != "" {
		t.Errorf("over plain HTTP: %v", file)
	}
}
