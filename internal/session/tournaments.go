package session

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session/roomcode"
	"github.com/noderunner/bingle/internal/store"
	"github.com/noderunner/bingle/internal/tournament"
)

// A tournament (the tournaments plan, ADR 0082) is run from here: who may do what,
// and a sweep that reads the results of finished games and starts the next round.
// The games are ordinary games, with a clock, between two accounts, and what they
// come to is copied to the tournament's own tables, so that they can go when
// retention wants them once the tournament is over.

// Errors of tournaments.
var (
	ErrInvalidTournament = errors.New("session: invalid tournament")
	ErrNotOrganiser      = errors.New("session: only the organiser may do that")
	ErrTournamentAccount = errors.New("session: tournaments are for accounts: sign in")
	ErrMayNotEnter       = errors.New("session: you may not enter tournaments here")
	ErrNotEntered        = errors.New("session: that player is not in this tournament")
	ErrTooFewEntries     = errors.New("session: not enough players have entered yet")
)

// Limits of a tournament.
const (
	maxTournamentPlayers     = 128
	maxTournamentRounds      = 64
	maxTournamentName        = 80
	maxTournamentDescription = 2000
	maxTournamentList        = 100
	tournamentSweep          = time.Minute
	roundAttempts            = 5
)

// TournamentSettings is what an organiser chooses. A pointer that is nil takes
// the default.
type TournamentSettings struct {
	Name                  string `json:"name"`
	Description           string `json:"description,omitempty"`
	GameDefID             string `json:"game_def_id"`
	Format                string `json:"format"`
	ClockSeconds          int    `json:"clock_seconds"`
	ClockIncrementSeconds int    `json:"clock_increment_seconds,omitempty"`
	GamesPerPairing       int    `json:"games_per_pairing,omitempty"` // 1 or 2; 0: 2
	Rated                 *bool  `json:"rated,omitempty"`             // default on
	PieRule               *bool  `json:"pie_rule,omitempty"`          // default on, where the game offers it
	MinPlayers            int    `json:"min_players,omitempty"`
	MaxPlayers            int    `json:"max_players,omitempty"`
	Rounds                int    `json:"rounds,omitempty"` // Swiss; 0: from the number of players
}

// settled checks settings and gives the row they make.
func (m *Manager) settled(s TournamentSettings) (store.Tournament, error) {
	bad := func(format string, args ...any) (store.Tournament, error) {
		return store.Tournament{}, fmt.Errorf("%w: "+format, append([]any{ErrInvalidTournament}, args...)...)
	}
	name := strings.TrimSpace(s.Name)
	if name == "" || utf8.RuneCountInString(name) > maxTournamentName {
		return bad("a name of 1 to %d characters", maxTournamentName)
	}
	if utf8.RuneCountInString(s.Description) > maxTournamentDescription {
		return bad("a description of at most %d characters", maxTournamentDescription)
	}
	if s.Format != tournament.RoundRobin && s.Format != tournament.Swiss {
		return bad("the format is %q or %q", tournament.RoundRobin, tournament.Swiss)
	}
	compiled, ok := m.catalog.Get(s.GameDefID)
	if !ok {
		return store.Tournament{}, fmt.Errorf("%w: %q", ErrGameDefNotFound, s.GameDefID)
	}
	if len(compiled.Definition.Seats) != 2 {
		return bad("a tournament is for a game of two seats")
	}
	if s.ClockSeconds <= 0 || s.ClockSeconds > MaxClockSeconds {
		return bad("a game clock of 1 to %d seconds: every tournament game has one", MaxClockSeconds)
	}
	if s.ClockIncrementSeconds < 0 || s.ClockIncrementSeconds > MaxClockIncrementSeconds {
		return bad("an increment of 0 to %d seconds", MaxClockIncrementSeconds)
	}
	games := s.GamesPerPairing
	if games == 0 {
		games = 2
	}
	if games != 1 && games != 2 {
		return bad("a pairing is 1 game or 2, with sides swapped")
	}
	lo, hi := s.MinPlayers, s.MaxPlayers
	if lo == 0 {
		lo = 2
	}
	if hi == 0 {
		hi = maxTournamentPlayers
	}
	if lo < 2 || hi > maxTournamentPlayers || lo > hi {
		return bad("players from 2 to %d, the fewest no more than the most", maxTournamentPlayers)
	}
	if s.Rounds < 0 || s.Rounds > maxTournamentRounds {
		return bad("0 to %d rounds", maxTournamentRounds)
	}
	if s.Format == tournament.RoundRobin && s.Rounds != 0 {
		return bad("a round robin takes as many rounds as it takes")
	}
	return store.Tournament{
		Name: name, Description: strings.TrimSpace(s.Description), GameDefID: s.GameDefID, GameName: compiled.Definition.Name, Format: s.Format,
		ClockSeconds: s.ClockSeconds, ClockIncrement: s.ClockIncrementSeconds, GamesPerPairing: games,
		Rated: s.Rated == nil || *s.Rated, PieRule: s.PieRule == nil || *s.PieRule,
		MinPlayers: lo, MaxPlayers: hi, Rounds: s.Rounds,
	}, nil
}

