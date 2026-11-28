// Package web serves bingle's browser UI: the pages (the lobby at "/", a game at
// "/game/{code}", the profile, the notification pages and the practice batches), whose
// templates are filled in here, and the static files they load. It is mounted by
// internal/app at "/", after every more specific mount ("/healthz", "/api",
// "/ws/{code}", "/mcp"), so it can act as the router's fallback/root without
// swallowing those more specific paths.
package web

import (
	"bytes"
	"cmp"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/clientbuilds"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/i18n"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/trace"
	"github.com/noderunner/bingle/internal/version"
)

//go:embed templates/*.html.tmpl
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// templates is parsed once at package init from every embedded
// templates/*.html.tmpl file.
var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"engineVersion": func() string { return version.Engine },
	// Filled in for each router, by brandedTemplates.
	"brandName":     func() string { return config.DefaultBrandName },
	"brandFooter":   func() string { return "bingle" },
	"brandHasIcon":  func() bool { return false },
	"discordURL":    func() string { return "" },
	"brandIconURL":  func() string { return "/static/favicon.svg" },
	"brandIconType": func() string { return "image/svg+xml" },
	// Filled in for a router that offers the bingle-3d client (WithClientBuilds).
	"clientMenu":   func() bool { return false },
	"clientBuilds": func() []clientBuild { return nil },
	// quickCode is a file name or a command inside a sentence.
	"quickCode": func(s string) template.HTML {
		return template.HTML("<code>" + template.HTMLEscapeString(s) + "</code>")
	}, //nolint:gosec // escaped
	// Filled in with the quick setup page's path when there is one (WithClientBuilds).
	"clientSetupPage": func() string { return "" },
	// clientPlatforms are the platforms the quick setup page has a build for
	// ("windows linux"), where a page offers to open a game in the client (ADR 0163).
	"clientPlatforms": func() string { return "" },
	// clientDownload is one build's download button, for any page that offers the client.
	"clientDownload": clientDownload,
	// t is the words of a key in the page's language (ADR 0141):
	// {{t .Lang "key" "name" value …}}.
	"t": i18n.HTML,
	// realTimeInput is a number of minutes inside a sentence of the create form,
	// for an "_html" key to place where its language puts it.
	"realTimeInput": func(id string, value int, label template.HTML) template.HTML {
		return template.HTML(fmt.Sprintf(`<input type="number" id="%s" min="1" max="60" step="1" value="%d" aria-label="%s">`, template.HTMLEscapeString(id), value, label)) //nolint:gosec // label is the catalog's, escaped by t
	},
}).ParseFS(templateFS, "templates/*.html.tmpl"))

// brandIconPath is where an instance's own icon is served.
const brandIconPath = "/brand/icon"

// brandedTemplates fills the name and icon of this instance into the templates.
func brandedTemplates(brand config.Brand) *template.Template {
	t := template.Must(templates.Clone())
	iconURL, iconType := "/static/favicon.svg", "image/svg+xml"
	if brand.Icon != nil {
		iconURL, iconType = brandIconPath, brand.IconType
	}
	// The footer has always written the platform's own name small; another name is as it was given.
	footer := func() string {
		if brand.Name == "" || brand.Name == config.DefaultBrandName {
			return "bingle"
		}
		return brand.Name
	}
	return t.Funcs(template.FuncMap{
		"brandName":     brand.DisplayName,
		"brandFooter":   footer,
		"brandHasIcon":  func() bool { return brand.Icon != nil },
		"discordURL":    func() string { return brand.DiscordURL },
		"brandIconURL":  func() string { return iconURL },
		"brandIconType": func() string { return iconType },
	})
}

// clientBuild is one build of the bingle-3d client in the logo's menu.
type clientBuild struct {
	// Name is the platform's, as a person says it; File and Version are the
	// archive's. Icon is the platform's mark, when it has one.
	Name, File, Version string
	Icon                template.HTML
}

