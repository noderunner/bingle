package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ReactionPrefs is what a person decided about the reactions of their seat.
// Their reactions are declined for them by default, so that a game never
// waits for an answer; the prefs name the occasions on which they want to be
// asked instead: Targeted when an action is aimed at them (their player, one
// of their units or an area where they stand), AskAll after any other
// player's action, AskSeats after the actions of those seats.
//
// A row saved before the choice was reversed holds All (every reaction was
// passed) and Seats (those whose actions were passed) instead, and no V: see
// Legacy.
//
// Targeted, AskAll and AskSeats are the choice for every reaction; Abilities
// holds the reactions the person set apart, by ability id (Madness and the
// Unspeakable Oath, say, can differ).
type ReactionPrefs struct {
	V         int                    `json:"v"`
	Targeted  bool                   `json:"targeted"`
	AskAll    bool                   `json:"ask_all"`
	AskSeats  []int                  `json:"ask_seats"`
	Abilities map[string]AbilityPref `json:"abilities,omitempty"`

	All   bool  `json:"all,omitempty"`
	Seats []int `json:"seats,omitempty"`
}

// AbilityPref is the choice of when to be asked about one reaction.
type AbilityPref struct {
	Targeted bool  `json:"targeted"`
	AskAll   bool  `json:"ask_all"`
	AskSeats []int `json:"ask_seats"`
}

// Legacy says the row was saved by the earlier, opposite choice (what to pass,
// not what to be asked).
func (p ReactionPrefs) Legacy() bool { return p.V < 2 }

// SetReactionPrefs stores seat's choice for the game.
func (s *Store) SetReactionPrefs(ctx context.Context, gameID int64, seat int, prefs ReactionPrefs) error {
	prefs.V, prefs.All, prefs.Seats = 2, false, nil
	if prefs.AskSeats == nil {
		prefs.AskSeats = []int{}
	}
	raw, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("encoding reaction prefs: %w", err)
	}
	_, err = s.exec(ctx, "SetReactionPrefs",
		`INSERT INTO reaction_prefs (game_id, seat, prefs) VALUES (?, ?, ?)
		 ON CONFLICT(game_id, seat) DO UPDATE SET prefs = excluded.prefs`,
		gameID, seat, string(raw))
	if err != nil {
		return fmt.Errorf("storing reaction prefs (game_id=%d seat=%d): %w", gameID, seat, err)
	}
	return nil
}

// ReactionPrefs returns seat's saved choice; found is false when none was
// saved (the session then applies the defaults).
func (s *Store) ReactionPrefs(ctx context.Context, gameID int64, seat int) (prefs ReactionPrefs, found bool, err error) {
	var raw string
	err = s.queryRow(ctx, "ReactionPrefs", `SELECT prefs FROM reaction_prefs WHERE game_id = ? AND seat = ?`, gameID, seat).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ReactionPrefs{}, false, nil
	}
	if err != nil {
		return ReactionPrefs{}, false, fmt.Errorf("reading reaction prefs (game_id=%d seat=%d): %w", gameID, seat, err)
	}
	if err := json.Unmarshal([]byte(raw), &prefs); err != nil {
		return ReactionPrefs{}, false, nil
	}
	if prefs.AskSeats == nil {
		prefs.AskSeats = []int{}
	}
	return prefs, true, nil
}

// ChatMessage is one line of a game's table talk.
type ChatMessage struct {
	ID        int64
	GameID    int64
	Seat      int // 0 for a spectator
	Name      string
	Body      string
	CreatedAt string
}

// AddChatMessage appends a message to the game's chat.
func (s *Store) AddChatMessage(ctx context.Context, gameID int64, seat int, name, principalID, body string) (ChatMessage, error) {
	res, err := s.exec(ctx, "AddChatMessage.insert", `INSERT INTO chat_messages (game_id, seat, name, principal_id, body) VALUES (?, ?, ?, ?, ?)`, gameID, seat, name, principalID, body)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("storing chat message (game_id=%d): %w", gameID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ChatMessage{}, err
	}
	// A game keeps its latest 500 lines.
	if _, err := s.exec(ctx, "AddChatMessage.trim",
		`DELETE FROM chat_messages WHERE game_id = ? AND id <= (
		   SELECT id FROM chat_messages WHERE game_id = ? ORDER BY id DESC LIMIT 1 OFFSET 500)`, gameID, gameID); err != nil {
		return ChatMessage{}, fmt.Errorf("trimming chat (game_id=%d): %w", gameID, err)
	}
	var m ChatMessage
	err = s.queryRow(ctx, "AddChatMessage.readBack", `SELECT id, game_id, seat, name, body, created_at FROM chat_messages WHERE id = ?`, id).
		Scan(&m.ID, &m.GameID, &m.Seat, &m.Name, &m.Body, &m.CreatedAt)
	if err != nil {
		return ChatMessage{}, err
	}
	return m, nil
}