// signedIn is the account of the request.
func signedIn(ctx context.Context) (*store.Account, bool) {
	p, ok := auth.From(ctx)
	if !ok || p.Account == nil {
		return nil, false
	}
	return p.Account, true
}

// organises says the request's account may run t: its organiser, or an
// administrator signed in by hand rather than through a token.
func organises(ctx context.Context, t store.Tournament) bool {
	p, ok := auth.From(ctx)
	if !ok || p.Account == nil {
		return false
	}
	return p.Account.ID == t.OrganiserID || (p.Account.Admin && p.Scopes == nil)
}

// mayView hides a draft, or one abandoned before it opened, from everyone but
// who runs it.
func mayView(ctx context.Context, t store.Tournament) bool {
	if t.Status != store.TournamentDraft && (t.Status != store.TournamentAbandoned || t.StartedAt != "") {
		return true
	}
	return organises(ctx, t)
}

// SetMayEnterTournament gives the manager the capability check for entering, which
// holds however the request arrived.
func (m *Manager) SetMayEnterTournament(may func(context.Context) bool) { m.mayEnterTournament = may }

// CreateTournament stores a draft for the request's account, which the route has
// checked may make one.
func (m *Manager) CreateTournament(ctx context.Context, s TournamentSettings) (TournamentDetail, error) {
	acc, ok := signedIn(ctx)
	if !ok {
		return TournamentDetail{}, ErrTournamentAccount
	}
	t, err := m.settled(s)
	if err != nil {
		return TournamentDetail{}, err
	}
	t.OrganiserID = acc.ID
	id, err := m.store.CreateTournament(ctx, t)
	if err != nil {
		return TournamentDetail{}, err
	}
	return m.Tournament(ctx, id)
}

// organised reads a tournament for its organiser.
func (m *Manager) organised(ctx context.Context, id int64) (store.Tournament, error) {
	t, err := m.store.GetTournament(ctx, id)
	if err != nil {
		return t, err
	}
	if !organises(ctx, t) {
		if !mayView(ctx, t) {
			return t, store.ErrNotFound
		}
		return t, ErrNotOrganiser
	}
	return t, nil
}

// UpdateTournament changes a draft's settings.
func (m *Manager) UpdateTournament(ctx context.Context, id int64, s TournamentSettings) (TournamentDetail, error) {
	if _, err := m.organised(ctx, id); err != nil {
		return TournamentDetail{}, err
	}
	t, err := m.settled(s)
	if err != nil {
		return TournamentDetail{}, err
	}
	t.ID = id
	if err := m.store.UpdateTournament(ctx, t); err != nil {
		return TournamentDetail{}, err
	}
	return m.Tournament(ctx, id)
}

// OpenTournament opens a draft to entries.
func (m *Manager) OpenTournament(ctx context.Context, id int64) (TournamentDetail, error) {
	if _, err := m.organised(ctx, id); err != nil {
		return TournamentDetail{}, err
	}
	if err := m.store.OpenTournament(ctx, id); err != nil {
		return TournamentDetail{}, err
	}
	return m.Tournament(ctx, id)
}

// EnterTournament enters the request's account.
func (m *Manager) EnterTournament(ctx context.Context, id int64) (TournamentDetail, error) {
	acc, ok := signedIn(ctx)
	if !ok {
		return TournamentDetail{}, ErrTournamentAccount
	}
	if m.mayEnterTournament != nil && !m.mayEnterTournament(ctx) {
		return TournamentDetail{}, ErrMayNotEnter
	}
	t, err := m.store.GetTournament(ctx, id)
	if err != nil {
		return TournamentDetail{}, err
	}
	if !mayView(ctx, t) {
		return TournamentDetail{}, store.ErrNotFound
	}
	if err := m.store.AddTournamentEntry(ctx, id, acc.ID, t.MaxPlayers); err != nil {
		return TournamentDetail{}, err
	}
	return m.Tournament(ctx, id)
}

