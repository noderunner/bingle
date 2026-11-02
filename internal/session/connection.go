package session

import (
	"sort"
	"sync"
	"time"
)

// Who is connected: a seat is connected while at least one socket that joined
// for it is open, so a second tab or device is harmless and a spectator's
// socket does not count. A seat with no socket left goes on showing as
// connected for a grace period, so that a reload or a brief drop flags nobody,
// and a socket that comes back marks it connected at once (ADR 0088).
//
// It lives in memory: a connection is worth nothing after a restart. Showing it
// is all this does; only a real-time game will attach consequences to it.

// ConnectionTimes are how a game's sockets are watched. A zero field takes its
// default; tests shorten them.
type ConnectionTimes struct {
	// PingEvery is how often the server pings each game socket.
	PingEvery time.Duration
	// PingTimeout is how long a ping waits for its answer before the socket
	// is closed as dead.
	PingTimeout time.Duration
	// Grace is how long a seat with no socket left still shows as connected.
	Grace time.Duration
}

const (
	defaultPingEvery   = 15 * time.Second
	defaultPingTimeout = 10 * time.Second
	defaultGrace       = 20 * time.Second
)

func (t ConnectionTimes) withDefaults() ConnectionTimes {
	if t.PingEvery <= 0 {
		t.PingEvery = defaultPingEvery
	}
	if t.PingTimeout <= 0 {
		t.PingTimeout = defaultPingTimeout
	}
	if t.Grace <= 0 {
		t.Grace = defaultGrace
	}
	return t
}

// SetConnectionTimes replaces how sockets are watched. It is called once, before
// the server takes any connection.
func (m *Manager) SetConnectionTimes(t ConnectionTimes) { m.connTimes = t.withDefaults() }

// ConnectionTimes says how sockets are watched.
func (m *Manager) ConnectionTimes() ConnectionTimes { return m.connTimes.withDefaults() }

// connectionEventData is the wire DTO of a "connection" Event.
type connectionEventData struct {
	Seat      int  `json:"seat"`
	Connected bool `json:"connected"`
}

// seatConn is one seat's sockets. It belongs to whoever holds the seat, so it
// follows them when they change seats; its sockets hold on to it, not to the
// seat number. A seat of 0 means nobody holds it any more.
type seatConn struct {
	seat    int
	sockets int
	grace   *time.Timer // running while there are no sockets and the seat still shows
}

// connTracker holds the seats that show as connected, room by room.
type connTracker struct {
	mu    sync.Mutex
	rooms map[string]map[int]*seatConn
	// absent holds the seats a bot is playing until their player reconnects.
	absent map[string]map[int]bool
}

func (m *Manager) publishConnection(room string, seat int, connected bool) {
	m.hub.publish(room, Event{Type: "connection", Data: connectionEventData{Seat: seat, Connected: connected}})
}

// TrackConnection notes that a socket has opened for the seat and tells the room
// if that connects it. The function it returns says the socket has closed.
//
// Events are published with the tracker's lock held, so that a room hears of a
// seat's changes in the order they happened: publishing never blocks, and the
// hub never calls back here.
func (m *Manager) TrackConnection(roomCode string, seat int) func() {
	if seat <= 0 {
		return func() {}
	}
	roomCode = canonRoom(roomCode)
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.rooms == nil {
		t.rooms = map[string]map[int]*seatConn{}
	}
	if t.rooms[roomCode] == nil {
		t.rooms[roomCode] = map[int]*seatConn{}
	}
	sc := t.rooms[roomCode][seat]
	if sc == nil {
		sc = &seatConn{seat: seat}
		t.rooms[roomCode][seat] = sc
		m.publishConnection(roomCode, seat, true)
	}
	if sc.grace != nil {
		sc.grace.Stop()
		sc.grace = nil
	}
	sc.sockets++
	t.clearAbsent(roomCode, seat)
	var once sync.Once
	return func() { once.Do(func() { m.socketClosed(roomCode, sc) }) }
}

