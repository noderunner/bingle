package session

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/rating"
	"github.com/noderunner/bingle/internal/store"
)

// A moderator may take a rated game out of the ratings, for both its players
// (ADR 0157). The family's ratings are replayed without it when its history
// can be trusted to reproduce them; otherwise only the game's own change is
// reversed. The removal is in the audit log, both players are mailed, and a
// tournament's standings say who removed what.

// ErrNotModerator is the answer to a removal, or a look at somebody's rated
// games, by anybody without the moderate capability.
var ErrNotModerator = errors.New("session: that is for moderators")

// ErrNotRatedGame is the answer to removing a game that moved no rating.
var ErrNotRatedGame = errors.New("session: no rated result was written for that game")

// ErrAlreadyRemoved is the answer to removing a game twice.
var ErrAlreadyRemoved = errors.New("session: that game is out of the ratings already")

// maxRatedGames bounds the list of an account's rated games.
const maxRatedGames = 100

// SetMayModerate gives the manager the check for the moderate capability.
// Without one, only the admin group, signed in by hand, may moderate.
func (m *Manager) SetMayModerate(may func(context.Context) bool) { m.mayModerate = may }

// moderator is the request's account if it may moderate.
func (m *Manager) moderator(ctx context.Context) (*store.Account, error) {
	p, ok := auth.From(ctx)
	if !ok || p.Account == nil {
		return nil, ErrNotModerator
	}
	if m.mayModerate != nil {
		if !m.mayModerate(ctx) {
			return nil, ErrNotModerator
		}
	} else if !p.Account.Admin || p.Scopes != nil {
		return nil, ErrNotModerator
	}
	return p.Account, nil
}

// RatingRemovalInfo says who took a game out of the ratings, and how.
type RatingRemovalInfo struct {
	By string `json:"by"`
	// At is when (RFC 3339).
	At string `json:"at"`
	// Recomputed: the family's ratings were replayed without the game. When
	// false, only its own change was reversed.
	Recomputed bool `json:"recomputed"`
}

// RatedGameInfo is one of an account's rated results, for a moderator.
type RatedGameInfo struct {
	RoomCode string `json:"room_code"`
	Family   string `json:"family"`
	Game     string `json:"game"`
	// PlayedAt is when the result was written (RFC 3339).
	PlayedAt string `json:"played_at"`
	Opponent string `json:"opponent"`
	// Score is 1, 0.5 or 0.
	Score      float64            `json:"score"`
	Before     int                `json:"before"`
	After      int                `json:"after"`
	Tournament int64              `json:"tournament,omitempty"`
	Removed    *RatingRemovalInfo `json:"removed,omitempty"`
}

func removalOf(s store.RatedSide) *RatingRemovalInfo {
	if !s.Removed {
		return nil
	}
	at := s.RemovedAt
	if t, err := time.Parse(time.RFC3339Nano, at); err == nil {
		at = t.UTC().Format(time.RFC3339)
	}
	return &RatingRemovalInfo{By: s.RemovedBy, At: at, Recomputed: s.Recomputed}
}

// RatedGamesOf lists the rated results of the account with a handle, the newest
// first, for a moderator choosing one to remove.
func (m *Manager) RatedGamesOf(ctx context.Context, handle string) ([]RatedGameInfo, error) {
	if _, err := m.moderator(ctx); err != nil {
		return nil, err
	}
	acc, err := m.store.AccountByHandle(ctx, strings.TrimPrefix(strings.TrimSpace(handle), "@"))
	if err != nil {
		return nil, err
	}
	rows, err := m.store.AccountRatedGames(ctx, acc.ID, maxRatedGames)
	if err != nil {
		return nil, err
	}
	out := make([]RatedGameInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, RatedGameInfo{
			RoomCode: r.RoomCode, Family: r.Family, Game: r.FamilyName, PlayedAt: r.PlayedAt.UTC().Format(time.RFC3339), Opponent: r.OpponentHandle,
			Score: r.Score, Before: rounded(r.Before), After: rounded(r.After), Tournament: r.TournamentID, Removed: removalOf(r),
		})
	}
	return out, nil
}

// RatingMove is what a removal did to one player's rating.
type RatingMove struct {
	Handle string `json:"handle"`
	Before int    `json:"before"`
	// After is the rating now; absent when the player has no rated game left
	// in the family, and so no rating in it.
	After *int `json:"after,omitempty"`
}

// RatingRemovalResult is what a removal did.
type RatingRemovalResult struct {
	RoomCode   string       `json:"room_code"`
	Game       string       `json:"game"`
	Recomputed bool         `json:"recomputed"`
	Players    []RatingMove `json:"players"`
	// Others is how many more players' ratings the replay moved.
	Others int `json:"others"`
}