// LeaveTournament takes a player out: the request's account, or for its organiser
// the one with a handle. During registration that is the end of it. In a running
// tournament they forfeit their games in progress and any round-robin games still
// to come, are not paired again in Swiss, and what they have played stands.
func (m *Manager) LeaveTournament(ctx context.Context, id int64, handle string) (TournamentDetail, error) {
	acc, ok := signedIn(ctx)
	if !ok {
		return TournamentDetail{}, ErrTournamentAccount
	}
	t, err := m.store.GetTournament(ctx, id)
	if err != nil {
		return TournamentDetail{}, err
	}
	if !mayView(ctx, t) {
		return TournamentDetail{}, store.ErrNotFound
	}
	who := acc.ID
	if handle = strings.TrimPrefix(strings.TrimSpace(handle), "@"); handle != "" && !strings.EqualFold(handle, acc.Handle) {
		if !organises(ctx, t) {
			return TournamentDetail{}, ErrNotOrganiser
		}
		other, err := m.store.AccountByHandle(ctx, handle)
		if err != nil {
			return TournamentDetail{}, ErrNotEntered
		}
		who = other.ID
	}
	switch t.Status {
	case store.TournamentRegistration:
		if err := m.store.RemoveTournamentEntry(ctx, id, who); err != nil {
			return TournamentDetail{}, err
		}
	case store.TournamentRunning:
		m.tournamentsMu.Lock()
		defer m.tournamentsMu.Unlock()
		was, err := m.store.WithdrawTournamentEntry(ctx, id, who)
		if err != nil {
			return TournamentDetail{}, err
		}
		if !was {
			return TournamentDetail{}, ErrNotEntered
		}
		if err := m.forfeitGames(ctx, t, who); err != nil {
			return TournamentDetail{}, err
		}
		if t, err = m.store.GetTournament(ctx, id); err != nil {
			return TournamentDetail{}, err
		}
		m.advance(ctx, t)
	default:
		return TournamentDetail{}, store.ErrWrongTournamentState
	}
	return m.Tournament(ctx, id)
}

// forfeitGames ends a player's games in progress, in favour of their opponents.
func (m *Manager) forfeitGames(ctx context.Context, t store.Tournament, who int64) error {
	open, err := m.store.OpenTournamentGames(ctx, t.ID)
	if err != nil {
		return err
	}
	for _, line := range open {
		if line.RoomCode == "" || (line.PlayerA != who && line.PlayerB != who) {
			continue
		}
		if err := m.forfeitGame(ctx, line.RoomCode, who); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) forfeitGame(ctx context.Context, room string, who int64) error {
	unlock := m.lockRoom(ctx, room)
	defer unlock()
	game, err := m.loadGame(ctx, room, true)
	if err != nil {
		return err
	}
	if game.Status != "active" {
		return nil
	}
	compiled, err := m.compiledFor(game)
	if err != nil {
		return err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(players))
	for _, p := range players {
		ids = append(ids, p.PrincipalID)
	}
	accounts, err := m.store.AccountsOfPrincipals(ctx, ids)
	if err != nil {
		return err
	}
	var winner *int
	for _, p := range players {
		if accounts[p.PrincipalID] != who {
			seat := p.Seat
			winner = &seat
		}
	}
	if err := m.store.FinishGame(ctx, game.ID, winner, store.EndReasonForfeit); err != nil {
		return err
	}
	m.finished(game, gameFamily(compiled), store.EndReasonForfeit)
	m.publishState(ctx, room, "forfeit")
	return nil
}

// StartTournament closes the entries and plays the first round.
func (m *Manager) StartTournament(ctx context.Context, id int64) (TournamentDetail, error) {
	t, err := m.organised(ctx, id)
	if err != nil {
		return TournamentDetail{}, err
	}
	if t.Status != store.TournamentRegistration {
		return TournamentDetail{}, store.ErrWrongTournamentState
	}
	entries, err := m.store.TournamentEntries(ctx, id)
	if err != nil {
		return TournamentDetail{}, err
	}
	if len(entries) < t.MinPlayers {
		return TournamentDetail{}, fmt.Errorf("%w: %d of at least %d", ErrTooFewEntries, len(entries), t.MinPlayers)
	}
	compiled, ok := m.catalog.Get(t.GameDefID)
	if !ok {
		return TournamentDetail{}, fmt.Errorf("%w: %q", ErrGameDefNotFound, t.GameDefID)
	}
	seed, err := newSeed()
	if err != nil {
		return TournamentDetail{}, err
	}
	family, now := gameFamily(compiled), m.now()
	ids := make([]int64, len(entries))
	for i := range entries {
		ids[i] = entries[i].AccountID
		r, err := m.ratingNow(ctx, entries[i].AccountID, family, now)
		if err != nil {
			return TournamentDetail{}, err
		}
		entries[i].Rating = r.Rating
	}
	order := map[int64]int{}
	for i, id := range tournament.Shuffled(ids, seed) {
		order[id] = i + 1
	}
	for i := range entries {
		entries[i].SeedNo = order[entries[i].AccountID]
	}
	rounds := tournament.RoundRobinRounds(len(entries))
	if t.Format == tournament.Swiss {
		most := rounds
		rounds = tournament.DefaultSwissRounds(len(entries))
		if t.Rounds > 0 {
			rounds = t.Rounds
		}
		rounds = min(rounds, most)
	}
	m.tournamentsMu.Lock()
	defer m.tournamentsMu.Unlock()
	if err := m.store.StartTournament(ctx, id, seed, rounds, entries, now); err != nil {
		return TournamentDetail{}, err
	}
	if t, err = m.store.GetTournament(ctx, id); err != nil {
		return TournamentDetail{}, err
	}
	m.advance(ctx, t)
	return m.Tournament(ctx, id)
}

func newSeed() (int64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("session: a tournament's seed: %w", err)
	}
	return int64(binary.LittleEndian.Uint64(b[:]) >> 1), nil
}

