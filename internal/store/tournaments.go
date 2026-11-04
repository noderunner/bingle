package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// The states of a tournament (the tournaments plan, ADR 0082).
const (
	TournamentDraft        = "draft"
	TournamentRegistration = "registration"
	TournamentCheckIn      = "check_in" // real-time only
	TournamentRunning      = "running"
	TournamentFinished     = "finished"
	TournamentAbandoned    = "abandoned"
)

// The end reasons a tournament gives a game, and the reasons of a result that
// no game was played for.
const (
	EndReasonForfeit   = "forfeit"   // a player dropped out, or was removed
	EndReasonAbandoned = "abandoned" // the tournament stopped
	EndReasonNoShow    = "no_show"   // a player missed the first-move deadline
	EndReasonNoShows   = "no_shows"  // both did: nobody scores
	ResultBye          = "bye"
)

// ErrTournamentFull is returned when a tournament has no room for another entry.
var ErrTournamentFull = errors.New("store: the tournament is full")

// ErrWrongTournamentState is returned when a tournament is not in the state a
// change needs.
var ErrWrongTournamentState = errors.New("store: the tournament is not in that state")

// Tournament is a tournament's row.
type Tournament struct {
	ID              int64
	Name            string
	Description     string
	GameDefID       string
	GameName        string
	Format          string
	ClockSeconds    int
	ClockIncrement  int
	GamesPerPairing int
	Rated           bool
	PieRule         bool
	MinPlayers      int
	MaxPlayers      int
	Rounds          int
	TotalRounds     int
	CurrentRound    int
	Status          string
	EndDetail       string
	Seed            int64
	OrganiserID     int64
	Organiser       string // the organiser's handle
	Entries         int
	CreatedAt       string
	StartedAt       string
	EndedAt         string

	// A real-time tournament (ADR 0090). StartsAt and NextRoundAt are RFC 3339;
	// NextRoundAt is empty but for the break between two rounds.
	RealTime         bool
	StartsAt         string
	CheckInSeconds   int
	RoundBreak       int
	FirstMoveSeconds int
	NextRoundAt      string
}

const tournamentCols = `t.id, t.name, t.description, t.game_def_id, t.game_name, t.format, t.clock_seconds, t.clock_increment, t.games_per_pairing,
	t.rated, t.pie_rule, t.min_players, t.max_players, t.rounds, t.total_rounds, t.current_round, t.status, t.end_detail, t.seed, t.organiser_id, a.handle,
	(SELECT COUNT(*) FROM tournament_entries e WHERE e.tournament_id = t.id AND e.withdrawn = 0),
	t.created_at, COALESCE(t.started_at, ''), COALESCE(t.ended_at, ''),
	t.real_time, t.starts_at, t.check_in_seconds, t.round_break_seconds, t.first_move_seconds, t.next_round_at`

const tournamentFrom = ` FROM tournaments t JOIN accounts a ON a.id = t.organiser_id `

func scanTournament(row rowScanner) (Tournament, error) {
	var t Tournament
	var rated, pie, realTime int
	err := row.Scan(&t.ID, &t.Name, &t.Description, &t.GameDefID, &t.GameName, &t.Format, &t.ClockSeconds, &t.ClockIncrement, &t.GamesPerPairing,
		&rated, &pie, &t.MinPlayers, &t.MaxPlayers, &t.Rounds, &t.TotalRounds, &t.CurrentRound, &t.Status, &t.EndDetail, &t.Seed, &t.OrganiserID, &t.Organiser,
		&t.Entries, &t.CreatedAt, &t.StartedAt, &t.EndedAt,
		&realTime, &t.StartsAt, &t.CheckInSeconds, &t.RoundBreak, &t.FirstMoveSeconds, &t.NextRoundAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Tournament{}, ErrNotFound
	}
	if err != nil {
		return Tournament{}, err
	}
	t.Rated, t.PieRule, t.RealTime = rated != 0, pie != 0, realTime != 0
	return t, nil
}

