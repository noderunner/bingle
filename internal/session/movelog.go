package session

import (
	"context"
	"log/slog"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/store"
)

// The move funnel's log.
//
// Every move in bingle — played by a person, a bot, an auto-passed reaction or
// the Auto-Play watcher, in a JSON game or a Go ruleset — goes through
// applyAndRecordAs. Instrumented once here, every game gets the same line, and
// no logging call goes into any game's code (ADR 0025, and the plan's section
// 6a):
//
//	level=DEBUG msg="move applied" game=courtfall room=7F-a1b2c3d4e5f6 seq=37
//	  seat=2 move="Q d5>d4 2/2" dur=1.8ms locked=true
//
// What it may carry is ADR 0063's business, not this file's convenience: the
// room is a reference rather than a code, the player is not named, and the
// position is the **public** view unless BINGLE_DEBUG_SECRETS says otherwise,
// because a position carries committed dice, hidden hands and RNG state.

// moveLine is what logMove says about one move.
type moveLine struct {
	seq       int
	notation  string
	dur       time.Duration
	locked    bool
	ended     bool
	endReason string
	// state is the position after the move, logged only when a debug room or
	// BINGLE_DEBUG_SECRETS asks for it.
	state *state.BoardState
	// err is set when the ruleset refused the move.
	err error
}

// logMove writes the one line per move.
//
// It is at debug, so an ordinary server says nothing per move; a move the
// rules refused is at info, because that is worth seeing without turning the
// firehose on. The one room BINGLE_DEBUG_ROOM names is logged at info
// whatever else is set.
func (m *Manager) logMove(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, player store.Player, mv state.Move, line moveLine) {
	family := gameFamily(compiled)
	watched := m.watchedRoom(game.RoomCode)
	if !watched && !m.debugGame(family) {
		return
	}

	level := slog.LevelDebug
	msg := "move applied"
	if line.err != nil {
		level = slog.LevelInfo
		msg = "move refused by the rules"
	} else if watched {
		level = slog.LevelInfo
	}

	lg := m.logger()
	if !watched && !lg.Enabled(ctx, level) {
		// Nothing would be written, so none of the formatting below is worth
		// doing: a move costs this much and no more when debug is off.
		return
	}

	attrs := []slog.Attr{
		slog.String("game", family),
		logging.Room(game.RoomCode),
		slog.Int("seq", line.seq),
		slog.Int("seat", player.Seat),
		slog.String("move", line.notation),
		slog.Duration("dur", line.dur),
	}
	if line.notation == "" {
		// A refused move has no canonical notation, so say what was offered
		// in the structured form instead.
		attrs[4] = slog.String("move", mv.Action+" "+string(mv.From)+">"+string(mv.To))
	}
	if mv.Action != "" {
		attrs = append(attrs, slog.String("action", mv.Action))
	}
	if line.locked {
		attrs = append(attrs, slog.Bool("locked", true))
	}
	if line.ended {
		attrs = append(attrs, slog.Bool("ended", true), slog.String("end_reason", line.endReason))
	}
	if line.err != nil {
		attrs = append(attrs, slog.String("err", line.err.Error()))
	}
	if pos := m.positionFor(compiled, line.state, watched); pos != "" {
		attrs = append(attrs, slog.String("position", pos))
	}
	lg.LogAttrs(ctx, level, msg, attrs...)
}

// positionFor renders the position for a debug line, or "" when it is not
// wanted.
//
// What it renders is the **public** view: the plan's section 6g, because
// Courtfall commits dice in secret, Cthulhu Wars hides Elder Signs, Nyet!
// hides hands, and every position carries RNG state. The full position is
// development-only, behind BINGLE_DEBUG_SECRETS.
func (m *Manager) positionFor(compiled *catalog.CompiledGame, st *state.BoardState, watched bool) string {
	if st == nil || (!watched && !m.debug.Secrets) {
		return ""
	}
	if m.debug.Secrets {
		return state.RenderText(compiled, st)
	}
	return state.RenderText(compiled, state.Public(compiled, st, 0))
}
