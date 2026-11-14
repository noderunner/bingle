package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// handleRatedFamilies implements GET /ratings: the games with a leaderboard.
func (h *handlers) handleRatedFamilies(w http.ResponseWriter, r *http.Request) {
	list, err := h.mgr.RatedFamilies(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": list})
}

// handleLeaderboard implements GET /ratings/{family}?limit=: one game's
// leaderboard, the highest rating first.
func (h *handlers) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	family := chi.URLParam(r, "family")
	list, err := h.mgr.Leaderboard(r.Context(), family, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"family": family, "players": list})
}

// handlePlayerRatings implements GET /players/{handle}/ratings: an account's
// ratings. Others see those good enough for a leaderboard; the account itself
// sees all of them.
func (h *handlers) handlePlayerRatings(w http.ResponseWriter, r *http.Request) {
	list, err := h.mgr.PlayerRatings(r.Context(), chi.URLParam(r, "handle"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ratings": list})
}

// handlePlayerBadges implements GET /players/{handle}/badges: the badges an
// account holds (ADR 0130), for accounts only. The holder also sees those that
// wait for their game to end, and the room each was earned in.
func (h *handlers) handlePlayerBadges(w http.ResponseWriter, r *http.Request) {
	list, err := h.mgr.PlayerBadges(r.Context(), chi.URLParam(r, "handle"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"badges": list})
}
