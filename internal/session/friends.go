package session

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/store"
)

// Friends, between accounts (ADR 0105). One asks by handle and the other accepts or declines. Direct
// messages and direct calls are between friends, and a friend can be invited to a game.

const (
	// maxFriends bounds an account's friends and the requests it has waiting.
	maxFriends = 500
	// friendAskAgain is how long after a refusal the same person may not ask again. The one who
	// refused can always ask them.
	friendAskAgain = 30 * 24 * time.Hour
)

var (
	// ErrNotFriend is returned for what only a friend may do.
	ErrNotFriend = errors.New("session: that is for your friends: send a friend request first")
	// ErrNoAccount is returned for a handle that is nobody's.
	ErrNoAccount = errors.New("session: no player has that handle")
	// ErrFriendExists is returned when the two are friends, or one has asked already.
	ErrFriendExists = errors.New("session: you are friends already, or have asked already")
	// ErrFriendLater is returned to somebody asking again too soon after being declined.
	ErrFriendLater = errors.New("session: that player has not accepted a request from you lately: try again later")
	// ErrFriendLimit is returned at the most friends an account can have.
	ErrFriendLimit = errors.New("session: that is as many friends and requests as an account can have")
	// ErrNoRequest is returned when answering a request that is not there.
	ErrNoRequest = errors.New("session: that player has not asked to be your friend")
	// ErrInvitesOff is returned when inviting somebody who has chosen not to be invited.
	ErrInvitesOff = errors.New("session: that player is not taking invitations to games")
	// ErrNoInvite is returned when answering an invitation that is not there.
	ErrNoInvite = errors.New("session: you have no invitation to that game")
	// ErrInviteNotSeated is returned when inviting to a game without a seat in it.
	ErrInviteNotSeated = errors.New("session: invite friends to a game you are playing in")
)

// Friend is one line of the friends list.
type Friend struct {
	Name    string `json:"name"` // "@ana"
	Handle  string `json:"handle"`
	Address string `json:"address"`
	// Online says they can be seen to be here. CanWrite says a message would reach them: they are
	// online, or are hidden but have themselves written or rung.
	Online   bool `json:"online"`
	CanWrite bool `json:"can_write"`
}

// FriendInvite is an invitation to a game, as the friends list shows it.
type FriendInvite struct {
	Room   string `json:"room"`
	Game   string `json:"game"`
	Def    string `json:"def"`
	From   string `json:"from"` // "@ana"
	Status string `json:"status"`
	// Notice is news of the game, shown until dismissed.
	Notice string `json:"notice,omitempty"`
}

// FriendsList is everything the friends list shows.
type FriendsList struct {
	Friends  []Friend       `json:"friends"`
	Incoming []Friend       `json:"incoming"`
	Outgoing []Friend       `json:"outgoing"`
	Invites  []FriendInvite `json:"invites"`
}

// FriendsOf reads an account's list. Whether a friend is online says nothing of those not yet friends.
func (m *Manager) FriendsOf(ctx context.Context, accountID int64, address string) (FriendsList, error) {
	out := FriendsList{Friends: []Friend{}, Incoming: []Friend{}, Outgoing: []Friend{}, Invites: []FriendInvite{}}
	rows, err := m.store.Friendships(ctx, accountID)
	if err != nil {
		return out, err
	}
	for _, f := range rows {
		line := Friend{Name: "@" + f.Handle, Handle: f.Handle, Address: AddressOf(f.PrincipalID)}
		switch {
		case f.Status == store.FriendAccepted:
			line.Online = m.seenHere(line.Address) != ""
			line.CanWrite = m.canReach(address, line.Address)
			out.Friends = append(out.Friends, line)
		case f.Requester == accountID:
			out.Outgoing = append(out.Outgoing, line)
		default:
			out.Incoming = append(out.Incoming, line)
		}
	}
	invites, err := m.store.GameInvites(ctx, accountID)
	if err != nil {
		return out, err
	}
	for _, g := range invites {
		name, _ := m.GameListLabel(g.GameDefID)
		if name == "" {
			if compiled, ok := m.catalog.Get(g.GameDefID); ok {
				name = compiled.Definition.Title()
			} else {
				continue
			}
		}
		out.Invites = append(out.Invites, FriendInvite{Room: g.RoomCode, Game: name, Def: g.GameDefID, From: "@" + g.FromHandle, Status: g.Status, Notice: g.Notice})
	}
	return out, nil
}