// RemoveRatedGame takes the rated game played in room out of the ratings, for
// both its players. ipHash is the moderator's address as the audit log keeps
// it.
func (m *Manager) RemoveRatedGame(ctx context.Context, room, ipHash string) (RatingRemovalResult, error) {
	mod, err := m.moderator(ctx)
	if err != nil {
		return RatingRemovalResult{}, err
	}
	// No result is written while the ratings are rewritten.
	m.resultsMu.Lock()
	defer m.resultsMu.Unlock()
	game, err := m.store.RatedGame(ctx, strings.TrimSpace(room))
	if errors.Is(err, store.ErrNotFound) {
		return RatingRemovalResult{}, ErrNotRatedGame
	}
	if err != nil {
		return RatingRemovalResult{}, err
	}
	if game[0].Removed {
		return RatingRemovalResult{}, ErrAlreadyRemoved
	}
	family := game[0].Family
	history, err := m.store.FamilyHistory(ctx, family)
	if err != nil {
		return RatingRemovalResult{}, err
	}
	stored, err := m.store.FamilyRatings(ctx, family)
	if err != nil {
		return RatingRemovalResult{}, err
	}
	rm := store.RatingRemoval{RoomCode: game[0].RoomCode, Family: family, By: mod.ID, At: m.now()}
	if sameRatings(replayRatings(history, ""), stored) {
		rm.Recomputed = true
		rm.Ratings, rm.Dropped = replacements(replayRatings(history, rm.RoomCode), stored)
	} else {
		for _, side := range game {
			r, ok := ratingIn(stored, side.AccountID)
			if !ok {
				continue
			}
			if r.Games <= 1 {
				rm.Dropped = append(rm.Dropped, r.AccountID)
				continue
			}
			opponents, err := m.store.OpponentsIn(ctx, side.AccountID, family, rm.RoomCode)
			if err != nil {
				return RatingRemovalResult{}, err
			}
			r.Rating -= side.After - side.Before
			r.Games--
			r.Opponents = opponents
			rm.Ratings = append(rm.Ratings, r)
		}
	}
	for _, side := range game {
		for _, what := range []string{"played", outcomeOfScore(side.Score)} {
			rm.Counters = append(rm.Counters, store.Counter{AccountID: side.AccountID, Key: store.StatKey(what, family)})
		}
	}
	if err := m.store.RemoveRatedGame(ctx, rm); err != nil {
		if errors.Is(err, store.ErrAlreadyRemoved) {
			return RatingRemovalResult{}, ErrAlreadyRemoved
		}
		return RatingRemovalResult{}, err
	}

	out := RatingRemovalResult{RoomCode: rm.RoomCode, Game: game[0].FamilyName, Recomputed: rm.Recomputed}
	players := map[int64]bool{}
	for _, side := range game {
		players[side.AccountID] = true
		move := RatingMove{Handle: side.Handle}
		if r, ok := ratingIn(stored, side.AccountID); ok {
			move.Before = rounded(r.Rating)
		}
		if r, ok := ratingIn(rm.Ratings, side.AccountID); ok {
			after := rounded(r.Rating)
			move.After = &after
		}
		out.Players = append(out.Players, move)
	}
	for _, r := range rm.Ratings {
		if !players[r.AccountID] {
			out.Others++
		}
	}
	for _, id := range rm.Dropped {
		if !players[id] {
			out.Others++
		}
	}
	m.auditRemoval(ctx, mod, out, ipHash)
	for i, side := range game {
		m.mailRemoval(ctx, mod.Handle, side, game[1-i].Handle)
	}
	m.publishState(ctx, rm.RoomCode, "rated")
	return out, nil
}

func outcomeOfScore(score float64) string {
	switch score {
	case 1:
		return "won"
	case 0:
		return "lost"
	}
	return "drawn"
}

func ratingIn(list []store.Rating, account int64) (store.Rating, bool) {
	for _, r := range list {
		if r.AccountID == account {
			return r, true
		}
	}
	return store.Rating{}, false
}

