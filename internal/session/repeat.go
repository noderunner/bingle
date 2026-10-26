package session

import (
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// repeatWindow is how many recent arrangements of a seat's pieces a bot
// remembers; a decision that would bring one of them back is avoided.
const repeatWindow = 12

// shuffleTracker remembers, per room and seat, where a bot's pieces stood at
// its recent decisions. Bots search positions, not histories, so left alone
// a deterministic bot can move a unit to and fro (or take and give up a Gate)
// for ever; the tracker takes such moves out of its choices.
type shuffleTracker struct {
	mu   sync.Mutex
	seen map[string]map[int][]string
}

// note records key as seat's current arrangement.
func (t *shuffleTracker) note(room string, seat int, key string) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.seen == nil {
		t.seen = map[string]map[int][]string{}
	}
	if t.seen[room] == nil {
		t.seen[room] = map[int][]string{}
	}
	list := t.seen[room][seat]
	if len(list) == 0 || list[len(list)-1] != key {
		list = append(list, key)
		if len(list) > repeatWindow {
			list = list[len(list)-repeatWindow:]
		}
		t.seen[room][seat] = list
	}
	return append([]string(nil), list...)
}

// forget drops a finished room.
func (t *shuffleTracker) forget(room string) {
	t.mu.Lock()
	delete(t.seen, room)
	t.mu.Unlock()
}

// avoidShuffles removes the moves that only rearrange seat's pieces into an
// arrangement it recently had, as long as other moves remain.
func (m *Manager) avoidShuffles(room string, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	guard, ok := state.RepetitionGuardOf(game)
	if !ok {
		return legal
	}
	recent := m.shuffles.note(room, seat, guard.RepeatKey(game, st, seat))
	kept := make([]state.Move, 0, len(legal))
	for _, mv := range legal {
		if guard.Repositions(mv) {
			res, err := state.Apply(game, st, seat, mv)
			if err == nil && containsKey(recent, guard.RepeatKey(game, res.NewState, seat)) {
				continue
			}
		}
		kept = append(kept, mv)
	}
	if len(kept) == 0 {
		return legal
	}
	return kept
}

// botChoices narrows a bot's legal moves to the ones it should consider: the
// moves that only shuffle its pieces back into a recent arrangement go first,
// then the game's own strategy guide (see state.BotGuide) picks the sensible
// ones.
func (m *Manager) botChoices(room string, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	legal = m.avoidShuffles(room, game, st, seat, legal)
	return state.GuidedMoves(game, st, seat, legal)
}

func containsKey(list []string, key string) bool {
	for _, k := range list {
		if k == key {
			return true
		}
	}
	return false
}
