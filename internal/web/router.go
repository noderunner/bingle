// Package web serves bingle's browser UI: the pages (the lobby at "/", a game at
// "/game/{code}", the profile, the notification pages and the practice batches), whose
// templates are filled in here, and the static files they load. It is mounted by
// internal/app at "/", after every more specific mount ("/healthz", "/api",
// "/ws/{code}", "/mcp"), so it can act as the router's fallback/root without
// swallowing those more specific paths.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

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
}).ParseFS(templateFS, "templates/*.html.tmpl"))

// NewRouter returns a chi.Router serving bingle's browser UI: "GET /" renders
// index.html.tmpl with mgr.ListGameDefs' result as template data (so the
// create form's game_def_id select offers every real registered game
// definition), and "/static/..." serves the embedded static assets
// (app.js, style.css) index.html.tmpl references.
func NewRouter(mgr *session.Manager) http.Handler {
	r := chi.NewRouter()

	h := &handlers{mgr: mgr}
	r.Get("/", h.handleIndex)
	r.Get("/game/{code}", h.handleGame)
	r.Get("/profile", h.handleProfile)
	r.Get("/login", h.handleAccountPage("login", "Sign in"))
	r.Get("/signup", h.handleAccountPage("signup", "Sign up"))
	r.Get("/account", h.handleAccountPage("account", "Your account"))
	r.Get("/account/confirm", h.handleAccountPage("confirm", "Confirm your account"))
	r.Get("/forgot", h.handleAccountPage("forgot", "Forgot your password"))
	r.Get("/account/reset", h.handleAccountPage("reset", "Choose a new password"))
	r.Get("/notify/confirm", h.handleNotifyPage("confirm"))
	r.Get("/notify/stop", h.handleNotifyPage("stop"))
	r.Post("/notify/confirm", h.handleNotifyConfirm)
	r.Post("/notify/stop", h.handleNotifyStop)
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
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	return r
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
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}

// handlers holds the dependencies shared by every handler in this package.
type handlers struct {
	mgr *session.Manager
}

// indexPageData is index.html.tmpl's template data.
type indexPageData struct {
	GameDefs []session.GameDefSummary
}

// gamePageData is game.html.tmpl's template data.
type gamePageData struct {
	Code string
}

// handleIndex implements GET /: render index.html.tmpl with the catalog's
// current game definitions, so the create-game form's game_def_id select is
// always populated from real, registered game defs.
// render writes one template, timed as a span so that a page which is slow to
// build says which template it was in the slow-request journal.
func render(r *http.Request, w http.ResponseWriter, name string, data any) error {
	_, span := trace.Start(r.Context(), "web.render")
	span.Set("template", name)
	defer span.End()
	return templates.ExecuteTemplate(w, name, data)
}

func (h *handlers) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := indexPageData{GameDefs: h.mgr.ListGameDefs(r.Context())}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := render(r, w, "index.html.tmpl", data); err != nil {
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
	data := gamePageData{Code: chi.URLParam(r, "code")}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Only this page may use the microphone (voice chat), and nothing may use the camera.
	w.Header().Set("Permissions-Policy", "microphone=(self), camera=()")
	if err := render(r, w, "game.html.tmpl", data); err != nil {
		http.Error(w, "web: rendering game page failed", http.StatusInternalServerError)
		return
	}
}

// batchesPageData is batches.html.tmpl's template data: the batch to report on,
// empty for the list.
type batchesPageData struct {
	ID string
}

// handleBatches serves the practice-batch pages: the list at /batches and a
// batch's report at /batches/{id}. Like the game page it is only a shell; the
// script fetches /api/batches itself.
func (h *handlers) handleBatches(w http.ResponseWriter, r *http.Request) {
	data := batchesPageData{ID: chi.URLParam(r, "id")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := render(r, w, "batches.html.tmpl", data); err != nil {
		http.Error(w, "web: rendering batches page failed", http.StatusInternalServerError)
		return
	}
}

// handleProfile serves the profile page: the player's settings and finished
// games, both kept in the browser, so the page is only a shell.
func (h *handlers) handleProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := render(r, w, "profile.html.tmpl", nil); err != nil {
		http.Error(w, "web: rendering profile page failed", http.StatusInternalServerError)
		return
	}
}

// accountPageData is account.html.tmpl's data: which of the account pages.
type accountPageData struct {
	Page, Title string
}

// handleAccountPage serves one of the account pages (ADR 0068). They are
// shells: the script asks /auth/me who the browser is.
func (h *handlers) handleAccountPage(page, title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := render(r, w, "account.html.tmpl", accountPageData{Page: page, Title: title}); err != nil {
			http.Error(w, "web: rendering the account page failed", http.StatusInternalServerError)
		}
	}
}

// notifyPageData is notify.html.tmpl's data: which action the page offers.
type notifyPageData struct {
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
		if err := render(r, w, "notify.html.tmpl", notifyPageData{Action: action}); err != nil {
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