// CreateTournament stores a new draft and returns its id.
func (s *Store) CreateTournament(ctx context.Context, t Tournament) (int64, error) {
	res, err := s.exec(ctx, "CreateTournament",
		`INSERT INTO tournaments (name, description, game_def_id, game_name, format, clock_seconds, clock_increment, games_per_pairing, rated, pie_rule, min_players, max_players, rounds, organiser_id, real_time, starts_at, check_in_seconds, round_break_seconds, first_move_seconds)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Description, t.GameDefID, t.GameName, t.Format, t.ClockSeconds, t.ClockIncrement, t.GamesPerPairing, boolToInt(t.Rated), boolToInt(t.PieRule), t.MinPlayers, t.MaxPlayers, t.Rounds, t.OrganiserID, boolToInt(t.RealTime), t.StartsAt, t.CheckInSeconds, t.RoundBreak, t.FirstMoveSeconds)
	if err != nil {
		return 0, fmt.Errorf("creating a tournament: %w", err)
	}
	return res.LastInsertId()
}

// UpdateTournament changes the settings of a draft.
func (s *Store) UpdateTournament(ctx context.Context, t Tournament) error {
	res, err := s.exec(ctx, "UpdateTournament",
		`UPDATE tournaments SET name = ?, description = ?, game_def_id = ?, game_name = ?, format = ?, clock_seconds = ?, clock_increment = ?, games_per_pairing = ?, rated = ?, pie_rule = ?, min_players = ?, max_players = ?, rounds = ?, real_time = ?, starts_at = ?, check_in_seconds = ?, round_break_seconds = ?, first_move_seconds = ?
		 WHERE id = ? AND status = ?`,
		t.Name, t.Description, t.GameDefID, t.GameName, t.Format, t.ClockSeconds, t.ClockIncrement, t.GamesPerPairing, boolToInt(t.Rated), boolToInt(t.PieRule), t.MinPlayers, t.MaxPlayers, t.Rounds, boolToInt(t.RealTime), t.StartsAt, t.CheckInSeconds, t.RoundBreak, t.FirstMoveSeconds, t.ID, TournamentDraft)
	if err != nil {
		return fmt.Errorf("changing tournament %d: %w", t.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWrongTournamentState
	}
	return nil
}

// GetTournament reads one tournament.
func (s *Store) GetTournament(ctx context.Context, id int64) (Tournament, error) {
	return scanTournament(s.queryRow(ctx, "GetTournament", `SELECT `+tournamentCols+tournamentFrom+`WHERE t.id = ?`, id))
}

// ListTournaments lists the tournaments anyone may see, and the drafts of the
// given organiser (all drafts when allDrafts), the newest first.
func (s *Store) ListTournaments(ctx context.Context, organiserID int64, allDrafts bool, limit int) ([]Tournament, error) {
	rows, err := s.query(ctx, "ListTournaments", `SELECT `+tournamentCols+tournamentFrom+`
		WHERE (t.status != ? AND (t.status != ? OR t.started_at IS NOT NULL)) OR t.organiser_id = ? OR ?
		ORDER BY CASE t.status WHEN ? THEN 0 WHEN ? THEN 0 WHEN ? THEN 1 ELSE 2 END, t.id DESC LIMIT ?`,
		TournamentDraft, TournamentAbandoned, organiserID, boolToInt(allDrafts), TournamentRegistration, TournamentCheckIn, TournamentRunning, limit)
	if err != nil {
		return nil, fmt.Errorf("listing tournaments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Tournament{}
	for rows.Next() {
		t, err := scanTournament(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RunningTournaments lists the tournaments under way.
func (s *Store) RunningTournaments(ctx context.Context) ([]Tournament, error) {
	rows, err := s.query(ctx, "RunningTournaments", `SELECT `+tournamentCols+tournamentFrom+`WHERE t.status = ? ORDER BY t.id`, TournamentRunning)
	if err != nil {
		return nil, fmt.Errorf("listing running tournaments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Tournament
	for rows.Next() {
		t, err := scanTournament(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// OpenTournament moves a draft on to registration.
func (s *Store) OpenTournament(ctx context.Context, id int64) error {
	res, err := s.exec(ctx, "OpenTournament", `UPDATE tournaments SET status = ? WHERE id = ? AND status = ?`, TournamentRegistration, id, TournamentDraft)
	if err != nil {
		return fmt.Errorf("opening tournament %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWrongTournamentState
	}
	return nil
}

// TournamentEntry is a player in a tournament.
type TournamentEntry struct {
	AccountID int64
	Handle    string
	Rating    float64
	SeedNo    int
	Withdrawn bool
	CheckedIn bool
}

// AddTournamentEntry enters an account, while there is room. Entering twice
// changes nothing, and coming back after leaving during registration is entering
// again.
func (s *Store) AddTournamentEntry(ctx context.Context, tournamentID, accountID int64, limit int) error {
	tx, err := s.begin(ctx, "AddTournamentEntry")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err := tx.queryRow(ctx, "AddTournamentEntry.status", `SELECT status FROM tournaments WHERE id = ?`, tournamentID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status != TournamentRegistration {
		return ErrWrongTournamentState
	}
	var count, mine int
	if err := tx.queryRow(ctx, "AddTournamentEntry.count",
		`SELECT COUNT(*), COALESCE(SUM(account_id = ?), 0) FROM tournament_entries WHERE tournament_id = ?`, accountID, tournamentID).Scan(&count, &mine); err != nil {
		return err
	}
	if mine > 0 {
		return tx.Commit()
	}
	if count >= limit {
		return ErrTournamentFull
	}
	if _, err := tx.exec(ctx, "AddTournamentEntry.insert", `INSERT INTO tournament_entries (tournament_id, account_id) VALUES (?, ?)`, tournamentID, accountID); err != nil {
		return fmt.Errorf("entering tournament %d: %w", tournamentID, err)
	}
	return tx.Commit()
}

// RemoveTournamentEntry takes an entry out before the tournament starts.
func (s *Store) RemoveTournamentEntry(ctx context.Context, tournamentID, accountID int64) error {
	res, err := s.exec(ctx, "RemoveTournamentEntry",
		`DELETE FROM tournament_entries WHERE tournament_id = ? AND account_id = ?
		   AND (SELECT status FROM tournaments WHERE id = ?) IN (?, ?)`, tournamentID, accountID, tournamentID, TournamentRegistration, TournamentCheckIn)
	if err != nil {
		return fmt.Errorf("leaving tournament %d: %w", tournamentID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWrongTournamentState
	}
	return nil
}

// WithdrawTournamentEntry marks a player of a running tournament as out of it.
// It says whether they were still in it.
func (s *Store) WithdrawTournamentEntry(ctx context.Context, tournamentID, accountID int64) (bool, error) {
	res, err := s.exec(ctx, "WithdrawTournamentEntry",
		`UPDATE tournament_entries SET withdrawn = 1 WHERE tournament_id = ? AND account_id = ? AND withdrawn = 0`, tournamentID, accountID)
	if err != nil {
		return false, fmt.Errorf("withdrawing from tournament %d: %w", tournamentID, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// TournamentEntries lists a tournament's players.
func (s *Store) TournamentEntries(ctx context.Context, tournamentID int64) ([]TournamentEntry, error) {
	rows, err := s.query(ctx, "TournamentEntries",
		`SELECT e.account_id, a.handle, e.rating, e.seed_no, e.withdrawn, e.checked_in FROM tournament_entries e JOIN accounts a ON a.id = e.account_id
		 WHERE e.tournament_id = ? ORDER BY e.seed_no, e.entered_at, e.account_id`, tournamentID)
	if err != nil {
		return nil, fmt.Errorf("reading the entries of tournament %d: %w", tournamentID, err)
	}
	defer func() { _ = rows.Close() }()
	out := []TournamentEntry{}
	for rows.Next() {
		var e TournamentEntry
		var withdrawn, checkedIn int
		if err := rows.Scan(&e.AccountID, &e.Handle, &e.Rating, &e.SeedNo, &withdrawn, &checkedIn); err != nil {
			return nil, err
		}
		e.Withdrawn, e.CheckedIn = withdrawn != 0, checkedIn != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// StartTournament moves a tournament from the given state to running with its seed,
// the seeding of its players and the number of its rounds.
func (s *Store) StartTournament(ctx context.Context, id int64, from string, seed int64, totalRounds int, entries []TournamentEntry, now time.Time) error {
	tx, err := s.begin(ctx, "StartTournament")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.exec(ctx, "StartTournament.status",
		`UPDATE tournaments SET status = ?, seed = ?, total_rounds = ?, started_at = ? WHERE id = ? AND status = ?`,
		TournamentRunning, seed, totalRounds, now.UTC().Format(time.RFC3339Nano), id, from)
	if err != nil {
		return fmt.Errorf("starting tournament %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWrongTournamentState
	}
	// Somebody entering or leaving while the caller was reading would throw the
	// number of rounds out: it says so, and the organiser presses again.
	var count int
	if err := tx.queryRow(ctx, "StartTournament.count", `SELECT COUNT(*) FROM tournament_entries WHERE tournament_id = ?`, id).Scan(&count); err != nil {
		return err
	}
	if count != len(entries) {
		return ErrWrongTournamentState
	}
	for _, e := range entries {
		if _, err := tx.exec(ctx, "StartTournament.entry",
			`UPDATE tournament_entries SET rating = ?, seed_no = ? WHERE tournament_id = ? AND account_id = ?`, e.Rating, e.SeedNo, id, e.AccountID); err != nil {
			return fmt.Errorf("seeding tournament %d: %w", id, err)
		}
	}
	return tx.Commit()
}

// EndTournament finishes or abandons a running tournament. It says whether this
// call was the one that did.
func (s *Store) EndTournament(ctx context.Context, id int64, status, detail string, now time.Time) (bool, error) {
	res, err := s.exec(ctx, "EndTournament",
		`UPDATE tournaments SET status = ?, end_detail = ?, ended_at = ? WHERE id = ? AND status NOT IN (?, ?)`,
		status, detail, now.UTC().Format(time.RFC3339Nano), id, TournamentFinished, TournamentAbandoned)
	if err != nil {
		return false, fmt.Errorf("ending tournament %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// TournamentGame is one line of a tournament's record.
type TournamentGame struct {
	ID           int64
	TournamentID int64
	Round        int
	Board        int
	GameNo       int
	PlayerA      int64
	PlayerB      int64 // 0 for a bye
	GameID       int64 // 0 when there is no game (any more)
	RoomCode     string
	Done         bool
	ScoreA       float64
	ScoreB       float64
	Reason       string
}

const tournamentGameCols = `id, tournament_id, round, board, game_no, player_a, COALESCE(player_b, 0), COALESCE(game_id, 0), room_code, score_a, score_b, reason`

func scanTournamentGame(row rowScanner) (TournamentGame, error) {
	var g TournamentGame
	var a, b sql.NullFloat64
	if err := row.Scan(&g.ID, &g.TournamentID, &g.Round, &g.Board, &g.GameNo, &g.PlayerA, &g.PlayerB, &g.GameID, &g.RoomCode, &a, &b, &g.Reason); err != nil {
		return TournamentGame{}, err
	}
	g.Done, g.ScoreA, g.ScoreB = a.Valid, a.Float64, b.Float64
	return g, nil
}

func (s *Store) tournamentGames(ctx context.Context, name, where string, args ...any) ([]TournamentGame, error) {
	rows, err := s.query(ctx, name, `SELECT `+tournamentGameCols+` FROM tournament_games WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("reading tournament games: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []TournamentGame{}
	for rows.Next() {
		g, err := scanTournamentGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// TournamentGames lists a tournament's record, round by round.
func (s *Store) TournamentGames(ctx context.Context, tournamentID int64) ([]TournamentGame, error) {
	return s.tournamentGames(ctx, "TournamentGames", `tournament_id = ? ORDER BY round, board, game_no`, tournamentID)
}

// OpenTournamentGames lists a tournament's games with no result yet.
func (s *Store) OpenTournamentGames(ctx context.Context, tournamentID int64) ([]TournamentGame, error) {
	return s.tournamentGames(ctx, "OpenTournamentGames", `tournament_id = ? AND score_a IS NULL ORDER BY round, board, game_no`, tournamentID)
}

// SetTournamentResult writes the result of a line of the record, once.
func (s *Store) SetTournamentResult(ctx context.Context, id int64, scoreA, scoreB float64, reason string) error {
	if _, err := s.exec(ctx, "SetTournamentResult",
		`UPDATE tournament_games SET score_a = ?, score_b = ?, reason = ? WHERE id = ? AND score_a IS NULL`, scoreA, scoreB, reason, id); err != nil {
		return fmt.Errorf("writing the result of tournament game %d: %w", id, err)
	}
	return nil
}

// NewTournamentGame is one game of a round to be made. With no RoomCode no game
// is made, and the scores are its result: a bye or a forfeit. Deferred, it has no
// result either, and the game comes later (StartTournamentGame).
type NewTournamentGame struct {
	Board, GameNo    int
	PlayerA, PlayerB int64
	ScoreA, ScoreB   float64
	Reason           string
	Deferred         bool

	RoomCode, GameDefID, BoardState, Clock, RealTime string
	FirstSeat                                        int
	Rated                                            bool
	Seats                                            [2]NewSeat
}

// NewSeat is a seat of a tournament game.
type NewSeat struct {
	Seat        int
	Name, Color string
	PrincipalID string
}

// CreateTournamentRound makes a round's games and moves the tournament on to it,
// all at once, so a crash leaves a whole round or none. It returns the games made,
// in the order given (0 for a line with no game). A room code already in use fails
// the whole round, for the caller to try again with others.
func (s *Store) CreateTournamentRound(ctx context.Context, tournamentID int64, round int, games []NewTournamentGame) ([]int64, error) {
	tx, err := s.begin(ctx, "CreateTournamentRound")
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.exec(ctx, "CreateTournamentRound.round",
		`UPDATE tournaments SET current_round = ?, next_round_at = '' WHERE id = ? AND status = ? AND current_round = ?`, round, tournamentID, TournamentRunning, round-1)
	if err != nil {
		return nil, fmt.Errorf("starting round %d of tournament %d: %w", round, tournamentID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrWrongTournamentState
	}
	ids := make([]int64, len(games))
	for i, g := range games {
		var gameID sql.NullInt64
		var playerB sql.NullInt64
		if g.PlayerB != 0 {
			playerB = sql.NullInt64{Int64: g.PlayerB, Valid: true}
		}
		var scoreA, scoreB sql.NullFloat64
		switch {
		case g.Deferred:
		case g.RoomCode == "":
			scoreA, scoreB = sql.NullFloat64{Float64: g.ScoreA, Valid: true}, sql.NullFloat64{Float64: g.ScoreB, Valid: true}
		default:
			id, err := insertTournamentGame(ctx, tx, tournamentID, g)
			if err != nil {
				return nil, err
			}
			gameID = sql.NullInt64{Int64: id, Valid: true}
			ids[i] = id
		}
		if _, err := tx.exec(ctx, "CreateTournamentRound.line",
			`INSERT INTO tournament_games (tournament_id, round, board, game_no, player_a, player_b, game_id, room_code, score_a, score_b, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tournamentID, round, g.Board, g.GameNo, g.PlayerA, playerB, gameID, g.RoomCode, scoreA, scoreB, g.Reason); err != nil {
			return nil, fmt.Errorf("recording a game of tournament %d: %w", tournamentID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}

// VoidGame ends a game with no winner and no change to anybody's rating, in one
// write, so that nothing can ever pick its result up.
func (s *Store) VoidGame(ctx context.Context, gameID int64, reason string) error {
	if _, err := s.exec(ctx, "VoidGame",
		`UPDATE games SET status = 'finished', impulse_seat = NULL, winner_seat = NULL, end_reason = ?, draw_offer_seat = NULL,
			rating_status = CASE rated WHEN 1 THEN ? ELSE rating_status END, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status != 'finished'`, reason, RatingVoid, gameID); err != nil {
		return fmt.Errorf("voiding game_id=%d: %w", gameID, err)
	}
	return nil
}

// AccountContact is the contact turn mail for an account's tournament games goes
// through: its own row, which no browser holds the secret of, so that nobody can
// point it at another address. It follows the account's address. secretHash and
// stopToken are used only for a new row.
func (s *Store) AccountContact(ctx context.Context, accountID int64, address, addrHash, secretHash, stopToken string, now time.Time) (int64, error) {
	var id int64
	var old string
	err := s.queryRow(ctx, "AccountContact.read", `SELECT id, addr_hash FROM notify_contacts WHERE account_id = ?`, accountID).Scan(&id, &old)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := s.exec(ctx, "AccountContact.insert",
			`INSERT INTO notify_contacts (address, addr_hash, secret_hash, stop_token, verified_at, account_id) VALUES (?, ?, ?, ?, ?, ?)`,
			address, addrHash, secretHash, stopToken, stamp(now), accountID)
		if err != nil {
			return 0, fmt.Errorf("making the contact of account %d: %w", accountID, err)
		}
		return res.LastInsertId()
	case err != nil:
		return 0, err
	}
	if old != addrHash {
		if _, err := s.exec(ctx, "AccountContact.update", `UPDATE notify_contacts SET address = ?, addr_hash = ?, verified_at = ? WHERE id = ?`, address, addrHash, stamp(now), id); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// PendingRealTimeTournaments lists the real-time tournaments that have not started,
// for the sweep to open their check-in and start them on time.
func (s *Store) PendingRealTimeTournaments(ctx context.Context) ([]Tournament, error) {
	rows, err := s.query(ctx, "PendingRealTimeTournaments",
		`SELECT `+tournamentCols+tournamentFrom+`WHERE t.status IN (?, ?) AND t.real_time = 1 ORDER BY t.id`, TournamentRegistration, TournamentCheckIn)
	if err != nil {
		return nil, fmt.Errorf("listing real-time tournaments to start: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Tournament
	for rows.Next() {
		t, err := scanTournament(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// OpenTournamentCheckIn moves a tournament from registration to check-in.
func (s *Store) OpenTournamentCheckIn(ctx context.Context, id int64) error {
	if _, err := s.exec(ctx, "OpenTournamentCheckIn", `UPDATE tournaments SET status = ? WHERE id = ? AND status = ?`, TournamentCheckIn, id, TournamentRegistration); err != nil {
		return fmt.Errorf("opening the check-in of tournament %d: %w", id, err)
	}
	return nil
}

// CheckInTournamentEntry checks a player in, and says whether they have an entry.
// Doing it twice changes nothing.
func (s *Store) CheckInTournamentEntry(ctx context.Context, tournamentID, accountID int64) (bool, error) {
	res, err := s.exec(ctx, "CheckInTournamentEntry",
		`UPDATE tournament_entries SET checked_in = 1 WHERE tournament_id = ? AND account_id = ?`, tournamentID, accountID)
	if err != nil {
		return false, fmt.Errorf("checking in to tournament %d: %w", tournamentID, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DropUncheckedTournamentEntries takes out the players who have not checked in.
func (s *Store) DropUncheckedTournamentEntries(ctx context.Context, tournamentID int64) error {
	if _, err := s.exec(ctx, "DropUncheckedTournamentEntries",
		`DELETE FROM tournament_entries WHERE tournament_id = ? AND checked_in = 0`, tournamentID); err != nil {
		return fmt.Errorf("dropping the players who did not check in to tournament %d: %w", tournamentID, err)
	}
	return nil
}

// MarkTournamentStarted notes when a tournament that never got to play was due to
// start, which keeps it in the list for the people who entered it.
func (s *Store) MarkTournamentStarted(ctx context.Context, id int64, now time.Time) error {
	if _, err := s.exec(ctx, "MarkTournamentStarted", `UPDATE tournaments SET started_at = ? WHERE id = ?`, now.UTC().Format(time.RFC3339Nano), id); err != nil {
		return fmt.Errorf("marking tournament %d started: %w", id, err)
	}
	return nil
}

// SetTournamentNextRound notes when the next round starts, after a break.
func (s *Store) SetTournamentNextRound(ctx context.Context, id int64, at time.Time) error {
	if _, err := s.exec(ctx, "SetTournamentNextRound", `UPDATE tournaments SET next_round_at = ? WHERE id = ?`, at.UTC().Format(time.RFC3339), id); err != nil {
		return fmt.Errorf("setting the next round of tournament %d: %w", id, err)
	}
	return nil
}

// SeatsWithMoves lists the seats with a move of their own in a game, as
// SeatsThatMoved counts them.
func (s *Store) SeatsWithMoves(ctx context.Context, gameID int64) ([]int, error) {
	rows, err := s.query(ctx, "SeatsWithMoves", `SELECT DISTINCT seat FROM moves WHERE game_id = ? AND auto = 0 AND autoplay = 0`, gameID)
	if err != nil {
		return nil, fmt.Errorf("listing the seats that moved in game_id=%d: %w", gameID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []int
	for rows.Next() {
		var seat int
		if err := rows.Scan(&seat); err != nil {
			return nil, err
		}
		out = append(out, seat)
	}
	return out, rows.Err()
}

// insertTournamentGame makes the game of a line and seats its players.
func insertTournamentGame(ctx context.Context, tx *Tx, tournamentID int64, g NewTournamentGame) (int64, error) {
	res, err := tx.exec(ctx, "insertTournamentGame.game",
		`INSERT INTO games (room_code, game_def_id, board_state, status, impulse_seat, clock, rated, tournament_id, real_time) VALUES (?, ?, ?, 'active', ?, ?, ?, ?, ?)`,
		g.RoomCode, g.GameDefID, g.BoardState, g.FirstSeat, g.Clock, boolToInt(g.Rated), tournamentID, g.RealTime)
	if err != nil {
		return 0, fmt.Errorf("making a game of tournament %d: %w", tournamentID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, seat := range g.Seats {
		if _, err := tx.exec(ctx, "insertTournamentGame.player",
			`INSERT INTO players (game_id, seat, name, color, principal_id) VALUES (?, ?, ?, ?, ?)`, id, seat.Seat, seat.Name, seat.Color, seat.PrincipalID); err != nil {
			return 0, fmt.Errorf("seating a game of tournament %d: %w", tournamentID, err)
		}
	}
	return id, nil
}

// StartTournamentGame makes the game of a line that was waiting for one: the second
// game of a real-time pairing, once the first is over.
func (s *Store) StartTournamentGame(ctx context.Context, lineID, tournamentID int64, g NewTournamentGame) error {
	tx, err := s.begin(ctx, "StartTournamentGame")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	id, err := insertTournamentGame(ctx, tx, tournamentID, g)
	if err != nil {
		return err
	}
	res, err := tx.exec(ctx, "StartTournamentGame.line",
		`UPDATE tournament_games SET game_id = ?, room_code = ? WHERE id = ? AND room_code = '' AND score_a IS NULL`, id, g.RoomCode, lineID)
	if err != nil {
		return fmt.Errorf("recording a game of tournament %d: %w", tournamentID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWrongTournamentState
	}
	return tx.Commit()
}
