package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Direct messages: two accounts that are here (see here.go) write to each
// other, and may put a voice call up between them (ADR 0073).
//
// A message is addressed to the **address** of an account and sent by a
// signed-in one, so its sender is the account it says — under its handle —
// and anybody who is here can be written to.
//
// Nothing is stored: the messages live in memory, the last few hundred per
// pair, and a correspondence is forgotten a couple of hours after its last
// message (and at a restart). It is a way to say "fancy a game?", not a mailbox.
//
// Delivery is the hub: a browser listens on a channel of its own (dmRoom) over
// /ws-dm, and messages, the ringing of a call and the small WebRTC messages of
// one all travel on it — REST in, WebSocket out, as a game's voice chat does.

const (
	// maxDMRunes bounds one message.
	maxDMRunes = 500
	// maxDMThread is how many messages a pair keeps, and maxDMThreads how
	// many pairs are remembered at once.
	maxDMThread  = 200
	maxDMThreads = 2000
	// dmTTL is how long after its last message a correspondence is kept.
	dmTTL = 2 * time.Hour
)

var (
	// ErrNotHere is returned when a browser writes (or calls) without having
	// said who it is.
	ErrNotHere = errors.New("session: say that you are here before writing to somebody")
	// ErrNoSuchPerson is returned when the person written to is not here.
	ErrNoSuchPerson = errors.New("session: that person is not here")
	// ErrNotInCall is returned for a message about a call that is not up.
	ErrNotInCall = errors.New("session: there is no call with that person")
)

// DirectMessage is one message from one browser to another.
type DirectMessage struct {
	ID int64 `json:"id"`
	// From and To are addresses; Name is what the sender went by.
	From string `json:"from"`
	Name string `json:"name"`
	To   string `json:"to"`
	Text string `json:"text"`
	At   string `json:"at"`
}

// DirectThread is one correspondence, as the list of them shows it.
type DirectThread struct {
	// With is the other person's address, and Name what they go by (empty
	// once they have gone).
	With   string `json:"with"`
	Name   string `json:"name,omitempty"`
	LastID int64  `json:"last_id"`
	LastAt string `json:"last_at"`
	// Text is the last thing said, and Count how many messages are kept.
	Text  string `json:"text"`
	Count int    `json:"count"`
}

