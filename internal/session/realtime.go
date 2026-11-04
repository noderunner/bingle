package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/store"
)

// Real-time play: a game chosen at setup whose players are expected to stay
// until it ends (ADR 0089). With a timer, time running out is the only
// consequence, and all this adds is a watcher that lets a flag fall with nobody
// looking. Without one, the player the game waits on is warned, then the table
// is, and from then on any other connected person may have a tactical bot play
// that one turn.
//
// Nothing here reaches a ruleset: a consequence is an ordinary move, marked as
// auto-play.

const (
	defaultFirstWarning  = 2 * 60
	defaultSecondWarning = 5 * 60
	// minWarning and maxWarning bound either wait, in seconds.
	minWarning = 30
	maxWarning = 60 * 60
	// realTimeEvery is how often the watcher looks. Warnings and flags are this
	// late at worst.
	realTimeEvery = 5 * time.Second
)

var (
	// ErrInvalidRealTime is returned for warnings outside the allowed range.
	ErrInvalidRealTime = fmt.Errorf("session: a real-time warning must be between %d seconds and %d minutes", minWarning, maxWarning/60)
	// ErrNoAutoPlayOffer is returned when there is no turn on offer to a bot:
	// not a real-time game without a timer, the second warning has not passed, or
	// the turn has just been played.
	ErrNoAutoPlayOffer = errors.New("session: no turn is on offer to a bot right now")
	// ErrOwnTurnOffer is returned to the player the game is waiting on.
	ErrOwnTurnOffer = errors.New("session: it is your own turn: play it, or turn on your Auto-Play Timeout")
	// ErrNotConnected is returned to a player whose seat shows as disconnected.
	ErrNotConnected = errors.New("session: only a connected player can do that: open the game first")
)

// RealTimeSettings is what is chosen at setup, and stored as JSON in
// games.real_time. The second warning counts from the first. A wait of zero takes
// its default.
type RealTimeSettings struct {
	FirstWarningSeconds  int `json:"first_warning_seconds"`
	SecondWarningSeconds int `json:"second_warning_seconds"`
}

func (s RealTimeSettings) resolved() (RealTimeSettings, error) {
	if s.FirstWarningSeconds == 0 {
		s.FirstWarningSeconds = defaultFirstWarning
	}
	if s.SecondWarningSeconds == 0 {
		s.SecondWarningSeconds = defaultSecondWarning
	}
	for _, w := range []int{s.FirstWarningSeconds, s.SecondWarningSeconds} {
		if w < minWarning || w > maxWarning {
			return s, ErrInvalidRealTime
		}
	}
	return s, nil
}

func parseRealTime(stored string) *RealTimeSettings {
	if stored == "" {
		return nil
	}
	var s RealTimeSettings
	if json.Unmarshal([]byte(stored), &s) != nil {
		return nil
	}
	return &s
}

// RealTimeWait says how long the game has been waiting on a person, in a game
// where that has consequences.
type RealTimeWait struct {
	Seat int `json:"seat"`
	// Stage is how many warnings have passed: 0, 1 or 2. At 2 the turn is on offer.
	Stage           int    `json:"stage"`
	FirstWarningAt  string `json:"first_warning_at"`
	SecondWarningAt string `json:"second_warning_at"`
}

// RealTimeSnapshot is a real-time game's settings as clients see them. Wait is
// nil when the game is waiting on nobody the warnings apply to.
type RealTimeSnapshot struct {
	RealTimeSettings
	Wait *RealTimeWait `json:"wait,omitempty"`
	// FirstMove is set while a seat of a real-time tournament's game must make its
	// first move in time, or forfeit.
	FirstMove *FirstMove `json:"first_move,omitempty"`
	// KickVotes are the votes open to have a bot replace a player.
	KickVotes []KickVote `json:"kick_votes,omitempty"`
}

// realTimeOfTournamentGame is what a real-time tournament's games store. They have a
// clock, so the warnings never apply, and the settings say only that it is real-time.
const realTimeOfTournamentGame = `{"first_warning_seconds":120,"second_warning_seconds":300}`

// FirstMove is the deadline for a first move in a real-time tournament's game.
type FirstMove struct {
	Seat int    `json:"seat"`
	By   string `json:"by"`
}

// slowTurnEventData is the wire DTO of a "slow_turn" Event.
type slowTurnEventData struct {
	Seat  int `json:"seat"`
	Stage int `json:"stage"`
}

