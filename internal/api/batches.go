package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/batch"
)

// WithBatches mounts the bot practice batch endpoints (/batches) backed by bm.
func WithBatches(bm *batch.Manager) Option { return func(h *handlers) { h.batches = bm } }

// batchRequest is POST /batches' body.
type batchRequest struct {
	Name string `json:"name"`
	batch.Spec
}

func (h *handlers) handleBatchStart(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if !decodeBody(w, r, &req) {
		return
	}
	id, err := h.batches.Start(r.Context(), req.Name, req.Spec)
	if err != nil {
		writeError(w, err)
		return
	}
	info, err := h.batches.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

func (h *handlers) handleBatchList(w http.ResponseWriter, r *http.Request) {
	list, err := h.batches.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"batches": list, "default_workers": batch.DefaultWorkers()})
}

func batchID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad batch id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func (h *handlers) handleBatchGet(w http.ResponseWriter, r *http.Request) {
	id, ok := batchID(w, r)
	if !ok {
		return
	}
	info, err := h.batches.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleBatchReport returns the batch's status together with the aggregated
// report of the games finished so far.
func (h *handlers) handleBatchReport(w http.ResponseWriter, r *http.Request) {
	id, ok := batchID(w, r)
	if !ok {
		return
	}
	info, err := h.batches.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	rep, err := h.batches.Report(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"batch": info, "report": rep})
}

func (h *handlers) handleBatchGames(w http.ResponseWriter, r *http.Request) {
	id, ok := batchID(w, r)
	if !ok {
		return
	}
	games, err := h.batches.Games(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": games})
}

func (h *handlers) handleBatchEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := batchID(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(chi.URLParam(r, "n"))
	if err != nil {
		http.Error(w, "bad game number", http.StatusBadRequest)
		return
	}
	ev, err := h.batches.Events(r.Context(), id, n)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": ev})
}

func (h *handlers) handleBatchCancel(w http.ResponseWriter, r *http.Request) {
	id, ok := batchID(w, r)
	if !ok {
		return
	}
	if err := h.batches.Cancel(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	info, _ := h.batches.Get(r.Context(), id)
	writeJSON(w, http.StatusOK, info)
}

func (h *handlers) handleBatchDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := batchID(w, r)
	if !ok {
		return
	}
	if err := h.batches.Delete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