// AbandonTournament stops a tournament: its games end with no winner and no
// rating change, and no games are left behind.
func (m *Manager) AbandonTournament(ctx context.Context, id int64) (TournamentDetail, error) {
	t, err := m.organised(ctx, id)
	if err != nil {
		return TournamentDetail{}, err
	}
	if t.Status == store.TournamentFinished || t.Status == store.TournamentAbandoned {
		return TournamentDetail{}, store.ErrWrongTournamentState
	}
	m.tournamentsMu.Lock()
	defer m.tournamentsMu.Unlock()
	if err := m.abandon(ctx, t, ""); err != nil {
		return TournamentDetail{}, err
	}
	return m.Tournament(ctx, id)
}

// abandon ends what is open and then the tournament. The sweep takes up the work
// again if this stops half way, on seeing a game ended this way.
func (m *Manager) abandon(ctx context.Context, t store.Tournament, detail string) error {
	open, err := m.store.OpenTournamentGames(ctx, t.ID)
	if err != nil {
		return err
	}
	for _, line := range open {
		if line.RoomCode != "" {
			if err := m.voidRoom(ctx, line.RoomCode); err != nil {
				return err
			}
		}
		if err := m.store.SetTournamentResult(ctx, line.ID, 0, 0, store.EndReasonAbandoned); err != nil {
			return err
		}
	}
	_, err = m.store.EndTournament(ctx, t.ID, store.TournamentAbandoned, detail, m.now())
	return err
}

func (m *Manager) voidRoom(ctx context.Context, room string) error {
	unlock := m.lockRoom(ctx, room)
	defer unlock()
	game, err := m.store.GetGame(ctx, room)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if game.Status == "finished" {
		return nil
	}
	if err := m.store.VoidGame(ctx, game.ID, store.EndReasonAbandoned); err != nil {
		return err
	}
	m.publishState(ctx, room, "abandoned")
	return nil
}

// RunTournaments keeps tournaments moving: a game whose flag has fallen with
// nobody looking ends, and a round whose games are all over gives way to the next.
// A game that ends says so at once; this is for the clock, and for a crash.
func (m *Manager) RunTournaments(ctx context.Context) {
	m.AdvanceTournaments(ctx)
	t := time.NewTicker(tournamentSweep)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.AdvanceTournaments(ctx)
		}
	}
}

// AdvanceTournaments looks at every running tournament once. It is exported so
// that a test can wait for it.
func (m *Manager) AdvanceTournaments(ctx context.Context) {
	m.tournamentsMu.Lock()
	defer m.tournamentsMu.Unlock()
	list, err := m.store.RunningTournaments(ctx)
	if err != nil {
		m.logger().Error("reading the running tournaments", "err", err)
		return
	}
	for _, t := range list {
		m.advance(ctx, t)
	}
}

// advance moves one tournament on as far as it can go. A failure is logged and
// tried again by the sweep. The caller holds tournamentsMu.
func (m *Manager) advance(ctx context.Context, t store.Tournament) {
	if err := m.advanceErr(ctx, t); err != nil {
		m.logger().Error("moving a tournament on", "tournament", t.ID, "err", err)
	}
}

