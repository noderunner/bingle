package api

import (
	"net/http"
	"strconv"

	"github.com/noderunner/bingle/internal/session"
)

// GET /games lists the games that are still going (the *All games* tab), and
// /here is who is about: the accounts that have said so lately. See
// internal/session/openlist.go and here.go.

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

// hereRequest is POST /here's body: whether the account is here (true by
// default) or leaving.
type hereRequest struct {
	Here *bool `json:"here,omitempty"`
}

// handlePostHere implements POST /here: mark this account present (or away)
// and answer with everybody who is here. Accounts only (ADR 0073).
func (h *handlers) handlePostHere(w http.ResponseWriter, r *http.Request) {
	addr, name, ok := member(w, r)
	if !ok {
		return
	}
	var req hereRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Here != nil && !*req.Here {
		name = ""
	}
	people, err := h.mgr.Here(addr, name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": people, "address": addr})
}

// handleGetHere implements GET /here: who is about, without saying that the
// caller is. For accounts: it is who they could write to.
func (h *handlers) handleGetHere(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := member(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": h.mgr.WhoIsHere()})
}
