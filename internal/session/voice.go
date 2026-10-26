package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Voice chat is peer to peer (WebRTC): the players' browsers carry the audio
// between them. The server only keeps the list of who is in the call and
// relays the small JSON messages (session descriptions and ICE candidates)
// with which two browsers find each other. It never looks inside them, and it
// never sees any audio.

// voiceTTL is how long a seat stays in the call without a sign of life. The
// page renews it every few seconds, so a closed tab or a lost connection
// leaves the call by itself.
const voiceTTL = 45 * time.Second

// maxSignalBytes bounds one relayed message (a session description is a few
// kilobytes).
const maxSignalBytes = 16 << 10

// A seat may relay this many messages per window: a call setup with several
// peers sends dozens of ICE candidates in a burst.
const (
	signalBurst  = 300
	signalWindow = 10 * time.Second
)

// ErrVoiceNotJoined is returned when a seat that is not in the call tries to
// signal.
var ErrVoiceNotJoined = errors.New("session: join the voice chat before signalling")

// ErrVoicePeerGone is returned when the seat a message is for is not in the call.
var ErrVoicePeerGone = errors.New("session: that player is not in the voice chat")

// ErrSignalTooFast is returned when a seat relays messages faster than allowed.
var ErrSignalTooFast = errors.New("session: too many voice messages")

// ErrBadSignal is returned for a relayed message that is not allowed.
var ErrBadSignal = errors.New("session: bad voice message")

// VoicePeer is a seat that is in the call.
type VoicePeer struct {
	Seat int    `json:"seat"`
	Name string `json:"name"`
}

// voiceEventData is the wire form of a "voice" event, sent to the whole room
// when somebody joins or leaves the call.
type voiceEventData struct {
	Kind string `json:"kind"` // "join" or "leave"
	Seat int    `json:"seat"`
	Name string `json:"name"`
}

// RTCSignal is the wire form of an "rtc" event: a message from one seat to
// another, delivered only to the connection of the seat it is for.
type RTCSignal struct {
	From int             `json:"from"`
	Kind string          `json:"kind"` // "offer", "answer", "ice" or "retry" (please call me again)
	Data json.RawMessage `json:"data"`
}

type voiceEntry struct {
	name  string
	timer *time.Timer
}

// voiceTracker remembers, per room, which seats are in the call. In memory
// only: a call does not outlive a restart.
type voiceTracker struct {
	mu    sync.Mutex
	rooms map[string]map[int]*voiceEntry
	seen  map[string][]time.Time // rate limiting, by room|seat
}

// set puts a seat in the call (or takes it out) and arms the expiry. It
// reports whether the membership changed.
func (v *voiceTracker) set(room string, seat int, name string, active bool, expire func()) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.rooms == nil {
		v.rooms = map[string]map[int]*voiceEntry{}
	}
	seats := v.rooms[room]
	old, was := seats[seat]
	if was {
		old.timer.Stop()
	}
	if !active {
		if was {
			delete(seats, seat)
			if len(seats) == 0 {
				delete(v.rooms, room)
			}
		}
		return was
	}
	if seats == nil {
		seats = map[int]*voiceEntry{}
		v.rooms[room] = seats
	}
	seats[seat] = &voiceEntry{name: name, timer: time.AfterFunc(voiceTTL, expire)}
	return !was || old.name != name
}

func (v *voiceTracker) list(room string) []VoicePeer {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := []VoicePeer{}
	for seat, e := range v.rooms[room] {
		out = append(out, VoicePeer{Seat: seat, Name: e.name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seat < out[j].Seat })
	return out
}

func (v *voiceTracker) in(room string, seat int) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.rooms[room][seat]
	return ok
}

func (v *voiceTracker) allow(key string, now time.Time) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.seen == nil {
		v.seen = map[string][]time.Time{}
	}
	recent := v.seen[key][:0]
	for _, t := range v.seen[key] {
		if now.Sub(t) < signalWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= signalBurst {
		v.seen[key] = recent
		return false
	}
	v.seen[key] = append(recent, now)
	return true
}

