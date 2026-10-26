package state

import (
	"fmt"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// RenderText draws st as a monospace text board plus a legend, suitable for
// terminals and for handing to a language model:
//
//	    a    b    c    d    e
//	5   .    .    .    .    .
//	4   .   wF   bS    .    .
//	...
//
// A cell shows the top piece as <color initial><piece code>, followed by the
// stack height when more than one piece is stacked. Stacks of two or more
// are spelled out bottom to top beneath the board, and each seat's reserve
// is listed.
func RenderText(game *catalog.CompiledGame, st *BoardState) string {
	if rs, ok := RulesetOf(game); ok {
		if t, ok := rs.(TextRenderer); ok {
			return t.Text(game, st)
		}
	}
	g := game.Board
	var b strings.Builder

	cellText := func(id board.PositionID) string {
		stack := st.Stack(id)
		if len(stack) == 0 {
			return "."
		}
		top := stack[len(stack)-1]
		s := colorInitial(game, top.Color) + pieceCode(game, top.Piece)
		if len(stack) > 1 {
			s += fmt.Sprintf("%d", len(stack))
		}
		return s
	}

	if g.IsGrid() {
		width := 0
		for _, id := range g.Positions() {
			if n := len(cellText(id)); n > width {
				width = n
			}
		}
		if width < 3 {
			width = 3
		}
		algebraic := game.Definition.Board.Generator != nil && game.Definition.Board.Generator.Labels == string(board.LabelAlgebraic)

		// Column header.
		b.WriteString("    ")
		for x := 0; x < g.Width(); x++ {
			label := fmt.Sprintf("%d", x)
			if algebraic {
				label = string(rune('a' + x))
			}
			fmt.Fprintf(&b, " %-*s", width, label)
		}
		b.WriteByte('\n')
		for y := 0; y < g.Height(); y++ {
			rowLabel := fmt.Sprintf("%d", y)
			if algebraic {
				rowLabel = fmt.Sprintf("%d", g.Height()-y)
			}
			fmt.Fprintf(&b, "%-3s ", rowLabel)
			for x := 0; x < g.Width(); x++ {
				id, _ := g.At(board.Coord{X: x, Y: y})
				fmt.Fprintf(&b, " %-*s", width, cellText(id))
			}
			b.WriteByte('\n')
		}
	} else if g.IsHex() {
		// Each row is shifted half a cell to the right of the previous one,
		// like the parallelogram board it is.
		b.WriteString("   ")
		for q := 0; q < g.Width(); q++ {
			fmt.Fprintf(&b, "%-4s", string(rune('a'+q)))
		}
		b.WriteByte('\n')
		for r := 0; r < g.Height(); r++ {
			fmt.Fprintf(&b, "%2d %s", r+1, strings.Repeat("  ", r))
			for q := 0; q < g.Width(); q++ {
				id, ok := g.At(board.Coord{X: q, Y: r})
				if !ok {
					b.WriteString("    ") // outside a hexhex board
					continue
				}
				fmt.Fprintf(&b, "%-4s", cellText(id))
			}
			b.WriteByte('\n')
		}
	} else {
		listed := 0
		for _, id := range g.Positions() {
			if s := cellText(id); s != "." {
				fmt.Fprintf(&b, "%s: %s\n", id, s)
				listed++
			}
		}
		if listed == 0 {
			b.WriteString("(no pieces on the board)\n")
		}
	}

	// Stacks of two or more pieces, spelled out.
	var tall []board.PositionID
	for _, id := range st.Occupied() {
		if st.Height(id) > 1 {
			tall = append(tall, id)
		}
	}
	sort.Slice(tall, func(i, j int) bool { return tall[i] < tall[j] })
	if len(tall) > 0 {
		b.WriteString("Stacks (bottom -> top):\n")
		for _, id := range tall {
			parts := make([]string, 0, st.Height(id))
			for _, o := range st.Stack(id) {
				parts = append(parts, colorInitial(game, o.Color)+pieceCode(game, o.Piece))
			}
			fmt.Fprintf(&b, "  %s: %s\n", id, strings.Join(parts, " "))
		}
	}

	if len(game.Definition.Reserves) > 0 {
		pools := make([]string, 0, len(game.Definition.Reserves))
		for pool := range game.Definition.Reserves {
			pools = append(pools, pool)
		}
		sort.Strings(pools)
		b.WriteString("Reserves:\n")
		for _, seat := range game.Definition.Seats {
			parts := make([]string, 0, len(pools))
			for _, pool := range pools {
				parts = append(parts, fmt.Sprintf("%s %d", pool, st.Reserve(seat.Color, pool)))
			}
			fmt.Fprintf(&b, "  %s (%s): %s\n", seatName(seat.Label, seat.Color), colorInitial(game, seat.Color), strings.Join(parts, ", "))
		}
	}

	// No trailing blanks on any line.
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}

func seatName(label, color string) string {
	if label != "" {
		return label
	}
	return color
}

func colorInitial(game *catalog.CompiledGame, color string) string {
	if color == "" {
		return "?"
	}
	return strings.ToLower(color[:1])
}

func pieceCode(game *catalog.CompiledGame, piece string) string {
	if code := game.Definition.Pieces[piece].Code; code != "" {
		return code
	}
	if piece == "" {
		return "?"
	}
	return strings.ToUpper(piece[:1])
}