// platformIcons are small marks for the platforms that have one, in the text's
// color. A platform without one is shown by its name.
var platformIcons = map[string]template.HTML{
	"windows-x86_64": `<svg class="platform-icon" viewBox="0 0 16 16" width="18" height="18" aria-hidden="true" focusable="false"><path fill="currentColor" d="M1 2.5l6-.8v5.6H1zM8 1.6l7-1v6.7H8zM1 8.7h6v5.6l-6-.8zM8 8.7h7v6.7l-7-1z"/></svg>`,
	"linux-x86_64":   `<svg class="platform-icon" viewBox="0 0 16 16" width="18" height="18" aria-hidden="true" focusable="false"><path fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" d="M8 1.2c-1.7 0-2.4 1.4-2.4 3 0 1-.5 1.6-1.2 2.6-.9 1.3-1.6 2.9-1.6 4.4 0 .8.4 1.2 1 1.4.5.2.9.6 1.3 1.1.4.5 1.2.5 1.6.1.4-.4 2.6-.4 3.2 0 .4.4 1.2.4 1.6-.1.4-.5.8-.9 1.3-1.1.6-.2 1-.6 1-1.4 0-1.5-.7-3.1-1.6-4.4-.7-1-1.2-1.6-1.2-2.6 0-1.6-.7-3-2.4-3z"/><circle cx="6.6" cy="4.3" r=".7" fill="currentColor"/><circle cx="9.4" cy="4.3" r=".7" fill="currentColor"/><path fill="currentColor" d="M6.9 5.6h2.2L8 6.6z"/></svg>`,
}

// clientDownload is the button that downloads one build: the platform's icon
// and the build's version, or the platform's name and the version when it has
// no icon. It is a link that downloads, and role is its ARIA role, if any.
func clientDownload(b clientBuild, role string) template.HTML {
	e := template.HTMLEscapeString
	label := b.Name + " " + b.Version
	inner := string(b.Icon) + e(b.Version)
	if b.Icon == "" {
		inner = e(label)
	}
	attrs := ""
	if role != "" {
		attrs = ` role="` + e(role) + `" tabindex="-1"`
	}
	return template.HTML(`<a class="button client-download"` + attrs + ` href="/api/client/files/` + e(url.PathEscape(b.File)) + `" download title="` + e(b.Version) + `" aria-label="` + e(label) + `">` + inner + `</a>`) //nolint:gosec // every value is escaped
}

// platformNames are the names of the platforms bingle-3d is built for. They
// are names, so no language has its own; a platform not listed here is shown
// as its archive calls it.
var platformNames = map[string]string{
	"windows-x86_64": "Windows",
	"linux-x86_64":   "Linux",
	"steam-frame":    "Steam Frame",
}

// quickSetupPath is the page that walks a player through the client (ADR 0151). The
// announcements and the logo menu link to it, so it stays where it is.
const quickSetupPath = "/client"

// quickSetupPlatform is one section of the quick setup page: a platform the page
// has steps for, and the build the server offers for it.
type quickSetupPlatform struct {
	// ID is "windows" or "linux"; Program is the file to run, and Build is the archive.
	ID, Program string
	Build       clientBuild
}

// quickSetupPrograms are the platforms the page has steps for, by platform, and the
// program each archive unpacks to. Steam Frame installs on the headset and has none.
var quickSetupPrograms = []struct{ Platform, ID, Program string }{
	{"windows-x86_64", "windows", "bingle-3d.exe"},
	{"linux-x86_64", "linux", "./bingle-3d"},
}

// Option customises NewRouter.
type Option func(*handlers)

// WithClientBuilds puts a menu on the logo of every page, with a download
// for each build in set and the client's setup file (ADR 0151). A nil set
// leaves the logo the plain link home it has always been.
func WithClientBuilds(set *clientbuilds.Set) Option {
	return func(h *handlers) {
		if set == nil {
			return
		}
		var builds []clientBuild
		for _, b := range set.Builds() {
			builds = append(builds, clientBuild{Name: cmp.Or(platformNames[b.Platform], b.Platform), File: b.File, Version: b.Version, Icon: platformIcons[b.Platform]})
		}
		for _, q := range quickSetupPrograms {
			for _, b := range builds {
				if platformNames[q.Platform] == b.Name {
					h.quickSetup = append(h.quickSetup, quickSetupPlatform{ID: q.ID, Program: q.Program, Build: b})
				}
			}
		}
		page := func() string { return "" }
		if len(h.quickSetup) > 0 {
			page = func() string { return quickSetupPath }
		}
		var ids []string
		for _, q := range h.quickSetup {
			ids = append(ids, q.ID)
		}
		platforms := strings.Join(ids, " ")
		h.templates = h.templates.Funcs(template.FuncMap{
			"clientMenu":      func() bool { return true },
			"clientBuilds":    func() []clientBuild { return builds },
			"clientSetupPage": page,
			"clientPlatforms": func() string { return platforms },
		})
	}
}

