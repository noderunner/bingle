package session

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/store"
)

// Taking over a seat (ADR 0091): a tactical bot takes a seat from its player for
// good, when the table votes them out of a real-time game, or when their time runs
// out at a table of three or more. The record says who played the moves before it.
//
// Nothing here reaches a ruleset: the seat changes hands, and the moves that follow
// are a bot's like any other. The votes live in memory, as a connection does: a
// restart forgets them.

const (
	// ReasonKicked and ReasonTimeout are why a bot took a seat.
	ReasonKicked  = "kicked"
	ReasonTimeout = "timeout"

	// kickVoteLasts is how long a vote stays open.
	kickVoteLasts = 2 * time.Minute
	// kickVoteAgain is how long whoever opened a failed vote waits to open it again.
	kickVoteAgain = 5 * time.Minute
	// takeoverSeats is the smallest table where a bot takes a seat: at two the
	// other player wins on time, and nobody is left to vote.
	takeoverSeats = 3
)

var (
	// ErrKickNotAllowed is returned for a vote that cannot be held.
	ErrKickNotAllowed = errors.New("session: no vote to replace that player")
	// ErrWasReplaced is returned to a person a bot has replaced in this game.
	ErrWasReplaced = errors.New("session: a bot has taken your seat in this game, and you can no longer play in it: you can still watch")
)

// SeatReplaced says who held a seat before a bot took it: they played its moves up
// to and including move MoveSeq.
type SeatReplaced struct {
	Name    string `json:"name"`
	Reason  string `json:"reason"`
	MoveSeq int    `json:"move_seq"`
}

