package session

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/store"
)

// Every game that counts adds to its players' lasting counts when it ends,
// and earns them the badges checked at the end (ADR 0130). A game counts for
// player badges when every seat is held by a different account, and for bot
// badges when one account plays bots only; either way every seat must have
// moved. A tutorial or a guest at a seat makes it count for nobody. It is
// counted by the results worker, after its rated result if it has one, so
// that nothing here ever slows a move.

// recordCounts counts every finished game waiting to be, one at a time. The
// caller holds resultsMu, so nothing else writes the counts meanwhile.
func (m *Manager) recordCounts(ctx context.Context) {
	games, err := m.store.PendingCountGames(ctx)
	if err != nil {
		m.countFailed(err)
		return
	}
	for _, game := range games {
		if err := m.countGame(ctx, game); err != nil {
			m.countFailed(fmt.Errorf("game %d: %w", game.ID, err))
		}
	}
}

func (m *Manager) countFailed(err error) {
	m.mx.GameCount(metrics.CountFailed)
	m.logger().Error("counting a finished game for its players", "err", err)
}

// table is who sat at a finished game.
type table struct {
	// accounts maps each seat an account held to it.
	accounts map[int]int64
	// bots maps each seat a bot held to the bot's id.
	bots map[int]string
}

// countGame works out what one finished game adds to its players' counts and
// which end badges it earns them, and writes it all at once.
func (m *Manager) countGame(ctx context.Context, game store.Game) error {
	skip := func() error {
		m.mx.GameCount(metrics.CountSkipped)
		shown, err := m.store.SkipCount(ctx, game.ID, m.now())
		if err != nil {
			return err
		}
		m.announce(ctx, shown)
		return nil
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok || game.Tutorial != "" {
		return skip()
	}
	at, ok, err := m.seatsOf(ctx, game, compiled)
	if err != nil {
		return err
	}
	// Players against players, or one account against bots; nothing else.
	players := len(at.bots) == 0
	if !ok || (!players && len(at.accounts) != 1) {
		return skip()
	}
	// A seat that never moved was a no-show, or a quick way to hand
	// somebody a win.
	moved, err := m.store.SeatsThatMoved(ctx, game.ID)
	if err != nil {
		return err
	}
	if moved < len(compiled.Definition.Seats) {
		return skip()
	}
	facts, err := m.gameFacts(ctx, game.ID)
	if err != nil {
		return err
	}
	family := gameFamily(compiled)
	var counts map[int64]map[string]int
	if players {
		counts = countsOf(family, game, at.accounts, facts)
	}
	awards, err := m.endAwards(ctx, compiled, game, at, facts, counts)
	if err != nil {
		return err
	}
	written, err := m.store.RecordCounts(ctx, game.ID, counts, awards, m.now())
	if err != nil {
		return err
	}
	m.mx.GameCount(metrics.CountCounted)
	m.announce(ctx, written)
	return nil
}

// seatsOf is who held each seat of a game. ok is false unless every seat is
// held, each by a bot or an account, and no account holds two.
func (m *Manager) seatsOf(ctx context.Context, game store.Game, compiled *catalog.CompiledGame) (table, bool, error) {
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return table{}, false, err
	}
	if len(players) != len(compiled.Definition.Seats) {
		return table{}, false, nil
	}
	var principals []string
	for _, p := range players {
		if p.Bot == "" {
			if p.PrincipalID == "" {
				return table{}, false, nil
			}
			principals = append(principals, p.PrincipalID)
		}
	}
	byPrincipal, err := m.store.AccountsOfPrincipals(ctx, principals)
	if err != nil {
		return table{}, false, err
	}
	t := table{accounts: map[int]int64{}, bots: map[int]string{}}
	seen := map[int64]bool{}
	for _, p := range players {
		if p.Bot != "" {
			t.bots[p.Seat] = p.Bot
			continue
		}
		a := byPrincipal[p.PrincipalID]
		if a == 0 || seen[a] {
			return table{}, false, nil
		}
		seen[a] = true
		t.accounts[p.Seat] = a
	}
	return t, true, nil
}

// gameFacts reads the facts of every selection of a game, in order.
func (m *Manager) gameFacts(ctx context.Context, gameID int64) ([]state.Fact, error) {
	rows, err := m.store.GameFacts(ctx, gameID)
	if err != nil {
		return nil, err
	}
	var out []state.Fact
	for _, raw := range rows {
		var fs []state.Fact
		if err := json.Unmarshal([]byte(raw), &fs); err != nil {
			return nil, fmt.Errorf("reading stored facts: %w", err)
		}
		out = append(out, fs...)
	}
	return out, nil
}

// countsOf is what a finished game adds to the counts of the accounts at its
// seats: a result each, every fact about their seat, and each opponent met.
// A seat is the one the fact named when it was reported; the pie rule's swap
// moves players between seats, so a fact before it is the seat's, not the
// player's.
func countsOf(family string, game store.Game, accounts map[int]int64, facts []state.Fact) map[int64]map[string]int {
	rated := ratedResult(game)
	out := make(map[int64]map[string]int, len(accounts))
	for seat, account := range accounts {
		c := map[string]int{store.StatKey(store.CountGame+"played", family): 1}
		c[store.StatKey(store.CountGame+outcomeOf(game, seat), family)] = 1
		for other, opponent := range accounts {
			if other == seat {
				continue
			}
			c[store.MetKey(opponent, false)]++
			if rated {
				c[store.MetKey(opponent, true)]++
			}
		}
		out[account] = c
	}
	for _, f := range facts {
		if account, ok := accounts[f.Seat]; ok {
			out[account][store.StatKey(store.CountFact+f.Name, family)]++
		}
	}
	return out
}

// ratedResult says the game is rated and its result was written.
func ratedResult(game store.Game) bool {
	return game.Rated && game.RatingStatus == store.RatingRecorded
}

// outcomeOf is "won", "drawn" or "lost" for seat.
func outcomeOf(game store.Game, seat int) string {
	switch {
	case game.WinnerSeat == nil:
		return "drawn"
	case *game.WinnerSeat == seat:
		return "won"
	}
	return "lost"
}