// NewRouter returns a chi.Router serving bingle's browser UI: "GET /" renders
// index.html.tmpl with mgr.ListGameDefs' result as template data (so the
// create form's game_def_id select offers every real registered game
// definition), and "/static/..." serves the embedded static assets
// (app.js, style.css) index.html.tmpl references.
//
// accountLanguage is the language the account a page is for chose, "" for
// none (auth.Resolver.AccountLanguage); nil when there are no accounts.
func NewRouter(mgr *session.Manager, brand config.Brand, accountLanguage func(*http.Request) string, opts ...Option) http.Handler {
	r := chi.NewRouter()

	h := &handlers{mgr: mgr, templates: brandedTemplates(brand), accountLanguage: accountLanguage}
	for _, opt := range opts {
		opt(h)
	}
	if brand.Icon != nil {
		r.Get(brandIconPath, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", brand.IconType)
			w.Header().Set("Cache-Control", "public, max-age=3600")
			// An SVG opened by itself could run a script; it is a picture and nothing else.
			w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			_, _ = w.Write(brand.Icon)
		})
	}
	r.Get("/", h.handleIndex)
	r.Get("/game/{code}", h.handleGame)
	r.Get("/profile", h.handleProfile)
	r.Get("/players/{handle}", h.handlePlayer)
	r.Get("/tournament/{id}", h.handleTournament)
	r.Get("/login", h.handleAccountPage("login", "common.sign_in"))
	r.Get("/signup", h.handleAccountPage("signup", "common.sign_up"))
	r.Get("/account", h.handleAccountPage("account", "common.your_account"))
	r.Get("/account/confirm", h.handleAccountPage("confirm", "account.title_confirm"))
	r.Get("/forgot", h.handleAccountPage("forgot", "account.title_forgot"))
	r.Get("/account/reset", h.handleAccountPage("reset", "account.title_reset"))
	r.Get("/admin", h.handleAdmin)
	r.Get("/notify/confirm", h.handleNotifyPage("confirm"))
	r.Get("/notify/stop", h.handleNotifyPage("stop"))
	r.Post("/notify/confirm", h.handleNotifyConfirm)
	r.Post("/notify/stop", h.handleNotifyStop)
	if len(h.quickSetup) > 0 {
		r.Get(quickSetupPath, h.handleQuickSetup)
	}
	r.Get("/batches", h.handleBatches)
	r.Get("/batches/{id}", h.handleBatches)

	staticSub, err := staticContent()
	if err != nil {
		// staticFS is embedded at build time from a directory guaranteed to
		// exist in this package, so this can only fail if that invariant is
		// broken (e.g. the static/ directory is deleted) — a build-time bug,
		// not a runtime condition callers need to handle.
		panic(err)
	}
	// A game's own page scripts (ADR 0124), served like the page's.
	r.Get("/static/games/{family}/*", func(w http.ResponseWriter, req *http.Request) {
		family := chi.URLParam(req, "family")
		scripts, ok := mgr.GameScripts(family)
		if !ok {
			http.NotFound(w, req)
			return
		}
		http.StripPrefix("/static/games/"+family+"/", http.FileServer(http.FS(scripts))).ServeHTTP(w, req)
	})
	r.Get("/static/i18n/{lang}.json", handleCatalog)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	return r
}