// activeAccount finds the account a handle names; one not yet confirmed is nobody yet.
func (m *Manager) activeAccount(ctx context.Context, handle string) (store.Account, error) {
	a, err := m.store.AccountByHandle(ctx, strings.TrimPrefix(strings.TrimSpace(handle), "@"))
	if errors.Is(err, store.ErrNotFound) || (err == nil && a.Status != store.AccountActive) {
		return store.Account{}, ErrNoAccount
	}
	return a, err
}

// tellFriends tells the pages of an account that its list has changed.
func (m *Manager) tellFriends(principalID string) {
	m.hub.publish(dmRoom(AddressOf(principalID)), Event{Type: "friends", Data: map[string]string{}})
}

// RequestFriend asks handle to be a friend of the account me. If they have already asked, they are
// friends at once. ip is where the request came from, for the limits on mail.
func (m *Manager) RequestFriend(ctx context.Context, me store.Account, handle, ip string) error {
	other, err := m.activeAccount(ctx, handle)
	if err != nil {
		return err
	}
	if other.ID == me.ID {
		return ErrNoAccount
	}
	now := m.now().UTC()
	if !m.chat.allow("friend|"+AddressOf(me.PrincipalID), now) {
		return ErrChatTooFast
	}
	requester, status, updated, err := m.store.FriendshipOf(ctx, me.ID, other.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return err
	case status == store.FriendAccepted, status == store.FriendPending && requester == me.ID:
		return ErrFriendExists
	case status == store.FriendPending:
		// Both want it.
		if err := m.store.SetFriendship(ctx, me.ID, other.ID, requester, store.FriendAccepted, now); err != nil {
			return err
		}
		m.tellFriends(me.PrincipalID)
		m.tellFriends(other.PrincipalID)
		return nil
	case requester == me.ID && now.Sub(updated) < friendAskAgain:
		return ErrFriendLater
	}
	rows, err := m.store.Friendships(ctx, me.ID)
	if err != nil {
		return err
	}
	if len(rows) >= maxFriends {
		return ErrFriendLimit
	}
	if err := m.store.SetFriendship(ctx, me.ID, other.ID, me.ID, store.FriendPending, now); err != nil {
		return err
	}
	m.tellFriends(me.PrincipalID)
	m.tellFriends(other.PrincipalID)
	m.mailFriendRequest(ctx, me, other, ip)
	return nil
}

// mailFriendRequest tells somebody who asked for it about a request. A failure to send is no failure
// of the request, which is waiting in the friends list.
func (m *Manager) mailFriendRequest(ctx context.Context, from, to store.Account, ip string) {
	if !m.MailEnabled() || !to.EmailVerified {
		return
	}
	settings, err := m.store.FriendSettings(ctx, to.ID)
	if err != nil || !settings.MailRequests {
		return
	}
	_ = m.SendAccountMail(ctx, "friend", to.Email, ip, func(base string) mail.Message {
		return mail.FriendRequest(to.Email, from.Handle, base)
	})
}

// AnswerFriend accepts or declines a request made to the account me.
func (m *Manager) AnswerFriend(ctx context.Context, me store.Account, handle string, accept bool) error {
	other, err := m.activeAccount(ctx, handle)
	if err != nil {
		return err
	}
	requester, status, _, err := m.store.FriendshipOf(ctx, me.ID, other.ID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && (status != store.FriendPending || requester == me.ID)) {
		return ErrNoRequest
	}
	if err != nil {
		return err
	}
	status = store.FriendDeclined
	if accept {
		status = store.FriendAccepted
	}
	if err := m.store.SetFriendship(ctx, me.ID, other.ID, requester, status, m.now().UTC()); err != nil {
		return err
	}
	m.tellFriends(me.PrincipalID)
	m.tellFriends(other.PrincipalID)
	return nil
}

// RemoveFriend ends a friendship, or takes back a request. Either can ask again.
func (m *Manager) RemoveFriend(ctx context.Context, me store.Account, handle string) error {
	other, err := m.activeAccount(ctx, handle)
	if err != nil {
		return err
	}
	requester, status, _, err := m.store.FriendshipOf(ctx, me.ID, other.ID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && status == store.FriendDeclined) {
		return nil
	}
	if err != nil {
		return err
	}
	// Somebody declined stays declined: taking a request back is no way round it. An incoming request
	// is answered, not removed.
	if status == store.FriendPending && requester != me.ID {
		return ErrNoRequest
	}
	if err := m.store.DeleteFriendship(ctx, me.ID, other.ID); err != nil {
		return err
	}
	m.tellFriends(me.PrincipalID)
	m.tellFriends(other.PrincipalID)
	return nil
}

