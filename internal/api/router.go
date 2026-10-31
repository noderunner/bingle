// Package api implements bingle's REST API as a chi.Router mounted by
// internal/app at "/api", per docs/history/plan.md section 6.8. Every handler here
// is a thin HTTP adapter over internal/session.Manager: request decoding,
// input validation, and error→status mapping (see errors.go) live here;
// game-lifecycle logic stays in internal/session.
package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/envs"
	"github.com/noderunner/bingle/internal/journal"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/transport"
)

// NewRouter returns a chi.Router for bingle's REST API, with routes
// relative to its eventual mount point — it does not bake in an "/api"
// prefix itself; internal/app mounts it at "/api".
func NewRouter(mgr *session.Manager, opts ...Option) chi.Router {
	r := chi.NewRouter()

	h := &handlers{mgr: mgr}
	for _, opt := range opts {
		opt(h)
	}

	if h.clientErrorsOn {
		r.Post("/client-error", h.handlePostClientError)
	}

	if h.guests != nil {
		r.Post("/guest", h.guests.HandleNewGuest)
		r.Get("/guest/key", h.guests.HandleGuestKey)
		r.Post("/guest/adopt", h.guests.HandleAdopt)
		r.Post("/guest/rotate", h.guests.HandleRotate)
		r.Post("/guest/discard", h.guests.HandleDiscard)
	}
	r.Get("/me/games", h.handleOwnGames)
	r.Post("/games", h.handleCreateGame)
	r.Post("/games/import", h.handleImportGame)
	r.Post("/bot-matches", h.handleBotMatch)
	r.Get("/game-defs", h.handleListGameDefs)
	r.Get("/game-defs/{id}", h.handleGameDef)
	r.Get("/game-defs/{id}/guide", h.handleGameGuide)
	r.Get("/game-defs/{id}/assets/*", h.handleGameAsset)
	r.Get("/bots", h.handleListBots)
	r.Get("/config", h.handleConfig)
	r.Get("/changelog", h.handleChangelog)
	r.Get("/bot-guide", h.handleBotGuide)
	r.Get("/bot-guide/assets/*", h.handleBotGuideAsset)
	r.Get("/puzzles", h.handleListPuzzles)
	r.Get("/puzzles/{id}", h.handleGetPuzzle)
	r.Get("/games", h.handleListOpenGames)
	r.Get("/here", h.handleGetHere)
	r.Post("/here", h.handlePostHere)
	r.Post("/dm", h.handlePostDirect)
	r.Post("/dm/history", h.handleDirectHistory)
	r.Post("/dm/threads", h.handleDirectThreads)
	r.Post("/dm/call", h.handleDirectCall)
	r.Post("/dm/signal", h.handleDirectSignal)
	r.Get("/games/{code}", h.handleGetGame)
	r.Post("/games/{code}/join", h.handleJoinGame)
	r.Post("/games/{code}/bots", h.handleAddBot)
	r.Post("/games/{code}/start", h.handleStartGame)
	r.Post("/games/{code}/seat", h.handleChooseSeat)
	r.Post("/games/{code}/seat-cult", h.handleSetSeatCult)
	r.Post("/games/{code}/leave", h.handleLeave)
	r.Post("/games/{code}/remove", h.handleRemoveSeat)
	r.Get("/games/{code}/reactions", h.handleGetReactions)
	r.Put("/games/{code}/reactions", h.handleSetReactions)
	r.Get("/games/{code}/autoplay", h.handleGetAutoPlay)
	r.Put("/games/{code}/autoplay", h.handleSetAutoPlay)
	r.Get("/chat", h.handleGetGlobalChat)
	r.Post("/chat", h.handlePostGlobalChat)
	r.Get("/games/{code}/chat", h.handleGetChat)
	r.Post("/games/{code}/chat", h.handlePostChat)
	r.Get("/games/{code}/presence", h.handleGetPresence)
	r.Get("/games/{code}/voice", h.handleGetVoice)
	r.Post("/games/{code}/voice", h.handlePostVoice)
	r.Post("/games/{code}/voice/signal", h.handlePostVoiceSignal)
	r.Get("/rtc/config", h.handleRTCConfig)
	r.Get("/notify/config", h.handleNotifyConfig)
	r.Post("/notify/contact", h.handleSaveContact)
	r.Post("/notify/contact/status", h.handleContactStatus)
	r.Post("/notify/contact/resend", h.handleContactResend)
	r.Post("/notify/contact/forget", h.handleContactForget)
	r.Get("/games/{code}/notify", h.handleGetGameNotify)
	r.Put("/games/{code}/notify", h.handleSetGameNotify)
	r.Post("/games/{code}/presence", h.handlePostPresence)
	r.Get("/games/{code}/state", h.handleGetGameState)
	r.Post("/games/{code}/moves", h.handleMakeMove)
	r.Get("/games/{code}/moves", h.handleListMoves)
	r.Get("/games/{code}/history", h.handleHistory)
	r.Get("/games/{code}/legal-moves", h.handleListLegalMoves)
	r.Get("/games/{code}/moves/legal", h.handleLegalMovesAll)
	r.Post("/games/{code}/undo", h.handleUndo)
	r.Post("/games/{code}/resign", h.handleResign)
	r.Post("/games/{code}/draw", h.handleDraw)
	r.Post("/games/{code}/takeback", h.handleTakeback)
	r.Post("/games/{code}/simulate", h.handleSimulate)
	r.Post("/games/{code}/hint", h.handleHint)
	r.Get("/games/{code}/export", h.handleExport)
	r.Get("/games/{code}/observe", h.handleObserve)

	// In-memory training environments (when an envs manager is supplied).
	if h.envs != nil {
		r.Post("/envs", h.handleEnvReset)
		r.Get("/envs", h.handleEnvList)
		r.Post("/envs/batch-step", h.handleEnvBatchStep)
		r.Get("/envs/{id}", h.handleEnvObserve)
		r.Post("/envs/{id}/step", h.handleEnvStep)
		r.Post("/envs/{id}/fork", h.handleEnvFork)
		r.Delete("/envs/{id}", h.handleEnvClose)
		r.Get("/env-specs/{game}", h.handleEnvSpec)
		r.Post("/matches", h.handleMatch)
	}

	// Bot practice batches (when a batch manager is supplied).
	if h.batches != nil {
		r.Post("/batches", h.handleBatchStart)
		r.Get("/batches", h.handleBatchList)
		r.Get("/batches/{id}", h.handleBatchGet)
		r.Get("/batches/{id}/report", h.handleBatchReport)
		r.Get("/batches/{id}/games", h.handleBatchGames)
		r.Get("/batches/{id}/games/{n}/events", h.handleBatchEvents)
		r.Post("/batches/{id}/cancel", h.handleBatchCancel)
		r.Delete("/batches/{id}", h.handleBatchDelete)
	}

	return r
}