// handleCatalog serves what js/i18n.js loads for a language: the languages,
// and the catalog with English's words where it lacks its own. The page asks
// every time, and is told when it has not changed.
func handleCatalog(w http.ResponseWriter, r *http.Request) {
	body, etag := i18n.Page(chi.URLParam(r, "lang"))
	if body == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

// staticContent returns staticFS rooted at its "static" subdirectory, so
// embedded files are served at e.g. "/static/app.js" rather than
// "/static/static/app.js".
func staticContent() (fs.FS, error) {
	return fs.Sub(staticFS, "static")
}

// StaticHandler serves the embedded static files under "/static/".
//
// It exists so the private metrics listener can serve the same stylesheet and
// the same chart module the rest of the site uses: the Health page then needs
// no assets of its own (docs/history/observability-plan.md section 5). It
// serves files and nothing else — no page, no session, no manager.
func StaticHandler() http.Handler {
	sub, err := staticContent()
	if err != nil {
		panic(err) // see NewRouter: a build-time invariant, not a runtime case
	}
	// The catalogs too: js/util.js, which the Health page loads, loads js/i18n.js (ADR 0141).
	r := chi.NewRouter()
	r.Get("/static/i18n/{lang}.json", handleCatalog)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	return r
}

// handlers holds the dependencies shared by every handler in this package.
type handlers struct {
	mgr             *session.Manager
	templates       *template.Template
	accountLanguage func(*http.Request) string
	// quickSetup are the platforms the quick setup page has steps for, empty when the
	// server offers no build for any of them.
	quickSetup []quickSetupPlatform
}

// page is what every template is given: the language the page is in (ADR
// 0141), its direction, the language chosen ("" for the browser's own) and the
// browser's own, which the Language setting offers first.
type page struct {
	Lang, Dir, Chosen, Browser string
}

// page is the request's page data.
func (h *handlers) page(r *http.Request) page {
	account := ""
	if h.accountLanguage != nil {
		account = h.accountLanguage(r)
	}
	lang := i18n.ForRequest(r, account)
	return page{Lang: lang, Dir: i18n.Dir(lang), Chosen: i18n.Choice(r, account), Browser: i18n.Browser(r)}
}

// indexPageData is index.html.tmpl's template data.
type indexPageData struct {
	page
	GameDefs []session.GameDefSummary
}

// gamePageData is game.html.tmpl's template data.
type gamePageData struct {
	page
	Code string
}

// handleIndex implements GET /: render index.html.tmpl with the catalog's
// current game definitions, so the create-game form's game_def_id select is
// always populated from real, registered game defs.
// render writes one template, timed as a span so that a page which is slow to
// build says which template it was in the slow-request journal.
func (h *handlers) render(r *http.Request, w http.ResponseWriter, name string, data any) error {
	_, span := trace.Start(r.Context(), "web.render")
	span.Set("template", name)
	defer span.End()
	// The page's language comes from these (ADR 0141).
	w.Header().Add("Vary", "Accept-Language, Cookie")
	return h.templates.ExecuteTemplate(w, name, data)
}

func (h *handlers) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := indexPageData{page: h.page(r), GameDefs: h.mgr.ListGameDefs(r.Context())}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.render(r, w, "index.html.tmpl", data); err != nil {
		http.Error(w, "web: rendering index page failed", http.StatusInternalServerError)
		return
	}
}

// handleGame implements GET /game/{code}: render game.html.tmpl with the
// requested room code. This handler deliberately does not validate that the
// room code refers to a real, existing game — the page shell always renders
// 200, and board.js's own client-side fetch to GET /api/games/{code}/state
// is what surfaces a 404 for an unknown room (see docs/history/plan.md section 6.11
// and tests/app/web_board_state_test.go's documented reasoning).
func (h *handlers) handleGame(w http.ResponseWriter, r *http.Request) {
	data := gamePageData{page: h.page(r), Code: chi.URLParam(r, "code")}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Only this page may use the microphone (voice chat), and nothing may use the camera.
	w.Header().Set("Permissions-Policy", "microphone=(self), camera=()")
	if err := h.render(r, w, "game.html.tmpl", data); err != nil {
		http.Error(w, "web: rendering game page failed", http.StatusInternalServerError)
		return
	}
}

// batchesPageData is batches.html.tmpl's template data: the batch to report on,
// empty for the list.
type batchesPageData struct {
	page
	ID string
}