// MustBeFriends refuses what is only for friends: address is where the message or call would go.
// It looks no further than the account's own friends, so it says nothing of who else is here.
func (m *Manager) MustBeFriends(ctx context.Context, accountID int64, address string) error {
	rows, err := m.store.Friendships(ctx, accountID)
	if err != nil {
		return err
	}
	for _, f := range rows {
		if f.Status == store.FriendAccepted && AddressOf(f.PrincipalID) == address {
			return nil
		}
	}
	return ErrNotFriend
}

// InviteFriend invites a friend to a game the account me has a seat in. It is kept until answered or
// the game ends, since the friend may be away, and it reaches any page they have open at once.
func (m *Manager) InviteFriend(ctx context.Context, me store.Account, handle, room string) error {
	other, err := m.activeAccount(ctx, handle)
	if err != nil {
		return err
	}
	if _, status, _, err := m.store.FriendshipOf(ctx, me.ID, other.ID); errors.Is(err, store.ErrNotFound) || (err == nil && status != store.FriendAccepted) {
		return ErrNotFriend
	} else if err != nil {
		return err
	}
	now := m.now().UTC()
	if !m.chat.allow("friend|"+AddressOf(me.PrincipalID), now) {
		return ErrChatTooFast
	}
	if settings, err := m.store.FriendSettings(ctx, other.ID); err != nil {
		return err
	} else if !settings.AcceptInvites {
		return ErrInvitesOff
	}
	game, err := m.store.GetGame(ctx, room)
	if err != nil {
		return err
	}
	if game.Status == "finished" {
		return ErrGameNotActive
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return err
	}
	seated := false
	for _, p := range players {
		seated = seated || p.PrincipalID == me.PrincipalID
	}
	if !seated {
		return ErrInviteNotSeated
	}
	if err := m.store.AddGameInvite(ctx, game.ID, other.ID, me.ID, now); err != nil {
		return err
	}
	m.tellFriends(other.PrincipalID)
	return nil
}

// AnswerInvite accepts or declines an invitation of the account me.
func (m *Manager) AnswerInvite(ctx context.Context, me store.Account, room string, accept bool) error {
	game, err := m.store.GetGame(ctx, canonRoom(room))
	if err != nil {
		return err
	}
	status := store.InviteDeclined
	if accept {
		status = store.InviteAccepted
	}
	found, err := m.store.AnswerGameInvite(ctx, game.ID, me.ID, status)
	if err != nil {
		return err
	}
	if !found {
		return ErrNoInvite
	}
	m.tellFriends(me.PrincipalID)
	// Accepted, they get the game's time; either way, it may be what a schedule was waiting for.
	if accept {
		err = m.tellAcceptedTime(ctx, game, me)
	} else if sched, serr := m.store.ScheduleOf(ctx, game.ID); serr == nil {
		err = m.resolveSchedule(ctx, game, sched)
	}
	m.publishState(ctx, game.RoomCode, "join")
	return err
}

// DismissInvite takes an invitation off the list of the account me: one unanswered is declined.
func (m *Manager) DismissInvite(ctx context.Context, me store.Account, room string) error {
	if err := m.store.DismissGameInvite(ctx, me.ID, canonRoom(room)); err != nil {
		return err
	}
	m.tellFriends(me.PrincipalID)
	return nil
}

// FriendSettings reads what an account chose about its friends.
func (m *Manager) FriendSettings(ctx context.Context, accountID int64) (store.FriendSettings, error) {
	return m.store.FriendSettings(ctx, accountID)
}

// SetFriendSettings stores what an account chose about its friends, and applies it at once.
func (m *Manager) SetFriendSettings(ctx context.Context, accountID int64, address string, s store.FriendSettings) error {
	if err := m.store.SetFriendSettings(ctx, accountID, s); err != nil {
		return err
	}
	m.SetHereHidden(address, !s.ShowPresence)
	return nil
}

// HidesPresence says whether an account has chosen not to be seen. If that cannot be read, it has.
func (m *Manager) HidesPresence(ctx context.Context, accountID int64) bool {
	s, err := m.store.FriendSettings(ctx, accountID)
	return err != nil || !s.ShowPresence
}