func (m *Manager) advanceErr(ctx context.Context, t store.Tournament) error {
	if _, ok := m.catalog.Get(t.GameDefID); !ok {
		return m.abandon(ctx, t, "This game is no longer offered.")
	}
	// A round of forfeits and byes alone needs no playing, so this can go through
	// several rounds at once.
	for {
		open, err := m.store.OpenTournamentGames(ctx, t.ID)
		if err != nil {
			return err
		}
		waiting := false
		for _, line := range open {
			done, abandoned, err := m.readResult(ctx, line)
			if err != nil {
				return err
			}
			if abandoned {
				return m.abandon(ctx, t, "")
			}
			waiting = waiting || !done
		}
		if waiting {
			return nil
		}
		if t.CurrentRound >= t.TotalRounds {
			return m.finishTournament(ctx, t)
		}
		more, err := m.startRound(ctx, t)
		if err != nil {
			return err
		}
		if !more {
			return m.finishTournament(ctx, t)
		}
		if t, err = m.store.GetTournament(ctx, t.ID); err != nil {
			return err
		}
	}
}

func (m *Manager) finishTournament(ctx context.Context, t store.Tournament) error {
	_, err := m.store.EndTournament(ctx, t.ID, store.TournamentFinished, "", m.now())
	return err
}

// readResult copies a finished game's result to the tournament, and ends a game
// whose flag has fallen, by reading it. done says the line has a result; abandoned
// that the game was ended by abandoning the tournament.
func (m *Manager) readResult(ctx context.Context, line store.TournamentGame) (done, abandoned bool, err error) {
	game, err := m.loadGame(ctx, line.RoomCode, false)
	if errors.Is(err, store.ErrNotFound) {
		// Nothing should delete a game of a running tournament. It is not worth
		// stopping everyone for: nobody scores.
		return true, false, m.store.SetTournamentResult(ctx, line.ID, 0, 0, "lost")
	}
	if err != nil {
		return false, false, err
	}
	if game.Status != "finished" {
		return false, false, nil
	}
	if game.EndReason == store.EndReasonAbandoned {
		return true, true, nil
	}
	a, b := 0.5, 0.5
	switch {
	case game.WinnerSeat != nil:
		winner, err := m.accountAtSeat(ctx, game, *game.WinnerSeat)
		if err != nil {
			return false, false, err
		}
		a, b = 0, 1
		if winner == line.PlayerA {
			a, b = 1, 0
		}
	case game.EndReason == store.EndReasonRetired:
		a, b = 0, 0
	}
	return true, false, m.store.SetTournamentResult(ctx, line.ID, a, b, game.EndReason)
}

// accountAtSeat is the account now at a seat: a pie-rule swap changes who that is.
func (m *Manager) accountAtSeat(ctx context.Context, game store.Game, seat int) (int64, error) {
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return 0, err
	}
	for _, p := range players {
		if p.Seat == seat {
			accounts, err := m.store.AccountsOfPrincipals(ctx, []string{p.PrincipalID})
			if err != nil {
				return 0, err
			}
			return accounts[p.PrincipalID], nil
		}
	}
	return 0, fmt.Errorf("session: game %s has nobody at seat %d", game.RoomCode, seat)
}

// startRound makes the next round's games. It says false when there is no round
// to play: Swiss with fewer than two players left.
func (m *Manager) startRound(ctx context.Context, t store.Tournament) (bool, error) {
	entries, err := m.store.TournamentEntries(ctx, t.ID)
	if err != nil {
		return false, err
	}
	record, err := m.store.TournamentGames(ctx, t.ID)
	if err != nil {
		return false, err
	}
	round := t.CurrentRound + 1
	var pairs []tournament.Pair
	if t.Format == tournament.Swiss {
		pairs = swissPairs(entries, record, t.Seed, round)
		if len(pairs) == 0 || (len(pairs) == 1 && pairs[0].B == 0) {
			return false, nil
		}
	} else {
		ids := make([]int64, len(entries))
		for i, e := range entries {
			ids[i] = e.AccountID
		}
		pairs = tournament.RoundRobinRound(ids, round)
	}
	withdrawn := map[int64]bool{}
	for _, e := range entries {
		withdrawn[e.AccountID] = e.Withdrawn
	}
	var lastErr error
	for range roundAttempts {
		lines, err := m.roundLines(ctx, t, pairs, withdrawn)
		if err != nil {
			return false, err
		}
		if _, err = m.store.CreateTournamentRound(ctx, t.ID, round, lines); err == nil {
			m.announceRound(ctx, lines)
			return true, nil
		}
		if errors.Is(err, store.ErrWrongTournamentState) {
			return false, err
		}
		lastErr = err // most likely a room code already in use: try others
	}
	return false, lastErr
}

