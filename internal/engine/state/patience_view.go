package state

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// The view a game of patience sends its page (ADR 0122): the shape Solitaire's
// ruleset sent, so static/js/solitaire.js draws a declared game as it did.

// patienceMode is how the page lays the table out: a stock drawn to a waste,
// a stock dealt in rows, or no stock.
func patienceMode(p *catalog.Patience) string {
	switch {
	case p.Draw > 0:
		return "klondike"
	case p.Row > 0:
		return "spider"
	}
	return "freecell"
}

// patienceView is st as viewer sees it, ready to draw.
func patienceView(game *catalog.CompiledGame, st *BoardState, viewer int) map[string]any {
	p := game.Patience
	e := patienceOf(game, st)
	v := map[string]any{
		"solitaire":     patienceMode(p),
		"table":         nil,
		"columns":       p.Columns,
		"cells":         p.Cells,
		"alternate":     p.Suit == "alternate",
		"home":          e.homeCount(),
		"total":         p.Total,
		"panel_actions": []string{"from-", "to-", patienceDraw, patienceRecycle, patienceRow},
	}
	if st.table != nil {
		v["table"] = st.table.View(game.Cards, viewer)
	}
	if p.Foundations > 0 {
		suits := make([]string, p.Foundations)
		for k := range suits {
			suits[k] = e.foundationSuit(k + 1)
		}
		v["suits"] = suits
	}
	if out := patienceOutcome(game, st); !out.Ended {
		v["impulse_seat"] = game.Definition.Seats[0].Seat
	}
	v["prompt"] = fmt.Sprintf("%d of %d cards home", e.homeCount(), p.Total)
	v["hint"] = "Click a card, then where it should go, or drag it."
	v["impulse"] = v["hint"]
	return v
}

// withPatienceView gives a game of patience's position its view under
// ext.view, worked out for viewer and never stored.
func withPatienceView(game *catalog.CompiledGame, st *BoardState, viewer int) *BoardState {
	if st == nil || st.ext != nil {
		return st
	}
	raw, err := json.Marshal(map[string]any{"view": patienceView(game, st, viewer)})
	if err != nil {
		return st
	}
	out := st.Clone()
	out.ext = json.RawMessage(raw)
	return out
}

// The sample of a game of patience: its own hints played from a fixed deal,
// far enough to have turned cards over and sent some home.
const (
	patienceSampleSeed  = 21
	patienceSamplePlies = 45
)

var patienceSamples sync.Map // game id → view

func patienceSample(game *catalog.CompiledGame) any {
	if v, ok := patienceSamples.Load(game.Definition.ID); ok {
		return v
	}
	seat := game.Definition.Seats[0].Seat
	st := declaredStart(game, patienceSampleSeed)
	for ply := 0; ply < patienceSamplePlies; ply++ {
		moves := GuidedMoves(game, st, seat, LegalSelections(game, st, seat))
		if len(moves) == 0 {
			break
		}
		res, err := Apply(game, st, seat, moves[0])
		if err != nil || res.GameEnded {
			break
		}
		st = res.NewState
	}
	v := patienceView(game, st, 0)
	delete(v, "impulse_seat")
	v["prompt"], v["hint"], v["impulse"] = "", "", ""
	patienceSamples.Store(game.Definition.ID, v)
	return v
}
