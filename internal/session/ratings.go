package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/rating"
	"github.com/noderunner/bingle/internal/store"
)

// A rated game moves its players' ratings when it ends (the tournaments plan,
// ADR 0080). It is chosen when the game is made, is played between two accounts
// on equal clocks from the standard start, and allows nothing that could change
// its result afterwards.
//
// A game that ends marks itself pending in the transaction that ends it, and the
// result is written after that, outside the room lock, so a move never waits for
// a rating and a crash can only delay one.

// ErrCannotBeRated is the answer to a rated game that breaks the rules for one.
var ErrCannotBeRated = errors.New("session: this game cannot be rated")

// ErrRatedGame is the answer to something a rated game does not allow.
var ErrRatedGame = errors.New("session: not in a rated or tournament game")

// resultsRetry is how often pending results are looked at again, in case writing
// one failed.
const resultsRetry = time.Minute

// checkRatable refuses a rated game that breaks the rules for one.
func checkRatable(ctx context.Context, compiled *catalog.CompiledGame, opts CreateOptions) error {
	refuse := func(why string) error { return fmt.Errorf("%w: %s", ErrCannotBeRated, why) }
	if compiled.Definition.Prototype {
		return refuse("a prototype's rules are still changing, so its games are not rated")
	}
	if p, ok := auth.From(ctx); !ok || p.Account == nil {
		return refuse("a rated game needs an account: sign in")
	}
	if len(compiled.Definition.Seats) != 2 {
		return refuse("only a game of two seats can be rated")
	}
	if opts.Opponent != "" || len(opts.SeatBots) > 0 {
		return refuse("bots do not play rated games")
	}
	if len(opts.Also) > 0 {
		return refuse("a rated game needs two different players")
	}
	if len(opts.ClockSeconds) != 2 || opts.MoveTimeSeconds > 0 {
		return refuse("a rated game needs a game clock")
	}
	if opts.ClockSeconds[0] != opts.ClockSeconds[1] {
		return refuse("a rated game gives both players the same time")
	}
	if opts.StartTPS != "" || opts.StartState != "" || opts.Tutorial != "" {
		return refuse("a rated game starts from the standard position")
	}
	return nil
}

// mayJoinRated refuses a new seat in a rated game to anybody but a second
// account.
func (m *Manager) mayJoinRated(ctx context.Context, players []store.Player) error {
	p, ok := auth.From(ctx)
	if !ok || p.Account == nil {
		return fmt.Errorf("%w: a rated game needs an account: sign in", ErrCannotBeRated)
	}
	id := auth.IDFrom(ctx)
	for _, other := range players {
		if other.PrincipalID == id {
			return fmt.Errorf("%w: a rated game needs two different players", ErrCannotBeRated)
		}
	}
	return nil
}

// finished is said of every game that ends without being retired: it is counted,
// and a rated game's result is written, away from the room lock its caller holds.
func (m *Manager) finished(game store.Game, family, reason string) {
	m.mx.Finished(family, reason)
	if game.Rated {
		// The caller's context ends with its request, and the result must not.
		go m.RecordResults(context.Background())
	}
	if game.TournamentID != 0 {
		go m.AdvanceTournaments(context.Background())
	}
}

// RunResults writes the results left pending by a crash or a failed write, and
// looks again now and then, until ctx ends.
func (m *Manager) RunResults(ctx context.Context) {
	m.RecordResults(ctx)
	t := time.NewTicker(resultsRetry)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.RecordResults(ctx)
		}
	}
}

// RecordResults writes the result of every rated game that has ended and has not
// had one, one at a time. It is exported so that a test can wait for it.
func (m *Manager) RecordResults(ctx context.Context) {
	m.resultsMu.Lock()
	defer m.resultsMu.Unlock()
	games, err := m.store.PendingRatedGames(ctx)
	if err != nil {
		m.resultFailed(err)
		return
	}
	for _, game := range games {
		if err := m.recordResult(ctx, game); err != nil {
			m.resultFailed(err)
			continue
		}
		m.publishState(ctx, game.RoomCode, "rated")
	}
}

func (m *Manager) resultFailed(err error) {
	m.mx.RatedResult(metrics.RatedFailed)
	m.logger().Error("writing the result of a rated game", "err", err)
}