// swissPairs works out what a Swiss round needs from the record, and pairs it.
func swissPairs(entries []store.TournamentEntry, record []store.TournamentGame, seed int64, round int) []tournament.Pair {
	players := map[int64]*tournament.SwissPlayer{}
	for _, e := range entries {
		players[e.AccountID] = &tournament.SwissPlayer{ID: e.AccountID, Rating: e.Rating, Met: map[int64]bool{}}
	}
	for _, g := range record {
		a, b := players[g.PlayerA], players[g.PlayerB]
		if a != nil {
			a.Score += g.ScoreA
		}
		if g.PlayerB == 0 {
			if a != nil {
				a.HadBye = true
			}
			continue
		}
		if b != nil {
			b.Score += g.ScoreB
		}
		if a != nil && b != nil {
			a.Met[g.PlayerB], b.Met[g.PlayerA] = true, true
			a.FirstSeats++
			b.FirstSeats--
		}
	}
	var field []tournament.SwissPlayer
	for _, e := range entries {
		if !e.Withdrawn {
			field = append(field, *players[e.AccountID])
		}
	}
	return tournament.SwissRound(field, seed, round)
}

// roundLines turns a round's pairings into its games, with fresh room codes. A
// withdrawn player's game is not played: the other player has it, and when both
// have gone nobody does.
func (m *Manager) roundLines(ctx context.Context, t store.Tournament, pairs []tournament.Pair, withdrawn map[int64]bool) ([]store.NewTournamentGame, error) {
	compiled, err := m.tournamentGameDef(t)
	if err != nil {
		return nil, err
	}
	var lines []store.NewTournamentGame
	games := float64(t.GamesPerPairing)
	for i, p := range pairs {
		board := i + 1
		if p.B == 0 {
			if !withdrawn[p.A] {
				lines = append(lines, store.NewTournamentGame{Board: board, GameNo: 1, PlayerA: p.A, ScoreA: games, Reason: store.ResultBye})
			}
			continue
		}
		for no := 1; no <= t.GamesPerPairing; no++ {
			a, b := p.A, p.B
			if no == 2 {
				a, b = b, a
			}
			line := store.NewTournamentGame{Board: board, GameNo: no, PlayerA: a, PlayerB: b, Reason: store.EndReasonForfeit}
			switch {
			case withdrawn[a] && withdrawn[b]:
			case withdrawn[a]:
				line.ScoreB = 1
			case withdrawn[b]:
				line.ScoreA = 1
			default:
				if err := m.fillGame(ctx, t, compiled, &line); err != nil {
					return nil, err
				}
			}
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// tournamentGameDef is the game a tournament plays: with the pie rule as it
// chose, where the game offers the choice.
func (m *Manager) tournamentGameDef(t store.Tournament) (*catalog.CompiledGame, error) {
	base, ok := m.catalog.Get(t.GameDefID)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrGameDefNotFound, t.GameDefID)
	}
	for _, o := range base.Definition.Options {
		if o.ID != catalog.PieOption {
			continue
		}
		id, err := m.catalog.Derive(t.GameDefID, map[string]any{catalog.PieOption: t.PieRule})
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidOptions, err)
		}
		derived, ok := m.catalog.Get(id)
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrGameDefNotFound, id)
		}
		return derived, nil
	}
	return base, nil
}

// fillGame fills a line in with the game to make for it: the first seat is the
// line's PlayerA.
func (m *Manager) fillGame(ctx context.Context, t store.Tournament, compiled *catalog.CompiledGame, line *store.NewTournamentGame) error {
	line.Reason = ""
	a, err := m.store.AccountByID(ctx, line.PlayerA)
	if err != nil {
		return err
	}
	b, err := m.store.AccountByID(ctx, line.PlayerB)
	if err != nil {
		return err
	}
	opts := CreateOptions{GameDefID: compiled.Definition.ID}
	start, first, _, err := resolveStart(compiled, opts)
	if err != nil {
		return err
	}
	board, err := start.MarshalJSON()
	if err != nil {
		return fmt.Errorf("session: marshal a tournament game's board: %w", err)
	}
	s1, s2 := compiled.Definition.Seats[0], compiled.Definition.Seats[1]
	clock, err := newGameClock([]int{s1.Seat, s2.Seat}, []int{t.ClockSeconds, t.ClockSeconds}, t.ClockIncrement)
	if err != nil {
		return err
	}
	stored, err := clock.marshal()
	if err != nil {
		return err
	}
	line.RoomCode, line.GameDefID, line.BoardState, line.Clock, line.FirstSeat, line.Rated = roomcode.New(), compiled.Definition.ID, string(board), stored, first, t.Rated
	line.Seats = [2]store.NewSeat{
		{Seat: s1.Seat, Name: a.Handle, Color: s1.Color, PrincipalID: a.PrincipalID},
		{Seat: s2.Seat, Name: b.Handle, Color: s2.Color, PrincipalID: b.PrincipalID},
	}
	return nil
}

