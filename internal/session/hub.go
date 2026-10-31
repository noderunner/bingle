package session

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is the server->client WebSocket message envelope, per docs/history/plan.md's
// "WebSocket message envelope" section. Type distinguishes what kind of
// notification this is ("joined" | "state" | "move" | "turn" | "error");
// Data carries whatever payload matches that Type.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// moveEventData is the wire DTO MakeMove publishes as a "move" Event's Data,
// (docs/history/plan.md section 6.9). It mirrors
// REST's POST .../moves response shape rather than the raw (untagged)
// MoveOutcome: BoardState is json.RawMessage so it's sent as a nested JSON
// object, not double-encoded as an escaped string, and TurnSeat is nullable
// (nil when the move ended the game), matching the convention REST's
// handleMakeMove already uses for the same reason.
type moveEventData struct {
	Seq        int      `json:"seq"`
	Seat       int      `json:"seat"`
	Color      string   `json:"color"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	Captured   *string  `json:"captured"`
	Action     string   `json:"action,omitempty"`
	Piece      string   `json:"piece,omitempty"`
	Count      int      `json:"count,omitempty"`
	Path       []string `json:"path,omitempty"`
	Drops      []int    `json:"drops,omitempty"`
	Aim        string   `json:"aim,omitempty"`
	Notation   string   `json:"notation,omitempty"`
	TurnSeat   *int     `json:"turn_seat"`
	GameEnded  bool     `json:"game_ended"`
	WinnerSeat *int     `json:"winner_seat"`
	EndReason  string   `json:"end_reason,omitempty"`
	EndDetail  string   `json:"end_detail,omitempty"`
	Status     string   `json:"status"`
	// BoardState is the v1 flat top-of-stack view; State is the full
	// position document (stacks, reserves, ply).
	BoardState json.RawMessage `json:"board_state"`
	State      json.RawMessage `json:"state"`

	// TurnDeadline is when the next player runs out of time (RFC 3339), and
	// ServerTime the server's clock, for timed games.
	TurnDeadline string `json:"turn_deadline,omitempty"`
	ServerTime   string `json:"server_time,omitempty"`
	// Clock is the game clock after the move, for a game played with one.
	Clock *ClockSnapshot `json:"clock,omitempty"`

	// Auto: the server played the move for a person (a reaction they chose
	// to pass automatically). AutoPlay: a tactical bot played it for a person
	// whose Auto-Play Timeout ran out.
	Auto     bool `json:"auto,omitempty"`
	AutoPlay bool `json:"auto_play,omitempty"`
	// Locked: the move rolled dice (or drew a hidden token): it cannot be
	// taken back.
	Locked bool `json:"locked,omitempty"`
}

// stateEventData is the wire DTO of a "state" Event: a full snapshot sent
// when something other than a move changed the game — a player took a seat
// ("join"), a move was taken back ("undo") or a player resigned ("resign").
type stateEventData struct {
	Reason     string          `json:"reason"`
	Removed    []int           `json:"removed,omitempty"` // undo: sequence numbers taken back
	Status     string          `json:"status"`
	TurnSeat   *int            `json:"turn_seat"`
	MoveSeq    int             `json:"move_seq"`
	WinnerSeat *int            `json:"winner_seat"`
	EndReason  string          `json:"end_reason,omitempty"`
	EndDetail  string          `json:"end_detail,omitempty"`
	Seats      []seatEventData `json:"seats"`
	BoardState json.RawMessage `json:"board_state"`
	State      json.RawMessage `json:"state"`

	// Lobby: the game waits for its host to start it; HostSeat is the host's
	// seat.
	Lobby    bool `json:"lobby,omitempty"`
	HostSeat int  `json:"host_seat,omitempty"`

	MoveTimeLimit int            `json:"move_time_seconds,omitempty"`
	TurnDeadline  string         `json:"turn_deadline,omitempty"`
	ServerTime    string         `json:"server_time,omitempty"`
	Clock         *ClockSnapshot `json:"clock,omitempty"`

	// DrawOfferSeat is the seat currently offering a draw (null if none).
	DrawOfferSeat *int `json:"draw_offer_seat"`

	// TakebackSeat is the seat asking to take back its last move (null if
	// none) and TakebackToSeq how many moves would remain.
	TakebackSeat  *int `json:"takeback_seat"`
	TakebackToSeq int  `json:"takeback_to_seq,omitempty"`
}

// seatEventData is one seat of a stateEventData.
type seatEventData struct {
	Seat   int    `json:"seat"`
	Color  string `json:"color"`
	Label  string `json:"label,omitempty"`
	Name   string `json:"name"`
	Bot    string `json:"bot,omitempty"`
	Handle string `json:"handle,omitempty"`
	Guest  bool   `json:"guest,omitempty"`
}

// stateEventFrom builds a "state" event payload from a snapshot.
func stateEventFrom(snap StateSnapshot, reason string, removed []int) stateEventData {
	seats := make([]seatEventData, len(snap.Seats))
	for i, s := range snap.Seats {
		seats[i] = seatEventData{Seat: s.Seat, Color: s.Color, Label: s.Label, Name: s.Name, Bot: s.Bot, Handle: s.Handle, Guest: s.Guest}
	}
	ev := stateEventData{
		Reason:        reason,
		Removed:       removed,
		Status:        snap.Status,
		TurnSeat:      snap.TurnSeat,
		MoveSeq:       snap.MoveSeq,
		WinnerSeat:    snap.WinnerSeat,
		EndReason:     snap.EndReason,
		EndDetail:     snap.EndDetail,
		Seats:         seats,
		Lobby:         snap.Lobby,
		HostSeat:      snap.HostSeat,
		BoardState:    json.RawMessage(snap.BoardState),
		State:         json.RawMessage(snap.State),
		MoveTimeLimit: snap.MoveTimeLimit,
		Clock:         snap.Clock,
		ServerTime:    snap.ServerTime.Format(time.RFC3339),
		DrawOfferSeat: snap.DrawOfferSeat,

		TakebackSeat:  snap.TakebackSeat,
		TakebackToSeq: snap.TakebackToSeq,
	}
	if snap.TurnDeadline != nil {
		ev.TurnDeadline = snap.TurnDeadline.Format(time.RFC3339)
	}
	return ev
}

// hub is an in-process, per-room publish-subscribe broadcaster. It holds no
// state that needs to survive a restart (unlike everything in Store) — it's
// purely a live-notification side channel; a client that misses an event can
// always re-fetch current state via GetState.
type hub struct {
	mu   sync.Mutex
	subs map[string]map[chan Event]struct{} // roomCode -> set of subscriber channels
	// tap, when set, sees every published event (after the subscribers). It
	// must return quickly: it runs on the publisher's goroutine.
	tap func(roomCode string, ev Event)
}

// hubSubscriberBuffer bounds how many undelivered events a subscriber's
// channel can hold before publish starts dropping events for it, so a slow
// consumer can never block the whole broadcast.
const hubSubscriberBuffer = 16

// newHub returns a ready-to-use *hub.
func newHub() *hub {
	return &hub{subs: make(map[string]map[chan Event]struct{})}
}

// subscribe registers a new buffered channel for roomCode and returns it
// (receive-only) plus an unsubscribe function that removes and closes it.
// unsubscribe is safe to call more than once — only the first call has any
// effect.
func (h *hub) subscribe(roomCode string) (events <-chan Event, unsubscribe func()) {
	ch := make(chan Event, hubSubscriberBuffer)

	h.mu.Lock()
	room, ok := h.subs[roomCode]
	if !ok {
		room = make(map[chan Event]struct{})
		h.subs[roomCode] = room
	}
	room[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe = func() {
		once.Do(func() {
			h.mu.Lock()
			if room, ok := h.subs[roomCode]; ok {
				delete(room, ch)
				if len(room) == 0 {
					delete(h.subs, roomCode)
				}
			}
			h.mu.Unlock()
			close(ch)
		})
	}

	return ch, unsubscribe
}

// publish sends ev to every current subscriber of roomCode. Each send is
// non-blocking: a subscriber whose buffer is full simply misses ev, rather
// than blocking the broadcast for every other subscriber.
func (h *hub) publish(roomCode string, ev Event) {
	h.mu.Lock()
	for ch := range h.subs[roomCode] {
		select {
		case ch <- ev:
		default:
		}
	}
	tap := h.tap
	h.mu.Unlock()
	if tap != nil {
		tap(roomCode, ev)
	}
}

// setTap installs the function that sees every event.
func (h *hub) setTap(f func(roomCode string, ev Event)) {
	h.mu.Lock()
	h.tap = f
	h.mu.Unlock()
}

// count returns how many subscribers are listening to roomCode.
func (h *hub) count(roomCode string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[roomCode])
}