// auditRemoval writes the removal into the admin audit log, under the
// moderator's account: "Yavalath, room ABC123: @ana 1662 → 1500, @bea 1338 →
// 1500; recomputed, 2 others moved".
func (m *Manager) auditRemoval(ctx context.Context, mod *store.Account, out RatingRemovalResult, ipHash string) {
	parts := make([]string, 0, len(out.Players))
	for _, p := range out.Players {
		after := "none"
		if p.After != nil {
			after = fmt.Sprint(*p.After)
		}
		parts = append(parts, fmt.Sprintf("@%s %d → %s", p.Handle, p.Before, after))
	}
	how := "reversed"
	if out.Recomputed {
		how = fmt.Sprintf("recomputed, %d others moved", out.Others)
	}
	detail := fmt.Sprintf("%s, room %s: %s; %s", out.Game, out.RoomCode, strings.Join(parts, ", "), how)
	if err := m.store.AddAuthEvent(ctx, mod.ID, "rating-removed", detail, ipHash); err != nil {
		m.logger().Error("writing the audit log", "kind", "rating-removed", "err", err)
	}
}

// mailRemoval tells a player, at their confirmed address, that a game of theirs
// was taken out of the ratings. A mail that cannot go costs no more than itself.
func (m *Manager) mailRemoval(ctx context.Context, moderator string, side store.RatedSide, opponent string) {
	if !m.MailEnabled() {
		return
	}
	acc, err := m.store.AccountByID(ctx, side.AccountID)
	if err != nil || !acc.EmailVerified || acc.Email == "" {
		return
	}
	_ = m.SendAccountMail(ctx, "rating", acc.Email, "", func(base string) mail.Message {
		return mail.RatingRemoved(acc.MailLanguage(), acc.Email, moderator, side.FamilyName, opponent, side.RoomCode, acc.Handle, base)
	})
}

// ---- replaying a family's ratings ----------------------------------------------------

// replayed is one account's rating as a replay leaves it.
type replayed struct {
	player    rating.Player
	games     int
	opponents map[int64]bool
	last      time.Time
}

// replayRatings works out a family's ratings from its history alone, game by
// game in the order they were written, as recordResult did: each player's
// rating aged by the time since their last game, both updated from where they
// stood before it. It leaves out the games removed already and the one in
// skip. It is pure, so a replay of the same history always gives the same
// ratings.
func replayRatings(history []store.RatedSide, skip string) map[int64]*replayed {
	out := map[int64]*replayed{}
	for i := 0; i+1 < len(history); i++ {
		a, b := history[i], history[i+1]
		if a.RoomCode != b.RoomCode || !a.PlayedAt.Equal(b.PlayedAt) || a.AccountID != b.OpponentID {
			continue
		}
		i++
		if a.Removed || a.RoomCode == skip {
			continue
		}
		at := a.PlayedAt
		before := func(id int64) rating.Player {
			r, ok := out[id]
			if !ok {
				return rating.New()
			}
			return r.player.Aged(at.Sub(r.last))
		}
		pa, pb := before(a.AccountID), before(b.AccountID)
		for _, side := range []struct {
			s        store.RatedSide
			me, them rating.Player
		}{{a, pa, pb}, {b, pb, pa}} {
			r, ok := out[side.s.AccountID]
			if !ok {
				r = &replayed{opponents: map[int64]bool{}}
				out[side.s.AccountID] = r
			}
			r.player = rating.Update(side.me, []rating.Result{{Opponent: side.them, Score: side.s.Score}})
			r.games++
			r.opponents[side.s.OpponentID] = true
			r.last = at
		}
	}
	return out
}

// replayTolerance is how far a replayed number may be from the stored one and
// still be the same: the arithmetic is the same, so only another machine's
// rounding could tell them apart.
const replayTolerance = 1e-6

// sameRatings says a replay reproduces every stored rating of the family: its
// history is complete, and a replay without a game can be trusted.
func sameRatings(replay map[int64]*replayed, stored []store.Rating) bool {
	if len(replay) != len(stored) {
		return false
	}
	near := func(a, b float64) bool { return math.Abs(a-b) <= replayTolerance*math.Max(1, math.Abs(b)) }
	for _, s := range stored {
		r, ok := replay[s.AccountID]
		if !ok || r.games != s.Games || !near(r.player.Rating, s.Rating) || !near(r.player.Deviation, s.Deviation) || !near(r.player.Volatility, s.Volatility) {
			return false
		}
	}
	return true
}

// replacements are the ratings a replay writes over the stored ones, and the
// accounts it leaves with none.
func replacements(replay map[int64]*replayed, stored []store.Rating) ([]store.Rating, []int64) {
	var write []store.Rating
	var drop []int64
	for _, s := range stored {
		r, ok := replay[s.AccountID]
		if !ok {
			drop = append(drop, s.AccountID)
			continue
		}
		next := s
		next.Rating, next.Deviation, next.Volatility = r.player.Rating, r.player.Deviation, r.player.Volatility
		next.Games, next.Opponents, next.LastGameAt = r.games, len(r.opponents), r.last
		if next != s {
			write = append(write, next)
		}
	}
	return write, drop
}
