package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/store"
)

// A game set up for later (ADR 0109): each route is for an account, about the room in its path.

// scheduleRequest is the body of the routes.
type scheduleRequest struct {
	Slot  int64       `json:"slot,omitempty"`
	On    bool        `json:"on,omitempty"`
	Slots []time.Time `json:"slots,omitempty"`
}

func scheduleRoute(w http.ResponseWriter, r *http.Request, act func(store.Account, string, scheduleRequest) error) {
	me, ok := memberAccount(w, r)
	if !ok {
		return
	}
	var req scheduleRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := act(me, chi.URLParam(r, "code"), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleScheduleVote implements POST /games/{code}/schedule/vote: whether I can make a time.
func (h *handlers) handleScheduleVote(w http.ResponseWriter, r *http.Request) {
	scheduleRoute(w, r, func(me store.Account, room string, req scheduleRequest) error {
		return h.mgr.VoteSlot(r.Context(), me, room, req.Slot, req.On)
	})
}

// handleScheduleDecide implements POST /games/{code}/schedule/decide: the creator stops waiting.
func (h *handlers) handleScheduleDecide(w http.ResponseWriter, r *http.Request) {
	scheduleRoute(w, r, func(me store.Account, room string, _ scheduleRequest) error {
		return h.mgr.DecideSchedule(r.Context(), me, room)
	})
}

// handleScheduleSlots implements POST /games/{code}/schedule/slots: the creator offers new times.
func (h *handlers) handleScheduleSlots(w http.ResponseWriter, r *http.Request) {
	scheduleRoute(w, r, func(me store.Account, room string, req scheduleRequest) error {
		return h.mgr.OfferNewTimes(r.Context(), me, room, req.Slots)
	})
}

// handleScheduleCancel implements POST /games/{code}/schedule/cancel.
func (h *handlers) handleScheduleCancel(w http.ResponseWriter, r *http.Request) {
	scheduleRoute(w, r, func(me store.Account, room string, _ scheduleRequest) error {
		return h.mgr.CancelSchedule(r.Context(), me, room)
	})
}

// handleScheduleICS implements GET /games/{code}/schedule.ics: the game's time for a calendar.
func (h *handlers) handleScheduleICS(w http.ResponseWriter, r *http.Request) {
	if _, ok := memberAccount(w, r); !ok {
		return
	}
	code := chi.URLParam(r, "code")
	body, err := h.mgr.GameCalendar(r.Context(), code, strings.TrimSuffix(h.inviteURL(r, code), "/game/"+code))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="bingle-game.ics"`)
	_, _ = w.Write([]byte(body))
}
