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

// Every game that counts adds to its players' lasting counts when it ends
// (ADR 0130): results and facts per family, and the opponents met. A game
// counts when every seat is held by a different account and every seat has
// moved; a tutorial, a guest or a bot at a seat makes it count for nobody.
// It is counted by the results worker, after its rated result if it has one,
// so that nothing here ever slows a move.

// recordCounts counts every finished game waiting to be, one at a time. The
// caller holds resultsMu.
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

// countGame works out what one finished game adds to its players' counts, and
// writes it.
func (m *Manager) countGame(ctx context.Context, game store.Game) error {
	skip := func() error {
		m.mx.GameCount(metrics.CountSkipped)
		return m.store.SkipCount(ctx, game.ID)
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok || game.Tutorial != "" {
		return skip()
	}
	accounts, ok, err := m.seatAccounts(ctx, game, compiled)
	if err != nil {
		return err
	}
	if !ok {
		return skip()
	}
	// A seat that never moved was a no-show, or a quick way to hand
	// somebody a win.
	moved, err := m.store.SeatsThatMoved(ctx, game.ID)
	if err != nil {
		return err
	}
	if moved < len(accounts) {
		return skip()
	}
	facts, err := m.gameFacts(ctx, game.ID)
	if err != nil {
		return err
	}
	counts := countsOf(gameFamily(compiled), game, accounts, facts)
	if err := m.store.RecordCounts(ctx, game.ID, counts); err != nil {
		return err
	}
	m.mx.GameCount(metrics.CountCounted)
	return nil
}

// seatAccounts maps each seat of a game to the account holding it. ok is
// false unless every seat of the game is held, by a different account each.
func (m *Manager) seatAccounts(ctx context.Context, game store.Game, compiled *catalog.CompiledGame) (map[int]int64, bool, error) {
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return nil, false, err
	}
	if len(players) != len(compiled.Definition.Seats) {
		return nil, false, nil
	}
	principals := make([]string, 0, len(players))
	for _, p := range players {
		if p.Bot != "" || p.PrincipalID == "" {
			return nil, false, nil
		}
		principals = append(principals, p.PrincipalID)
	}
	byPrincipal, err := m.store.AccountsOfPrincipals(ctx, principals)
	if err != nil {
		return nil, false, err
	}
	out := make(map[int]int64, len(players))
	seen := map[int64]bool{}
	for _, p := range players {
		a := byPrincipal[p.PrincipalID]
		if a == 0 || seen[a] {
			return nil, false, nil
		}
		seen[a] = true
		out[p.Seat] = a
	}
	return out, true, nil
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
	rated := game.Rated && game.RatingStatus == store.RatingRecorded
	out := make(map[int64]map[string]int, len(accounts))
	for seat, account := range accounts {
		c := map[string]int{store.StatKey(store.CountGame+"played", family): 1}
		outcome := "drawn"
		if game.WinnerSeat != nil {
			outcome = "lost"
			if *game.WinnerSeat == seat {
				outcome = "won"
			}
		}
		c[store.StatKey(store.CountGame+outcome, family)] = 1
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
