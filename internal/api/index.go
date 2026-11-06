package api

import (
	"net/http"

	"github.com/noderunner/bingle/internal/version"
)

// WithDocs sets the API's documentation (the text of docs/api.md), served at GET /docs.
func WithDocs(md string) Option {
	return func(h *handlers) { h.docs = md }
}

// indexEndpoint is one route the index points a newcomer at.
type indexEndpoint struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	About  string `json:"about"`
}

// apiIndex is GET /api's body: enough to play a game with curl, and where to read the rest.
type apiIndex struct {
	Version   string          `json:"version"`
	Docs      string          `json:"docs"`
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
	{"GET", "/api/games/{code}/moves/legal", "every legal move of the seat to move"},
	{"POST", "/api/games/{code}/moves", "play a move: {player_name, notation}"},
	{"GET", "/api/games/{code}/moves", "the moves played so far"},
	{"POST", "/api/games/{code}/resign", "resign: {player_name}"},
}

// handleIndex implements GET / (GET /api): where to start.
func (h *handlers) handleIndex(w http.ResponseWriter, r *http.Request) {
	idx := apiIndex{
		Version:   version.Engine,
		MCP:       "/mcp",
		WebSocket: "/ws/{code}",
		Start: []string{
			"POST /api/guest, and send its key with every request",
			"GET /api/game-defs, then GET /api/game-defs/{id} for a game's rules and notation",
			`POST /api/games {"game_def_id": "hex-7", "player_name": "Ana", "opponent": "random"}`,
			"GET /api/games/{code}/moves/legal, then POST /api/games/{code}/moves {player_name, notation}",
		},
		Endpoints: indexEndpoints,
	}
	if h.docs != "" {
		idx.Docs = "/api/docs"
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

// noSuchEndpoint is the error of a path the API does not have.
const noSuchEndpoint = "api: no such endpoint; GET /api lists where to start"

func handleNoSuchEndpoint(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, errorResponse{Error: noSuchEndpoint})
}

func handleWrongMethod(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "api: that endpoint does not take " + r.Method + "; GET /api lists where to start"})
}
