package state

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// FormatMove renders mv in game's canonical move notation.
//
// Generic notation (the default):
//
//	step        from-to            e.g. "2,0-2,1"
//	place       piece@to           e.g. "stone@3,3" (just "3,3" when the
//	                               action places only one kind of piece)
//	move-stack  from/count>p:n>p:n e.g. "c3/3>d3:1>e3:2"
//	jump        fromxlandxland     e.g. "c3xe5xg7"
//	slide       from-to[/aim]      e.g. "d1-d5/f5" (the aim is the shot that
//	                               follows the slide, when the action has one)
//
// With notation "ptn" (Tak's Portable Tak Notation) placements are "a1"
// (default piece), "Sa1" (piece code prefix) and stack moves are
// "<count><square><direction><drops>", e.g. "3c3>111".
func FormatMove(game *catalog.CompiledGame, mv Move) string {
	if rs, ok := RulesetOf(game); ok {
		return rs.Format(game, mv)
	}
	act, ok := game.Action(mv.Action)
	if !ok {
		return string(mv.From) + "-" + string(mv.To)
	}
	if game.Definition.Notation == "ptn" {
		if s, ok := formatPTN(game, act, mv); ok {
			return s
		}
	}
	switch act.Type {
	case catalog.ActionPass:
		return "pass"
	case catalog.ActionSwap:
		return "swap"
	case catalog.ActionPlace:
		if len(act.Pieces) == 1 {
			// Only one thing can be placed: the square says it all.
			return string(mv.To)
		}
		return mv.Piece + "@" + string(mv.To)
	case catalog.ActionJump:
		var b strings.Builder
		b.WriteString(string(mv.From))
		for _, p := range mv.Path {
			b.WriteByte('x')
			b.WriteString(string(p))
		}
		return b.String()
	case catalog.ActionMoveStack:
		var b strings.Builder
		b.WriteString(string(mv.From))
		b.WriteByte('/')
		b.WriteString(strconv.Itoa(mv.Count))
		for i, p := range mv.Path {
			b.WriteByte('>')
			b.WriteString(string(p))
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(mv.Drops[i]))
		}
		return b.String()
	case catalog.ActionSlide:
		out := string(mv.From) + "-" + string(mv.To)
		if mv.Aim != "" {
			out += "/" + string(mv.Aim)
		}
		return out
	default:
		return string(mv.From) + "-" + string(mv.To)
	}
}

// ptnDirection maps a grid step to PTN's direction character.
func ptnDirection(dx, dy int) (byte, bool) {
	switch {
	case dx == 0 && dy == -1:
		return '+', true
	case dx == 0 && dy == 1:
		return '-', true
	case dx == -1 && dy == 0:
		return '<', true
	case dx == 1 && dy == 0:
		return '>', true
	}
	return 0, false
}

func ptnStep(dir byte) (dx, dy int, ok bool) {
	switch dir {
	case '+':
		return 0, -1, true
	case '-':
		return 0, 1, true
	case '<':
		return -1, 0, true
	case '>':
		return 1, 0, true
	}
	return 0, 0, false
}

func formatPTN(game *catalog.CompiledGame, act catalog.ResolvedAction, mv Move) (string, bool) {
	switch act.Type {
	case catalog.ActionPlace:
		prefix := ""
		if len(act.Pieces) > 0 && mv.Piece != act.Pieces[0] {
			code := game.Definition.Pieces[mv.Piece].Code
			if code == "" {
				return "", false
			}
			prefix = code
		}
		return prefix + string(mv.To), true
	case catalog.ActionMoveStack:
		if len(mv.Path) == 0 {
			return "", false
		}
		g := game.Board
		fc, ok1 := g.Coord(mv.From)
		pc, ok2 := g.Coord(mv.Path[0])
		if !ok1 || !ok2 {
			return "", false
		}
		dir, ok := ptnDirection(pc.X-fc.X, pc.Y-fc.Y)
		if !ok {
			return "", false
		}
		var b strings.Builder
		if mv.Count > 1 {
			b.WriteString(strconv.Itoa(mv.Count))
		}
		b.WriteString(string(mv.From))
		b.WriteByte(dir)
		// A single-square move drops everything there, so PTN omits the
		// drop list ("3c3>"); longer moves list one digit per square.
		if len(mv.Path) > 1 {
			for _, d := range mv.Drops {
				b.WriteString(strconv.Itoa(d))
			}
		}
		return b.String(), true
	}
	return "", false
}