// recordResult works out what a rated game does to its players and writes it.
func (m *Manager) recordResult(ctx context.Context, game store.Game) error {
	void := func(status string, outcome metrics.RatedOutcome) error {
		m.mx.RatedResult(outcome)
		return m.store.SetRatingStatus(ctx, game.ID, status)
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	// A prototype has no ratings; this is a game rated before it became one.
	if !ok || compiled.Definition.Prototype {
		return void(store.RatingVoid, metrics.RatedVoid)
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return err
	}
	if len(players) != 2 || players[0].Bot != "" || players[1].Bot != "" {
		return void(store.RatingVoid, metrics.RatedVoid)
	}
	accounts, err := m.store.AccountsOfPrincipals(ctx, []string{players[0].PrincipalID, players[1].PrincipalID})
	if err != nil {
		return err
	}
	a, b := accounts[players[0].PrincipalID], accounts[players[1].PrincipalID]
	if a == 0 || b == 0 || a == b {
		return void(store.RatingVoid, metrics.RatedVoid)
	}
	// A game somebody never played a move in is not a game: a no-show, or a
	// quick way to hand somebody a win.
	moved, err := m.store.SeatsThatMoved(ctx, game.ID)
	if err != nil {
		return err
	}
	if moved < 2 {
		return void(store.RatingNoMoves, metrics.RatedNoMoves)
	}
	family := gameFamily(compiled)
	now := m.now()
	between, err := m.store.RatedGamesBetween(ctx, a, b, family, now.Add(-24*time.Hour))
	if err != nil {
		return err
	}
	if between >= rating.MaxGamesPerDay {
		return void(store.RatingDailyLimit, metrics.RatedDailyLimit)
	}

	playerA, err := m.ratingNow(ctx, a, family, now)
	if err != nil {
		return err
	}
	playerB, err := m.ratingNow(ctx, b, family, now)
	if err != nil {
		return err
	}
	scoreA, scoreB := 0.5, 0.5
	if game.WinnerSeat != nil {
		scoreA, scoreB = 0, 1
		if *game.WinnerSeat == players[0].Seat {
			scoreA, scoreB = 1, 0
		}
	}
	newA := rating.Update(playerA, []rating.Result{{Opponent: playerB, Score: scoreA}})
	newB := rating.Update(playerB, []rating.Result{{Opponent: playerA, Score: scoreB}})
	results := [2]store.RatedResult{
		{AccountID: a, OpponentID: b, Seat: players[0].Seat, Score: scoreA, Before: playerA.Rating, Rating: newA.Rating, Deviation: newA.Deviation, Volatility: newA.Volatility},
		{AccountID: b, OpponentID: a, Seat: players[1].Seat, Score: scoreB, Before: playerB.Rating, Rating: newB.Rating, Deviation: newB.Deviation, Volatility: newB.Volatility},
	}
	if err := m.store.RecordRatedGame(ctx, game, family, m.familyName(compiled), results, now); err != nil {
		return err
	}
	m.mx.RatedResult(metrics.RatedRecorded)
	return nil
}

// ratingNow is an account's rating in a family as it stands today: a new
// player's if it has none, and less certain for the time since its last game.
func (m *Manager) ratingNow(ctx context.Context, accountID int64, family string, now time.Time) (rating.Player, error) {
	r, ok, err := m.store.RatingOf(ctx, accountID, family)
	if err != nil {
		return rating.Player{}, err
	}
	if !ok {
		return rating.New(), nil
	}
	return aged(r, now), nil
}

func aged(r store.Rating, now time.Time) rating.Player {
	return rating.Player{Rating: r.Rating, Deviation: r.Deviation, Volatility: r.Volatility}.Aged(now.Sub(r.LastGameAt))
}

// familyName is the name of a game with no layout in it: Hex, not Hex (11×11).
func (m *Manager) familyName(compiled *catalog.CompiledGame) string {
	if base, ok := m.catalog.Get(gameFamily(compiled)); ok {
		return store.FamilyNameOf(base.Definition.Name)
	}
	return store.FamilyNameOf(compiled.Definition.Name)
}

// SeatRating is a player's rating, shown at the seat of a rated game.
type SeatRating struct {
	Rating      int  `json:"rating"`
	Provisional bool `json:"provisional,omitempty"`
}

// RatingChange is what a rated game did to the player at a seat.
type RatingChange struct {
	Seat   int `json:"seat"`
	Before int `json:"before"`
	After  int `json:"after"`
}

// fillRatings adds what a rated game shows: each seat's rating, and what the game
// did to them once it has been recorded. Failing to read them costs the page a
// number and no more.
func (m *Manager) fillRatings(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, seats []SeatInfo) []RatingChange {
	if !game.Rated {
		return nil
	}
	ids := make([]string, 0, len(seats))
	for _, s := range seats {
		ids = append(ids, s.PrincipalID)
	}
	accounts, err := m.store.AccountsOfPrincipals(ctx, ids)
	if err != nil {
		return nil
	}
	family := gameFamily(compiled)
	now := m.now()
	for i := range seats {
		account := accounts[seats[i].PrincipalID]
		if account == 0 {
			continue
		}
		p, err := m.ratingNow(ctx, account, family, now)
		if err != nil {
			continue
		}
		seats[i].Rating = &SeatRating{Rating: rounded(p.Rating), Provisional: p.Provisional()}
	}
	if game.RatingStatus != store.RatingRecorded {
		return nil
	}
	changes, err := m.store.RatingChanges(ctx, game.RoomCode)
	if err != nil {
		return nil
	}
	out := make([]RatingChange, 0, len(changes))
	for _, c := range changes {
		out = append(out, RatingChange{Seat: c.Seat, Before: rounded(c.Before), After: rounded(c.After)})
	}
	return out
}

func rounded(v float64) int { return int(v + 0.5) }

// isPrototype reports whether a family is a prototype's. Ratings earned before a
// game became one stay in the database, and are shown nowhere.
func (m *Manager) isPrototype(family string) bool {
	cg, ok := m.catalog.Get(family)
	return ok && cg.Definition.Prototype
}

// RatedFamily is a game with a leaderboard.
type RatedFamily struct {
	Family  string `json:"family"`
	Name    string `json:"name"`
	Players int    `json:"players"`
}

// RatedFamilies lists the games whose leaderboards have anybody on them.
func (m *Manager) RatedFamilies(ctx context.Context) ([]RatedFamily, error) {
	list, err := m.store.RatedFamilies(ctx, rating.MinGames, rating.MinOpponents)
	if err != nil {
		return nil, err
	}
	out := make([]RatedFamily, 0, len(list))
	for _, f := range list {
		if m.isPrototype(f.Family) {
			continue
		}
		out = append(out, RatedFamily{Family: f.Family, Name: f.Name, Players: f.Players})
	}
	return out, nil
}

// LeaderboardEntry is a line of a leaderboard: who, their rating and how many
// games, and never when or against whom.
type LeaderboardEntry struct {
	Rank        int    `json:"rank"`
	Handle      string `json:"handle"`
	Rating      int    `json:"rating"`
	Provisional bool   `json:"provisional,omitempty"`
	Games       int    `json:"games"`
}

// maxLeaderboard bounds a leaderboard.
const maxLeaderboard = 100

// Leaderboard is a game family's leaderboard, the highest rating first.
func (m *Manager) Leaderboard(ctx context.Context, family string, limit int) ([]LeaderboardEntry, error) {
	if limit <= 0 || limit > maxLeaderboard {
		limit = maxLeaderboard
	}
	if m.isPrototype(family) {
		return []LeaderboardEntry{}, nil
	}
	rows, err := m.store.Leaderboard(ctx, family, rating.MinGames, rating.MinOpponents, limit)
	if err != nil {
		return nil, err
	}
	now := m.now()
	out := make([]LeaderboardEntry, 0, len(rows))
	for i, r := range rows {
		out = append(out, LeaderboardEntry{Rank: i + 1, Handle: r.Handle, Rating: rounded(r.Rating), Provisional: aged(r, now).Provisional(), Games: r.Games})
	}
	return out, nil
}

// PlayerRating is one of an account's ratings, on its profile.
type PlayerRating struct {
	Family      string `json:"family"`
	Name        string `json:"name"`
	Rating      int    `json:"rating"`
	Provisional bool   `json:"provisional,omitempty"`
	Games       int    `json:"games"`
	Won         int    `json:"won"`
	Drawn       int    `json:"drawn"`
	Lost        int    `json:"lost"`
	// Listed says the rating is public: others see only those that are.
	Listed bool `json:"listed"`
}

// PlayerRatings lists the ratings of the account with a handle: every one for the
// account itself, and to anybody else only those good enough for a leaderboard.
func (m *Manager) PlayerRatings(ctx context.Context, handle string) ([]PlayerRating, error) {
	acc, err := m.store.AccountByHandle(ctx, handle)
	if err != nil {
		return nil, err
	}
	p, ok := auth.From(ctx)
	mine := ok && p.Account != nil && p.Account.ID == acc.ID
	rows, err := m.store.AccountRatings(ctx, acc.ID)
	if err != nil {
		return nil, err
	}
	stats, err := m.store.Stats(ctx, acc.ID)
	if err != nil {
		return nil, err
	}
	now := m.now()
	out := make([]PlayerRating, 0, len(rows))
	for _, r := range rows {
		listed := rating.Listed(r.Games, r.Opponents)
		if (!listed && !mine) || m.isPrototype(r.Family) {
			continue
		}
		out = append(out, PlayerRating{
			Family: r.Family, Name: r.FamilyName, Rating: rounded(r.Rating), Provisional: aged(r, now).Provisional(), Games: r.Games,
			Won: stats[store.StatKey("won", r.Family)], Drawn: stats[store.StatKey("drawn", r.Family)], Lost: stats[store.StatKey("lost", r.Family)],
			Listed: listed,
		})
	}
	return out, nil
}