// DirectSignal is the wire form of a "dm-rtc" event: one browser's WebRTC
// message for another.
type DirectSignal struct {
	From string          `json:"from"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// DirectCall is the wire form of a "dm-call" event: somebody is calling, or a
// call has ended.
type DirectCall struct {
	// Kind is "ring" (this person wants to talk, or has answered) or "end".
	Kind string `json:"kind"`
	From string `json:"from"`
	Name string `json:"name,omitempty"`
	// Dropped says the call ended because the person joined another one.
	Dropped bool `json:"dropped,omitempty"`
}

// dmRoom is the hub channel a browser hears its own messages on.
func dmRoom(address string) string { return "dm:" + address }

// dmKey names the correspondence between two addresses, whichever way round.
func dmKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

type dmThread struct {
	msgs []DirectMessage
	last time.Time
}

// dmBox holds every correspondence in memory.
type dmBox struct {
	mu      sync.Mutex
	threads map[string]*dmThread
	nextID  int64
}

// add appends a message and returns it, with its number.
func (b *dmBox) add(msg DirectMessage, now time.Time) DirectMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.threads == nil {
		b.threads = map[string]*dmThread{}
	}
	b.sweep(now)
	b.nextID++
	msg.ID = b.nextID
	key := dmKey(msg.From, msg.To)
	t := b.threads[key]
	if t == nil {
		t = &dmThread{}
		b.threads[key] = t
	}
	t.msgs = append(t.msgs, msg)
	if len(t.msgs) > maxDMThread {
		t.msgs = t.msgs[len(t.msgs)-maxDMThread:]
	}
	t.last = now
	return msg
}

// sweep forgets the correspondences nobody has added to for a while, and the
// oldest of them when there are too many. The caller holds the lock.
func (b *dmBox) sweep(now time.Time) {
	for key, t := range b.threads {
		if now.Sub(t.last) > dmTTL {
			delete(b.threads, key)
		}
	}
	for len(b.threads) > maxDMThreads {
		oldest, at := "", time.Time{}
		for key, t := range b.threads {
			if oldest == "" || t.last.Before(at) {
				oldest, at = key, t.last
			}
		}
		delete(b.threads, oldest)
	}
}

// thread returns the messages between two addresses after afterID.
func (b *dmBox) thread(a, other string, afterID int64) []DirectMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.threads[dmKey(a, other)]
	if t == nil {
		return nil
	}
	out := make([]DirectMessage, 0, len(t.msgs))
	for _, m := range t.msgs {
		if m.ID > afterID {
			out = append(out, m)
		}
	}
	return out
}

// threadsOf lists the correspondences one address has, the newest first.
func (b *dmBox) threadsOf(addr string) []DirectThread {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []DirectThread
	for key, t := range b.threads {
		one, two, _ := strings.Cut(key, "|")
		with := ""
		switch addr {
		case one:
			with = two
		case two:
			with = one
		default:
			continue
		}
		if len(t.msgs) == 0 {
			continue
		}
		last := t.msgs[len(t.msgs)-1]
		out = append(out, DirectThread{With: with, LastID: last.ID, LastAt: last.At, Text: last.Text, Count: len(t.msgs)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastID > out[j].LastID })
	return out
}

// SendDirect writes text from the account at address from to the address
// to, and delivers it to both of them (so every tab of the sender sees it too).
func (m *Manager) SendDirect(from, to, text string) (DirectMessage, error) {
	name := m.NameHere(from)
	if name == "" {
		return DirectMessage{}, ErrNotHere
	}
	if to == "" || to == from {
		return DirectMessage{}, fmt.Errorf("%w: write to somebody else", ErrNoSuchPerson)
	}
	if m.NameHere(to) == "" {
		return DirectMessage{}, ErrNoSuchPerson
	}
	text = cleanChat(text)
	if text == "" {
		return DirectMessage{}, ErrChatEmpty
	}
	if utf8.RuneCountInString(text) > maxDMRunes {
		return DirectMessage{}, fmt.Errorf("session: a message is at most %d characters", maxDMRunes)
	}
	now := m.now().UTC()
	if !m.chat.allow("dm|"+from, now) {
		return DirectMessage{}, ErrChatTooFast
	}
	msg := m.dms.add(DirectMessage{From: from, Name: name, To: to, Text: text, At: now.Format(time.RFC3339)}, now)
	m.hub.publish(dmRoom(to), Event{Type: "dm", Data: msg})
	m.hub.publish(dmRoom(from), Event{Type: "dm", Data: msg})
	return msg, nil
}

// DirectMessages returns the correspondence between this browser and another,
// after the message afterID (0 for all of it).
func (m *Manager) DirectMessages(addr, with string, afterID int64) []DirectMessage {
	out := m.dms.thread(addr, with, afterID)
	if out == nil {
		out = []DirectMessage{}
	}
	return out
}

// DirectThreads lists the correspondences of this browser, newest first, with
// the names of the people who are still here.
func (m *Manager) DirectThreads(addr string) []DirectThread {
	out := m.dms.threadsOf(addr)
	for i := range out {
		out[i].Name = m.NameHere(out[i].With)
	}
	if out == nil {
		out = []DirectThread{}
	}
	return out
}

// SubscribeDirect returns a channel of the events for one browser — its
// messages, the ringing of a call and the WebRTC messages of one — and a
// function that stops the delivery.
func (m *Manager) SubscribeDirect(address string) (<-chan Event, func()) {
	return m.hub.subscribe(dmRoom(address))
}

// ---- a call between two people ---------------------------------------------

// JoinDirectCall says that this browser wants to talk to the address to (or,
// when they are already calling, that it answers). The other side is told, and
// any other call this browser was in is left: one voice at a time.
func (m *Manager) JoinDirectCall(from, to string) error {
	name := m.NameHere(from)
	if name == "" {
		return ErrNotHere
	}
	if to == "" || to == from {
		return fmt.Errorf("%w: call somebody else", ErrNoSuchPerson)
	}
	if m.NameHere(to) == "" {
		return ErrNoSuchPerson
	}
	m.claimVoice(from, CallWhere{Peer: to})
	m.hub.publish(dmRoom(to), Event{Type: "dm-call", Data: DirectCall{Kind: "ring", From: from, Name: name}})
	return nil
}

// LeaveDirectCall ends the call this browser is in, telling the other side.
func (m *Manager) LeaveDirectCall(from string) {
	where, ok := m.calls.release(from)
	if !ok || where.Peer == "" {
		return
	}
	m.hub.publish(dmRoom(where.Peer), Event{Type: "dm-call", Data: DirectCall{Kind: "end", From: from}})
}

// SignalDirect relays one WebRTC message from this browser to the person it is
// in a call with. The data is passed on untouched and never logged: it carries
// network addresses.
func (m *Manager) SignalDirect(from, to, kind string, data json.RawMessage) error {
	switch kind {
	case "offer", "answer", "ice", "retry":
	default:
		return fmt.Errorf("%w: kind must be offer, answer, ice or retry", ErrBadSignal)
	}
	if len(data) == 0 || len(data) > maxSignalBytes || !json.Valid(data) || string(data) == "null" {
		return fmt.Errorf("%w: data must be JSON of at most %d bytes", ErrBadSignal, maxSignalBytes)
	}
	if where, ok := m.calls.in(from); !ok || where.Peer != to {
		return ErrNotInCall
	}
	if !m.chat.allow("dmrtc|"+from, m.now()) {
		return ErrSignalTooFast
	}
	m.hub.publish(dmRoom(to), Event{Type: "dm-rtc", Data: DirectSignal{From: from, Kind: kind, Data: data}})
	return nil
}

// ---- one voice at a time ----------------------------------------------------

// CallWhere says where a browser's voice is: in the call of a game (Room, Seat
// and the Name that seat is held by) or in a direct call with Peer.
type CallWhere struct {
	Room string
	Seat int
	Name string
	Peer string
}

// callRegistry remembers the one call each principal is in. An anonymous
// client is not tracked, and keeps the old free-for-all.
type callRegistry struct {
	mu sync.Mutex
	at map[string]CallWhere
}

// claim puts addr in where and returns the call it was in before, if any.
func (c *callRegistry) claim(addr string, where CallWhere) (CallWhere, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.at == nil {
		c.at = map[string]CallWhere{}
	}
	old, had := c.at[addr]
	c.at[addr] = where
	return old, had
}

// release takes addr out of whatever call it was in.
func (c *callRegistry) release(addr string) (CallWhere, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	old, had := c.at[addr]
	delete(c.at, addr)
	return old, had
}

// in reports the call addr is in.
func (c *callRegistry) in(addr string) (CallWhere, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, ok := c.at[addr]
	return w, ok
}

// claimVoice puts a browser in one call and takes it out of any other: a person
// can be in one voice chat at a time, and joining a new one leaves the old.
func (m *Manager) claimVoice(addr string, where CallWhere) {
	if addr == "" {
		return
	}
	old, had := m.calls.claim(addr, where)
	if !had || old == where {
		return
	}
	switch {
	case old.Room != "":
		if m.voice.set(old.Room, old.Seat, old.Name, false, nil) {
			m.hub.publish(old.Room, Event{Type: "voice", Data: voiceEventData{Kind: "leave", Seat: old.Seat, Name: old.Name}})
		}
	case old.Peer != "":
		m.hub.publish(dmRoom(old.Peer), Event{Type: "dm-call", Data: DirectCall{Kind: "end", From: addr}})
		m.hub.publish(dmRoom(addr), Event{Type: "dm-call", Data: DirectCall{Kind: "end", From: old.Peer, Dropped: true}})
	}
}

// dropVoiceClaim forgets where a browser's voice was (it has left of its own
// accord).
func (m *Manager) dropVoiceClaim(addr string) {
	if addr != "" {
		m.calls.release(addr)
	}
}