func (m *Manager) realTimeSnapshot(game store.Game, players []store.Player) *RealTimeSnapshot {
	s := parseRealTime(game.RealTime)
	if s == nil {
		return nil
	}
	snap := &RealTimeSnapshot{RealTimeSettings: *s, Wait: m.realTimeWait(game, players)}
	if game.Status == "active" {
		snap.KickVotes = m.kickVoteViews(game.RoomCode, players)
	}
	return snap
}

// firstMoveOf is the first-move deadline a real-time tournament's game is under, if any.
func (m *Manager) firstMoveOf(ctx context.Context, game store.Game) *FirstMove {
	if game.TournamentID == 0 || game.RealTime == "" || game.Status != "active" {
		return nil
	}
	t, err := m.store.GetTournament(ctx, game.TournamentID)
	if err != nil {
		return nil
	}
	seat, by, _, due := m.firstMoveDue(ctx, t, game)
	if !due {
		return nil
	}
	return &FirstMove{Seat: seat, By: by.UTC().Format(time.RFC3339)}
}

func samePerson(a, b store.Player) bool {
	return strings.EqualFold(a.Name, b.Name)
}

// realTimeWait is the wait the warnings apply to: a real-time game with no timer,
// waiting on a person, at a table with at least one other person.
func (m *Manager) realTimeWait(game store.Game, players []store.Player) *RealTimeWait {
	s := parseRealTime(game.RealTime)
	if s == nil || game.Status != "active" || game.ImpulseSeat == nil || game.Clock != "" || game.MoveTimeLimit > 0 {
		return nil
	}
	var waitingOn *store.Player
	for i := range players {
		if players[i].Seat == *game.ImpulseSeat && players[i].Bot == "" {
			waitingOn = &players[i]
		}
	}
	if waitingOn == nil {
		return nil
	}
	company := false
	for _, p := range players {
		company = company || (p.Bot == "" && !samePerson(p, *waitingOn))
	}
	if !company {
		return nil
	}
	started, ok := parseTimestamp(game.UpdatedAt)
	if !ok {
		return nil
	}
	// The wait restarts at every decision the game accepts, as updated_at does,
	// and time the server was down does not count.
	first := m.turnStart(started).Add(time.Duration(s.FirstWarningSeconds) * time.Second)
	second := first.Add(time.Duration(s.SecondWarningSeconds) * time.Second)
	stage := 0
	switch now := m.now(); {
	case !now.Before(second):
		stage = 2
	case !now.Before(first):
		stage = 1
	}
	return &RealTimeWait{
		Seat: *game.ImpulseSeat, Stage: stage,
		FirstWarningAt:  first.UTC().Format(time.RFC3339),
		SecondWarningAt: second.UTC().Format(time.RFC3339),
	}
}

// AutoPlayTurn has a tactical bot play the turn the game has been waiting on since
// past the second warning, at the asking of another connected person at the table.
// If the player is disconnected at that moment, the bot goes on playing their turns
// until they reconnect.
func (m *Manager) AutoPlayTurn(ctx context.Context, roomCode, name string) (StateSnapshot, error) {
	roomCode = canonRoom(roomCode)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), botBackgroundLimit)
	defer cancel()
	unlock := m.lockRoom(ctx, roomCode)
	defer unlock()
	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil {
		return StateSnapshot{}, err
	}
	me, err := m.personSeat(ctx, game, name)
	if err != nil {
		return StateSnapshot{}, err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return StateSnapshot{}, err
	}
	wait := m.realTimeWait(game, players)
	if wait == nil || wait.Stage < 2 {
		return StateSnapshot{}, ErrNoAutoPlayOffer
	}
	for _, p := range players {
		if p.Seat == wait.Seat && samePerson(p, me) {
			return StateSnapshot{}, ErrOwnTurnOffer
		}
	}
	if !m.seatConnected(roomCode, me.Seat) {
		return StateSnapshot{}, ErrNotConnected
	}
	if !m.seatConnected(roomCode, wait.Seat) {
		m.markAbsent(roomCode, wait.Seat)
	}
	if !m.playTurnFor(ctx, roomCode, game, wait.Seat) {
		return StateSnapshot{}, ErrNoAutoPlayOffer
	}
	m.mx.RealTime(metrics.RealTimeOfferTaken)
	m.logger().Info("a bot played a slow turn", "room", roomCode, "seat", wait.Seat, "asked_by_seat", me.Seat)
	m.runBots(ctx, roomCode)
	return m.readState(ctx, roomCode, nil)
}

