package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// presenceTTL is how long a "present" mark lasts without being renewed. The
// page renews it while the player keeps it on, so a closed tab or a lost
// connection clears itself.
const presenceTTL = 150 * time.Second

// ErrNotSeated is returned when a name that holds no human seat tries to mark
// itself present.
var ErrNotSeated = errors.New("session: only a player with a seat can mark themselves present")

// presenceEventData is the wire DTO of a "presence" Event.
type presenceEventData struct {
	Seat    int  `json:"seat"`
	Present bool `json:"present"`
}

// presenceTracker remembers, per room, which seats' players have said they
// are watching the game right now. It lives in memory only: a presence mark is
// worth nothing after a restart.
type presenceTracker struct {
	mu    sync.Mutex
	seats map[string]map[int]*time.Timer // room -> seat -> expiry
}

// set marks (or clears) a seat and arms the expiry. It reports whether the
// mark changed.
func (p *presenceTracker) set(room string, seat int, present bool, expire func()) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.seats == nil {
		p.seats = map[string]map[int]*time.Timer{}
	}
	seats := p.seats[room]
	old, was := seats[seat]
	if was {
		old.Stop()
	}
	if !present {
		if was {
			delete(seats, seat)
			if len(seats) == 0 {
				delete(p.seats, room)
			}
		}
		return was
	}
	if seats == nil {
		seats = map[int]*time.Timer{}
		p.seats[room] = seats
	}
	seats[seat] = time.AfterFunc(presenceTTL, expire)
	return !was
}

// list returns the seats currently marked present.
func (p *presenceTracker) list(room string) []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []int
	for s := range p.seats[room] {
		out = append(out, s)
	}
	return out
}

// SetPresent marks the seat held by name present (or away) and tells the room
// when that changed. Calling it again with present=true renews the mark.
func (m *Manager) SetPresent(ctx context.Context, roomCode, name string, present bool) error {
	roomCode = canonRoom(roomCode)
	game, err := m.store.GetGame(ctx, roomCode)
	if err != nil {
		return err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return err
	}
	seat := 0
	for _, p := range players {
		if p.Bot == "" && strings.EqualFold(p.Name, strings.TrimSpace(name)) {
			seat = p.Seat
		}
	}
	if seat == 0 {
		return ErrNotSeated
	}
	changed := m.presence.set(roomCode, seat, present, func() {
		if m.presence.set(roomCode, seat, false, nil) {
			m.hub.publish(roomCode, Event{Type: "presence", Data: presenceEventData{Seat: seat, Present: false}})
		}
	})
	if changed {
		m.hub.publish(roomCode, Event{Type: "presence", Data: presenceEventData{Seat: seat, Present: present}})
	}
	return nil
}

// Present lists the seats of the room whose players are marked present.
func (m *Manager) Present(roomCode string) []int {
	out := m.presence.list(canonRoom(roomCode))
	if out == nil {
		out = []int{}
	}
	return out
}
