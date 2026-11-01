package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// tutorialSummary is one entry of a game's list of tutorials.
type tutorialSummary struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	Order   int    `json:"order,omitempty"`
	Game    string `json:"game"`
	Steps   int    `json:"steps"`
}

// handleListTutorials implements GET /game-defs/{id}/tutorials: the
// tutorials of the family the game belongs to, by kind then order.
func (h *handlers) handleListTutorials(w http.ResponseWriter, r *http.Request) {
	family, list, err := h.mgr.Tutorials(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]tutorialSummary, 0, len(list))
	for _, t := range list {
		out = append(out, tutorialSummary{ID: t.ID, Kind: t.Kind, Title: t.Title, Summary: t.Summary, Order: t.Order, Game: t.Game, Steps: len(t.Steps)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"family": family, "tutorials": out})
}

// handleGetTutorial implements GET /game-defs/{id}/tutorials/{tutorial}: one
// tutorial with its steps, for the tutorial page.
func (h *handlers) handleGetTutorial(w http.ResponseWriter, r *http.Request) {
	_, t, err := h.mgr.Tutorial(chi.URLParam(r, "id"), chi.URLParam(r, "tutorial"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// startTutorialRequest is the body of POST /tutorials.
type startTutorialRequest struct {
	GameDefID  string `json:"game_def_id"` // any game of the family
	Tutorial   string `json:"tutorial"`
	PlayerName string `json:"player_name"`
}

// handleStartTutorial implements POST /tutorials: open a room for a
// tutorial, with the learner in every seat; it answers as POST /games does.
func (h *handlers) handleStartTutorial(w http.ResponseWriter, r *http.Request) {
	var req startTutorialRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}
	if strings.TrimSpace(req.GameDefID) == "" || strings.TrimSpace(req.Tutorial) == "" {
		writeError(w, errors.New("api: game_def_id and tutorial are required"))
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	res, err := h.mgr.StartTutorial(r.Context(), req.GameDefID, req.Tutorial, strings.TrimSpace(req.PlayerName))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, createGameResponse{RoomCode: res.RoomCode, Seat: res.Seat, Color: res.Color, Status: res.Status})
}
