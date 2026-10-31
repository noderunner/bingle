package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// clientIP is the address the request came from.
//
// Forwarding headers are believed only when the operator has said there is a
// proxy in front of bingle (BINGLE_TRUST_PROXY); otherwise anyone could claim
// any address. That matters here because this address is what the rate
// limiters count against: behind an unread proxy every request in the world
// shares one address, and the first visitor to trip a limit would trip it for
// everybody.
func (h *handlers) clientIP(r *http.Request) string {
	return h.transport.PeerIP(r)
}

// handleNotifyConfig implements GET /notify/config: whether email
// notifications are set up, and the address they come from (so players can
// whitelist it). No secrets.
func (h *handlers) handleNotifyConfig(w http.ResponseWriter, r *http.Request) {
	from, on := h.mgr.NotifyFrom()
	if !on {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "from": from})
}

// contactRequest is the body of the /notify/contact endpoints.
type contactRequest struct {
	Address       string `json:"address"`
	ContactID     int64  `json:"contact_id"`
	ContactSecret string `json:"contact_secret"`
}

// handleSaveContact implements POST /notify/contact: ask to be notified at an
// address. A confirmation link is mailed to it; nothing else is mailed before
// it is followed. Without contact_id and contact_secret a new contact is made
// (and its secret returned once); with them the address of that contact is
// changed. The address is never returned.
func (h *handlers) handleSaveContact(w http.ResponseWriter, r *http.Request) {
	var req contactRequest
	if !decodeBody(w, r, &req) {
		return
	}
	info, err := h.mgr.SaveContact(r.Context(), h.clientIP(r), req.Address, req.ContactID, req.ContactSecret)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, info)
}

// handleContactStatus implements POST /notify/contact/status.
func (h *handlers) handleContactStatus(w http.ResponseWriter, r *http.Request) {
	var req contactRequest
	if !decodeBody(w, r, &req) {
		return
	}
	info, err := h.mgr.ContactStatus(r.Context(), req.ContactID, req.ContactSecret)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, info)
}

// handleContactResend implements POST /notify/contact/resend: mail the
// confirmation link again.
func (h *handlers) handleContactResend(w http.ResponseWriter, r *http.Request) {
	var req contactRequest
	if !decodeBody(w, r, &req) {
		return
	}
	info, err := h.mgr.ResendConfirmation(r.Context(), h.clientIP(r), req.ContactID, req.ContactSecret)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleContactForget implements POST /notify/contact/forget: delete the
// address and every game setting that used it.
func (h *handlers) handleContactForget(w http.ResponseWriter, r *http.Request) {
	var req contactRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.mgr.ForgetContact(r.Context(), req.ContactID, req.ContactSecret); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleGetGameNotify implements GET /games/{code}/notify?player_name=: the
// seat's setting, for the seat's owner.
func (h *handlers) handleGetGameNotify(w http.ResponseWriter, r *http.Request) {
	got, err := h.mgr.GameNotify(r.Context(), chi.URLParam(r, "code"), r.URL.Query().Get("player_name"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, got)
}

// gameNotifyRequest is PUT /games/{code}/notify's body.
type gameNotifyRequest struct {
	PlayerName    string `json:"player_name"`
	ContactID     int64  `json:"contact_id"`
	ContactSecret string `json:"contact_secret"`
	Enabled       bool   `json:"enabled"`
	AwayMinutes   int    `json:"away_minutes"`
}

// handleSetGameNotify implements PUT /games/{code}/notify: switch the seat's
// turn mail on or off, for the seat's owner. Switching it on needs a verified
// contact (contact_id and its contact_secret) the caller owns.
func (h *handlers) handleSetGameNotify(w http.ResponseWriter, r *http.Request) {
	var req gameNotifyRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	got, err := h.mgr.SetGameNotify(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.ContactID, req.ContactSecret, req.Enabled, req.AwayMinutes)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}
