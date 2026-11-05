package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/store"
)

// Who may find a game, and who may sit where (ADR 0107). A game's room code still lets anybody open
// it and watch: these decide what the lists show, and whose a seat is.

var (
	// ErrSeatReserved is returned for a seat kept for somebody else.
	ErrSeatReserved = errors.New("session: that seat is reserved for somebody else: you can watch this game")
	// ErrBadReservation is returned for a seat that cannot be reserved as asked.
	ErrBadReservation = errors.New("session: seats can be reserved only for your friends, and only seats left open")
)

// GameReservation is a seat kept for a friend, as a page shows it.
type GameReservation struct {
	Seat   int    `json:"seat"`
	Handle string `json:"handle"`
	Yours  bool   `json:"yours,omitempty"`
}

// viewerAccount is the account of the request, 0 for anybody else.
func viewerAccount(ctx context.Context) int64 {
	if p, ok := auth.From(ctx); ok && p.Account != nil {
		return p.Account.ID
	}
	return 0
}

// checkReservations makes sure the seats asked for are open ones, each kept for a friend of the
// creator, who must have an account. It returns the accounts.
func (m *Manager) checkReservations(ctx context.Context, opts CreateOptions, taken map[int]bool) (map[int]store.Account, error) {
	if len(opts.Reserved) == 0 {
		return nil, nil
	}
	me := viewerAccount(ctx)
	if me == 0 {
		return nil, fmt.Errorf("%w: sign in first", ErrBadReservation)
	}
	out := map[int]store.Account{}
	seen := map[int64]bool{}
	for seat, handle := range opts.Reserved {
		if taken[seat] {
			return nil, fmt.Errorf("%w: seat %d is taken", ErrBadReservation, seat)
		}
		other, err := m.activeAccount(ctx, handle)
		if err != nil {
			return nil, fmt.Errorf("%w: no player is called %q", ErrBadReservation, handle)
		}
		_, status, _, err := m.store.FriendshipOf(ctx, me, other.ID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && status != store.FriendAccepted) {
			return nil, fmt.Errorf("%w: @%s is not your friend", ErrBadReservation, other.Handle)
		}
		if err != nil {
			return nil, err
		}
		if seen[other.ID] {
			return nil, fmt.Errorf("%w: @%s has a seat already", ErrBadReservation, other.Handle)
		}
		seen[other.ID] = true
		out[seat] = other
	}
	return out, nil
}

// saveGameAccess records what the creator chose, and invites each friend a seat is kept for.
func (m *Manager) saveGameAccess(ctx context.Context, gameID int64, opts CreateOptions, reserved map[int]store.Account) error {
	if !opts.InviteOnly && len(reserved) == 0 {
		return nil
	}
	me := viewerAccount(ctx)
	if err := m.store.SetGameAccess(ctx, gameID, store.GameAccess{InviteOnly: opts.InviteOnly, CreatorAccount: me}); err != nil {
		return err
	}
	now := m.now().UTC()
	for seat, other := range reserved {
		if err := m.store.ReserveSeat(ctx, gameID, seat, other.ID); err != nil {
			return err
		}
		if err := m.store.AddGameInvite(ctx, gameID, other.ID, me, now); err != nil {
			return err
		}
		m.tellFriends(other.PrincipalID)
	}
	return nil
}

// reservedFor maps the kept seats of a game to their accounts.
func (m *Manager) reservedFor(ctx context.Context, gameID int64) (map[int]int64, error) {
	rows, err := m.store.SeatReservations(ctx, gameID)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	out := make(map[int]int64, len(rows))
	for _, r := range rows {
		out[r.Seat] = r.AccountID
	}
	return out, nil
}

// mayHaveSeat refuses a seat kept for somebody else.
func mayHaveSeat(ctx context.Context, reserved map[int]int64, seat int) error {
	if holder, ok := reserved[seat]; ok && holder != viewerAccount(ctx) {
		return ErrSeatReserved
	}
	return nil
}

// GameReservations lists the kept seats of a game, as the one asking sees them.
func (m *Manager) GameReservations(ctx context.Context, room string) ([]GameReservation, bool, error) {
	game, err := m.store.GetGame(ctx, canonRoom(room))
	if err != nil {
		return nil, false, err
	}
	access, err := m.store.GameAccessOf(ctx, game.ID)
	if err != nil {
		return nil, false, err
	}
	rows, err := m.store.SeatReservations(ctx, game.ID)
	if err != nil {
		return nil, false, err
	}
	me := viewerAccount(ctx)
	out := make([]GameReservation, 0, len(rows))
	for _, r := range rows {
		out = append(out, GameReservation{Seat: r.Seat, Handle: r.Handle, Yours: me != 0 && r.AccountID == me})
	}
	return out, access.InviteOnly, nil
}

// ReleaseSeat lets the host give up a seat kept for a friend who is not sitting there.
func (m *Manager) ReleaseSeat(ctx context.Context, roomCode, by string, seat int) (StateSnapshot, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(ctx, roomCode)
	defer unlock()
	game, players, me, err := m.lobbyContext(ctx, roomCode, by)
	if err != nil {
		return StateSnapshot{}, err
	}
	if hostOf(players) != me.Seat {
		return StateSnapshot{}, ErrNotHost
	}
	if err := m.store.ReleaseSeat(ctx, game.ID, seat); err != nil {
		return StateSnapshot{}, err
	}
	m.publishState(ctx, roomCode, "join")
	return m.readState(ctx, roomCode, nil)
}

// isHostNamed says whether the person named by is the host at this table.
func (m *Manager) isHostNamed(players []store.Player, by string) bool {
	host := hostOf(players)
	for _, p := range players {
		if p.Seat == host && strings.EqualFold(p.Name, by) {
			return true
		}
	}
	return false
}

// listedAccess is whether a game goes on a list for the viewer, and how it is shown.
type listedAccess struct {
	Show        bool
	FriendsOnly bool
	Open        int  // the seats the viewer could sit at
	Yours       bool // a seat is kept for the viewer
}

func (m *Manager) listedAccess(ctx context.Context, gameID int64, players []store.Player, seats []int, viewer int64, friends func() map[int64]bool) (listedAccess, error) {
	taken := make(map[int]bool, len(players))
	for _, p := range players {
		taken[p.Seat] = true
	}
	access, err := m.store.GameAccessOf(ctx, gameID)
	if err != nil {
		return listedAccess{}, err
	}
	reserved, err := m.reservedFor(ctx, gameID)
	if err != nil {
		return listedAccess{}, err
	}
	out := listedAccess{}
	free := 0
	for _, seat := range seats {
		if taken[seat] {
			continue
		}
		holder, kept := reserved[seat]
		switch {
		case !kept:
			free++
		case viewer != 0 && holder == viewer:
			out.Open++
			out.Yours = true
		}
	}
	out.Open += free
	if access.InviteOnly {
		return out, nil // on no list, not even a friend's
	}
	if len(reserved) == 0 {
		out.Show = true
		return out, nil
	}
	out.FriendsOnly = free == 0
	if !out.FriendsOnly {
		out.Show = true
		return out, nil
	}
	// Friends of its creator can find it, and whoever a seat is kept for.
	mine := viewer != 0 && access.CreatorAccount == viewer
	for _, holder := range reserved {
		mine = mine || (viewer != 0 && holder == viewer)
	}
	out.Show = mine || (viewer != 0 && friends()[access.CreatorAccount])
	return out, nil
}