// rtcRoom is the hub room that carries the messages for one seat, so that
// nothing else listening to the game (other seats, spectators, anything that
// follows the game's events) ever receives them.
func rtcRoom(room string, seat int) string {
	return room + "#rtc#" + strconv.Itoa(seat)
}

// humanSeat finds the seat of the person called name in the game.
func (m *Manager) humanSeat(ctx context.Context, roomCode, name string) (int, string, error) {
	game, err := m.store.GetGame(ctx, roomCode)
	if err != nil {
		return 0, "", err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return 0, "", err
	}
	for _, p := range players {
		if p.Bot == "" && strings.EqualFold(p.Name, strings.TrimSpace(name)) {
			return p.Seat, p.Name, nil
		}
	}
	return 0, "", ErrNotSeated
}

// SetVoice puts the seat held by name in the voice chat of the game (or takes
// it out), and tells the room when that changed. Calling it again with
// active=true renews the seat's place; it lapses after voiceTTL without that.
// Only people with a seat can talk: spectators cannot join.
func (m *Manager) SetVoice(ctx context.Context, roomCode, name string, active bool) ([]VoicePeer, error) {
	roomCode = canonRoom(roomCode)
	seat, who, err := m.humanSeat(ctx, roomCode, name)
	if err != nil {
		return nil, err
	}
	changed := m.voice.set(roomCode, seat, who, active, func() {
		if m.voice.set(roomCode, seat, who, false, nil) {
			m.hub.publish(roomCode, Event{Type: "voice", Data: voiceEventData{Kind: "leave", Seat: seat, Name: who}})
		}
	})
	if changed {
		kind := "leave"
		if active {
			kind = "join"
		}
		m.hub.publish(roomCode, Event{Type: "voice", Data: voiceEventData{Kind: kind, Seat: seat, Name: who}})
	}
	return m.voice.list(roomCode), nil
}

// VoicePeers lists the seats that are in the voice chat of the game.
func (m *Manager) VoicePeers(roomCode string) []VoicePeer {
	return m.voice.list(canonRoom(roomCode))
}

// SignalVoice relays a message from the seat held by name to the seat `to`,
// which must be in the call too. kind is "offer", "answer", "ice" or "retry"; data is
// whatever the browsers exchange, passed on untouched (and never logged: it
// carries network addresses).
func (m *Manager) SignalVoice(ctx context.Context, roomCode, name string, to int, kind string, data json.RawMessage) error {
	roomCode = canonRoom(roomCode)
	switch kind {
	case "offer", "answer", "ice", "retry":
	default:
		return fmt.Errorf("%w: kind must be offer, answer, ice or retry", ErrBadSignal)
	}
	if len(data) == 0 || len(data) > maxSignalBytes || !json.Valid(data) || string(data) == "null" {
		return fmt.Errorf("%w: data must be JSON of at most %d bytes", ErrBadSignal, maxSignalBytes)
	}
	from, who, err := m.humanSeat(ctx, roomCode, name)
	if err != nil {
		return err
	}
	if from == to {
		return fmt.Errorf("%w: not to yourself", ErrBadSignal)
	}
	if !m.voice.in(roomCode, from) {
		return ErrVoiceNotJoined
	}
	if !m.voice.in(roomCode, to) {
		return ErrVoicePeerGone
	}
	if !m.voice.allow(roomCode+"|"+strconv.Itoa(from)+"|"+strings.ToLower(who), m.now()) {
		return ErrSignalTooFast
	}
	m.hub.publish(rtcRoom(roomCode, to), Event{Type: "rtc", Data: RTCSignal{From: from, Kind: kind, Data: data}})
	return nil
}

// SubscribeRTC returns the channel of the messages for one seat of a game
// (the "rtc" events), and a function that stops the delivery.
func (m *Manager) SubscribeRTC(roomCode string, seat int) (<-chan Event, func()) {
	return m.hub.subscribe(rtcRoom(canonRoom(roomCode), seat))
}
