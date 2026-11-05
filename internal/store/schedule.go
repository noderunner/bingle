package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// A schedule's statuses.
const (
	SchedScheduled   = "scheduled"
	SchedPolling     = "polling"
	SchedNoAgreement = "no_agreement"
	SchedStarted     = "started"
	SchedClosed      = "closed"
)

// InviteDropped is an invitation the creator stopped waiting for.
const InviteDropped = "dropped"

// Schedule is a game set up for later.
type Schedule struct {
	GameID         int64
	CreatorAccount int64
	Status         string
	StartAt        string
	DecideBy       string
	IcsSeq         int
}

// SaveSchedule writes a game's schedule.
func (s *Store) SaveSchedule(ctx context.Context, v Schedule) error {
	_, err := s.exec(ctx, "SaveSchedule",
		`INSERT INTO game_schedules (game_id, creator_account, status, start_at, decide_by, ics_seq) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (game_id) DO UPDATE SET status = excluded.status, start_at = excluded.start_at, decide_by = excluded.decide_by, ics_seq = excluded.ics_seq`,
		v.GameID, v.CreatorAccount, v.Status, v.StartAt, v.DecideBy, v.IcsSeq)
	return err
}

// ScheduleOf reads a game's schedule, ErrNotFound if it has none.
func (s *Store) ScheduleOf(ctx context.Context, gameID int64) (Schedule, error) {
	v := Schedule{GameID: gameID}
	err := s.queryRow(ctx, "ScheduleOf", `SELECT creator_account, status, start_at, decide_by, ics_seq FROM game_schedules WHERE game_id = ?`, gameID).
		Scan(&v.CreatorAccount, &v.Status, &v.StartAt, &v.DecideBy, &v.IcsSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return Schedule{}, ErrNotFound
	}
	return v, err
}

// OpenSchedules lists the rooms of the games still waiting for a decision.
func (s *Store) OpenSchedules(ctx context.Context) ([]string, error) {
	rows, err := s.query(ctx, "OpenSchedules",
		`SELECT g.room_code FROM game_schedules c JOIN games g ON g.id = c.game_id WHERE c.status IN (?, ?)`, SchedPolling, SchedNoAgreement)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var room string
		if err := rows.Scan(&room); err != nil {
			return nil, err
		}
		out = append(out, room)
	}
	return out, rows.Err()
}

// Slot is a time offered for a game.
type Slot struct {
	ID int64
	At time.Time
	// Voters are the accounts that can make it.
	Voters map[int64]bool
}

// ReplaceSlots offers these times for a game instead of any before, with nobody's answers.
func (s *Store) ReplaceSlots(ctx context.Context, gameID int64, times []time.Time) error {
	if _, err := s.exec(ctx, "ReplaceSlots.votes", `DELETE FROM slot_votes WHERE slot_id IN (SELECT id FROM game_slots WHERE game_id = ?)`, gameID); err != nil {
		return err
	}
	if _, err := s.exec(ctx, "ReplaceSlots.slots", `DELETE FROM game_slots WHERE game_id = ?`, gameID); err != nil {
		return err
	}
	for _, at := range times {
		if _, err := s.exec(ctx, "ReplaceSlots.add", `INSERT INTO game_slots (game_id, at) VALUES (?, ?)`, gameID, at.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

// SlotsOf lists a game's times, the earliest first, with who can make each.
func (s *Store) SlotsOf(ctx context.Context, gameID int64) ([]Slot, error) {
	rows, err := s.query(ctx, "SlotsOf", `SELECT id, at FROM game_slots WHERE game_id = ? ORDER BY at`, gameID)
	if err != nil {
		return nil, err
	}
	var out []Slot
	for rows.Next() {
		var at string
		var slot Slot
		if err := rows.Scan(&slot.ID, &at); err != nil {
			_ = rows.Close()
			return nil, err
		}
		slot.At, err = time.Parse(time.RFC3339, at)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		slot.Voters = map[int64]bool{}
		out = append(out, slot)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	votes, err := s.query(ctx, "SlotsOf.votes", `SELECT v.slot_id, v.account_id FROM slot_votes v JOIN game_slots g ON g.id = v.slot_id WHERE g.game_id = ?`, gameID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = votes.Close() }()
	for votes.Next() {
		var slot, account int64
		if err := votes.Scan(&slot, &account); err != nil {
			return nil, err
		}
		for i := range out {
			if out[i].ID == slot {
				out[i].Voters[account] = true
			}
		}
	}
	return out, votes.Err()
}

// SetVote says whether an account can make a time.
func (s *Store) SetVote(ctx context.Context, slotID, accountID int64, on bool) error {
	var err error
	if on {
		_, err = s.exec(ctx, "SetVote.on", `INSERT OR IGNORE INTO slot_votes (slot_id, account_id) VALUES (?, ?)`, slotID, accountID)
	} else {
		_, err = s.exec(ctx, "SetVote.off", `DELETE FROM slot_votes WHERE slot_id = ? AND account_id = ?`, slotID, accountID)
	}
	return err
}

// Invitee is somebody invited to a game, and how they answered.
type Invitee struct {
	Account Account
	Status  string
}

// InviteesOf lists those invited to a game.
func (s *Store) InviteesOf(ctx context.Context, gameID int64) ([]Invitee, error) {
	rows, err := s.query(ctx, "InviteesOf",
		`SELECT i.status, `+accountCols("accounts")+` FROM game_invites i JOIN accounts ON accounts.id = i.to_account
		 WHERE i.game_id = ? AND accounts.deleted_at IS NULL ORDER BY accounts.handle_lower`, gameID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Invitee
	for rows.Next() {
		var v Invitee
		if err := rows.Scan(append([]any{&v.Status}, v.Account.fields()...)...); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SetInviteNotice leaves news of a game on an invitation.
func (s *Store) SetInviteNotice(ctx context.Context, gameID, to int64, notice string) error {
	_, err := s.exec(ctx, "SetInviteNotice", `UPDATE game_invites SET notice = ? WHERE game_id = ? AND to_account = ?`, notice, gameID, to)
	return err
}