// ListChat returns up to limit messages of the game with id > afterID, oldest
// first. With afterID 0 it returns the latest limit messages.
func (s *Store) ListChat(ctx context.Context, gameID int64, afterID int64, limit int) ([]ChatMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows *Rows
	var err error
	if afterID > 0 {
		rows, err = s.query(ctx, "ListChat",
			`SELECT id, game_id, seat, name, body, created_at FROM chat_messages
			 WHERE game_id = ? AND id > ? ORDER BY id LIMIT ?`, gameID, afterID, limit)
	} else {
		rows, err = s.query(ctx, "ListChat",
			`SELECT id, game_id, seat, name, body, created_at FROM (
			   SELECT id, game_id, seat, name, body, created_at FROM chat_messages
			   WHERE game_id = ? ORDER BY id DESC LIMIT ?) ORDER BY id`, gameID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("listing chat (game_id=%d): %w", gameID, err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]ChatMessage, 0)
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.GameID, &m.Seat, &m.Name, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GlobalChatMessage is one line of the global chat room.
type GlobalChatMessage struct {
	ID        int64
	Name      string
	Body      string
	CreatedAt string
}

// AddGlobalChat appends a message to the global chat room, which keeps its
// latest 500 lines.
func (s *Store) AddGlobalChat(ctx context.Context, name, principalID, body string) (GlobalChatMessage, error) {
	res, err := s.exec(ctx, "AddGlobalChat.insert", `INSERT INTO global_chat (name, principal_id, body) VALUES (?, ?, ?)`, name, principalID, body)
	if err != nil {
		return GlobalChatMessage{}, fmt.Errorf("storing global chat message: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return GlobalChatMessage{}, err
	}
	if _, err := s.exec(ctx, "AddGlobalChat.trim",
		`DELETE FROM global_chat WHERE id <= (SELECT id FROM global_chat ORDER BY id DESC LIMIT 1 OFFSET 500)`); err != nil {
		return GlobalChatMessage{}, fmt.Errorf("trimming global chat: %w", err)
	}
	var m GlobalChatMessage
	if err := s.queryRow(ctx, "AddGlobalChat.readBack", `SELECT id, name, body, created_at FROM global_chat WHERE id = ?`, id).
		Scan(&m.ID, &m.Name, &m.Body, &m.CreatedAt); err != nil {
		return GlobalChatMessage{}, err
	}
	return m, nil
}

// ListGlobalChat returns up to limit messages of the global chat with id >
// afterID, oldest first. With afterID 0 it returns the latest limit messages.
func (s *Store) ListGlobalChat(ctx context.Context, afterID int64, limit int) ([]GlobalChatMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows *Rows
	var err error
	if afterID > 0 {
		rows, err = s.query(ctx, "ListGlobalChat",
			`SELECT id, name, body, created_at FROM global_chat WHERE id > ? ORDER BY id LIMIT ?`, afterID, limit)
	} else {
		rows, err = s.query(ctx, "ListGlobalChat",
			`SELECT id, name, body, created_at FROM (
			   SELECT id, name, body, created_at FROM global_chat ORDER BY id DESC LIMIT ?) ORDER BY id`, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("listing global chat: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]GlobalChatMessage, 0)
	for rows.Next() {
		var m GlobalChatMessage
		if err := rows.Scan(&m.ID, &m.Name, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetLobby marks a game as waiting for its host to start it.
func (s *Store) SetLobby(ctx context.Context, gameID int64, on bool) error {
	res, err := s.exec(ctx, "SetLobby", `UPDATE games SET lobby = ? WHERE id = ?`, boolToInt(on), gameID)
	if err != nil {
		return fmt.Errorf("setting lobby for game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// StartGame makes a waiting game active with turnSeat to move.
func (s *Store) StartGame(ctx context.Context, gameID int64, turnSeat int) error {
	res, err := s.exec(ctx, "StartGame",
		`UPDATE games SET status = 'active', turn_seat = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'waiting'`,
		turnSeat, gameID)
	if err != nil {
		return fmt.Errorf("starting game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MovePlayerSeat gives the player now at fromSeat the open seat toSeat (and
// its color) in a game that has not started.
func (s *Store) MovePlayerSeat(ctx context.Context, gameID int64, fromSeat, toSeat int, color string) error {
	res, err := s.exec(ctx, "MovePlayerSeat",
		`UPDATE players SET seat = ?, color = ? WHERE game_id = ? AND seat = ?`, toSeat, color, gameID, fromSeat)
	if err != nil {
		return fmt.Errorf("moving player (game_id=%d seat %d to %d): %w", gameID, fromSeat, toSeat, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RemovePlayer takes the player at seat out of a game that has not started.
func (s *Store) RemovePlayer(ctx context.Context, gameID int64, seat int) error {
	res, err := s.exec(ctx, "RemovePlayer", `DELETE FROM players WHERE game_id = ? AND seat = ?`, gameID, seat)
	if err != nil {
		return fmt.Errorf("removing player (game_id=%d seat=%d): %w", gameID, seat, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RenamePlayer changes the name shown for the player at seat.
func (s *Store) RenamePlayer(ctx context.Context, gameID int64, seat int, name string) error {
	_, err := s.exec(ctx, "RenamePlayer", `UPDATE players SET name = ? WHERE game_id = ? AND seat = ?`, name, gameID, seat)
	if err != nil {
		return fmt.Errorf("renaming player (game_id=%d seat=%d): %w", gameID, seat, err)
	}
	return nil
}

// DefaultAutoPlayTimeout is the Auto-Play Timeout a person starts from, in
// seconds: 24 hours.
const DefaultAutoPlayTimeout = 24 * 60 * 60

// AutoPlayPrefs is a person's Auto-Play Timeout for one game: when it is
// their turn to act and they have not for TimeoutSeconds, a tactical bot
// plays for them. It is off by default.
type AutoPlayPrefs struct {
	Enabled        bool
	TimeoutSeconds int
}

// SetAutoPlay stores seat's Auto-Play Timeout for the game.
func (s *Store) SetAutoPlay(ctx context.Context, gameID int64, seat int, prefs AutoPlayPrefs) error {
	_, err := s.exec(ctx, "SetAutoPlay",
		`INSERT INTO autoplay_prefs (game_id, seat, enabled, timeout_seconds) VALUES (?, ?, ?, ?)
		 ON CONFLICT(game_id, seat) DO UPDATE SET enabled = excluded.enabled, timeout_seconds = excluded.timeout_seconds`,
		gameID, seat, boolToInt(prefs.Enabled), prefs.TimeoutSeconds)
	if err != nil {
		return fmt.Errorf("storing auto-play prefs (game_id=%d seat=%d): %w", gameID, seat, err)
	}
	return nil
}

// AutoPlay returns seat's Auto-Play Timeout (off, 24 hours, when none was
// saved).
func (s *Store) AutoPlay(ctx context.Context, gameID int64, seat int) (AutoPlayPrefs, error) {
	var enabled, secs int
	err := s.queryRow(ctx, "AutoPlay", `SELECT enabled, timeout_seconds FROM autoplay_prefs WHERE game_id = ? AND seat = ?`, gameID, seat).Scan(&enabled, &secs)
	if errors.Is(err, sql.ErrNoRows) {
		return AutoPlayPrefs{TimeoutSeconds: DefaultAutoPlayTimeout}, nil
	}
	if err != nil {
		return AutoPlayPrefs{}, fmt.Errorf("reading auto-play prefs (game_id=%d seat=%d): %w", gameID, seat, err)
	}
	if secs <= 0 {
		secs = DefaultAutoPlayTimeout
	}
	return AutoPlayPrefs{Enabled: enabled != 0, TimeoutSeconds: secs}, nil
}

// DueAutoPlay is a game whose player to move has an Auto-Play Timeout: the
// session decides, with its clock, whether it has run out.
type DueAutoPlay struct {
	RoomCode       string
	Seat           int
	TimeoutSeconds int
	// UpdatedAt is when the game last changed (a move, or the start): the
	// player to move has been waiting since.
	UpdatedAt string
}

// AutoPlayCandidates lists the active games whose player to move has turned
// the Auto-Play Timeout on.
func (s *Store) AutoPlayCandidates(ctx context.Context) ([]DueAutoPlay, error) {
	rows, err := s.query(ctx, "AutoPlayCandidates",
		`SELECT g.room_code, a.seat, a.timeout_seconds, g.updated_at
		 FROM games g JOIN autoplay_prefs a ON a.game_id = g.id AND a.seat = g.turn_seat
		 WHERE g.status = 'active' AND a.enabled = 1`)
	if err != nil {
		return nil, fmt.Errorf("listing auto-play candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DueAutoPlay
	for rows.Next() {
		var d DueAutoPlay
		if err := rows.Scan(&d.RoomCode, &d.Seat, &d.TimeoutSeconds, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning auto-play candidate: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ChangeLobbyGame gives a game that has not started another definition (the
// cults of its open seats were changed) and the new starting position.
func (s *Store) ChangeLobbyGame(ctx context.Context, gameID int64, gameDefID, boardState string) error {
	res, err := s.exec(ctx, "ChangeLobbyGame",
		`UPDATE games SET game_def_id = ?, board_state = ?, initial_state = '', initial_turn_seat = 0, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status = 'waiting'`,
		gameDefID, boardState, gameID)
	if err != nil {
		return fmt.Errorf("changing the definition of game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
