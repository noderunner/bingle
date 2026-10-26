package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/puzzles"
)

// puzzleDTO is a puzzle as the API lists it. The solution is withheld unless
// asked for, so a client can show a puzzle without spoiling it.
type puzzleDTO struct {
	ID         string          `json:"id"`
	GameDefID  string          `json:"game_def_id"`
	Title      string          `json:"title"`
	ToMove     int             `json:"to_move"`
	WinIn      int             `json:"win_in"`
	Plies      int             `json:"plies"`
	TPS        string          `json:"tps,omitempty"`
	State      json.RawMessage `json:"state"`
	FirstMoves []string        `json:"first_moves,omitempty"`
	Solution   []string        `json:"solution,omitempty"`
}

func puzzleToDTO(p puzzles.Puzzle, reveal bool) puzzleDTO {
	d := puzzleDTO{
		ID: p.ID, GameDefID: p.GameDefID, Title: p.Title, ToMove: p.ToMove,
		WinIn: p.WinIn, Plies: p.Plies, TPS: p.TPS, State: p.State,
	}
	if reveal {
		d.FirstMoves, d.Solution = p.FirstMoves, p.Solution
	}
	return d
}

// handleListPuzzles implements GET /puzzles?game_def_id=&max_win_in=.
func (h *handlers) handleListPuzzles(w http.ResponseWriter, r *http.Request) {
	maxWin, _ := strconv.Atoi(r.URL.Query().Get("max_win_in"))
	list, err := puzzles.Filter(r.URL.Query().Get("game_def_id"), maxWin)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]puzzleDTO, 0, len(list))
	for _, p := range list {
		out = append(out, puzzleToDTO(p, false))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetPuzzle implements GET /puzzles/{id}?reveal=1.
func (h *handlers) handleGetPuzzle(w http.ResponseWriter, r *http.Request) {
	p, ok := puzzles.Get(chi.URLParam(r, "id"))
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(errorResponse{Error: "puzzle not found"})
		return
	}
	writeJSON(w, http.StatusOK, puzzleToDTO(p, r.URL.Query().Get("reveal") == "1"))
}
