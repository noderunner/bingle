package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/tutorial"
)

// tutorialSummary is one entry of a game's list of tutorials.
type tutorialSummary struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	Order   int    `json:"order,omitempty"`
	// Series is the series the tutorial is part of, if it names one; the
	// list has a series' tutorials in its order.
	Series string `json:"series,omitempty"`
	Game   string `json:"game"`
	Steps  int    `json:"steps"`
	// Done says the signed-in account has finished it.
	Done bool `json:"done,omitempty"`
}

// handleListTutorials implements GET /game-defs/{id}/tutorials: the
// tutorials offered with this game, by kind then order.
func (h *handlers) handleListTutorials(w http.ResponseWriter, r *http.Request) {
	family, list, err := h.mgr.TutorialsFor(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	var done map[string]bool
	if p, ok := auth.From(r.Context()); ok && p.Account != nil {
		if done, err = h.mgr.TutorialsDone(r.Context(), p.Account.ID, family); err != nil {
			writeError(w, err)
			return
		}
	}
	out := make([]tutorialSummary, 0, len(list))
	for _, t := range list {
		out = append(out, tutorialSummary{ID: t.ID, Kind: t.Kind, Title: t.Title, Summary: t.Summary, Order: t.Order, Series: t.Series,
			Game: t.Game, Steps: len(t.Steps), Done: done[t.ID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"family": family, "tutorials": out})
}

// tutorialResponse is one tutorial as written, with each step's guide: the
// selections of the learner's moves, for a client to point them at, and the
// position a step shows.
type tutorialResponse struct {
	tutorial.Tutorial
	Guide []tutorial.StepGuide `json:"guide"`
	// Next is the tutorial after it in its series, if any.
	Next *nextTutorial `json:"next,omitempty"`
}

// nextTutorial is the tutorial a finished one leads on to.
type nextTutorial struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// handleGetTutorial implements GET /game-defs/{id}/tutorials/{tutorial}: one
// tutorial with its steps and their guide, for the tutorial page.
func (h *handlers) handleGetTutorial(w http.ResponseWriter, r *http.Request) {
	t, guide, err := h.mgr.TutorialGuide(chi.URLParam(r, "id"), chi.URLParam(r, "tutorial"))
	if err != nil {
		writeError(w, err)
		return
	}
	res := tutorialResponse{Tutorial: t, Guide: guide}
	if next, ok := h.mgr.NextTutorial(t); ok {
		res.Next = &nextTutorial{ID: next.ID, Title: next.Title}
	}
	writeJSON(w, http.StatusOK, res)
}

// tutorialDoneRequest is the body of POST /tutorials/done.
type tutorialDoneRequest struct {
	RoomCode string `json:"room_code"`
}

// handleTutorialDone implements POST /tutorials/done: the learner has come to the
// end of the tutorial a room plays, and a signed-in account keeps that, for the
// marks on the lists. A guest's browser keeps its own, so for a guest nothing is
// recorded; nor is anything for a room whose script has not all been played.
func (h *handlers) handleTutorialDone(w http.ResponseWriter, r *http.Request) {
	var req tutorialDoneRequest
	if !decodeBody(w, r, &req) {
		return
	}
	p, ok := auth.From(r.Context())
	if !ok || p.Account == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"recorded": false})
		return
	}
	recorded, err := h.mgr.TutorialDone(r.Context(), *p.Account, strings.TrimSpace(req.RoomCode))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": recorded})
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
