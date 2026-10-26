package hive

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation is the Universal Hive Protocol's (the one hivegame.com uses): a
// tile is its colour (w or b), its kind (Q B S G A M L P) and, for kinds that
// come in several, its number: wQ, bA2. A move is the tile and where it ends,
// told by a tile already on the table and which side of it the tile lands on:
//
//	wS1            the first tile of the game
//	bG1 wS1-       bG1 right of wS1      (a symbol after the name: the right side)
//	wA1 \bG1       wA1 above and left of bG1 (a symbol before the name: the left side)
//	wB1 bQ         the Beetle climbs on top of bQ (no symbol)
//
// Symbols: "-" beside, "/" and "\" the diagonal sides. Before the name: "-bQ"
// left, "\bQ" top left, "/bQ" bottom left; after it: "bQ-" right, "bQ/" top
// right, "bQ\" bottom right. A player who cannot move writes pass. A move also
// carries its cells, so older text such as ant@j10 and j10-k10 still reads.

var kindLetter = [numKinds]byte{'Q', 'B', 'S', 'G', 'A', 'M', 'L', 'P'}

// single kinds come once, so their names carry no number.
func isSingle(kind int) bool {
	return kind == kQueen || kind == kMosquito || kind == kLadybug || kind == kPillbug
}

func (e *eng) startCount(kind int) int {
	n := e.game.Definition.Reserves[kindName[kind]]
	if n < 1 {
		n = 1
	}
	return n
}

// tileName writes a tile in the protocol's form.
func tileName(seat, kind, num int) string {
	c := byte('w')
	if seat == 2 {
		c = 'b'
	}
	if isSingle(kind) {
		return fmt.Sprintf("%c%c", c, kindLetter[kind])
	}
	return fmt.Sprintf("%c%c%d", c, kindLetter[kind], num)
}

