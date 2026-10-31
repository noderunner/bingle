package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/store"
)

// maxChatRunes bounds one chat message.
const maxChatRunes = 500

// chatBurst and chatWindow bound how fast one name may talk: a flood of
// messages is refused rather than delivered.
const (
	chatBurst  = 12
	chatWindow = 10 * time.Second
)

// ErrChatEmpty is returned for a message with nothing in it.
var ErrChatEmpty = errors.New("session: the message is empty")

// ErrChatTooFast is returned when a name sends messages faster than allowed.
var ErrChatTooFast = errors.New("session: you are sending messages too fast")

// ChatLine is one message of a game's chat.
type ChatLine struct {
	ID    int64  `json:"id"`
	Seat  int    `json:"seat"` // 0 for a spectator
	Color string `json:"color,omitempty"`
	Name  string `json:"name"`
	Text  string `json:"text"`
	At    string `json:"at"`
	// Handle is the account that said it, and Guest says a guest did.
	Handle string `json:"handle,omitempty"`
	Guest  bool   `json:"guest,omitempty"`

	principal string
}

// chatEventData is the wire form of a "chat" event.
type chatEventData = ChatLine

// chatLimiter remembers when each name last spoke.
type chatLimiter struct {
	mu   sync.Mutex
	seen map[string][]time.Time
}

func (c *chatLimiter) allow(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen == nil {
		c.seen = map[string][]time.Time{}
	}
	recent := c.seen[key][:0]
	for _, t := range c.seen[key] {
		if now.Sub(t) < chatWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= chatBurst {
		c.seen[key] = recent
		return false
	}
	c.seen[key] = append(recent, now)
	return true
}

// cleanChat trims a message and replaces line breaks and control characters.
func cleanChat(text string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(text) {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// SendChat adds a message from the person called name to the game's chat and
// tells everyone watching. A seated player's messages carry their seat;
// anybody else speaks as a spectator.
func (m *Manager) SendChat(ctx context.Context, roomCode, name, text string) (ChatLine, error) {
	roomCode = canonRoom(roomCode)
	text = cleanChat(text)
	if text == "" {
		return ChatLine{}, ErrChatEmpty
	}
	if utf8.RuneCountInString(text) > maxChatRunes {
		return ChatLine{}, fmt.Errorf("session: a message is at most %d characters", maxChatRunes)
	}
	game, err := m.store.GetGame(ctx, roomCode)
	if err != nil {
		return ChatLine{}, err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return ChatLine{}, err
	}
	seat, color, speaker := 0, "", strings.TrimSpace(name)
	for _, p := range players {
		if strings.EqualFold(p.Name, speaker) {
			if p.Bot != "" {
				return ChatLine{}, ErrNameIsABot
			}
			// Speaking as a player is for that player alone.
			if err := m.mayActFor(ctx, &p); err != nil {
				return ChatLine{}, err
			}
			seat, color, speaker = p.Seat, p.Color, p.Name
		}
	}
	// Speaking as somebody's handle is for that account alone.
	if seat == 0 {
		if err := m.mayUseName(ctx, speaker); err != nil {
			return ChatLine{}, err
		}
	}
	if !m.chat.allow(roomCode+"|"+strings.ToLower(speaker), m.now()) {
		return ChatLine{}, ErrChatTooFast
	}
	msg, err := m.store.AddChatMessage(ctx, game.ID, seat, speaker, auth.IDFrom(ctx), text)
	if err != nil {
		return ChatLine{}, err
	}
	line := chatLineOf(msg, color)
	lines := []ChatLine{line}
	m.identifyLines(ctx, lines)
	line = lines[0]
	m.hub.publish(roomCode, Event{Type: "chat", Data: chatEventData(line)})
	return line, nil
}

// ChatLog returns the game's chat: the latest limit messages, or those after
// message afterID.
func (m *Manager) ChatLog(ctx context.Context, roomCode string, afterID int64, limit int) ([]ChatLine, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.store.GetGame(ctx, roomCode)
	if err != nil {
		return nil, err
	}
	msgs, err := m.store.ListChat(ctx, game.ID, afterID, limit)
	if err != nil {
		return nil, err
	}
	colors := map[int]string{}
	if players, err := m.store.ListPlayers(ctx, game.ID); err == nil {
		for _, p := range players {
			colors[p.Seat] = p.Color
		}
	}
	out := make([]ChatLine, len(msgs))
	for i, mm := range msgs {
		out[i] = chatLineOf(mm, colors[mm.Seat])
	}
	m.identifyLines(ctx, out)
	return out, nil
}

func chatLineOf(m store.ChatMessage, color string) ChatLine {
	if m.Seat == 0 {
		color = ""
	}
	return ChatLine{ID: m.ID, Seat: m.Seat, Color: color, Name: m.Name, Text: m.Body, At: m.CreatedAt, principal: m.PrincipalID}
}

// identifyLines says who said each line: an account's handle, or a guest.
func (m *Manager) identifyLines(ctx context.Context, lines []ChatLine) {
	ids := make([]string, len(lines))
	for i, l := range lines {
		ids[i] = l.principal
	}
	who, err := m.store.Identities(ctx, ids)
	if err != nil {
		return
	}
	for i := range lines {
		if id, ok := who[lines[i].principal]; ok {
			lines[i].Handle, lines[i].Guest = id.Handle, id.Guest
		}
	}
}

// GlobalChatRoom is the hub room the global chat publishes on. No game can
// have this code: room codes are upper-case letters and digits.
const GlobalChatRoom = "~global"

// GlobalChatLine is one message of the global chat room.
type GlobalChatLine struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Text string `json:"text"`
	At   string `json:"at"`
}

// SendGlobalChat adds a message from the person called name to the global chat
// room and tells everyone who is listening.
func (m *Manager) SendGlobalChat(ctx context.Context, name, text string) (GlobalChatLine, error) {
	text = cleanChat(text)
	if text == "" {
		return GlobalChatLine{}, ErrChatEmpty
	}
	if utf8.RuneCountInString(text) > maxChatRunes {
		return GlobalChatLine{}, fmt.Errorf("session: a message is at most %d characters", maxChatRunes)
	}
	speaker := strings.TrimSpace(name)
	if err := m.mayUseName(ctx, speaker); err != nil {
		return GlobalChatLine{}, err
	}
	if !m.chat.allow(GlobalChatRoom+"|"+strings.ToLower(speaker), m.now()) {
		return GlobalChatLine{}, ErrChatTooFast
	}
	msg, err := m.store.AddGlobalChat(ctx, speaker, auth.IDFrom(ctx), text)
	if err != nil {
		return GlobalChatLine{}, err
	}
	line := GlobalChatLine{ID: msg.ID, Name: msg.Name, Text: msg.Body, At: msg.CreatedAt}
	m.hub.publish(GlobalChatRoom, Event{Type: "chat", Data: line})
	return line, nil
}

// GlobalChatLog returns the global chat: the latest limit messages, or those
// after message afterID.
func (m *Manager) GlobalChatLog(ctx context.Context, afterID int64, limit int) ([]GlobalChatLine, error) {
	msgs, err := m.store.ListGlobalChat(ctx, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]GlobalChatLine, len(msgs))
	for i, mm := range msgs {
		out[i] = GlobalChatLine{ID: mm.ID, Name: mm.Name, Text: mm.Body, At: mm.CreatedAt}
	}
	return out, nil
}

// SubscribeGlobalChat returns a channel of the global chat's events and a
// function that stops the delivery.
func (m *Manager) SubscribeGlobalChat() (<-chan Event, func()) {
	return m.hub.subscribe(GlobalChatRoom)
}
