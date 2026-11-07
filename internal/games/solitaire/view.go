package solitaire

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/cards"
)

// View is what clients draw.
type View struct {
	// Solitaire names the game, and tells the page to draw this table.
	Solitaire   string `json:"solitaire"`
	ImpulseSeat int    `json:"impulse_seat,omitempty"`
	Prompt      string `json:"prompt"`
	// Impulse is what the player is asked to choose.
	Impulse string `json:"impulse,omitempty"`
	// Hint says how to play on this table, under the prompt.
	Hint string `json:"hint"`
	// Table is every zone of cards, as far as the viewer may see it.
	Table []cards.ZoneView `json:"table"`
	// Columns and Cells say how many of each the table has; Suits are the foundations in order.
	Columns int      `json:"columns,omitempty"`
	Cells   int      `json:"cells,omitempty"`
	Suits   []string `json:"suits,omitempty"`
	// Alternate says colours matter: a themed card then shows its classic suit too.
	Alternate bool `json:"alternate,omitempty"`
	// Home is how many cards are home, of Total.
	Home  int `json:"home"`
	Total int `json:"total"`
	// Bowling is the score sheet.
	Bowling *BowlingView `json:"bowling,omitempty"`
	// PanelActions are drawn on the table, not as buttons.
	PanelActions []string `json:"panel_actions"`
}

// BowlingView is the score sheet and where the game has got to.
type BowlingView struct {
	Frame  int          `json:"frame"`
	Ball   int          `json:"ball"`
	Fresh  bool         `json:"fresh"`
	Total  int          `json:"total"`
	Frames []FrameScore `json:"frames"`
}

// FrameScore is one frame of the sheet: its marks, and the running total once known.
type FrameScore struct {
	Marks []string `json:"marks"`
	Total *int     `json:"total,omitempty"`
}

func (e *eng) buildView(viewer int) *View {
	v := &View{
		Solitaire:   e.d.Mode,
		ImpulseSeat: e.d.Turn,
		Table:       e.d.Table.View(e.game.Cards, viewer),
		Total:       len(e.game.Cards.Deck("main")),
		PanelActions: []string{
			"pin-", "ball-",
		},
	}
	v.Bowling = e.bowlingView()
	v.Prompt = fmt.Sprintf("Frame %d, ball %d", e.d.Frame, e.d.Ball)
	v.Hint = e.bowlingHint()
	v.Impulse = v.Hint
	return v
}

func (e *eng) bowlingView() *BowlingView {
	totals := frameScores(e.d.Rolls)
	bv := &BowlingView{Frame: e.d.Frame, Ball: e.d.Ball, Fresh: e.d.Fresh, Total: bowlingTotal(e.d.Rolls)}
	for i, r := range e.d.Rolls {
		bv.Frames = append(bv.Frames, FrameScore{Marks: frameMarks(r), Total: totals[i]})
	}
	return bv
}

func (e *eng) bowlingHint() string {
	var b strings.Builder
	b.WriteString("Click the pins, then a ball.")
	if e.d.Fresh {
		b.WriteString(" The first play cannot reach the back row or take the centre pin alone.")
	} else {
		b.WriteString(" A play must include a pin next to one already down.")
	}
	return b.String()
}
