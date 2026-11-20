package api

import (
	"net/http"

	"github.com/noderunner/bingle/internal/version"
)

// WithDocs sets the API's documentation (the text of docs/api.md), served at GET /docs.
func WithDocs(md string) Option {
	return func(h *handlers) { h.docs = md }
}

// WithSchema sets the JSON Schema of the API's messages (docs/api.schema.json), served at GET /schema.
func WithSchema(schema []byte) Option {
	return func(h *handlers) { h.schema = schema }
}

// APIVersionHeader is the response header that carries version.API, on every
// answer of the API and on a WebSocket's handshake.
const APIVersionHeader = "Bingle-API-Version"

func apiVersionHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(APIVersionHeader, version.API)
		next.ServeHTTP(w, r)
	})
}

// indexEndpoint is one route the index points a newcomer at.
type indexEndpoint struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	About  string `json:"about"`
}

// apiIndex is GET /api's body: enough to play a game with curl, and where to read the rest.
type apiIndex struct {
	// Version is the server's version, which moves with every change to the
	// engine or the platform; APIVersion is the version of the API's messages,
	// whose major a client compares with the one it was built for (ADR 0134).
	Version    string `json:"version"`
	APIVersion string `json:"api_version"`
	// Docs is where the API's documentation is, and Schema the JSON Schema of
	// its messages (empty when this server has none built in).
	Docs      string          `json:"docs"`
	Schema    string          `json:"schema"`
	MCP       string          `json:"mcp"`
	WebSocket string          `json:"websocket"`
	Start     []string        `json:"start"`
	Endpoints []indexEndpoint `json:"endpoints"`
}

// indexEndpoints are the routes a client needs to find and play a game. The rest are in the docs.
var indexEndpoints = []indexEndpoint{
	{"POST", "/api/guest", "make a guest: {key}, to send as Authorization: Bearer <key>"},
	{"GET", "/api/game-defs", "the games: id, name, seats"},
	{"GET", "/api/game-defs/{id}", "one game: rules, notation_help, options, board"},
	{"GET", "/api/game-defs/{id}/guide", "the rules guide, as Markdown"},
	{"GET", "/api/bots", "the built-in opponents"},
	{"GET", "/api/games", "the open games waiting for players"},
	{"POST", "/api/games", "create a game: {game_def_id, player_name, opponent?, options?}"},
	{"POST", "/api/games/{code}/join", "take a seat: {player_name, seat?}"},
	{"GET", "/api/games/{code}", "the game: its seats and settings"},
	{"GET", "/api/games/{code}/state", "the position, whose impulse it is, and the result"},
	{"GET", "/api/games/{code}/observe", "the position as text, with your legal moves (?player_name=)"},
	{"GET", "/api/games/{code}/selections", "what the seat on impulse may select next (?made=)"},
	{"POST", "/api/games/{code}/moves", "play a move: {player_name, notation}"},
	{"GET", "/api/games/{code}/moves", "the moves played so far"},
	{"POST", "/api/games/{code}/resign", "resign: {player_name}"},
	{"POST", "/api/games/{code}/bots", "a bot takes an open seat of a waiting game: {player_name, bot, seat?}"},
	{"POST", "/api/games/{code}/start", "the host starts a game waiting in its lobby: {player_name}"},
	{"POST", "/api/games/{code}/seat", "move to an open seat before the game starts: {player_name, seat}"},
	{"POST", "/api/games/{code}/leave", "give up your seat before the game starts: {player_name}"},
	{"POST", "/api/games/{code}/remove", "the host clears another seat before the game starts: {player_name, seat}"},
	{"POST", "/api/bot-matches", "two built-in bots play each other, to watch: {game_def_id, bot_a, bot_b, delay_ms?}"},
}

// handleIndex implements GET / (GET /api): where to start.
func (h *handlers) handleIndex(w http.ResponseWriter, r *http.Request) {
	idx := apiIndex{
		Version:    version.Engine,
		APIVersion: version.API,
		MCP:        "/mcp",
		WebSocket:  "/ws/{code}",
		Start: []string{
			"POST /api/guest, and send its key with every request",
			"GET /api/game-defs, then GET /api/game-defs/{id} for a game's rules and notation",
			`POST /api/games {"game_def_id": "hex-7", "player_name": "Ana", "opponent": "random"}`,
			"GET /api/games/{code}/selections, then POST /api/games/{code}/moves {player_name, notation}",
		},
		Endpoints: indexEndpoints,
	}
	if h.docs != "" {
		idx.Docs = "/api/docs"
	}
	if len(h.schema) > 0 {
		idx.Schema = "/api/schema"
	}
	writeJSON(w, http.StatusOK, idx)
}

// handleDocs implements GET /docs: docs/api.md, as plain text a browser shows rather than saves.
func (h *handlers) handleDocs(w http.ResponseWriter, r *http.Request) {
	if h.docs == "" {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "api: this server has no documentation built in"})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(h.docs))
}

// handleSchema implements GET /schema: docs/api.schema.json, the JSON Schema
// of the messages of the API version this server speaks.
func (h *handlers) handleSchema(w http.ResponseWriter, r *http.Request) {
	if len(h.schema) == 0 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "api: this server has no schema built in"})
		return
	}
	w.Header().Set("Content-Type", "application/schema+json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(h.schema)
}

// noSuchEndpoint is the error of a path the API does not have.
const noSuchEndpoint = "api: no such endpoint; GET /api lists where to start"

func handleNoSuchEndpoint(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, errorResponse{Error: noSuchEndpoint})
}

func handleWrongMethod(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "api: that endpoint does not take " + r.Method + "; GET /api lists where to start"})
}
