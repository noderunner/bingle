package api

import (
	"net/http"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// The friends list (ADR 0105): REST in, and a "friends" event on /ws-dm when it changes.

// friendRequest is the body of the routes about one person or one
// invitation: POST /friends/request, /answer, /remove, /invite,
// /invite-answer, /dismiss, /block and /unblock.
type friendRequest struct {
	// Handle is the person's (request, answer, remove, invite, block, unblock).
	Handle string `json:"handle,omitempty"`
	// Accept is the answer (answer, invite-answer).
	Accept bool `json:"accept,omitempty"`
	// Room is the game's room code (invite, invite-answer, dismiss).
	Room string `json:"room,omitempty"`
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
	writeJSON(w, http.StatusOK, okResponse{OK: true})
}

// handleGetFriends implements GET /friends.
func (h *handlers) handleGetFriends(w http.ResponseWriter, r *http.Request) {
	me, ok := memberAccount(w, r)
	if !ok {
		return
	}
	list, err := h.mgr.FriendsOf(r.Context(), me.ID, session.AddressOf(me.PrincipalID))
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

// handleInviteAnswer implements POST /friends/invite-answer: yes or no to an invitation.
func (h *handlers) handleInviteAnswer(w http.ResponseWriter, r *http.Request) {
	friendRoute(w, r, func(me store.Account, req friendRequest) error {
		return h.mgr.AnswerInvite(r.Context(), me, req.Room, req.Accept)
	})
}

// handleFriendDismiss implements POST /friends/dismiss: an invitation goes away.
func (h *handlers) handleFriendDismiss(w http.ResponseWriter, r *http.Request) {
	friendRoute(w, r, func(me store.Account, req friendRequest) error {
		return h.mgr.DismissInvite(r.Context(), me, req.Room)
	})
}

// handleFriendBlock implements POST /friends/block: hear nothing more from somebody (ADR 0156).
func (h *handlers) handleFriendBlock(w http.ResponseWriter, r *http.Request) {
	friendRoute(w, r, func(me store.Account, req friendRequest) error {
		return h.mgr.BlockPlayer(r.Context(), me, req.Handle)
	})
}

// handleFriendUnblock implements POST /friends/unblock.
func (h *handlers) handleFriendUnblock(w http.ResponseWriter, r *http.Request) {
	friendRoute(w, r, func(me store.Account, req friendRequest) error {
		return h.mgr.UnblockPlayer(r.Context(), me, req.Handle)
	})
}

// blocksResponse is GET /friends/blocks' answer: those the account has blocked, by handle. Only the
// account itself can read it.
type blocksResponse struct {
	Blocked []session.BlockedPlayer `json:"blocked"`
}

// handleGetBlocks implements GET /friends/blocks.
func (h *handlers) handleGetBlocks(w http.ResponseWriter, r *http.Request) {
	me, ok := memberAccount(w, r)
	if !ok {
		return
	}
	list, err := h.mgr.BlockedBy(r.Context(), me.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, blocksResponse{Blocked: list})
}

// handleGetFriendSettings implements GET /friends/settings.
func (h *handlers) handleGetFriendSettings(w http.ResponseWriter, r *http.Request) {
	me, ok := memberAccount(w, r)
	if !ok {
		return
	}
	settings, err := h.mgr.FriendSettings(r.Context(), me.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// friendSettingsRequest is PUT /friends/settings' body: the settings to
// change. Those it leaves out stay as they are.
type friendSettingsRequest store.FriendSettings

// handlePutFriendSettings implements PUT /friends/settings.
func (h *handlers) handlePutFriendSettings(w http.ResponseWriter, r *http.Request) {
	me, ok := memberAccount(w, r)
	if !ok {
		return
	}
	current, err := h.mgr.FriendSettings(r.Context(), me.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	req := friendSettingsRequest(current)
	if !decodeBody(w, r, &req) {
		return
	}
	settings := store.FriendSettings(req)
	if err := h.mgr.SetFriendSettings(r.Context(), me.ID, session.AddressOf(me.PrincipalID), settings); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}