// announceRound sets the turn mail of a new round's players and tells the rooms.
func (m *Manager) announceRound(ctx context.Context, lines []store.NewTournamentGame) {
	for _, line := range lines {
		if line.RoomCode == "" {
			continue
		}
		game, err := m.store.GetGame(ctx, line.RoomCode)
		if err != nil {
			continue
		}
		m.tournamentMail(ctx, game.ID, line.PlayerA, line.Seats[0].Name)
		m.tournamentMail(ctx, game.ID, line.PlayerB, line.Seats[1].Name)
		m.publishState(ctx, line.RoomCode, "tournament")
	}
}

// tournamentMail asks for turn mail for a player's tournament game, at their
// account's confirmed address, because a correspondence game is unplayable without
// it. The player can turn it off in the game, and the mail's stop link stops all
// of it. A failure costs a mail and no more.
func (m *Manager) tournamentMail(ctx context.Context, gameID, accountID int64, name string) {
	if m.notify == nil {
		return
	}
	acc, err := m.store.AccountByID(ctx, accountID)
	if err != nil || !acc.EmailVerified || acc.Email == "" {
		return
	}
	contact, err := m.store.AccountContact(ctx, acc.ID, acc.Email, addrHash(acc.Email), hashOf(newToken()), newToken(), m.now())
	if err == nil {
		err = m.store.SetNotifyPref(ctx, store.NotifyPref{GameID: gameID, NameKey: nameKey(name), Enabled: true, ContactID: contact})
	}
	if err != nil {
		m.logger().Warn("setting the turn mail of a tournament game", "game", gameID, "err", err)
	}
}

// ---- what is shown -------------------------------------------------------------

// TournamentSummary is a line in the list of tournaments.
type TournamentSummary struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	GameDefID       string `json:"game_def_id"`
	GameName        string `json:"game_name"`
	Format          string `json:"format"`
	Status          string `json:"status"`
	ClockSeconds    int    `json:"clock_seconds"`
	ClockIncrement  int    `json:"clock_increment_seconds,omitempty"`
	GamesPerPairing int    `json:"games_per_pairing"`
	Rated           bool   `json:"rated"`
	PieRule         bool   `json:"pie_rule"`
	MinPlayers      int    `json:"min_players"`
	MaxPlayers      int    `json:"max_players"`
	Rounds          int    `json:"rounds,omitempty"`
	Players         int    `json:"players"`
	Organiser       string `json:"organiser"`
	CurrentRound    int    `json:"current_round"`
	TotalRounds     int    `json:"total_rounds,omitempty"`
	CreatedAt       string `json:"created_at"`
	StartedAt       string `json:"started_at,omitempty"`
	EndedAt         string `json:"ended_at,omitempty"`
}

func summaryOf(t store.Tournament) TournamentSummary {
	return TournamentSummary{
		ID: t.ID, Name: t.Name, GameDefID: t.GameDefID, GameName: t.GameName, Format: t.Format, Status: t.Status,
		ClockSeconds: t.ClockSeconds, ClockIncrement: t.ClockIncrement, GamesPerPairing: t.GamesPerPairing, Rated: t.Rated, PieRule: t.PieRule,
		MinPlayers: t.MinPlayers, MaxPlayers: t.MaxPlayers, Rounds: t.Rounds, Players: t.Entries, Organiser: t.Organiser,
		CurrentRound: t.CurrentRound, TotalRounds: t.TotalRounds, CreatedAt: t.CreatedAt, StartedAt: t.StartedAt, EndedAt: t.EndedAt,
	}
}

// TournamentEntryInfo is a player's line in the standings: while the tournament
// has not started, everyone is simply listed.
type TournamentEntryInfo struct {
	Handle    string  `json:"handle"`
	Withdrawn bool    `json:"withdrawn,omitempty"`
	Place     int     `json:"place,omitempty"`
	Score     float64 `json:"score"`
	Played    int     `json:"played"`
	Won       int     `json:"won"`
	Drawn     int     `json:"drawn"`
	Lost      int     `json:"lost"`
	// Buchholz is for Swiss; SonnebornBerger for both formats.
	Buchholz        float64 `json:"buchholz,omitempty"`
	SonnebornBerger float64 `json:"sonneborn_berger"`
}

// TournamentGameInfo is one game of a pairing. A bye has no second player.
type TournamentGameInfo struct {
	First    string      `json:"first"`
	Second   string      `json:"second,omitempty"`
	RoomCode string      `json:"room_code,omitempty"`
	Done     bool        `json:"done"`
	Scores   *[2]float64 `json:"scores,omitempty"`
	Reason   string      `json:"reason,omitempty"`
}

// TournamentRoundInfo is a round's games, a pairing's next to each other.
type TournamentRoundInfo struct {
	Round int                  `json:"round"`
	Games []TournamentGameInfo `json:"games"`
}

