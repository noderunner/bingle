package session

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// Who is here: the accounts that have said lately that they are looking at
// bingle, so that the page can list the people who are about — to see who is
// around, and to write to one of them (ADR 0073).
//
// A person is an account, signed in (guests are not listed: a direct message
// is an account's). Each is listed under its **address**, a hash of its
// principal that says nothing about it and never changes with a rename, and
// its handle. Knowing an address lets you write to it, never as it.
//
// It lives in memory: who is here is worth nothing after a restart.

// hereTTL is how long a browser counts as here without saying so again. The
// page renews it every minute.
const hereTTL = 150 * time.Second

// maxHere bounds how many browsers are remembered at once.
const maxHere = 500

// ErrTooManyHere is returned when more browsers than that say they are here.
var ErrTooManyHere = errors.New("session: too many browsers are here")

// AddressOf is the public name of a principal for direct messages and
// calls: eight bytes of a hash of its ID, which tell nothing about it and do
// not change when an account changes its handle. "" for no principal.
func AddressOf(principalID string) string {
	if principalID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("bingle-address:" + principalID))
	return hex.EncodeToString(sum[:8])
}

// Person is somebody who is here now.
type Person struct {
	// Address is where a direct message to them is sent.
	Address string `json:"address"`
	// Name is the account's handle, "@ana".
	Name string `json:"name"`
	// Since is when this browser first said it was here, and Seen when it
	// last did (RFC 3339).
	Since string `json:"since"`
	Seen  string `json:"seen"`
}

type hereEntry struct {
	name  string
	since time.Time
	seen  time.Time
	timer *time.Timer
}

// hereTracker remembers the browsers that are about, by address.
type hereTracker struct {
	mu  sync.Mutex
	who map[string]*hereEntry
}

// Here marks the account at addr as present, under name (its handle), and
// returns everybody who is here now (including it). Calling it again renews
// the mark; an empty name clears it.
func (m *Manager) Here(addr, name string) ([]Person, error) {
	name = strings.TrimSpace(name)
	t := &m.here
	t.mu.Lock()
	if t.who == nil {
		t.who = map[string]*hereEntry{}
	}
	if name == "" {
		if e, ok := t.who[addr]; ok {
			e.timer.Stop()
			delete(t.who, addr)
		}
		t.mu.Unlock()
		return m.WhoIsHere(), nil
	}
	now := m.now().UTC()
	e, ok := t.who[addr]
	if !ok {
		if len(t.who) >= maxHere {
			t.mu.Unlock()
			return nil, ErrTooManyHere
		}
		e = &hereEntry{since: now}
		t.who[addr] = e
	} else {
		e.timer.Stop()
	}
	e.name, e.seen = name, now
	e.timer = time.AfterFunc(hereTTL, func() { m.forgetHere(addr) })
	t.mu.Unlock()
	return m.WhoIsHere(), nil
}

func (m *Manager) forgetHere(addr string) {
	t := &m.here
	t.mu.Lock()
	if e, ok := t.who[addr]; ok {
		e.timer.Stop()
		delete(t.who, addr)
	}
	t.mu.Unlock()
}

// WhoIsHere lists the browsers that are about, the longest-standing first.
func (m *Manager) WhoIsHere() []Person {
	type entry struct {
		person Person
		since  time.Time
	}
	t := &m.here
	t.mu.Lock()
	list := make([]entry, 0, len(t.who))
	for addr, e := range t.who {
		list = append(list, entry{
			person: Person{
				Address: addr,
				Name:    e.name,
				Since:   e.since.Format(time.RFC3339),
				Seen:    e.seen.Format(time.RFC3339),
			},
			since: e.since,
		})
	}
	t.mu.Unlock()
	// By when each browser arrived (to the nanosecond: two can arrive within
	// the same second), then by address so that the order never wavers.
	sort.Slice(list, func(i, j int) bool {
		if !list[i].since.Equal(list[j].since) {
			return list[i].since.Before(list[j].since)
		}
		return list[i].person.Address < list[j].person.Address
	})
	out := make([]Person, 0, len(list))
	for _, e := range list {
		out = append(out, e.person)
	}
	return out
}

// NameHere is the name the browser at this address goes by, or "" when it is
// not here (a direct message needs it to say who wrote).
func (m *Manager) NameHere(address string) string {
	t := &m.here
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.who[address]; ok {
		return e.name
	}
	return ""
}