var (
	ptnSquareRE = regexp.MustCompile(`^[a-z][0-9]+$`)
	ptnPlaceRE  = regexp.MustCompile(`^([A-Za-z])([a-z][0-9]+)$`)
	ptnMoveRE   = regexp.MustCompile(`^([0-9]*)([a-z][0-9]+)([-+<>])([0-9]*)$`)
)

// ParseMove parses text in game's notation (see FormatMove) into a Move. It
// only checks syntax and that named squares exist; legality is Apply's job.
// A bare position-less token, or one the game's notation cannot express,
// yields an error.
func ParseMove(game *catalog.CompiledGame, text string) (Move, error) {
	if rs, ok := RulesetOf(game); ok {
		return rs.Parse(game, text)
	}
	s := strings.TrimSpace(text)
	// Strip annotation marks PTN allows: "!", "?", "'" and the "*"
	// flatten marker.
	s = strings.TrimRight(s, "!?'*")
	if s == "" {
		return Move{}, fmt.Errorf("state: empty move")
	}
	g := game.Board

	if strings.EqualFold(s, "swap") {
		for _, a := range game.Actions {
			if a.Type == catalog.ActionSwap {
				return Move{Action: a.ID}, nil
			}
		}
		return Move{}, fmt.Errorf("state: this game has no swap action")
	}

	if strings.EqualFold(s, "pass") {
		for _, a := range game.Actions {
			if a.Type == catalog.ActionPass {
				return Move{Action: a.ID}, nil
			}
		}
		return Move{}, fmt.Errorf("state: this game has no pass action")
	}

	if game.Definition.Notation == "ptn" {
		if mv, ok, err := parsePTN(game, s); ok || err != nil {
			return mv, err
		}
	}

	// Generic: a bare square names a placement when exactly one place action
	// exists and it places a single kind of piece.
	if g.HasPosition(board.PositionID(s)) {
		var only *catalog.ResolvedAction
		count := 0
		for i := range game.Actions {
			if game.Actions[i].Type == catalog.ActionPlace {
				count++
				only = &game.Actions[i]
			}
		}
		if count == 1 && len(only.Pieces) == 1 {
			return Move{Action: only.ID, Piece: only.Pieces[0], To: board.PositionID(s)}, nil
		}
	}

	// Generic: place "piece@to".
	if i := strings.Index(s, "@"); i >= 0 {
		piece, to := s[:i], board.PositionID(s[i+1:])
		if !g.HasPosition(to) {
			return Move{}, fmt.Errorf("state: unknown position %q", to)
		}
		mv := Move{Piece: piece, To: to}
		for _, a := range game.Actions {
			if a.Type == catalog.ActionPlace {
				mv.Action = a.ID
				break
			}
		}
		if mv.Action == "" {
			return Move{}, fmt.Errorf("state: game has no place action")
		}
		if piece == "" {
			mv.Piece = ""
		}
		return mv, nil
	}

	// Generic: slide with a follow-up "from-to/aim".
	if i := strings.LastIndex(s, "/"); i > 0 && !strings.Contains(s[i:], ">") {
		var slide *catalog.ResolvedAction
		for k := range game.Actions {
			if game.Actions[k].Type == catalog.ActionSlide && game.Actions[k].Then != nil {
				slide = &game.Actions[k]
				break
			}
		}
		if slide != nil {
			aim := board.PositionID(s[i+1:])
			left := s[:i]
			for k := 0; k < len(left); k++ {
				if left[k] != '-' {
					continue
				}
				from, to := board.PositionID(left[:k]), board.PositionID(left[k+1:])
				if g.HasPosition(from) && g.HasPosition(to) && g.HasPosition(aim) {
					return Move{Action: slide.ID, From: from, To: to, Aim: aim}, nil
				}
			}
			return Move{}, fmt.Errorf("state: cannot parse slide %q (expected from-to/aim)", text)
		}
	}

	// Generic: stack "from/count>p:n>p:n".
	if i := strings.Index(s, "/"); i >= 0 {
		from := board.PositionID(s[:i])
		if !g.HasPosition(from) {
			return Move{}, fmt.Errorf("state: unknown position %q", from)
		}
		rest := strings.Split(s[i+1:], ">")
		count, err := strconv.Atoi(rest[0])
		if err != nil {
			return Move{}, fmt.Errorf("state: bad stack move %q: count %q", s, rest[0])
		}
		mv := Move{From: from, Count: count}
		for _, part := range rest[1:] {
			kv := strings.SplitN(part, ":", 2)
			pos := board.PositionID(kv[0])
			if !g.HasPosition(pos) {
				return Move{}, fmt.Errorf("state: unknown position %q", pos)
			}
			drop := 1
			if len(kv) == 2 {
				if drop, err = strconv.Atoi(kv[1]); err != nil {
					return Move{}, fmt.Errorf("state: bad stack move %q: drop %q", s, kv[1])
				}
			}
			mv.Path = append(mv.Path, pos)
			mv.Drops = append(mv.Drops, drop)
		}
		return mv.Normalize(game)
	}

	// Generic: jump "fromxland[xland...]".
	if strings.Contains(s, "x") {
		hasJump := false
		for _, a := range game.Actions {
			if a.Type == catalog.ActionJump {
				hasJump = true
			}
		}
		parts := strings.Split(s, "x")
		if hasJump && len(parts) >= 2 {
			ok := true
			for _, p := range parts {
				if !g.HasPosition(board.PositionID(p)) {
					ok = false
				}
			}
			if ok {
				mv := Move{From: board.PositionID(parts[0])}
				for _, p := range parts[1:] {
					mv.Path = append(mv.Path, board.PositionID(p))
				}
				return mv.Normalize(game)
			}
		}
	}

	// Generic: step "from-to". Position IDs may themselves contain '-',
	// so try every split point.
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			continue
		}
		from, to := board.PositionID(s[:i]), board.PositionID(s[i+1:])
		if g.HasPosition(from) && g.HasPosition(to) {
			return Move{Action: catalog.ActionStep, From: from, To: to}, nil
		}
	}

	return Move{}, fmt.Errorf("state: cannot parse move %q in this game's notation", text)
}

