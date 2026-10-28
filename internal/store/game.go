package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Game mirrors one row of the games table (see schema.sql and docs/history/plan.md
// section 6.6a).
type Game struct {
	ID         int64
	RoomCode   string
	GameDefID  string
	Status     string
	BoardState string
	TurnSeat   *int // nil ⇒ SQL NULL (game is waiting/finished)
	MoveSeq    int
	WinnerSeat *int // nil ⇒ SQL NULL
	// EndReason says why a finished game ended ("road", "resignation", ...);
	// empty while the game is unfinished.
	EndReason string
	// EndDetail adds specifics to EndReason, such as a final score.
	EndDetail string
	// MoveTimeLimit is the per-move time allowance in seconds (0 = none).
	// The current turn started at UpdatedAt.
	MoveTimeLimit int
	// Clock is the game clock (a chess clock) as JSON, empty when the game
	// is played without one: what each seat started with and what it has
	// left (see session.GameClock). The clock of the seat to move has been
	// running since UpdatedAt.
	Clock string
	// DrawOfferSeat is the seat currently offering a draw, or nil. Any move
	// (or undo) withdraws the offer.
	DrawOfferSeat *int
	// InitialState is the starting position document when the game began
	// from a custom position (a puzzle, a position from analysis); empty
	// means the game definition's standard start. InitialTurnSeat is who
	// moves first from it (0 = the definition's first seat).
	InitialState    string
	InitialTurnSeat int
	// TakebackSeat is the seat asking to take back moves, and TakebackSeq
	// the move sequence number the game would be rewound to (the position
	// after that many moves); nil when no request is open. Any move
	// withdraws the request.
	TakebackSeat *int
	TakebackSeq  int
	// Lobby says the game waits for its host to start it: it does not begin
	// by itself when the last seat fills.
	Lobby     bool
	CreatedAt string
	UpdatedAt string
}

// gameColumns is the column list every games SELECT uses, matching
// scanGameRow.
const gameColumns = `id, room_code, game_def_id, status, board_state, turn_seat, move_seq, winner_seat, end_reason, end_detail, move_time_limit, draw_offer_seat, initial_state, initial_turn_seat, takeback_seat, takeback_seq, created_at, updated_at, lobby, clock`

type rowScanner interface{ Scan(dest ...any) error }

func scanGameRow(row rowScanner) (Game, error) {
	var g Game
	var turnSeat, winnerSeat, drawOffer, takebackSeat sql.NullInt64
	var lobby int
	err := row.Scan(&g.ID, &g.RoomCode, &g.GameDefID, &g.Status, &g.BoardState, &turnSeat, &g.MoveSeq, &winnerSeat,
		&g.EndReason, &g.EndDetail, &g.MoveTimeLimit, &drawOffer, &g.InitialState, &g.InitialTurnSeat,
		&takebackSeat, &g.TakebackSeq, &g.CreatedAt, &g.UpdatedAt, &lobby, &g.Clock)
	if err != nil {
		return Game{}, err
	}
	g.Lobby = lobby != 0
	g.TurnSeat = nullInt64ToIntPtr(turnSeat)
	g.WinnerSeat = nullInt64ToIntPtr(winnerSeat)
	g.DrawOfferSeat = nullInt64ToIntPtr(drawOffer)
	g.TakebackSeat = nullInt64ToIntPtr(takebackSeat)
	return g, nil
}

// Player mirrors one row of the players table (see schema.sql).
type Player struct {
	ID     int64
	GameID int64
	Seat   int
	Name   string
	Color  string
	// Bot names the built-in bot playing this seat ("" for a human).
	Bot        string
	JoinedAt   string
	LastSeenAt string
}

// ErrNotFound is returned by lookups (e.g. GetGame) when no matching row
// exists. Callers should check for it via errors.Is.
var ErrNotFound = errors.New("store: not found")

// CreateGame inserts one games row (status "waiting", the given
// initialBoardState, move_seq 0, turn_seat and winner_seat left NULL) and one
// players row for the creator at seat 1, in a single transaction — both rows
// exist or neither does. Returns the freshly-inserted Game and Player
// (re-read after commit, so DEFAULT-populated columns like timestamps are
// accurate).
func (s *Store) CreateGame(ctx context.Context, roomCode, gameDefID, initialBoardState, creatorName, creatorColor string) (Game, Player, error) {
	return s.CreateGameAt(ctx, roomCode, gameDefID, initialBoardState, creatorName, creatorColor, 1)
}

// CreateGameAt is CreateGame with the creator seated at creatorSeat rather
// than seat 1.
func (s *Store) CreateGameAt(ctx context.Context, roomCode, gameDefID, initialBoardState, creatorName, creatorColor string, creatorSeat int) (Game, Player, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Game{}, Player{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO games (room_code, game_def_id, board_state) VALUES (?, ?, ?)`,
		roomCode, gameDefID, initialBoardState,
	)
	if err != nil {
		return Game{}, Player{}, fmt.Errorf("insert games row: %w", err)
	}
	gameID, err := res.LastInsertId()
	if err != nil {
		return Game{}, Player{}, fmt.Errorf("games insert LastInsertId: %w", err)
	}

	playerRes, err := tx.ExecContext(ctx,
		`INSERT INTO players (game_id, seat, name, color) VALUES (?, ?, ?, ?)`,
		gameID, creatorSeat, creatorName, creatorColor,
	)
	if err != nil {
		return Game{}, Player{}, fmt.Errorf("insert players row: %w", err)
	}
	playerID, err := playerRes.LastInsertId()
	if err != nil {
		return Game{}, Player{}, fmt.Errorf("players insert LastInsertId: %w", err)
	}

	game, err := scanGameByID(ctx, tx, gameID)
	if err != nil {
		return Game{}, Player{}, fmt.Errorf("reading back inserted game: %w", err)
	}
	player, err := scanPlayerByID(ctx, tx, playerID)
	if err != nil {
		return Game{}, Player{}, fmt.Errorf("reading back inserted player: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Game{}, Player{}, fmt.Errorf("commit transaction: %w", err)
	}

	return game, player, nil
}

// GetGame looks up a game by room code. Returns ErrNotFound (checkable via
// errors.Is) if no game has that room code.
func (s *Store) GetGame(ctx context.Context, roomCode string) (Game, error) {
	g, err := scanGameRow(s.db.QueryRowContext(ctx, `SELECT `+gameColumns+` FROM games WHERE room_code = ?`, roomCode))
	if errors.Is(err, sql.ErrNoRows) {
		return Game{}, ErrNotFound
	}
	if err != nil {
		return Game{}, fmt.Errorf("scanning games row (room_code=%q): %w", roomCode, err)
	}
	return g, nil
}

// SetInitialPosition records that a game began from a custom position: the
// position document and the seat that moves first from it. It does not
// touch board_state; callers create the game with the same document.
func (s *Store) SetInitialPosition(ctx context.Context, gameID int64, doc string, turnSeat int) error {
	res, err := s.db.ExecContext(ctx, `UPDATE games SET initial_state = ?, initial_turn_seat = ? WHERE id = ?`, doc, turnSeat, gameID)
	if err != nil {
		return fmt.Errorf("setting initial position for game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetTakebackRequest records (or, with a nil seat, clears) a takeback
// request: seat asks to rewind the game to the position after toSeq moves.
func (s *Store) SetTakebackRequest(ctx context.Context, gameID int64, seat *int, toSeq int) error {
	var v sql.NullInt64
	if seat != nil {
		v = sql.NullInt64{Int64: int64(*seat), Valid: true}
	} else {
		toSeq = 0
	}
	res, err := s.db.ExecContext(ctx, `UPDATE games SET takeback_seat = ?, takeback_seq = ? WHERE id = ?`, v, toSeq, gameID)
	if err != nil {
		return fmt.Errorf("setting takeback request for game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDrawOffer records (or, with nil, clears) a draw offer on a game.
func (s *Store) SetDrawOffer(ctx context.Context, gameID int64, seat *int) error {
	var v sql.NullInt64
	if seat != nil {
		v = sql.NullInt64{Int64: int64(*seat), Valid: true}
	}
	res, err := s.db.ExecContext(ctx, `UPDATE games SET draw_offer_seat = ? WHERE id = ?`, v, gameID)
	if err != nil {
		return fmt.Errorf("setting draw_offer_seat for game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetGameClock stores a game's clock (JSON; empty for no clock). It is set
// when the game is created and then only by RecordMove, which charges the
// mover in the same transaction as their move.
func (s *Store) SetGameClock(ctx context.Context, gameID int64, clock string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE games SET clock = ? WHERE id = ?`, clock, gameID)
	if err != nil {
		return fmt.Errorf("setting clock for game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMoveTimeLimit sets a game's per-move time allowance in seconds (0
// removes it). Returns ErrNotFound if gameID doesn't exist.
func (s *Store) SetMoveTimeLimit(ctx context.Context, gameID int64, seconds int) error {
	res, err := s.db.ExecContext(ctx, `UPDATE games SET move_time_limit = ? WHERE id = ?`, seconds, gameID)
	if err != nil {
		return fmt.Errorf("setting move_time_limit for game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// queryRowContexter is the subset of *sql.Tx / *sql.DB used by
// scanGameByID/scanPlayerByID, letting them run against either.
type queryRowContexter interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func scanGameByID(ctx context.Context, q queryRowContexter, id int64) (Game, error) {
	return scanGameRow(q.QueryRowContext(ctx, `SELECT `+gameColumns+` FROM games WHERE id = ?`, id))
}

func scanPlayerByID(ctx context.Context, q queryRowContexter, id int64) (Player, error) {
	var p Player

	row := q.QueryRowContext(ctx,
		`SELECT id, game_id, seat, name, color, bot, joined_at, last_seen_at
		 FROM players WHERE id = ?`,
		id,
	)
	if err := row.Scan(&p.ID, &p.GameID, &p.Seat, &p.Name, &p.Color, &p.Bot, &p.JoinedAt, &p.LastSeenAt); err != nil {
		return Player{}, err
	}

	return p, nil
}

// ListPlayers returns every players row for gameID, in seat order.
func (s *Store) ListPlayers(ctx context.Context, gameID int64) ([]Player, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, game_id, seat, name, color, bot, joined_at, last_seen_at
		 FROM players WHERE game_id = ? ORDER BY seat`,
		gameID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying players row (game_id=%d): %w", gameID, err)
	}
	defer func() { _ = rows.Close() }()

	var players []Player
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.ID, &p.GameID, &p.Seat, &p.Name, &p.Color, &p.Bot, &p.JoinedAt, &p.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scanning players row (game_id=%d): %w", gameID, err)
		}
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating players rows (game_id=%d): %w", gameID, err)
	}

	return players, nil
}

// AddPlayer inserts one players row (gameID, seat, name, color) and updates
// the game's status (and, only when newTurnSeat is non-nil, its turn_seat
// too) in one transaction — both happen or neither does. Used when a new
// (non-reconnecting) player joins; newStatus is whatever the caller
// (Manager) has already decided the game's status should become (e.g.
// "active" once the last seat fills, or unchanged otherwise). newTurnSeat is
// non-nil only when this join is the one that activates the game, in which
// case it's the first seat in the game definition's turn order; when nil,
// turn_seat is left untouched. Returns the freshly-inserted Player (re-read
// after commit, so DEFAULT-populated columns like timestamps are accurate).
func (s *Store) AddPlayer(ctx context.Context, gameID int64, seat int, name, color, newStatus string, newTurnSeat *int) (Player, error) {
	return s.AddBotPlayer(ctx, gameID, seat, name, color, "", newStatus, newTurnSeat)
}

// AddBotPlayer is AddPlayer for a seat that may be played by a built-in bot
// (bot != "" names which one; "" is an ordinary human player).
func (s *Store) AddBotPlayer(ctx context.Context, gameID int64, seat int, name, color, bot, newStatus string, newTurnSeat *int) (Player, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Player{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO players (game_id, seat, name, color, bot) VALUES (?, ?, ?, ?, ?)`,
		gameID, seat, name, color, bot,
	)
	if err != nil {
		return Player{}, fmt.Errorf("insert players row: %w", err)
	}
	playerID, err := res.LastInsertId()
	if err != nil {
		return Player{}, fmt.Errorf("players insert LastInsertId: %w", err)
	}

	if newTurnSeat != nil {
		if _, err := tx.ExecContext(ctx,
			`UPDATE games SET status = ?, turn_seat = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			newStatus, *newTurnSeat, gameID,
		); err != nil {
			return Player{}, fmt.Errorf("updating games row for game_id=%d: %w", gameID, err)
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`UPDATE games SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			newStatus, gameID,
		); err != nil {
			return Player{}, fmt.Errorf("updating games row for game_id=%d: %w", gameID, err)
		}
	}

	player, err := scanPlayerByID(ctx, tx, playerID)
	if err != nil {
		return Player{}, fmt.Errorf("reading back inserted player: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Player{}, fmt.Errorf("commit transaction: %w", err)
	}

	return player, nil
}

// nullInt64ToIntPtr converts a nullable SQL integer column into *int, per
// the Game struct's documented NULL-as-nil convention.
func nullInt64ToIntPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// DeleteInactiveGames removes every game whose last activity is before
// cutoff, with its players and moves (ON DELETE CASCADE), and returns how
// many games went.
func (s *Store) DeleteInactiveGames(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM games WHERE updated_at < ?`, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, fmt.Errorf("deleting inactive games: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// SetPlayerBot marks the player at seat in gameID as played by the named
// built-in bot (used for bot-versus-bot rooms).
func (s *Store) SetPlayerBot(ctx context.Context, gameID int64, seat int, bot string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE players SET bot = ? WHERE game_id = ? AND seat = ?`, bot, gameID, seat)
	if err != nil {
		return fmt.Errorf("setting bot for game_id=%d seat=%d: %w", gameID, seat, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UnfinishedGame is one game that has not ended, for the list of games
// anybody can look in on. It carries what such a list shows without reading
// each game's position: who is seated, and when it last moved on.
type UnfinishedGame struct {
	GameID    int64
	RoomCode  string
	GameDefID string
	Status    string
	Lobby     bool
	TurnSeat  *int
	MoveSeq   int
	// Seated is how many of the game's seats hold a player or a bot.
	Seated    int
	UpdatedAt string
}

// ListUnfinishedGames returns the games that are still going — waiting for
// players, in a lobby, or being played — newest change first, at most limit of
// them.
func (s *Store) ListUnfinishedGames(ctx context.Context, limit int) ([]UnfinishedGame, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT g.id, g.room_code, g.game_def_id, g.status, g.lobby, g.turn_seat, g.move_seq, g.updated_at,
		        (SELECT COUNT(*) FROM players p WHERE p.game_id = g.id)
		 FROM games g WHERE g.status != 'finished'
		 ORDER BY g.updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing unfinished games: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []UnfinishedGame
	for rows.Next() {
		var g UnfinishedGame
		var lobby int
		var turnSeat sql.NullInt64
		if err := rows.Scan(&g.GameID, &g.RoomCode, &g.GameDefID, &g.Status, &lobby, &turnSeat, &g.MoveSeq, &g.UpdatedAt, &g.Seated); err != nil {
			return nil, fmt.Errorf("scanning unfinished game: %w", err)
		}
		g.Lobby = lobby != 0
		g.TurnSeat = nullInt64ToIntPtr(turnSeat)
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing unfinished games: %w", err)
	}
	return out, nil
}