// NewWSHandler returns the http.HandlerFunc for GET /ws/{code}, per
// docs/history/plan.md section 6.9. It is mounted directly by internal/app (a
// sibling of the "/api" mount, not nested under it), rather than via
// NewRouter, since "/ws/{code}" has no "/api" prefix.
func NewWSHandler(mgr *session.Manager) http.HandlerFunc {
	h := &handlers{mgr: mgr}
	return h.handleWS
}

// handlers holds the dependencies shared by every handler in this package.
type handlers struct {
	mgr *session.Manager
	// guests makes guests for scripts (POST /guest); nil leaves it unmounted.
	guests *auth.Resolver
	envs   *envs.Manager
	// batches runs bot practice batches (nil when not mounted).
	batches *batch.Manager
	// publicURL is the base URL clients on other networks reach the server by
	// (invite links); empty when not configured.
	publicURL string
	// rtc is what voice chat clients are told about STUN and TURN servers.
	rtc RTCConfig
	// changelog is the newest release of CHANGELOG.md (nil when there is none).
	changelog *changelogEntry
	// log is the server's logger; nil means slog's default.
	log *slog.Logger
	// metrics is the registry; nil records nothing.
	metrics *metrics.Metrics
	// journal is the metrics database, where a browser's faults are kept;
	// nil counts them and keeps none.
	journal *journal.DB
	// clientErrors counts reports per address (see clienterror.go).
	clientErrors clientErrorLimiter
	// transport says how to read a request's scheme and peer address: behind
	// a trusted proxy both come from headers, and otherwise neither does.
	transport transport.Policy
	// clientErrorsOn mounts POST /client-error. It is off unless asked for,
	// because it is the one observability route on the public listener.
	clientErrorsOn bool
}

// WithGuests mounts POST /guest, which makes a guest for a script.
func WithGuests(rs *auth.Resolver) Option { return func(h *handlers) { h.guests = rs } }

// WithTransport tells the API how to read a request's peer address. Without
// it the zero policy applies: no proxy trusted, no forwarded header believed.
func WithTransport(p transport.Policy) Option {
	return func(h *handlers) { h.transport = p }
}

// WithClientErrors turns POST /client-error on, recording into lg, mx and db
// (any of which may be nil).
func WithClientErrors(lg *slog.Logger, mx *metrics.Metrics, db *journal.DB) Option {
	return func(h *handlers) {
		h.clientErrorsOn = true
		h.log = lg
		h.metrics = mx
		h.journal = db
	}
}

// Option customises NewRouter.
type Option func(*handlers)

// WithPublicURL sets the base URL that invite links and /config report: how
// players on other networks reach this server.
func WithPublicURL(u string) Option {
	return func(h *handlers) { h.publicURL = strings.TrimRight(u, "/") }
}

// WithEnvs mounts the in-memory training-environment endpoints (/envs,
// /env-specs, /matches) backed by em.
func WithEnvs(em *envs.Manager) Option { return func(h *handlers) { h.envs = em } }

// NewGlobalChatWSHandler returns the http.HandlerFunc for GET /ws-chat, the
// live feed of the global chat room.
func NewGlobalChatWSHandler(mgr *session.Manager) http.HandlerFunc {
	h := &handlers{mgr: mgr}
	return h.handleGlobalChatWS
}