// parsePTN returns (move, true, nil) for a well-formed PTN token,
// (_, false, nil) when s is simply not PTN-shaped, and (_, false, err) when
// it is PTN-shaped but wrong.
func parsePTN(game *catalog.CompiledGame, s string) (Move, bool, error) {
	g := game.Board

	placeAction := ""
	var placePieces []string
	stackAction := ""
	for _, a := range game.Actions {
		switch a.Type {
		case catalog.ActionPlace:
			if placeAction == "" {
				placeAction, placePieces = a.ID, a.Pieces
			}
		case catalog.ActionMoveStack:
			if stackAction == "" {
				stackAction = a.ID
			}
		}
	}

	if ptnSquareRE.MatchString(s) {
		sq := board.PositionID(s)
		if !g.HasPosition(sq) {
			return Move{}, false, fmt.Errorf("state: unknown square %q", s)
		}
		if placeAction == "" || len(placePieces) == 0 {
			return Move{}, false, fmt.Errorf("state: game has no place action")
		}
		return Move{Action: placeAction, Piece: placePieces[0], To: sq}, true, nil
	}

	if m := ptnPlaceRE.FindStringSubmatch(s); m != nil {
		code, sq := m[1], board.PositionID(m[2])
		if !g.HasPosition(sq) {
			return Move{}, false, fmt.Errorf("state: unknown square %q", m[2])
		}
		for _, p := range placePieces {
			if strings.EqualFold(game.Definition.Pieces[p].Code, code) {
				return Move{Action: placeAction, Piece: p, To: sq}, true, nil
			}
		}
		return Move{}, false, fmt.Errorf("state: unknown piece code %q in %q", code, s)
	}

	if m := ptnMoveRE.FindStringSubmatch(s); m != nil {
		if stackAction == "" {
			return Move{}, false, fmt.Errorf("state: game has no stack-move action")
		}
		from := board.PositionID(m[2])
		if !g.HasPosition(from) {
			return Move{}, false, fmt.Errorf("state: unknown square %q", m[2])
		}
		count := 1
		if m[1] != "" {
			n, err := strconv.Atoi(m[1])
			if err != nil || n < 1 {
				return Move{}, false, fmt.Errorf("state: bad carry count in %q", s)
			}
			count = n
		}
		dx, dy, _ := ptnStep(m[3][0])

		var drops []int
		if m[4] == "" {
			drops = []int{count}
		} else {
			for _, ch := range m[4] {
				drops = append(drops, int(ch-'0'))
			}
		}

		fc, ok := g.Coord(from)
		if !ok {
			return Move{}, false, fmt.Errorf("state: board has no coordinates for %s", from)
		}
		path := make([]board.PositionID, 0, len(drops))
		for i := range drops {
			p, ok := g.At(board.Coord{X: fc.X + (i+1)*dx, Y: fc.Y + (i+1)*dy})
			if !ok {
				return Move{}, false, fmt.Errorf("state: move %q runs off the board", s)
			}
			path = append(path, p)
		}
		mv := Move{Action: stackAction, From: from, Count: count, Path: path, Drops: drops}
		mv.To = path[len(path)-1]
		return mv, true, nil
	}

	return Move{}, false, nil
}