// playAbsentTurn plays one decision of a seat whose player is away from a game where
// the table asked for the bot, and reports whether it did. The caller holds the room
// lock.
func (m *Manager) playAbsentTurn(ctx context.Context, roomCode string, game store.Game, player store.Player, bs *state.BoardState, i int) (MoveOutcome, bool) {
	if !m.seatAbsent(roomCode, player.Seat) {
		return MoveOutcome{}, false
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return MoveOutcome{}, false
	}
	bot, ok := m.bots.Get(autoPlayBot)
	if !ok {
		return MoveOutcome{}, false
	}
	legal := state.LegalMoves(compiled, bs, player.Seat)
	if len(legal) == 0 {
		return MoveOutcome{}, false
	}
	outcome, err := m.applyAndRecordAs(ctx, compiled, game, player, m.botMove(ctx, bot, roomCode, compiled, bs, player.Seat, legal, i), playedForPlayer)
	if err != nil {
		return MoveOutcome{}, false
	}
	m.mx.RealTime(metrics.RealTimeAbsentMove)
	return outcome, true
}

// realTimeWarned remembers the warnings already given, so that each is given once.
// It lives in memory: a restart restarts the wait anyway.
type realTimeWarned struct {
	mu   sync.Mutex
	last map[string]warnedTurn
}

type warnedTurn struct {
	updatedAt string
	seq       int
	stage     int
}

// RunRealTime watches the running real-time games until ctx ends.
func (m *Manager) RunRealTime(ctx context.Context) {
	t := time.NewTicker(realTimeEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.RealTimeDue(ctx)
		}
	}
}

// RealTimeDue looks at every running real-time game once: a flag that has fallen
// falls, and a warning that has come due is given. It is exported so that a test can
// wait for it.
func (m *Manager) RealTimeDue(ctx context.Context) {
	games, err := m.store.RealTimeGames(ctx)
	if err != nil {
		m.logger().Error("reading the real-time games", "err", err)
		return
	}
	running := make(map[string]bool, len(games))
	for _, game := range games {
		running[game.RoomCode] = true
		if game.Clock != "" || game.MoveTimeLimit > 0 {
			if _, err := m.loadGame(ctx, game.RoomCode, false); err != nil {
				m.logger().Error("looking at a real-time game's clock", "room", game.RoomCode, "err", err)
			}
			continue
		}
		players, err := m.store.ListPlayers(ctx, game.ID)
		if err != nil {
			m.logger().Error("reading a real-time game's players", "room", game.RoomCode, "err", err)
			continue
		}
		wait := m.realTimeWait(game, players)
		if wait != nil && wait.Stage > 0 && m.warnedNow(game, wait.Stage) {
			m.hub.publish(game.RoomCode, Event{Type: "slow_turn", Data: slowTurnEventData{Seat: wait.Seat, Stage: wait.Stage}})
			kind := metrics.RealTimeFirstWarning
			if wait.Stage == 2 {
				kind = metrics.RealTimeSecondWarning
			}
			m.mx.RealTime(kind)
			m.logger().Info("a real-time turn is slow", "room", game.RoomCode, "seat", wait.Seat, "stage", wait.Stage)
		}
	}
	m.sweepKickVotes(ctx)
	m.forgetEndedRealTime(running)
}

// warnedNow says whether stage is news for this turn, and notes that it has been told.
func (m *Manager) warnedNow(game store.Game, stage int) bool {
	w := &m.rtWarned
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.last == nil {
		w.last = map[string]warnedTurn{}
	}
	key := warnedTurn{updatedAt: game.UpdatedAt, seq: game.MoveSeq, stage: stage}
	if old := w.last[game.RoomCode]; old.updatedAt == key.updatedAt && old.seq == key.seq && old.stage >= stage {
		return false
	}
	w.last[game.RoomCode] = key
	return true
}

func (m *Manager) forgetEndedRealTime(running map[string]bool) {
	w := &m.rtWarned
	w.mu.Lock()
	for room := range w.last {
		if !running[room] {
			delete(w.last, room)
		}
	}
	w.mu.Unlock()
	k := &m.kicks
	k.mu.Lock()
	for room := range k.again {
		if !running[room] {
			delete(k.again, room)
		}
	}
	k.mu.Unlock()
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	for room := range t.absent {
		if !running[room] {
			delete(t.absent, room)
		}
	}
}
