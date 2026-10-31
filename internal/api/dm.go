package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/session"
)

// Direct messages between two accounts that are here, and the voice call they
// may put up: REST in, WebSocket out (/ws-dm), as a game's voice chat does.
// The sender is the signed-in account (ADR 0073); see internal/session/dm.go.

// errNeedsAccount is the answer to a direct message, a direct call or "who
// is here" from a guest: they are an account's.
var errNeedsAccount = errors.New("api: direct messages and who is here are for accounts: sign in")

// member is the signed-in account of the request: its address and the name
// it goes by ("@ana"). It answers the request itself when there is none.
func member(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	p, ok := auth.From(r.Context())
	if !ok || p.Account == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": errNeedsAccount.Error()})
		return "", "", false
	}
	return session.AddressOf(p.ID), "@" + p.Account.Handle, true
}

// dmRequest is the body of POST /dm: a message for the address To.
type dmRequest struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

// dmReadRequest is the body of POST /dm/history: the correspondence with one
// person, after the message After (0 for all of it).
type dmReadRequest struct {
	With  string `json:"with"`
	After int64  `json:"after,omitempty"`
}

// dmCallRequest is the body of POST /dm/call: ring (or answer) the address To,
// or hang up with join=false.
type dmCallRequest struct {
	To   string `json:"to"`
	Join bool   `json:"join"`
}

// dmSignalRequest is the body of POST /dm/signal: one WebRTC message for the
// person this account is calling.
type dmSignalRequest struct {
	To   string          `json:"to"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// handlePostDirect implements POST /dm: write to somebody who is here.
func (h *handlers) handlePostDirect(w http.ResponseWriter, r *http.Request) {
	from, _, ok := member(w, r)
	if !ok {
		return
	}
	var req dmRequest
	if !decodeBody(w, r, &req) {
		return
	}
	msg, err := h.mgr.SendDirect(from, req.To, req.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

// handleDirectHistory implements POST /dm/history: what was said with one
// person.
func (h *handlers) handleDirectHistory(w http.ResponseWriter, r *http.Request) {
	addr, _, ok := member(w, r)
	if !ok {
		return
	}
	var req dmReadRequest
	if !decodeBody(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": h.mgr.DirectMessages(addr, req.With, req.After)})
}

// handleDirectThreads implements POST /dm/threads: the correspondences this
// account has, newest first, and its own address.
func (h *handlers) handleDirectThreads(w http.ResponseWriter, r *http.Request) {
	addr, _, ok := member(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"threads": h.mgr.DirectThreads(addr), "address": addr})
}

// handleDirectCall implements POST /dm/call: ring somebody (or answer them),
// or hang up.
func (h *handlers) handleDirectCall(w http.ResponseWriter, r *http.Request) {
	from, _, ok := member(w, r)
	if !ok {
		return
	}
	var req dmCallRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if !req.Join {
		h.mgr.LeaveDirectCall(from)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if err := h.mgr.JoinDirectCall(from, req.To); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDirectSignal implements POST /dm/signal: relay one WebRTC message to
// the person this account is calling.
func (h *handlers) handleDirectSignal(w http.ResponseWriter, r *http.Request) {
	from, _, ok := member(w, r)
	if !ok {
		return
	}
	var req dmSignalRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.mgr.SignalDirect(from, req.To, req.Kind, req.Data); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// NewDirectWSHandler returns the handler for GET /ws-dm: the signed-in
// account is pushed its messages, the ringing of a call and the WebRTC
// messages of one. Its session says who it is: there is nothing to send.
func NewDirectWSHandler(mgr *session.Manager) http.HandlerFunc {
	h := &handlers{mgr: mgr}
	return h.handleDirectWS
}

func (h *handlers) handleDirectWS(w http.ResponseWriter, r *http.Request) {
	address, _, ok := member(w, r)
	if !ok {
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
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