// handleBatches serves the practice-batch pages: the list at /batches and a
// batch's report at /batches/{id}. Like the game page it is only a shell; the
// script fetches /api/batches itself.
func (h *handlers) handleBatches(w http.ResponseWriter, r *http.Request) {
	data := batchesPageData{page: h.page(r), ID: chi.URLParam(r, "id")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.render(r, w, "batches.html.tmpl", data); err != nil {
		http.Error(w, "web: rendering batches page failed", http.StatusInternalServerError)
		return
	}
}

// handleTournament serves a tournament's page, which reads the tournament itself.
func (h *handlers) handleTournament(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.render(r, w, "tournament.html.tmpl", h.page(r)); err != nil {
		http.Error(w, "web: rendering tournament page failed", http.StatusInternalServerError)
		return
	}
}

// handleProfile serves the profile page: the player's settings and finished
// games, both kept in the browser, so the page is only a shell.
func (h *handlers) handleProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.render(r, w, "profile.html.tmpl", h.page(r)); err != nil {
		http.Error(w, "web: rendering profile page failed", http.StatusInternalServerError)
		return
	}
}

// quickSetupPageData is client.html.tmpl's template data.
type quickSetupPageData struct {
	page
	Platforms []quickSetupPlatform
}

// handleQuickSetup serves the page of steps for getting the client running against
// this server: a section for each platform it has a build for.
func (h *handlers) handleQuickSetup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.render(r, w, "client.html.tmpl", quickSetupPageData{page: h.page(r), Platforms: h.quickSetup}); err != nil {
		http.Error(w, "web: rendering the quick setup page failed", http.StatusInternalServerError)
	}
}

// handlePlayer serves a player's page, a shell: the script reads their badges
// and ratings, which say whether there is such a player.
func (h *handlers) handlePlayer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.render(r, w, "player.html.tmpl", h.page(r)); err != nil {
		http.Error(w, "web: rendering the player page failed", http.StatusInternalServerError)
	}
}

// accountPageData is account.html.tmpl's data: which of the account pages.
type accountPageData struct {
	page
	Page, Title string
}

// handleAccountPage serves one of the account pages (ADR 0068). They are
// shells: the script asks /auth/me who the browser is.
func (h *handlers) handleAccountPage(page, title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := h.render(r, w, "account.html.tmpl", accountPageData{page: h.page(r), Page: page, Title: title}); err != nil {
			http.Error(w, "web: rendering the account page failed", http.StatusInternalServerError)
		}
	}
}

// handleAdmin serves the administration page, a shell: /auth/admin/* says
// whether the browser may see it.
func (h *handlers) handleAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := h.render(r, w, "admin.html.tmpl", h.page(r)); err != nil {
		http.Error(w, "web: rendering the admin page failed", http.StatusInternalServerError)
	}
}

// notifyPageData is notify.html.tmpl's data: which action the page offers.
type notifyPageData struct {
	page
	Action string // "confirm" or "stop"
}

// handleNotifyPage serves the page behind the link in a notification mail. The
// page only shows a button: following the link does nothing by itself, so mail
// scanners and link previews cannot confirm or unsubscribe anybody. The button
// POSTs the token from the address to the same path.
func (h *handlers) handleNotifyPage(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if err := h.render(r, w, "notify.html.tmpl", notifyPageData{page: h.page(r), Action: action}); err != nil {
			http.Error(w, "web: rendering notify page failed", http.StatusInternalServerError)
		}
	}
}

// handleNotifyConfirm implements POST /notify/confirm?t=TOKEN: the address
// that received the link is confirmed.
func (h *handlers) handleNotifyConfirm(w http.ResponseWriter, r *http.Request) {
	info, err := h.mgr.ConfirmContact(r.Context(), r.URL.Query().Get("t"))
	h.notifyReply(w, info.Masked, err)
}

// handleNotifyStop implements POST /notify/stop?t=TOKEN: the address is never
// mailed again. Mail programs POST here too (RFC 8058 one-click unsubscribe).
func (h *handlers) handleNotifyStop(w http.ResponseWriter, r *http.Request) {
	info, err := h.mgr.StopMail(r.Context(), r.URL.Query().Get("t"))
	h.notifyReply(w, info.Masked, err)
}

func (h *handlers) notifyReply(w http.ResponseWriter, masked string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		status := http.StatusForbidden
		if errors.Is(err, session.ErrNotifyOff) {
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "This link is not valid any more. Ask for a new one."})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "masked": masked})
}
