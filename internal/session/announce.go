package session

import (
	"context"
	"fmt"

	"github.com/noderunner/bingle/internal/store"
)

// An award is announced (ADR 0130) to its holder, by a "badge" event to the
// account's pages, which show it and may raise a browser notification, never
// an email; and to the room, by a line from the server in the game's chat. An
// award that waits for its game's end is told to its holder at once and to
// the room when it is shown. An announcement that fails is logged and lost:
// the award itself was already written.

// BadgeEventData is the wire form of a "badge" event.
type BadgeEventData struct {
	Badge       string `json:"badge"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Room        string `json:"room,omitempty"`
	// Game is the name of the game it was earned in.
	Game string `json:"game,omitempty"`
	// Hidden says the others see it only when the game ends.
	Hidden bool `json:"hidden,omitempty"`
}

// announce tells holders and rooms of what a write of awards changed.
func (m *Manager) announce(ctx context.Context, a store.Awarded) {
	for _, aw := range a.New {
		m.tellHolder(ctx, aw)
		if aw.Visible {
			m.tellRoom(ctx, aw)
		}
	}
	for _, aw := range a.Shown {
		m.tellRoom(ctx, aw)
	}
}

func (m *Manager) tellHolder(ctx context.Context, aw store.Award) {
	principals, err := m.store.PrincipalsOfAccount(ctx, aw.AccountID)
	if err != nil {
		m.logger().Error("announcing a badge to its holder", "badge", aw.Badge, "err", err)
		return
	}
	ev := Event{Type: "badge", Data: BadgeEventData{
		Badge: aw.Badge, Name: aw.Name, Description: aw.Description,
		Room: aw.RoomCode, Game: aw.Detail, Hidden: !aw.Visible,
	}}
	for _, p := range principals {
		m.hub.publish(dmRoom(AddressOf(p)), ev)
	}
}

func (m *Manager) tellRoom(ctx context.Context, aw store.Award) {
	fail := func(err error) {
		m.logger().Error("announcing a badge in its game's chat", "badge", aw.Badge, "game_id", aw.GameID, "err", err)
	}
	if aw.GameID == 0 || aw.RoomCode == "" {
		return
	}
	account, err := m.store.AccountByID(ctx, aw.AccountID)
	if err != nil {
		fail(err)
		return
	}
	text := fmt.Sprintf("@%s earned the badge %s: %s", account.Handle, aw.Name, aw.Description)
	msg, err := m.store.AddChatMessage(ctx, aw.GameID, serverSeat, m.branded("Bingle"), "", text)
	if err != nil {
		// Retention may have deleted the game meanwhile.
		fail(err)
		return
	}
	m.hub.publish(canonRoom(aw.RoomCode), Event{Type: "chat", Data: chatEventData(chatLineOf(msg, ""))})
}
