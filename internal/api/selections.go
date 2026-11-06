package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// selectionsResponse is GET /games/{code}/selections' body (ADR 0114).
type selectionsResponse struct {
	Seat     int             `json:"seat"`
	Ply      int             `json:"ply"`
	Made     []string        `json:"made"`
	Options  []legalMoveDTO  `json:"options"`
	Complete bool            `json:"complete"`
	MayEnd   bool            `json:"may_end,omitempty"`
	State    json.RawMessage `json:"state,omitempty"`
}

// handleSelections implements GET /games/{code}/selections?made=…&ply=N: what
// the impulse seat may select after the selections made (joined by "; "), the
// position after them as that seat sees it, and whether they are a move.
func (h *handlers) handleSelections(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	var ply *int
	if raw := r.URL.Query().Get("ply"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, errors.New("api: ply must be an integer"))
			return
		}
		ply = &n
	}
	view, err := h.mgr.Selections(r.Context(), code, r.URL.Query().Get("made"), ply)
	if err != nil {
		writeError(w, err)
		return
	}
	out := selectionsResponse{Seat: view.Seat, Ply: view.Ply, Made: view.Made, Options: legalMoveDTOs(view.Options),
		Complete: view.Complete, MayEnd: view.MayEnd}
	if view.State != "" {
		out.State = json.RawMessage(view.State)
	}
	writeJSON(w, http.StatusOK, out)
}
