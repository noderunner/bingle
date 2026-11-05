package api

import (
	"net/http"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/store"
)

// The friends list (ADR 0105): REST in, and a "friends" event on /ws-dm when it changes.

// friendRequest is the body of the routes about one person.
type friendRequest struct {
	Handle string `json:"handle"`
	Accept bool   `json:"accept,omitempty"`
	Room   string `json:"room,omitempty"`
}

// friendSettings is the body of GET and PUT /friends/settings.
type friendSettings struct {
	MailRequests bool `json:"mail_requests"`
}

// memberAccount is the signed-in account of the request, answering the request if there is none.
func memberAccount(w http.ResponseWriter, r *http.Request) (store.Account, bool) {
	p, ok := auth.From(r.Context())
	if !ok || p.Account == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": errNeedsAccount.Error()})
		return store.Account{}, false
	}
	return *p.Account, true
}

// friendRoute decodes a request about one person and runs act on it.
func friendRoute(w http.ResponseWriter, r *http.Request, act func(store.Account, friendRequest) error) {
	me, ok := memberAccount(w, r)
	if !ok {
		return
	}
	var req friendRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := act(me, req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleGetFriends implements GET /friends.
func (h *handlers) handleGetFriends(w http.ResponseWriter, r *http.Request) {
	me, ok := memberAccount(w, r)
	if !ok {
		return
	}
	list, err := h.mgr.FriendsOf(r.Context(), me.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleFriendRequest implements POST /friends/request.
func (h *handlers) handleFriendRequest(w http.ResponseWriter, r *http.Request) {
	friendRoute(w, r, func(me store.Account, req friendRequest) error {
		return h.mgr.RequestFriend(r.Context(), me, req.Handle, h.clientIP(r))
	})
}

// handleFriendAnswer implements POST /friends/answer.
func (h *handlers) handleFriendAnswer(w http.ResponseWriter, r *http.Request) {
	friendRoute(w, r, func(me store.Account, req friendRequest) error {
		return h.mgr.AnswerFriend(r.Context(), me, req.Handle, req.Accept)
	})
}

// handleFriendRemove implements POST /friends/remove.
func (h *handlers) handleFriendRemove(w http.ResponseWriter, r *http.Request) {
	friendRoute(w, r, func(me store.Account, req friendRequest) error {
		return h.mgr.RemoveFriend(r.Context(), me, req.Handle)
	})
}

// handleFriendInvite implements POST /friends/invite.
func (h *handlers) handleFriendInvite(w http.ResponseWriter, r *http.Request) {
	friendRoute(w, r, func(me store.Account, req friendRequest) error {
		return h.mgr.InviteFriend(r.Context(), me, req.Handle, req.Room)
	})
}

// handleFriendDismiss implements POST /friends/dismiss: an invitation goes away.
func (h *handlers) handleFriendDismiss(w http.ResponseWriter, r *http.Request) {
	friendRoute(w, r, func(me store.Account, req friendRequest) error {
		return h.mgr.DismissInvite(r.Context(), me, req.Room)
	})
}

// handleGetFriendSettings implements GET /friends/settings.
func (h *handlers) handleGetFriendSettings(w http.ResponseWriter, r *http.Request) {
	me, ok := memberAccount(w, r)
	if !ok {
		return
	}
	on, err := h.mgr.FriendRequestMail(r.Context(), me.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, friendSettings{MailRequests: on})
}

// handlePutFriendSettings implements PUT /friends/settings.
func (h *handlers) handlePutFriendSettings(w http.ResponseWriter, r *http.Request) {
	me, ok := memberAccount(w, r)
	if !ok {
		return
	}
	var req friendSettings
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.mgr.SetFriendRequestMail(r.Context(), me.ID, req.MailRequests); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}
