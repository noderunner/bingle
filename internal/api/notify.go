package api

import (
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// clientIP is the address the request came from. Forwarding headers are not
// trusted: nothing in front of bingle is known to set them honestly.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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
	info, err := h.mgr.SaveContact(r.Context(), clientIP(r), req.Address, req.ContactID, req.ContactSecret)
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
	info, err := h.mgr.ResendConfirmation(r.Context(), clientIP(r), req.ContactID, req.ContactSecret)
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
// seat's setting. The seat's secret goes in the X-Seat-Token header.
func (h *handlers) handleGetGameNotify(w http.ResponseWriter, r *http.Request) {
	got, err := h.mgr.GameNotify(r.Context(), chi.URLParam(r, "code"), r.URL.Query().Get("player_name"), r.Header.Get("X-Seat-Token"))
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
	SeatToken     string `json:"seat_token"`
	ContactID     int64  `json:"contact_id"`
	ContactSecret string `json:"contact_secret"`
	Enabled       bool   `json:"enabled"`
	AwayMinutes   int    `json:"away_minutes"`
}

// handleSetGameNotify implements PUT /games/{code}/notify: switch the seat's
// turn mail on or off. Switching it on needs the seat's secret and a verified
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
	got, err := h.mgr.SetGameNotify(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.SeatToken, req.ContactID, req.ContactSecret, req.Enabled, req.AwayMinutes)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}
