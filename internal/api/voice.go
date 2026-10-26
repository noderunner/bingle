package api

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // coturn's REST credentials are defined as HMAC-SHA1
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// RTCConfig describes the network helpers voice chat uses: STUN servers, which
// let two browsers find the address they can be reached on, and optionally a
// TURN relay (coturn) for networks that block direct connections.
type RTCConfig struct {
	Stun       []string
	TurnURLs   []string
	TurnSecret string
	// TurnTTL is how long the credentials handed out for the relay last.
	TurnTTL time.Duration
}

// iceServer is one entry of RTCPeerConnection's iceServers.
type iceServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// iceServers builds the list for a client: the STUN servers, and the TURN
// relay with credentials made for now. They follow coturn's "use-auth-secret"
// scheme: the user name is the expiry time, the password the HMAC-SHA1 of it
// under the shared secret, so the relay needs no list of users and the
// credentials stop working by themselves.
func (c RTCConfig) iceServers(now time.Time) []iceServer {
	out := []iceServer{}
	if len(c.Stun) > 0 {
		out = append(out, iceServer{URLs: c.Stun})
	}
	if len(c.TurnURLs) > 0 && c.TurnSecret != "" {
		ttl := c.TurnTTL
		if ttl <= 0 {
			ttl = 6 * time.Hour
		}
		user := fmt.Sprintf("%d:bingle", now.Add(ttl).Unix())
		mac := hmac.New(sha1.New, []byte(c.TurnSecret))
		mac.Write([]byte(user))
		out = append(out, iceServer{URLs: c.TurnURLs, Username: user, Credential: base64.StdEncoding.EncodeToString(mac.Sum(nil))})
	}
	return out
}

// WithRTC sets the STUN/TURN settings voice chat clients are given.
func WithRTC(c RTCConfig) Option {
	return func(h *handlers) { h.rtc = c }
}

// handleRTCConfig implements GET /rtc/config: the iceServers for a voice call.
// The TURN credentials are time-limited.
func (h *handlers) handleRTCConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"ice_servers": h.rtc.iceServers(time.Now())})
}

// handleGetVoice implements GET /games/{code}/voice: who is in the call.
func (h *handlers) handleGetVoice(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"peers": h.mgr.VoicePeers(chi.URLParam(r, "code"))})
}

// voiceRequest is POST /games/{code}/voice's body.
type voiceRequest struct {
	PlayerName string `json:"player_name"`
	Active     bool   `json:"active"`
}

// handlePostVoice implements POST /games/{code}/voice: a player joins the call
// (or leaves it with active=false). Posting active=true again renews the place,
// which lapses after 45 seconds otherwise.
func (h *handlers) handlePostVoice(w http.ResponseWriter, r *http.Request) {
	var req voiceRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	peers, err := h.mgr.SetVoice(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.Active)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"peers": peers})
}

// signalRequest is POST /games/{code}/voice/signal's body.
type signalRequest struct {
	PlayerName string          `json:"player_name"`
	To         int             `json:"to"`
	Kind       string          `json:"kind"`
	Data       json.RawMessage `json:"data"`
}

// handlePostVoiceSignal implements POST /games/{code}/voice/signal: relay a
// message (offer, answer or ICE candidate) to another seat in the call.
func (h *handlers) handlePostVoiceSignal(w http.ResponseWriter, r *http.Request) {
	var req signalRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	if err := h.mgr.SignalVoice(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.To, req.Kind, req.Data); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