// TournamentYou is where the request's account stands with the tournament.
type TournamentYou struct {
	Entered   bool `json:"entered"`
	Withdrawn bool `json:"withdrawn,omitempty"`
	Organiser bool `json:"organiser"`
}

// TournamentDetail is the whole of a tournament.
type TournamentDetail struct {
	TournamentSummary
	Description string                `json:"description,omitempty"`
	EndDetail   string                `json:"end_detail,omitempty"`
	Standings   []TournamentEntryInfo `json:"standings"`
	Games       []TournamentRoundInfo `json:"schedule"`
	You         TournamentYou         `json:"you"`
}

// ListTournaments lists what the request's account may see.
func (m *Manager) ListTournaments(ctx context.Context) ([]TournamentSummary, error) {
	var me int64
	admin := false
	if p, ok := auth.From(ctx); ok && p.Account != nil {
		me, admin = p.Account.ID, p.Account.Admin && p.Scopes == nil
	}
	list, err := m.store.ListTournaments(ctx, me, admin, maxTournamentList)
	if err != nil {
		return nil, err
	}
	out := make([]TournamentSummary, 0, len(list))
	for _, t := range list {
		out = append(out, summaryOf(t))
	}
	return out, nil
}

// Tournament reads a tournament, its standings and its schedule. The standings
// are worked out from its own record, so they need no game row.
func (m *Manager) Tournament(ctx context.Context, id int64) (TournamentDetail, error) {
	t, err := m.store.GetTournament(ctx, id)
	if err != nil {
		return TournamentDetail{}, err
	}
	if !mayView(ctx, t) {
		return TournamentDetail{}, store.ErrNotFound
	}
	entries, err := m.store.TournamentEntries(ctx, id)
	if err != nil {
		return TournamentDetail{}, err
	}
	record, err := m.store.TournamentGames(ctx, id)
	if err != nil {
		return TournamentDetail{}, err
	}
	out := TournamentDetail{TournamentSummary: summaryOf(t), Description: t.Description, EndDetail: t.EndDetail, Standings: []TournamentEntryInfo{}, Games: []TournamentRoundInfo{}}
	out.You.Organiser = organises(ctx, t)

	handles := make(map[int64]string, len(entries))
	withdrawn := make(map[int64]bool, len(entries))
	ids := make([]int64, 0, len(entries))
	for _, e := range entries {
		handles[e.AccountID], withdrawn[e.AccountID] = e.Handle, e.Withdrawn
		ids = append(ids, e.AccountID)
	}
	if acc, ok := signedIn(ctx); ok {
		if _, in := handles[acc.ID]; in {
			out.You.Entered, out.You.Withdrawn = true, withdrawn[acc.ID]
		}
	}
	var results []tournament.Result
	for _, g := range record {
		if g.Done {
			results = append(results, tournament.Result{Round: g.Round, A: g.PlayerA, B: g.PlayerB, ScoreA: g.ScoreA, ScoreB: g.ScoreB})
		}
		if len(out.Games) == 0 || out.Games[len(out.Games)-1].Round != g.Round {
			out.Games = append(out.Games, TournamentRoundInfo{Round: g.Round})
		}
		info := TournamentGameInfo{First: handles[g.PlayerA], Second: handles[g.PlayerB], RoomCode: g.RoomCode, Done: g.Done, Reason: g.Reason}
		if g.Done {
			info.Scores = &[2]float64{g.ScoreA, g.ScoreB}
		}
		round := &out.Games[len(out.Games)-1]
		round.Games = append(round.Games, info)
	}
	started := t.StartedAt != ""
	for _, s := range tournament.Standings(t.Format, ids, results) {
		line := TournamentEntryInfo{
			Handle: handles[s.ID], Withdrawn: withdrawn[s.ID], Score: s.Score, Played: s.Games, Won: s.Won, Drawn: s.Drawn, Lost: s.Lost,
			SonnebornBerger: s.SonnebornBerger,
		}
		if started {
			line.Place = s.Place
		}
		if t.Format == tournament.Swiss {
			line.Buchholz = s.Buchholz
		}
		out.Standings = append(out.Standings, line)
	}
	return out, nil
}

// TournamentRef is what a game says of the tournament it is part of.
type TournamentRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// tournamentRef reads it, for a game that is part of one.
func (m *Manager) tournamentRef(ctx context.Context, game store.Game) *TournamentRef {
	if game.TournamentID == 0 {
		return nil
	}
	t, err := m.store.GetTournament(ctx, game.TournamentID)
	if err != nil {
		return nil
	}
	return &TournamentRef{ID: t.ID, Name: t.Name}
}
