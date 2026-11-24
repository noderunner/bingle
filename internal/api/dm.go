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

// Direct messages between two friends who are here, and the voice call they
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

// friendsOnly refuses a message or a call to anybody but a friend (ADR 0105). It answers the request
// itself when it says no.
func (h *handlers) friendsOnly(w http.ResponseWriter, r *http.Request, to string) bool {
	p, ok := auth.From(r.Context())
	if !ok || p.Account == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": errNeedsAccount.Error()})
		return false
	}
	if err := h.mgr.MustBeFriends(r.Context(), p.Account.ID, to); err != nil {
		writeError(w, err)
		return false
	}
	return true
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
	Kind string          `json:"kind"` // "offer", "answer", "ice" or "retry"
	Data json.RawMessage `json:"data"`
}

// dmHistoryResponse is POST /dm/history's answer: the messages, oldest first.
type dmHistoryResponse struct {
	Messages []session.DirectMessage `json:"messages"`
}

// dmThreadsResponse is POST /dm/threads' answer.
type dmThreadsResponse struct {
	// Threads are the account's correspondences, newest first.
	Threads []session.DirectThread `json:"threads"`
	// Address is the account's own address, the one others write to.
	Address string `json:"address"`
}

// okResponse is the answer of an action that has nothing more to say.
type okResponse struct {
	OK bool `json:"ok"`
}

// dmReady is the data of /ws-dm's first event, "ready": the socket is open,
// and Address is the account's own address.
type dmReady struct {
	Address string `json:"address"`
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
	if !h.friendsOnly(w, r, req.To) {
		return
	}
	msg, err := h.mgr.SendDirect(withReader(r), from, req.To, req.Text)
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
	writeJSON(w, http.StatusOK, dmHistoryResponse{Messages: h.mgr.DirectMessages(withReader(r), addr, req.With, req.After)})
}

// handleDirectThreads implements POST /dm/threads: the correspondences this
// account has, newest first, and its own address.
func (h *handlers) handleDirectThreads(w http.ResponseWriter, r *http.Request) {
	addr, _, ok := member(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dmThreadsResponse{Threads: h.mgr.DirectThreads(addr), Address: addr})
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
		writeJSON(w, http.StatusOK, okResponse{OK: true})
		return
	}
	if !h.friendsOnly(w, r, req.To) {
		return
	}
	if err := h.mgr.JoinDirectCall(from, req.To); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okResponse{OK: true})
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
	writeJSON(w, http.StatusOK, okResponse{OK: true})
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
	ctx := conn.CloseRead(withReader(r))
	events, unsubscribe := h.mgr.SubscribeDirect(address)
	defer unsubscribe()
	// The messages written to this account are put into its reader's
	// language while the socket is open (ADR 0143).
	lang := h.mgr.TranslationLanguage(ctx)
	defer h.mgr.ReadDirect(ctx, address)()
	if err := wsjson.Write(ctx, conn, session.Event{Type: "ready", Data: dmReady{Address: address}}); err != nil {
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
			if !keepsTranslation(ev, lang) {
				continue
			}
			if err := wsjson.Write(ctx, conn, ev); err != nil {
				return
			}
		}
	}
}
