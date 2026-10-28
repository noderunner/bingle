package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// A game clock (a chess clock) gives each player a store of time instead of an
// allowance per move: the clock of the player to move runs, and the first
// player to run out forfeits the game. It is a choice made when the game is
// created, for a game of two seats, and each seat's time can differ so that a
// stronger player can give a weaker one odds.
//
// What each seat has left lives in the games row as JSON, with the moment the
// running clock started. That moment is written by the transaction that records
// a move, to the nanosecond: the row's updated_at would do (every statement
// that bumps it is a turn passing) but SQLite's CURRENT_TIMESTAMP is truncated
// to whole seconds, which would cost the mover up to a second of their store on
// every move. The turn therefore began at the later of the two — the clock's own
// mark, and the last time anything else moved the game on (an undo, the game
// starting) — and the time left is read without writing anything.

// MaxClockSeconds bounds one side's clock (30 days), as MaxMoveTimeSeconds
// bounds a per-move allowance.
const MaxClockSeconds = 30 * 24 * 60 * 60

// ErrInvalidClock is returned by CreateGame for a clock that cannot be played
// with: a time out of range, one time per seat where the game has other than
// two, or a clock together with a per-move limit.
var ErrInvalidClock = errors.New("session: invalid game clock")

// GameClock is a game's clock: what each seat started with and what it has
// left, in seconds, by seat number. The seat to move is the game's turn seat,
// and its clock has been running since the game row's updated_at.
type GameClock struct {
	Initial map[string]int     `json:"initial"`
	Left    map[string]float64 `json:"left"`
	// Since is when the running clock last started (RFC 3339 with
	// nanoseconds), empty before the first move of the game.
	Since string `json:"since,omitempty"`
}

// newGameClock returns the clock a game starts with: seconds[i] is the time of
// the i-th seat of seats. It fails unless there is one positive time per seat.
func newGameClock(seats []int, seconds []int) (*GameClock, error) {
	if len(seconds) == 0 {
		return nil, nil
	}
	if len(seats) != 2 {
		return nil, fmt.Errorf("%w: a game clock is for two players, and this game seats %d", ErrInvalidClock, len(seats))
	}
	if len(seconds) != len(seats) {
		return nil, fmt.Errorf("%w: %d times for %d seats", ErrInvalidClock, len(seconds), len(seats))
	}
	c := &GameClock{Initial: map[string]int{}, Left: map[string]float64{}}
	for i, seat := range seats {
		if seconds[i] <= 0 || seconds[i] > MaxClockSeconds {
			return nil, fmt.Errorf("%w: %d seconds for seat %d (1 to %d)", ErrInvalidClock, seconds[i], seat, MaxClockSeconds)
		}
		k := seatKey(seat)
		c.Initial[k] = seconds[i]
		c.Left[k] = float64(seconds[i])
	}
	return c, nil
}

func seatKey(seat int) string { return strconv.Itoa(seat) }

// parseGameClock reads a stored clock; nil for a game played without one (or
// for a value that cannot be read, which is treated as none rather than
// stopping the game).
func parseGameClock(stored string) *GameClock {
	if stored == "" {
		return nil
	}
	var c GameClock
	if err := json.Unmarshal([]byte(stored), &c); err != nil || len(c.Left) == 0 {
		return nil
	}
	return &c
}

// marshal is the clock as it is stored.
func (c *GameClock) marshal() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("session: marshal the game clock: %w", err)
	}
	return string(b), nil
}

// LeftFor is the seconds seat has left, not counting a turn in progress.
func (c *GameClock) LeftFor(seat int) float64 {
	if c == nil {
		return 0
	}
	return c.Left[seatKey(seat)]
}

// InitialFor is the seconds seat started with.
func (c *GameClock) InitialFor(seat int) int {
	if c == nil {
		return 0
	}
	return c.Initial[seatKey(seat)]
}

// Seats lists the seats the clock holds a time for, in seat order.
func (c *GameClock) Seats() []int {
	if c == nil {
		return nil
	}
	var out []int
	for k := range c.Left {
		if n, err := strconv.Atoi(k); err == nil {
			out = append(out, n)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// startedAt is when the turn in progress began: the later of the clock's own
// mark and whatever else last moved the game on (fallback, the game row's
// updated_at, which is truncated to the second).
func (c *GameClock) startedAt(fallback time.Time) time.Time {
	if c == nil || c.Since == "" {
		return fallback
	}
	since, err := time.Parse(time.RFC3339Nano, c.Since)
	if err != nil || since.Before(fallback) {
		return fallback
	}
	return since.UTC()
}

// start marks the clock as running from now (the next turn's beginning).
func (c *GameClock) start(now time.Time) {
	if c != nil {
		c.Since = now.UTC().Format(time.RFC3339Nano)
	}
}

// snapshot describes the clock for clients, with running as the seat whose
// clock is counting down (0 when none is).
func (c *GameClock) snapshot(running int) *ClockSnapshot {
	if c == nil {
		return nil
	}
	out := &ClockSnapshot{Running: running}
	for _, seat := range c.Seats() {
		out.Seats = append(out.Seats, SeatClock{Seat: seat, Initial: c.InitialFor(seat), Left: c.LeftFor(seat)})
	}
	return out
}

// charge takes the time a turn took off the seat's clock, never below zero.
func (c *GameClock) charge(seat int, d time.Duration) {
	if c == nil || d <= 0 {
		return
	}
	k := seatKey(seat)
	left := c.Left[k] - d.Seconds()
	if left < 0 {
		left = 0
	}
	c.Left[k] = left
}