// seatReplacedEventData is the wire DTO of a "seat_replaced" Event.
type seatReplacedEventData struct {
	Seat   int    `json:"seat"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// KickVote is an open vote as clients see it. Seat is the target's, and Voters holds
// a seat for each person whose vote counts now.
type KickVote struct {
	Seat      int    `json:"seat"`
	Votes     int    `json:"votes"`
	Needed    int    `json:"needed"`
	Voters    []int  `json:"voters"`
	ExpiresAt string `json:"expires_at"`
}

// kickVoteEventData is the wire DTO of a "kick_vote" Event: every vote open now.
type kickVoteEventData struct {
	Votes []KickVote `json:"votes"`
}

type kickVote struct {
	opener  string
	voters  map[string]bool
	expires time.Time
	// told is what the room last heard, so that a change of who is connected is told.
	told [2]int
}

// kickVotes holds the votes of every room, by the person they are against, and when
// the opener of a failed vote may open it again, by opener and target.
type kickVotes struct {
	mu    sync.Mutex
	rooms map[string]map[string]*kickVote
	again map[string]map[[2]string]time.Time
}

func personKey(name string) string { return strings.ToLower(ownerOf(name)) }

// seatsOfPerson lists the seats a person plays, in the order of players.
func seatsOfPerson(players []store.Player, person string) []store.Player {
	var out []store.Player
	for _, p := range players {
		if p.Bot == "" && personKey(p.Name) == person {
			out = append(out, p)
		}
	}
	return out
}

// connectedPeople lists the people with a seat that shows as connected, each with
// the first of their seats.
func (m *Manager) connectedPeople(room string, players []store.Player) map[string]int {
	out := map[string]int{}
	for _, p := range players {
		if p.Bot != "" || !m.seatConnected(room, p.Seat) {
			continue
		}
		if _, seen := out[personKey(p.Name)]; !seen {
			out[personKey(p.Name)] = p.Seat
		}
	}
	return out
}

// tally counts a vote as things stand: those counted are the connected people and
// the target, who cannot vote, and a vote counts while its voter is connected.
func tally(v *kickVote, target string, connected map[string]int) (votes, needed int, voters []int) {
	counted := len(connected)
	if _, ok := connected[target]; !ok {
		counted++
	}
	for person := range v.voters {
		if seat, ok := connected[person]; ok {
			votes++
			voters = append(voters, seat)
		}
	}
	sort.Ints(voters)
	return votes, counted/2 + 1, voters
}

// VoteKick is name's vote to have a bot take the seats of the person at seat in a
// real-time game: it opens a vote, or joins the one that is open. The vote passes when
// more than half of those counted have voted for it.
func (m *Manager) VoteKick(ctx context.Context, roomCode, name string, seat int) (StateSnapshot, error) {
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
	if game.Status != "active" {
		return StateSnapshot{}, ErrGameNotActive
	}
	if game.Competitive() {
		return StateSnapshot{}, fmt.Errorf("%w: nobody can be voted out of a rated game", ErrRatedGame)
	}
	if game.RealTime == "" {
		return StateSnapshot{}, fmt.Errorf("%w: this is for a real-time game", ErrKickNotAllowed)
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return StateSnapshot{}, err
	}
	target := ""
	for _, p := range players {
		if p.Seat == seat && p.Bot == "" {
			target = personKey(p.Name)
		}
	}
	if target == "" {
		return StateSnapshot{}, fmt.Errorf("%w: nobody plays that seat but a bot", ErrKickNotAllowed)
	}
	voter := personKey(me.Name)
	if voter == target {
		return StateSnapshot{}, fmt.Errorf("%w: that is your own seat", ErrKickNotAllowed)
	}
	connected := m.connectedPeople(roomCode, players)
	if _, ok := connected[voter]; !ok {
		return StateSnapshot{}, ErrNotConnected
	}

	k := &m.kicks
	k.mu.Lock()
	k.expire(roomCode, m.now())
	v := k.rooms[roomCode][target]
	if v == nil {
		others := len(connected)
		if _, ok := connected[target]; ok {
			others--
		}
		if others < 2 {
			k.mu.Unlock()
			return StateSnapshot{}, fmt.Errorf("%w: it takes two other connected players", ErrKickNotAllowed)
		}
		if until, ok := k.again[roomCode][[2]string{voter, target}]; ok && m.now().Before(until) {
			k.mu.Unlock()
			return StateSnapshot{}, fmt.Errorf("%w: your last vote against them failed, so wait until %s", ErrKickNotAllowed, until.UTC().Format(time.RFC3339))
		}
		v = &kickVote{opener: voter, voters: map[string]bool{}, expires: m.now().Add(kickVoteLasts)}
		if k.rooms == nil {
			k.rooms = map[string]map[string]*kickVote{}
		}
		if k.rooms[roomCode] == nil {
			k.rooms[roomCode] = map[string]*kickVote{}
		}
		k.rooms[roomCode][target] = v
		m.mx.RealTime(metrics.RealTimeKickVote)
		m.logger().Info("a vote to replace a player opened", "room", roomCode, "seat", seat, "by_seat", me.Seat)
	}
	v.voters[voter] = true
	k.mu.Unlock()

	if err := m.settleKickVotes(ctx, roomCode, game, players, true); err != nil {
		return StateSnapshot{}, err
	}
	m.runBots(ctx, roomCode)
	return m.readState(ctx, roomCode, nil)
}

// expire ends the votes whose time is up. The caller holds the lock.
func (k *kickVotes) expire(room string, now time.Time) bool {
	changed := false
	for target, v := range k.rooms[room] {
		if now.Before(v.expires) {
			continue
		}
		delete(k.rooms[room], target)
		if k.again == nil {
			k.again = map[string]map[[2]string]time.Time{}
		}
		if k.again[room] == nil {
			k.again[room] = map[[2]string]time.Time{}
		}
		k.again[room][[2]string{v.opener, target}] = now.Add(kickVoteAgain)
		changed = true
	}
	return changed
}

// settleKickVotes carries out each vote of the room that has passed, drops those
// against somebody who is no longer playing, and tells the room if anything it knew
// has changed (tell says something has). The caller holds the room lock.
func (m *Manager) settleKickVotes(ctx context.Context, room string, game store.Game, players []store.Player, tell bool) error {
	connected := m.connectedPeople(room, players)
	k := &m.kicks
	k.mu.Lock()
	var passed []string
	for target, v := range k.rooms[room] {
		if len(seatsOfPerson(players, target)) == 0 {
			delete(k.rooms[room], target)
			tell = true
			continue
		}
		votes, needed, _ := tally(v, target, connected)
		if votes >= needed {
			passed = append(passed, target)
			delete(k.rooms[room], target)
			tell = true
		} else if v.told != [2]int{votes, needed} {
			v.told = [2]int{votes, needed}
			tell = true
		}
	}
	k.mu.Unlock()
	sort.Strings(passed)

	if len(passed) > 0 {
		compiled, ok := m.catalog.Get(game.GameDefID)
		if !ok {
			return ErrGameDefNotFound
		}
		for _, target := range passed {
			var err error
			// Each kick changes the clock and the row, so each starts from what the last left.
			if game, err = m.store.GetGame(ctx, room); err != nil {
				return err
			}
			if players, err = m.store.ListPlayers(ctx, game.ID); err != nil {
				return err
			}
			if err := m.takeOver(ctx, game, compiled, players, seatsOfPerson(players, target), ReasonKicked); err != nil {
				return err
			}
			m.mx.RealTime(metrics.RealTimeKick)
		}
		var err error
		if players, err = m.store.ListPlayers(ctx, game.ID); err != nil {
			return err
		}
	}
	if tell {
		m.hub.publish(room, Event{Type: "kick_vote", Data: kickVoteEventData{Votes: m.kickVoteViews(room, players)}})
	}
	return nil
}

// kickVoteViews is every vote open in the room, as clients see them.
func (m *Manager) kickVoteViews(room string, players []store.Player) []KickVote {
	connected := m.connectedPeople(room, players)
	k := &m.kicks
	k.mu.Lock()
	defer k.mu.Unlock()
	out := []KickVote{}
	for target, v := range k.rooms[room] {
		seats := seatsOfPerson(players, target)
		if len(seats) == 0 || !m.now().Before(v.expires) {
			continue
		}
		votes, needed, voters := tally(v, target, connected)
		out = append(out, KickVote{Seat: seats[0].Seat, Votes: votes, Needed: needed, Voters: voters, ExpiresAt: v.expires.UTC().Format(time.RFC3339)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seat < out[j].Seat })
	return out
}

// sweepKickVotes looks at the open votes: one may have run out of time, or passed
// because somebody who had not voted has gone.
func (m *Manager) sweepKickVotes(ctx context.Context) {
	k := &m.kicks
	k.mu.Lock()
	var rooms []string
	for room, votes := range k.rooms {
		if len(votes) > 0 {
			rooms = append(rooms, room)
		}
	}
	k.mu.Unlock()
	for _, room := range rooms {
		m.sweepRoomKickVotes(ctx, room)
	}
}

func (m *Manager) sweepRoomKickVotes(ctx context.Context, room string) {
	unlock := m.lockRoom(ctx, room)
	defer unlock()
	game, err := m.store.GetGame(ctx, room)
	if err != nil || game.Status != "active" {
		m.kicks.forget(room)
		return
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		m.logger().Error("reading a real-time game's players", "room", room, "err", err)
		return
	}
	k := &m.kicks
	k.mu.Lock()
	expired := k.expire(room, m.now())
	k.mu.Unlock()
	if err := m.settleKickVotes(ctx, room, game, players, expired); err != nil {
		m.logger().Error("settling a vote to replace a player", "room", room, "err", err)
		return
	}
	m.continueBots(room)
}

func (k *kickVotes) forget(room string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.rooms, room)
	delete(k.again, room)
}

// takeOver gives seats to a tactical bot for good. The caller holds the room lock,
// and has the bots carry on from there.
func (m *Manager) takeOver(ctx context.Context, game store.Game, compiled *catalog.CompiledGame, players, seats []store.Player, reason string) error {
	bot, ok := m.bots.Get(autoPlayBot)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownBot, autoPlayBot)
	}
	taken := make(map[string]bool, len(players))
	for _, p := range players {
		taken[strings.ToLower(p.Name)] = true
	}
	clock := parseGameClock(game.Clock)
	restart := false
	total := len(compiled.Definition.Seats)
	takeovers := make([]store.SeatTakeover, 0, len(seats))
	for _, p := range seats {
		name := ""
		for _, sd := range compiled.Definition.Seats {
			if sd.Seat == p.Seat {
				name = botName(bot, sd, total)
			}
		}
		// A person may already go by the name a bot would take.
		if taken[strings.ToLower(name)] {
			name = fmt.Sprintf("%s %d", name, p.Seat)
		}
		taken[strings.ToLower(name)] = true
		clock.drop(p.Seat)
		restart = restart || (game.ImpulseSeat != nil && *game.ImpulseSeat == p.Seat)
		takeovers = append(takeovers, store.SeatTakeover{
			Replaced: store.SeatReplacement{Seat: p.Seat, Name: p.Name, PrincipalID: p.PrincipalID, Reason: reason, MoveSeq: game.Ply},
			BotName:  name, Bot: bot.ID(),
		})
	}
	stored := game.Clock
	if clock != nil {
		var err error
		if stored, err = clock.marshal(); err != nil {
			return err
		}
	}
	if err := m.store.ReplaceSeats(ctx, game.ID, takeovers, stored, restart); err != nil {
		return err
	}
	for _, t := range takeovers {
		r := t.Replaced
		m.seatVacated(game.RoomCode, r.Seat)
		m.logger().Info("a bot took a seat", "room", game.RoomCode, "seat", r.Seat, "reason", reason)
		m.hub.publish(game.RoomCode, Event{Type: "seat_replaced", Data: seatReplacedEventData{Seat: r.Seat, Name: r.Name, Reason: reason}})
	}
	m.publishState(ctx, game.RoomCode, "takeover")
	return nil
}

// flagAtLargeTable is the time of the seat to move running out at a table of three
// or more: a bot takes the seat and the game goes on, unless no person would be left.
// A bot that was seated from the start only stops being timed. The caller holds the
// room lock.
func (m *Manager) flagAtLargeTable(ctx context.Context, game store.Game, compiled *catalog.CompiledGame) (store.Game, error) {
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return game, err
	}
	var flagged *store.Player
	people := 0
	for i := range players {
		if players[i].Seat == *game.ImpulseSeat {
			flagged = &players[i]
		}
		if players[i].Bot == "" {
			people++
		}
	}
	switch {
	case flagged == nil:
		return game, nil
	case flagged.Bot != "":
		if clock := parseGameClock(game.Clock); clock.holds(flagged.Seat) {
			clock.drop(flagged.Seat)
			stored, err := clock.marshal()
			if err != nil {
				return game, err
			}
			if err := m.store.SetGameClock(ctx, game.ID, stored); err != nil {
				return game, err
			}
		}
	case people <= 1:
		if err := m.store.FinishGame(ctx, game.ID, nil, "timeout"); err != nil {
			return game, err
		}
		m.finished(game, gameFamily(compiled), "timeout")
		m.publishState(ctx, game.RoomCode, "timeout")
		return m.store.GetGame(ctx, game.RoomCode)
	default:
		if err := m.takeOver(ctx, game, compiled, players, []store.Player{*flagged}, ReasonTimeout); err != nil {
			return game, err
		}
		m.mx.RealTime(metrics.RealTimeTimeoutBot)
	}
	// In the background: this may be a read, and it must not wait for a bot to think.
	m.continueBots(game.RoomCode)
	return m.store.GetGame(ctx, game.RoomCode)
}

// mayTakeSeat refuses a seat to a person a bot has replaced in this game.
func (m *Manager) mayTakeSeat(ctx context.Context, game store.Game, name string) error {
	replaced, err := m.store.WasReplaced(ctx, game.ID, auth.IDFrom(ctx), name)
	if err != nil {
		return err
	}
	if replaced {
		return ErrWasReplaced
	}
	return nil
}
