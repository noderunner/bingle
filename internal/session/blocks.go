package session

import (
	"context"
	"errors"
	"strings"

	"github.com/noderunner/bingle/internal/store"
)

// Blocks (ADR 0156). An account blocks another by handle, and from then on hears nothing from them:
// no friend request, invitation to a game or a muster, message or call. Blocking ends a friendship as
// removing it does. It is silent: the blocked person gets the answers they would get from somebody
// who removed them and does not respond, and nobody but the blocker can see the block. Chat in the
// global room and in games is not blocked.

// ErrBlocking is returned to an account trying to reach somebody it has blocked itself. Only the
// blocker gets it: it tells them nothing they do not know.
var ErrBlocking = errors.New("session: you have blocked that player: unblock them in your profile first")

// BlockedPlayer is one line of an account's blocks, as its profile shows them.
type BlockedPlayer struct {
	Name   string `json:"name"` // "@ana"
	Handle string `json:"handle"`
}

// blocks says whether blocker has blocked blocked. If that cannot be read, it has: a block that fails
// to hold would give it away.
func (m *Manager) blocks(ctx context.Context, blocker, blocked int64) bool {
	yes, err := m.store.Blocks(ctx, blocker, blocked)
	return err != nil || yes
}

// BlockPlayer blocks handle for the account me. A friendship, or a request me made, goes as with
// RemoveFriend, and any call or ring between the two ends. A request they made stays waiting, unseen.
func (m *Manager) BlockPlayer(ctx context.Context, me store.Account, handle string) error {
	other, err := m.activeAccount(ctx, handle)
	if err != nil {
		return err
	}
	if other.ID == me.ID {
		return ErrNoAccount
	}
	if err := m.store.Block(ctx, me.ID, other.ID, m.now().UTC()); err != nil {
		return err
	}
	requester, status, _, err := m.store.FriendshipOf(ctx, me.ID, other.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	// The other side is told only of what a removal would show them.
	if err == nil && (status == store.FriendAccepted || (status == store.FriendPending && requester == me.ID)) {
		if err := m.store.DeleteFriendship(ctx, me.ID, other.ID); err != nil {
			return err
		}
		m.tellFriends(other.PrincipalID)
	}
	m.LeaveDirectCall(AddressOf(me.PrincipalID), AddressOf(other.PrincipalID))
	m.tellFriends(me.PrincipalID)
	return nil
}

// UnblockPlayer lifts a block of the account me. What the other sent meanwhile and is still waiting,
// a friend request or an invitation, shows again.
func (m *Manager) UnblockPlayer(ctx context.Context, me store.Account, handle string) error {
	// Any account the list shows can be unblocked, active or not.
	other, err := m.store.AccountByHandle(ctx, strings.TrimPrefix(strings.TrimSpace(handle), "@"))
	if errors.Is(err, store.ErrNotFound) {
		return ErrNoAccount
	}
	if err != nil {
		return err
	}
	if err := m.store.Unblock(ctx, me.ID, other.ID); err != nil {
		return err
	}
	m.tellFriends(me.PrincipalID)
	return nil
}

// BlockedBy lists those the account me has blocked, by handle.
func (m *Manager) BlockedBy(ctx context.Context, me int64) ([]BlockedPlayer, error) {
	rows, err := m.store.Blocked(ctx, me)
	if err != nil {
		return nil, err
	}
	out := make([]BlockedPlayer, 0, len(rows))
	for _, a := range rows {
		out = append(out, BlockedPlayer{Name: "@" + a.Handle, Handle: a.Handle})
	}
	return out, nil
}
