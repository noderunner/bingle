package state

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// TPS (Tak Positional System) is Tak's standard single-line position
// notation, e.g. "x3,12,2S/x,22221,1/x5/x5/x5 1 7":
//
//   - rows are listed top rank first, separated by "/";
//   - a row is comma-separated cells; "x" is one empty square, "xN" a run
//     of N empties;
//   - a stack is its pieces' owner seats bottom to top ("1" = seat 1,
//     "2" = seat 2) with an optional trailing "S" (the top piece is a
//     standing stone) or "C" (the top piece is a capstone);
//   - then the seat to move and the full-move number.
//
// Reserves are not part of TPS; they are derived as "starting supply minus
// pieces of that color on the board".

// FormatTPS renders st, with seat to move, as a TPS string.
func FormatTPS(game *catalog.CompiledGame, st *BoardState, seatToMove int) (string, error) {
	if IsRuleset(game) {
		return "", errNoTPS(game)
	}
	g := game.Board
	if !g.IsGrid() {
		return "", fmt.Errorf("state: TPS needs a grid board")
	}
	pieceByCode := map[string]string{}
	for name, def := range game.Definition.Pieces {
		if def.Code != "" {
			pieceByCode[name] = def.Code
		}
	}
	defaultPiece := ""
	for _, a := range game.Actions {
		if a.Type == catalog.ActionPlace && len(a.Pieces) > 0 {
			defaultPiece = a.Pieces[0]
			break
		}
	}

	rows := make([]string, 0, g.Height())
	for y := 0; y < g.Height(); y++ {
		var cells []string
		empty := 0
		flush := func() {
			switch {
			case empty == 1:
				cells = append(cells, "x")
			case empty > 1:
				cells = append(cells, "x"+strconv.Itoa(empty))
			}
			empty = 0
		}
		for x := 0; x < g.Width(); x++ {
			id, _ := g.At(board.Coord{X: x, Y: y})
			stack := st.Stack(id)
			if len(stack) == 0 {
				empty++
				continue
			}
			flush()
			var b strings.Builder
			for _, o := range stack {
				seat, ok := game.SeatOfColor(o.Color)
				if !ok {
					return "", fmt.Errorf("state: piece color %q has no seat", o.Color)
				}
				b.WriteString(strconv.Itoa(seat))
			}
			top := stack[len(stack)-1]
			if top.Piece != defaultPiece {
				code, ok := pieceByCode[top.Piece]
				if !ok {
					return "", fmt.Errorf("state: piece %q has no notation code", top.Piece)
				}
				b.WriteString(code)
			}
			cells = append(cells, b.String())
		}
		flush()
		rows = append(rows, strings.Join(cells, ","))
	}

	moveNumber := st.Ply()/len(game.Definition.Seats) + 1
	return fmt.Sprintf("%s %d %d", strings.Join(rows, "/"), seatToMove, moveNumber), nil
}

// ParseTPS builds the position described by a TPS string. It returns the
// state (with reserves derived and the ply counter reconstructed), and the
// seat to move.
func ParseTPS(game *catalog.CompiledGame, tps string) (*BoardState, int, error) {
	if IsRuleset(game) {
		return nil, 0, errNoTPS(game)
	}
	g := game.Board
	if !g.IsGrid() {
		return nil, 0, fmt.Errorf("state: TPS needs a grid board")
	}
	fields := strings.Fields(tps)
	if len(fields) != 3 {
		return nil, 0, fmt.Errorf("state: TPS must be \"<rows> <player> <move>\", got %q", tps)
	}
	seat, err := strconv.Atoi(fields[1])
	if err != nil {
		return nil, 0, fmt.Errorf("state: TPS player %q is not a number", fields[1])
	}
	if _, ok := game.Seat(seat); !ok {
		return nil, 0, fmt.Errorf("state: TPS player %d is not a seat of this game", seat)
	}
	moveNumber, err := strconv.Atoi(fields[2])
	if err != nil || moveNumber < 1 {
		return nil, 0, fmt.Errorf("state: TPS move number %q is invalid", fields[2])
	}

	codeToPiece := map[string]string{}
	for name, def := range game.Definition.Pieces {
		if def.Code != "" {
			codeToPiece[strings.ToUpper(def.Code)] = name
		}
	}
	defaultPiece := ""
	for _, a := range game.Actions {
		if a.Type == catalog.ActionPlace && len(a.Pieces) > 0 {
			defaultPiece = a.Pieces[0]
			break
		}
	}

	rows := strings.Split(fields[0], "/")
	if len(rows) != g.Height() {
		return nil, 0, fmt.Errorf("state: TPS has %d rows, board has %d", len(rows), g.Height())
	}

	st := InitialState(game)
	for y, row := range rows {
		x := 0
		for _, cell := range strings.Split(row, ",") {
			if cell == "" {
				return nil, 0, fmt.Errorf("state: TPS row %d has an empty cell", y+1)
			}
			if cell[0] == 'x' {
				n := 1
				if len(cell) > 1 {
					if n, err = strconv.Atoi(cell[1:]); err != nil || n < 1 {
						return nil, 0, fmt.Errorf("state: TPS cell %q is invalid", cell)
					}
				}
				x += n
				continue
			}
			id, ok := g.At(board.Coord{X: x, Y: y})
			if !ok {
				return nil, 0, fmt.Errorf("state: TPS row %d is wider than the board", y+1)
			}
			digits := cell
			topPiece := defaultPiece
			if last := cell[len(cell)-1]; last < '0' || last > '9' {
				p, ok := codeToPiece[strings.ToUpper(string(last))]
				if !ok {
					return nil, 0, fmt.Errorf("state: TPS cell %q has unknown piece code %q", cell, string(last))
				}
				topPiece = p
				digits = cell[:len(cell)-1]
			}
			if digits == "" {
				return nil, 0, fmt.Errorf("state: TPS cell %q has no pieces", cell)
			}
			for i, d := range digits {
				seatNum := int(d - '0')
				def, ok := game.Seat(seatNum)
				if !ok {
					return nil, 0, fmt.Errorf("state: TPS cell %q names unknown seat %q", cell, string(d))
				}
				piece := defaultPiece
				if i == len(digits)-1 {
					piece = topPiece
				}
				st.Push(id, Occupant{Piece: piece, Color: def.Color})
			}
			x++
		}
		if x != g.Width() {
			return nil, 0, fmt.Errorf("state: TPS row %d covers %d squares, board is %d wide", y+1, x, g.Width())
		}
	}

	// Derive reserves: starting supply minus what is on the board.
	for _, id := range st.Occupied() {
		for _, o := range st.Stack(id) {
			pool := game.Pool(o.Piece)
			st.SetReserve(o.Color, pool, st.Reserve(o.Color, pool)-1)
		}
	}

	// Reconstruct the ply counter from the move number and side to move.
	seatIndex := 0
	for i, s := range game.Definition.TurnOrder.Seats {
		if s == seat {
			seatIndex = i
		}
	}
	st.SetPly((moveNumber-1)*len(game.Definition.Seats) + seatIndex)

	return st, seat, nil
}
