package session

import (
	"context"

	"github.com/noderunner/bingle/internal/rating"
)

// BotLadderGame is a game family on the bot ladder (ADR 0160): its bots, the
// highest rating first. The bot ladder is kept apart from people's ratings,
// and nothing here reads them.
type BotLadderGame struct {
	Family string           `json:"family"`
	Name   string           `json:"name"`
	Bots   []BotLadderEntry `json:"bots"`
}

// BotLadderEntry is a bot's line on the bot ladder of a family.
type BotLadderEntry struct {
	Rank        int    `json:"rank"`
	Bot         string `json:"bot"`
	Label       string `json:"label"`
	Rating      int    `json:"rating"`
	Provisional bool   `json:"provisional,omitempty"`
	Games       int    `json:"games"`
	Won         int    `json:"won"`
	Drawn       int    `json:"drawn"`
	Lost        int    `json:"lost"`
}

// BotLadder lists the bot ladder of every family the bots have played, by the
// game's name. A prototype's is left out, as its people's ratings are.
func (m *Manager) BotLadder(ctx context.Context) ([]BotLadderGame, error) {
	rows, err := m.store.BotRatings(ctx)
	if err != nil {
		return nil, err
	}
	out := []BotLadderGame{}
	for _, r := range rows {
		if m.isPrototype(r.Family) {
			continue
		}
		if n := len(out); n == 0 || out[n-1].Family != r.Family {
			out = append(out, BotLadderGame{Family: r.Family, Name: r.FamilyName})
		}
		g := &out[len(out)-1]
		label := r.Bot
		if b, ok := m.bots.Get(r.Bot); ok {
			label = b.Label()
		}
		p := rating.Player{Rating: r.Rating, Deviation: r.Deviation, Volatility: r.Volatility}
		g.Bots = append(g.Bots, BotLadderEntry{Rank: len(g.Bots) + 1, Bot: r.Bot, Label: label, Rating: rounded(r.Rating),
			Provisional: p.Provisional(), Games: r.Games, Won: r.Won, Drawn: r.Drawn, Lost: r.Lost})
	}
	return out, nil
}