// relation says where a tile lands on cell dest, by a tile beside it.
func (e *eng) relation(dest int) string {
	name := func(i int) string {
		top := len(e.cells[i]) - 1
		return tileName(int(e.cells[i][top].seat), int(e.cells[i][top].kind), int(e.nums[i][top]))
	}
	if len(e.cells[dest]) > 0 {
		return name(dest) // climbing on top of it
	}
	for d := 0; d < 6; d++ {
		n := e.g.nbr[dest][d]
		if !e.occupied(n) {
			continue
		}
		// dest lies in the opposite direction from n.
		switch (d + 3) % 6 {
		case 0:
			return name(n) + "-"
		case 1:
			return name(n) + "/"
		case 2:
			return `\` + name(n)
		case 3:
			return "-" + name(n)
		case 4:
			return "/" + name(n)
		default:
			return name(n) + `\`
		}
	}
	return ""
}

// annotate writes the protocol's text of mv into mv.Aim.
func (e *eng) annotate(mv *state.Move, seat int) {
	switch mv.Action {
	case actPlace:
		k, _ := kindOf(mv.Piece)
		mv.Aim = board.PositionID(strings.TrimSpace(tileName(seat, k, e.startCount(k)-e.res[seat][k]+1) + " " + e.relation(e.g.index[mv.To])))
	case actMove:
		from := e.g.index[mv.From]
		stack := e.cells[from]
		top := len(stack) - 1
		name := tileName(int(stack[top].seat), int(stack[top].kind), int(e.nums[from][top]))
		nums := e.nums[from]
		e.cells[from], e.nums[from] = stack[:top], nums[:top]
		rel := e.relation(e.g.index[mv.To])
		e.cells[from], e.nums[from] = stack, nums
		mv.Aim = board.PositionID(name + " " + rel)
	}
}

// sameMove reports whether a is the move cand: the same protocol text, or (for
// a move given by its cells) the same tile and cells.
func sameMove(a, cand state.Move) bool {
	if a.Aim != "" && cand.Aim != "" {
		return a.Aim == cand.Aim
	}
	if a.Action != cand.Action || a.From != cand.From || a.To != cand.To {
		return false
	}
	return a.Action != actPlace || a.Piece == cand.Piece
}

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	if mv.Aim != "" {
		return string(mv.Aim)
	}
	switch mv.Action {
	case actPlace:
		return fmt.Sprintf("%s@%s", mv.Piece, mv.To)
	case actMove:
		return fmt.Sprintf("%s-%s", mv.From, mv.To)
	}
	return actPass
}

// canonTile normalises a tile name such as wa1 to wA1; ok is false if it is not one.
func canonTile(t string) (string, bool) {
	if len(t) < 2 || len(t) > 3 {
		return "", false
	}
	c := t[0] | 0x20
	if c != 'w' && c != 'b' {
		return "", false
	}
	kind := -1
	for k, l := range kindLetter {
		if byte(l) == t[1]&^0x20 {
			kind = k
		}
	}
	if kind < 0 {
		return "", false
	}
	if isSingle(kind) {
		return string(c) + string(kindLetter[kind]), len(t) == 2
	}
	if len(t) != 3 || t[2] < '1' || t[2] > '3' {
		return "", false
	}
	return string(c) + string(kindLetter[kind]) + t[2:], true
}

// Parse reads the protocol's text (and, for older games, ant@j10 and j10-k10).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	raw := strings.TrimSpace(text)
	bad := func(why string) (state.Move, error) {
		return state.Move{}, fmt.Errorf("hive: cannot read %q as a move (%s; expected e.g. wQ, bA1 wQ-, wB1 /bA1 or pass)", text, why)
	}
	if strings.EqualFold(raw, actPass) {
		return state.Move{Action: actPass}, nil
	}
	cell := func(s string) (board.PositionID, bool) {
		id := board.PositionID(strings.ToLower(s))
		return id, game.Board.HasPosition(id)
	}
	if kind, at, ok := strings.Cut(strings.ToLower(raw), "@"); ok {
		if _, known := kindOf(kind); !known {
			return bad("unknown tile " + kind)
		}
		to, ok := cell(at)
		if !ok {
			return bad("unknown cell " + at)
		}
		return state.Move{Action: actPlace, Piece: kind, To: to}, nil
	}
	if a, b, ok := strings.Cut(raw, "-"); ok && !strings.Contains(raw, " ") && a != "" && b != "" {
		from, ok1 := cell(a)
		to, ok2 := cell(b)
		if ok1 && ok2 {
			return state.Move{Action: actMove, From: from, To: to}, nil
		}
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 || len(parts) > 2 {
		return bad("not a tile and a place")
	}
	name, ok := canonTile(parts[0])
	if !ok {
		return bad("unknown tile " + parts[0])
	}
	if len(parts) == 1 {
		return state.Move{Action: "uhp", Aim: board.PositionID(name)}, nil
	}
	rel := parts[1]
	prefix, suffix := "", ""
	switch {
	case strings.HasPrefix(rel, "-"), strings.HasPrefix(rel, "/"), strings.HasPrefix(rel, `\`):
		prefix, rel = rel[:1], rel[1:]
	case strings.HasSuffix(rel, "-"), strings.HasSuffix(rel, "/"), strings.HasSuffix(rel, `\`):
		suffix, rel = rel[len(rel)-1:], rel[:len(rel)-1]
	}
	ref, ok := canonTile(rel)
	if !ok {
		return bad("unknown tile " + rel)
	}
	return state.Move{Action: "uhp", Aim: board.PositionID(name + " " + prefix + ref + suffix)}, nil
}

// Steps: a placement is the tile, then the cell; a move is the two cells.
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	switch mv.Action {
	case actPlace:
		return []state.Step{{Kind: state.StepPiece, Value: mv.Piece}, {Kind: state.StepPos, Value: string(mv.To)}}
	case actMove:
		return []state.Step{{Kind: state.StepPos, Value: string(mv.From)}, {Kind: state.StepPos, Value: string(mv.To)}}
	}
	return []state.Step{{Kind: state.StepAction, Value: actPass}}
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			"Win by completely surrounding the opponent's Queen Bee with tiles of either colour. If one move surrounds both Queens, the game is a draw; surrounding only your own Queen loses.",
			fmt.Sprintf("Two passes in a row, or %d plies, draw the game.", game.ParamInt("max_plies", 300)),
		},
		NotationHelp: []string{
			"Moves use the Universal Hive Protocol notation, as on hivegame.com. A tile is its colour (w or b), its kind (Q Queen Bee, B Beetle, S Spider, G Grasshopper, A Soldier Ant, M Mosquito, L Ladybug, P Pillbug) and a number for kinds you have several of: wQ, bA2.",
			"A move is the tile, a space, and a tile beside which it lands: bG1 wS1- puts the Grasshopper to the right of wS1. A symbol after the name is the right side (wS1- right, wS1/ top right, wS1\\ bottom right); before the name it is the left (-wS1 left, \\wS1 top left, /wS1 bottom left). A Beetle (or Mosquito) climbing onto a tile writes just the name: wB1 bQ.",
			"The first tile of the game is written alone: wA1. pass is allowed only when you can neither place nor move.",
		},
	}
	if st, err := rs.Start(game, 0); err == nil {
		if moves := rs.Legal(game, st, 1); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	info.Actions = []state.RulesetAction{{ID: actPass, Label: "Pass", Hint: "You can neither place nor move a tile: pass the turn."}}
	return info
}