func (m *Manager) socketClosed(room string, sc *seatConn) {
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	sc.sockets--
	if sc.sockets > 0 || sc.seat == 0 {
		return
	}
	sc.grace = time.AfterFunc(m.ConnectionTimes().Grace, func() { m.graceOver(room, sc) })
}

func (m *Manager) graceOver(room string, sc *seatConn) {
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	// A socket may have come back as the timer fired, or the seat been given up.
	if sc.sockets > 0 || sc.seat == 0 {
		return
	}
	seat := sc.seat
	t.forget(room, sc)
	m.publishConnection(room, seat, false)
}

// forget removes sc from its room. The caller holds the lock.
func (t *connTracker) forget(room string, sc *seatConn) {
	delete(t.rooms[room], sc.seat)
	if len(t.rooms[room]) == 0 {
		delete(t.rooms, room)
	}
	sc.seat = 0
	if sc.grace != nil {
		sc.grace.Stop()
		sc.grace = nil
	}
}

// ConnectedSeats lists the seats of the room that show as connected, in order.
func (m *Manager) ConnectedSeats(roomCode string) []int {
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []int{}
	for seat := range t.rooms[canonRoom(roomCode)] {
		out = append(out, seat)
	}
	sort.Ints(out)
	return out
}

// seatHasSocket says a socket is open for the seat at this moment, with no grace
// period: a player whose page has just gone is sent their turn mail.
func (m *Manager) seatHasSocket(room string, seat int) bool {
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	sc := t.rooms[room][seat]
	return sc != nil && sc.sockets > 0
}

// seatsSwapped follows a pie-rule swap: the people in seats a and b have changed
// places, with their sockets still open.
func (m *Manager) seatsSwapped(room string, a, b int) {
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	if abs := t.absent[room]; abs != nil {
		wasA, wasB := abs[a], abs[b]
		delete(abs, a)
		delete(abs, b)
		if wasA {
			abs[b] = true
		}
		if wasB {
			abs[a] = true
		}
	}
	seats := t.rooms[room]
	sa, sb := seats[a], seats[b]
	if sa == nil && sb == nil {
		return
	}
	delete(seats, a)
	delete(seats, b)
	if sa != nil {
		sa.seat = b
		seats[b] = sa
	}
	if sb != nil {
		sb.seat = a
		seats[a] = sb
	}
	if (sa == nil) != (sb == nil) {
		m.publishConnection(room, a, sb != nil)
		m.publishConnection(room, b, sa != nil)
	}
}

// seatMoved follows a player to another seat before the game has started.
func (m *Manager) seatMoved(room string, from, to int) {
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	sc := t.rooms[room][from]
	if sc == nil {
		return
	}
	delete(t.rooms[room], from)
	sc.seat = to
	t.rooms[room][to] = sc
	m.publishConnection(room, from, false)
	m.publishConnection(room, to, true)
}

// seatVacated forgets a seat its player has left. A socket still open for it no
// longer counts for anything.
func (m *Manager) seatVacated(room string, seat int) {
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	if sc := t.rooms[room][seat]; sc != nil {
		t.forget(room, sc)
		m.publishConnection(room, seat, false)
	}
}

// seatConnected says the seat shows as connected, the grace period included.
func (m *Manager) seatConnected(room string, seat int) bool {
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rooms[room][seat] != nil
}

// markAbsent says a bot is playing the seat's turns until its player reconnects
// (see AutoPlayTurn). It lives in memory like the rest: a restart forgets it, and the
// warnings start again.
func (m *Manager) markAbsent(room string, seat int) {
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.absent == nil {
		t.absent = map[string]map[int]bool{}
	}
	if t.absent[room] == nil {
		t.absent[room] = map[int]bool{}
	}
	t.absent[room][seat] = true
}

func (m *Manager) seatAbsent(room string, seat int) bool {
	t := &m.conns
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.absent[room][seat]
}

// clearAbsent is the player coming back. The caller holds the lock.
func (t *connTracker) clearAbsent(room string, seat int) {
	delete(t.absent[room], seat)
	if len(t.absent[room]) == 0 {
		delete(t.absent, room)
	}
}
