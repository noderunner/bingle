package session

import (
	"context"
	"fmt"
	"time"

	"github.com/noderunner/bingle/internal/badges"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/store"
)

// A badge checked during play is awarded as soon as it holds (ADR 0130), by a
// worker away from the room lock: a move that reports facts moves its game's
// facts_seq on and wakes the worker, which reads the game's facts from its
// start, checks the during badges of its seats and moves the game's
// badges_seq up to it. The watermark says where to look, not what was found,
// so a crash only repeats a check, and an award is written once.
//
// An award that depends on a secret fact is shown only to its holder until
// the game ends; the results worker shows it then.

// wakeBadges asks the worker for a check, without waiting for it.
func (m *Manager) wakeBadges() {
	select {
	case m.badgesDue <- struct{}{}:
	default: // a check is already asked for
	}
}

// RunBadges checks the badges of games with new facts whenever a move
// reports some, and now and then in case a check failed, until ctx ends.
func (m *Manager) RunBadges(ctx context.Context) {
	m.CheckBadges(ctx)
	t := time.NewTicker(resultsRetry)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.badgesDue:
		case <-t.C:
		}
		m.CheckBadges(ctx)
	}
}

// CheckBadges checks the during badges of every game with facts it has not
// looked at. It is exported so that a test can wait for it.
func (m *Manager) CheckBadges(ctx context.Context) {
	m.badgesMu.Lock()
	defer m.badgesMu.Unlock()
	due, err := m.store.GamesWithBadgesDue(ctx)
	if err != nil {
		m.badgeFailed(err)
		return
	}
	for _, d := range due {
		if err := m.checkDuring(ctx, d); err != nil {
			m.badgeFailed(fmt.Errorf("game %d: %w", d.Game.ID, err))
		}
	}
}

func (m *Manager) badgeFailed(err error) {
	m.mx.BadgeCheck(metrics.BadgeFailed)
	m.logger().Error("checking the badges of a game during play", "err", err)
}

// checkDuring checks one game's during badges and moves its watermark.
func (m *Manager) checkDuring(ctx context.Context, d store.BadgesDue) error {
	game := d.Game
	done := func() error {
		m.mx.BadgeCheck(metrics.BadgeSkipped)
		_, err := m.store.RecordDuring(ctx, game.ID, d.Seq, nil)
		return err
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok || game.Tutorial != "" {
		return done()
	}
	var list []gameBadge
	for _, b := range badgesOf(compiled) {
		if b.def.When == badges.WhenDuring {
			list = append(list, b)
		}
	}
	if len(list) == 0 {
		return done()
	}
	// Players against players only: a bot badge is checked at the end.
	at, ok, err := m.seatsOf(ctx, game, compiled)
	if err != nil {
		return err
	}
	if !ok || len(at.bots) > 0 {
		return done()
	}
	facts, err := m.gameFacts(ctx, game.ID)
	if err != nil {
		return err
	}
	eg := played{compiled: compiled, game: game, family: gameFamily(compiled), facts: facts}
	now := m.now()
	var awards []store.Award
	for seat, account := range at.accounts {
		for _, b := range list {
			if !m.earns(b, eg, seat, nil) {
				continue
			}
			awards = append(awards, store.Award{
				AccountID: account, Badge: b.id, Name: b.def.Name, Description: b.def.Description,
				Family: eg.family, GameID: game.ID, RoomCode: game.RoomCode, Detail: compiled.Definition.Name,
				AwardedAt: now, Visible: !secretBadge(b, compiled),
			})
		}
	}
	written, err := m.store.RecordDuring(ctx, game.ID, d.Seq, awards)
	if err != nil {
		return err
	}
	m.mx.BadgeCheck(metrics.BadgeChecked)
	m.announce(ctx, written)
	return nil
}
