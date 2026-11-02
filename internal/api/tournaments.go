package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// tournamentID reads the id in a tournament's path; one that cannot be an id is
// simply not there.
func tournamentID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, store.ErrNotFound
	}
	return id, nil
}

// handleTournaments implements GET /tournaments.
func (h *handlers) handleTournaments(w http.ResponseWriter, r *http.Request) {
	list, err := h.mgr.ListTournaments(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tournaments": list})
}

// handleCreateTournament implements POST /tournaments: a new draft.
func (h *handlers) handleCreateTournament(w http.ResponseWriter, r *http.Request) {
	var s session.TournamentSettings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&s); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.mgr.CreateTournament(r.Context(), s)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// handleTournament implements GET /tournaments/{id}.
func (h *handlers) handleTournament(w http.ResponseWriter, r *http.Request) {
	h.tournamentAction(h.mgr.Tournament)(w, r)
}

// handleUpdateTournament implements PUT /tournaments/{id}: a draft's settings.
func (h *handlers) handleUpdateTournament(w http.ResponseWriter, r *http.Request) {
	id, err := tournamentID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var s session.TournamentSettings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&s); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.mgr.UpdateTournament(r.Context(), id, s)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// tournamentAction serves a call that takes nothing but the tournament's id: open,
// start, enter, abandon.
func (h *handlers) tournamentAction(do func(context.Context, int64) (session.TournamentDetail, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := tournamentID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		out, err := do(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// handleLeaveTournament implements POST /tournaments/{id}/leave: the caller leaves,
// or its organiser removes the player named by {handle}.
func (h *handlers) handleLeaveTournament(w http.ResponseWriter, r *http.Request) {
	id, err := tournamentID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		Handle string `json:"handle"`
	}
	// The body is optional: leaving oneself needs none.
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
			writeError(w, errors.New("api: the body should be {handle}"))
			return
		}
	}
	out, err := h.mgr.LeaveTournament(r.Context(), id, req.Handle)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
