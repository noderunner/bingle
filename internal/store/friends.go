package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Friendship statuses.
const (
	FriendPending  = "pending"
	FriendAccepted = "accepted"
	FriendDeclined = "declined"
)

// Friendship is the row of two accounts, as one of them sees it.
type Friendship struct {
	OtherID     int64
	Handle      string
	PrincipalID string
	Requester   int64
	Status      string
	UpdatedAt   string
}

func friendPair(a, b int64) (int64, int64) {
	if a > b {
		return b, a
	}
	return a, b
}

// Friendships lists an account's rows other than those declined, with accounts that still exist.
func (s *Store) Friendships(ctx context.Context, accountID int64) ([]Friendship, error) {
	rows, err := s.query(ctx, "Friendships",
		`SELECT a.id, a.handle, a.principal_id, f.requester, f.status, f.updated_at
		 FROM friendships f JOIN accounts a ON a.id = CASE WHEN f.low = ? THEN f.high ELSE f.low END
		 WHERE (f.low = ? OR f.high = ?) AND f.status != ? AND a.deleted_at IS NULL
		 ORDER BY a.handle_lower`, accountID, accountID, accountID, FriendDeclined)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Friendship
	for rows.Next() {
		var f Friendship
		if err := rows.Scan(&f.OtherID, &f.Handle, &f.PrincipalID, &f.Requester, &f.Status, &f.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FriendshipOf returns the row of two accounts, ErrNotFound if they have none.
func (s *Store) FriendshipOf(ctx context.Context, a, b int64) (requester int64, status string, updated time.Time, err error) {
	low, high := friendPair(a, b)
	err = s.queryRow(ctx, "FriendshipOf", `SELECT requester, status, updated_at FROM friendships WHERE low = ? AND high = ?`, low, high).Scan(&requester, &status, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", time.Time{}, ErrNotFound
	}
	return requester, status, updated, err
}

// SetFriendship writes the row of two accounts.
func (s *Store) SetFriendship(ctx context.Context, a, b, requester int64, status string, now time.Time) error {
	low, high := friendPair(a, b)
	_, err := s.exec(ctx, "SetFriendship",
		`INSERT INTO friendships (low, high, requester, status, updated_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (low, high) DO UPDATE SET requester = excluded.requester, status = excluded.status, updated_at = excluded.updated_at`,
		low, high, requester, status, stamp(now))
	return err
}

// DeleteFriendship forgets the row of two accounts.
func (s *Store) DeleteFriendship(ctx context.Context, a, b int64) error {
	low, high := friendPair(a, b)
	_, err := s.exec(ctx, "DeleteFriendship", `DELETE FROM friendships WHERE low = ? AND high = ?`, low, high)
	return err
}

// FriendSettings is what an account chose about its friends.
type FriendSettings struct {
	MailRequests bool `json:"mail_requests"`
	ShowPresence bool `json:"show_presence"`
	// AcceptInvites says other players may invite the account to games.
	AcceptInvites bool `json:"accept_invites"`
}

// FriendSettings reads an account's choices; with no row, the defaults: no mail, to be seen, and invitations.
func (s *Store) FriendSettings(ctx context.Context, accountID int64) (FriendSettings, error) {
	out := FriendSettings{ShowPresence: true, AcceptInvites: true}
	err := s.queryRow(ctx, "FriendSettings", `SELECT mail_requests, show_presence, accept_invites FROM friend_settings WHERE account_id = ?`, accountID).Scan(&out.MailRequests, &out.ShowPresence, &out.AcceptInvites)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	return out, err
}

// SetFriendSettings stores an account's choices.
func (s *Store) SetFriendSettings(ctx context.Context, accountID int64, v FriendSettings) error {
	_, err := s.exec(ctx, "SetFriendSettings",
		`INSERT INTO friend_settings (account_id, mail_requests, show_presence, accept_invites) VALUES (?, ?, ?, ?)
		 ON CONFLICT (account_id) DO UPDATE SET mail_requests = excluded.mail_requests, show_presence = excluded.show_presence, accept_invites = excluded.accept_invites`,
		accountID, v.MailRequests, v.ShowPresence, v.AcceptInvites)
	return err
}

// Invitation statuses.
const (
	InvitePending  = "pending"
	InviteAccepted = "accepted"
	InviteDeclined = "declined"
)

// GameInvite is an invitation to a game, waiting for an answer or with something to say.
type GameInvite struct {
	RoomCode   string
	GameDefID  string
	FromHandle string
	Status     string
	// Notice is news of the game for the one invited, shown until dismissed.
	Notice string
}

// AddGameInvite records an invitation, once per game and friend.
func (s *Store) AddGameInvite(ctx context.Context, gameID, to, from int64, now time.Time) error {
	// Somebody who declined is not asked again.
	_, err := s.exec(ctx, "AddGameInvite",
		`INSERT INTO game_invites (game_id, to_account, from_account, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (game_id, to_account) DO UPDATE SET from_account = excluded.from_account, created_at = excluded.created_at
		 WHERE game_invites.status != 'declined'`,
		gameID, to, from, stamp(now))
	return err
}

// AnswerGameInvite stores the answer to an invitation; it reports whether there was one.
func (s *Store) AnswerGameInvite(ctx context.Context, gameID, to int64, status string) (bool, error) {
	res, err := s.exec(ctx, "AnswerGameInvite", `UPDATE game_invites SET status = ? WHERE game_id = ? AND to_account = ?`, status, gameID, to)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DismissGameInvite takes an invitation off the list: an unanswered one is declined, and news goes.
func (s *Store) DismissGameInvite(ctx context.Context, to int64, roomCode string) error {
	_, err := s.exec(ctx, "DismissGameInvite",
		`UPDATE game_invites SET notice = '', status = CASE WHEN status = 'pending' THEN 'declined' ELSE status END
		 WHERE to_account = ? AND game_id IN (SELECT id FROM games WHERE room_code = ?)`, to, roomCode)
	return err
}

// GameInvites lists what an account has to see: invitations to games still to be played that it has
// not answered, and news of any game. The newest first.
func (s *Store) GameInvites(ctx context.Context, to int64) ([]GameInvite, error) {
	rows, err := s.query(ctx, "GameInvites",
		`SELECT g.room_code, g.game_def_id, a.handle, i.status, i.notice
		 FROM game_invites i JOIN games g ON g.id = i.game_id JOIN accounts a ON a.id = i.from_account
		 WHERE i.to_account = ? AND a.deleted_at IS NULL AND (i.notice != '' OR (i.status = 'pending' AND g.status != 'finished'))
		 ORDER BY i.created_at DESC, i.game_id DESC LIMIT 50`, to)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []GameInvite
	for rows.Next() {
		var g GameInvite
		if err := rows.Scan(&g.RoomCode, &g.GameDefID, &g.FromHandle, &g.Status, &g.Notice); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GameAccess says who may find a game: what its creator chose.
type GameAccess struct {
	InviteOnly     bool
	CreatorAccount int64
}

// SetGameAccess stores who may find a game.
func (s *Store) SetGameAccess(ctx context.Context, gameID int64, a GameAccess) error {
	_, err := s.exec(ctx, "SetGameAccess",
		`INSERT INTO game_access (game_id, invite_only, creator_account) VALUES (?, ?, ?)
		 ON CONFLICT (game_id) DO UPDATE SET invite_only = excluded.invite_only, creator_account = excluded.creator_account`,
		gameID, a.InviteOnly, a.CreatorAccount)
	return err
}

// GameAccessOf reads who may find a game; with no row, anybody.
func (s *Store) GameAccessOf(ctx context.Context, gameID int64) (GameAccess, error) {
	var a GameAccess
	err := s.queryRow(ctx, "GameAccessOf", `SELECT invite_only, creator_account FROM game_access WHERE game_id = ?`, gameID).Scan(&a.InviteOnly, &a.CreatorAccount)
	if errors.Is(err, sql.ErrNoRows) {
		return GameAccess{}, nil
	}
	return a, err
}

// SeatReservation is a seat kept for one account.
type SeatReservation struct {
	Seat      int
	AccountID int64
	Handle    string
}

// ReserveSeat keeps a seat for an account.
func (s *Store) ReserveSeat(ctx context.Context, gameID int64, seat int, accountID int64) error {
	_, err := s.exec(ctx, "ReserveSeat",
		`INSERT INTO seat_reservations (game_id, seat, account_id) VALUES (?, ?, ?)
		 ON CONFLICT (game_id, seat) DO UPDATE SET account_id = excluded.account_id`, gameID, seat, accountID)
	return err
}

// ReleaseSeat lets anybody sit at a seat again.
func (s *Store) ReleaseSeat(ctx context.Context, gameID int64, seat int) error {
	_, err := s.exec(ctx, "ReleaseSeat", `DELETE FROM seat_reservations WHERE game_id = ? AND seat = ?`, gameID, seat)
	return err
}

// SeatReservations lists a game's kept seats, in seat order.
func (s *Store) SeatReservations(ctx context.Context, gameID int64) ([]SeatReservation, error) {
	rows, err := s.query(ctx, "SeatReservations",
		`SELECT r.seat, r.account_id, a.handle FROM seat_reservations r JOIN accounts a ON a.id = r.account_id
		 WHERE r.game_id = ? ORDER BY r.seat`, gameID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []SeatReservation
	for rows.Next() {
		var r SeatReservation
		if err := rows.Scan(&r.Seat, &r.AccountID, &r.Handle); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
