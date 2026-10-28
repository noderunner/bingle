package api

import (
	"net/http"
	"strconv"

	"github.com/noderunner/bingle/internal/session"
)

// GET /games lists the games that are still going (the *All games* tab), and
// /here is who is about: the browsers that have said so lately. See
// internal/session/openlist.go and here.go.

// maxSecretRunes bounds a browser's own secret.
const maxSecretRunes = 128

// handleListOpenGames implements GET /games: every unfinished game of this
// server, newest change first, with who is seated and how many seats are open.
func (h *handlers) handleListOpenGames(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	games, err := h.mgr.ListOpenGames(r.Context(), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	if games == nil {
		games = []session.OpenGame{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": games})
}

// hereRequest is POST /here's body: the browser's own secret (never published)
// and the name its person goes by. An empty name says they are leaving.
type hereRequest struct {
	Secret     string `json:"secret"`
	PlayerName string `json:"player_name"`
}

// handlePostHere implements POST /here: mark this browser present (or, with an
// empty name, away) and answer with everybody who is here.
func (h *handlers) handlePostHere(w http.ResponseWriter, r *http.Request) {
	var req hereRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Secret == "" || len(req.Secret) > maxSecretRunes {
		writeError(w, errBadSecret)
		return
	}
	if req.PlayerName != "" {
		if err := checkName(req.PlayerName); err != nil {
			writeError(w, err)
			return
		}
	}
	people, err := h.mgr.Here(req.Secret, req.PlayerName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": people, "address": session.Address(req.Secret)})
}

// handleGetHere implements GET /here: who is about, without saying that the
// caller is.
func (h *handlers) handleGetHere(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"people": h.mgr.WhoIsHere()})
}
