package api

import (
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/session"
)

// handleWS implements GET /ws/{code}: validate name/role exactly like
// handleJoinGame's, join the room, and — only once that has succeeded —
// upgrade to a WebSocket connection that pushes a "joined" event first,
// followed by every subsequent session.Event published for this room until
// the client disconnects. See docs/history/plan.md section 6.9 for the full
// contract.
//
// With only=voice the socket belongs to a voice call, not to a page: it
// carries just the call's events (who is in it and the messages for the
// player's seat), it stays open while the player moves from page to page, and
// it does not count as the player looking at the game.
//
// Every rejection here (bad input, unknown room, seats full, ...) happens
// BEFORE websocket.Accept, as a plain http.Error — an HTTP error response
// can no longer be sent once the WebSocket upgrade has succeeded.
func (h *handlers) handleWS(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	name := r.URL.Query().Get("name")
	if err := checkName(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var role session.Role
	switch strings.TrimSpace(r.URL.Query().Get("role")) {
	case "", string(session.RolePlayer):
		role = session.RolePlayer
	case string(session.RoleSpectator):
		role = session.RoleSpectator
	default:
		http.Error(w, "api: invalid role", http.StatusBadRequest)
		return
	}

	result, err := h.mgr.JoinGame(r.Context(), code, name, role)
	if err != nil {
		http.Error(w, err.Error(), statusForError(err))
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		// websocket.Accept has already written an HTTP error response.
		return
	}
	defer func() { _ = conn.CloseNow() }()

	// This connection is server-push only: clients never send messages, so
	// hand reading over to a background goroutine (which also keeps
	// ping/pong/close frames flowing) and use the context it returns, which
	// is cancelled the moment the connection closes for any reason — that's
	// what causes the wsjson.Write below to start failing and the loop to
	// exit once the client disconnects.
	ctx := conn.CloseRead(r.Context())

	events, unsubscribe := h.mgr.Subscribe(code)
	defer unsubscribe()
	voiceOnly := r.URL.Query().Get("only") == "voice"
	// While a player's page is connected no "your turn" mail is sent to them.
	if !voiceOnly && result.Role == session.RolePlayer && result.Seat > 0 {
		defer h.mgr.TrackConnection(code, result.Seat)()
	}

	// The voice call's socket also receives the messages meant for its seat
	// (and only those): a nil channel for any other socket never fires.
	var rtc <-chan session.Event
	if voiceOnly && result.Role == session.RolePlayer && result.Seat > 0 {
		var stop func()
		rtc, stop = h.mgr.SubscribeRTC(code, result.Seat)
		defer stop()
	}

	// Subscribed before the player is told they are joined: a client that waits
	// for "joined" then loses nothing (a call must not miss the first offer).
	joined := joinGameResponse{
		Role:   string(result.Role),
		Seat:   result.Seat,
		Color:  result.Color,
		Status: result.Status,
	}
	if err := wsjson.Write(ctx, conn, session.Event{Type: "joined", Data: joined}); err != nil {
		return
	}

	for {
		var ev session.Event
		var ok bool
		select {
		case ev, ok = <-events:
		case ev, ok = <-rtc:
		}
		if !ok {
			return
		}
		if voiceOnly && ev.Type != "voice" && ev.Type != "rtc" {
			continue
		}
		if err := wsjson.Write(ctx, conn, ev); err != nil {
			return
		}
	}
}

// handleGlobalChatWS implements GET /ws-chat: a server-push stream of the
// global chat room's messages, for the home page. Nobody joins anything.
func (h *handlers) handleGlobalChatWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
	ctx := conn.CloseRead(r.Context())
	events, unsubscribe := h.mgr.SubscribeGlobalChat()
	defer unsubscribe()
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
