package api

import (
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/noderunner/bingle/internal/session"
)

// Direct messages between two browsers that are here, and the voice call they
// may put up: REST in, WebSocket out (/ws-dm), as a game's voice chat does.
// Every call carries the browser's own secret, which says who is writing
// without anybody being able to write as them; see internal/session/dm.go.

// dmRequest is the body of POST /dm: a message for the address To.
type dmRequest struct {
	Secret string `json:"secret"`
	To     string `json:"to"`
	Text   string `json:"text"`
}

// dmReadRequest is the body of POST /dm/history: the correspondence with one
// person, after the message After (0 for all of it).
type dmReadRequest struct {
	Secret string `json:"secret"`
	With   string `json:"with"`
	After  int64  `json:"after,omitempty"`
}

// dmCallRequest is the body of POST /dm/call: ring (or answer) the address To,
// or hang up with join=false.
type dmCallRequest struct {
	Secret string `json:"secret"`
	To     string `json:"to"`
	Join   bool   `json:"join"`
}

// dmSignalRequest is the body of POST /dm/signal: one WebRTC message for the
// person this browser is calling.
type dmSignalRequest struct {
	Secret string          `json:"secret"`
	To     string          `json:"to"`
	Kind   string          `json:"kind"`
	Data   json.RawMessage `json:"data"`
}

// checkSecret reports whether the secret is one a browser could have made.
func checkSecret(w http.ResponseWriter, secret string) bool {
	if secret == "" || len(secret) > maxSecretRunes {
		writeError(w, errBadSecret)
		return false
	}
	return true
}

// handlePostDirect implements POST /dm: write to somebody who is here.
func (h *handlers) handlePostDirect(w http.ResponseWriter, r *http.Request) {
	var req dmRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if !checkSecret(w, req.Secret) {
		return
	}
	msg, err := h.mgr.SendDirect(req.Secret, req.To, req.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

// handleDirectHistory implements POST /dm/history: what was said with one
// person. It is a POST because the browser's secret belongs in a body, not in
// a URL that ends up in logs.
func (h *handlers) handleDirectHistory(w http.ResponseWriter, r *http.Request) {
	var req dmReadRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if !checkSecret(w, req.Secret) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": h.mgr.DirectMessages(req.Secret, req.With, req.After)})
}

// handleDirectThreads implements POST /dm/threads: the correspondences this
// browser has, newest first.
func (h *handlers) handleDirectThreads(w http.ResponseWriter, r *http.Request) {
	var req dmReadRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if !checkSecret(w, req.Secret) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"threads": h.mgr.DirectThreads(req.Secret),
		"address": session.Address(req.Secret),
	})
}

// handleDirectCall implements POST /dm/call: ring somebody (or answer them),
// or hang up.
func (h *handlers) handleDirectCall(w http.ResponseWriter, r *http.Request) {
	var req dmCallRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if !checkSecret(w, req.Secret) {
		return
	}
	if !req.Join {
		h.mgr.LeaveDirectCall(req.Secret)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if err := h.mgr.JoinDirectCall(req.Secret, req.To); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDirectSignal implements POST /dm/signal: relay one WebRTC message to
// the person this browser is calling.
func (h *handlers) handleDirectSignal(w http.ResponseWriter, r *http.Request) {
	var req dmSignalRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if !checkSecret(w, req.Secret) {
		return
	}
	if err := h.mgr.SignalDirect(req.Secret, req.To, req.Kind, req.Data); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// dmHello is the first (and only) message a /ws-dm client sends: the secret of
// the browser whose messages it wants.
type dmHello struct {
	Type   string `json:"type"`
	Secret string `json:"secret"`
}

// NewDirectWSHandler returns the handler for GET /ws-dm: a browser says who it
// is with its secret, and is then pushed its messages, the ringing of a call
// and the WebRTC messages of one.
func NewDirectWSHandler(mgr *session.Manager) http.HandlerFunc {
	h := &handlers{mgr: mgr}
	return h.handleDirectWS
}

func (h *handlers) handleDirectWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	// The secret cannot travel in the URL (logs), so it comes as the first
	// message. Nothing else is ever read from this socket.
	var hello dmHello
	if err := wsjson.Read(r.Context(), conn, &hello); err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "say who you are first")
		return
	}
	if hello.Secret == "" || len(hello.Secret) > maxSecretRunes {
		_ = conn.Close(websocket.StatusPolicyViolation, "a secret is needed")
		return
	}
	address := session.Address(hello.Secret)
	ctx := conn.CloseRead(r.Context())
	events, unsubscribe := h.mgr.SubscribeDirect(address)
	defer unsubscribe()
	if err := wsjson.Write(ctx, conn, session.Event{Type: "ready", Data: map[string]string{"address": address}}); err != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if err := wsjson.Write(ctx, conn, ev); err != nil {
				return
			}
		}
	}
}
