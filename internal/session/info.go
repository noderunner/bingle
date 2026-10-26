package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
)

// GameInfo is a complete, client-facing description of a game definition:
// everything a UI needs to draw the board, label the pieces and offer the
// right actions without parsing position IDs or knowing game-specific
// rules, and everything an AI client needs to read the rules.
type GameInfo struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Rules       []string     `json:"rules,omitempty"`
	Notation    string       `json:"notation"`
	NotationDoc []string     `json:"notation_help,omitempty"`
	Seats       []SeatDefDTO `json:"seats"`
	Board       BoardDTO     `json:"board"`
	Pieces      []PieceDTO   `json:"pieces"`
	Pools       []PoolDTO    `json:"pools,omitempty"`
	// Cards describes the suits, card types and zones of a card game.
	Cards *CardsDTO `json:"cards,omitempty"`
	// Counters are reserve pools that hold no pieces, only a number per
	// seat (captured stones).
	Counters []CounterDTO   `json:"counters,omitempty"`
	Actions  []ActionDTO    `json:"actions"`
	Reserves map[string]int `json:"reserves,omitempty"`
	Opening  *OpeningDTO    `json:"opening,omitempty"`
	WinRules []string       `json:"win_rules,omitempty"`
	// Scores lists tallies worth showing live (e.g. Tak's flats on top).
	Scores []ScoreDTO `json:"scores,omitempty"`
	Family string     `json:"family"`
	// Engine is "ruleset" for games whose rules are written in Go: their
	// moves are a sequence of small decisions, and positions carry a
	// derived view (tints, badges, counters) under state.ext.view.
	Engine string `json:"engine,omitempty"`
	// Display is the legend for the tints and badges of a ruleset view.
	Display *dsl.DisplayDef `json:"display,omitempty"`
	// CountLabels says what a count selection means after an action.
	CountLabels map[string]string `json:"count_labels,omitempty"`
	// Params are the definition's tunable numbers (victory points needed).
	Params map[string]any `json:"params,omitempty"`
	// Guide lists the tabs of the rules window when the game ships its own
	// long-form guide; fetch the pages from /game-defs/{id}/guide.
	Guide []GuideTabInfo `json:"guide,omitempty"`
	// Options are the setup choices the game offers its creator (which cults
	// sit at the table, an expansion); send the chosen ones as "options" when
	// creating the game.
	Options []dsl.OptionDef `json:"options,omitempty"`
	// Start is the starting position (what a new game looks like), for
	// previews.
	Start StartDTO `json:"start"`
	// Preview is a sample game in play (the squares as in Start.Cells) for
	// games that start with an empty board; absent when the start is picture
	// enough.
	Preview map[string][]PieceOnBoardDTO `json:"preview,omitempty"`
}

// StartDTO is a starting position: stacks per square (bottom first) and each
// color's reserve.
type StartDTO struct {
	Cells    map[string][]PieceOnBoardDTO `json:"cells"`
	Reserves map[string]map[string]int    `json:"reserves,omitempty"`
}

// PieceOnBoardDTO is one piece on a square.
type PieceOnBoardDTO struct {
	Piece string `json:"piece"`
	Color string `json:"color"`
}

// ScoreDTO describes a running tally: how many squares have a top piece with
// the given tag (or of the given piece type) owned by each seat.
type ScoreDTO struct {
	Label string `json:"label"`
	Tag   string `json:"tag,omitempty"`
	Piece string `json:"piece,omitempty"`
}

// SeatDefDTO describes a seat.
type SeatDefDTO struct {
	Seat  int    `json:"seat"`
	Color string `json:"color"`
	Label string `json:"label"`
}

// BoardDTO describes the board graph and how to draw it.
type BoardDTO struct {
	Kind      string              `json:"kind"` // "grid", "hex" or "explicit"
	Style     string              `json:"style"`
	Width     int                 `json:"width,omitempty"`
	Height    int                 `json:"height,omitempty"`
	Labels    string              `json:"labels,omitempty"` // "xy" or "algebraic" for grids
	Positions []PositionDTO       `json:"positions"`
	Edges     [][2]string         `json:"edges,omitempty"`
	Regions   map[string][]string `json:"regions,omitempty"`
	// Borders maps region name -> seat color, for tinting board edges.
	Borders map[string]string `json:"borders,omitempty"`
	// Canvas and Areas describe an "areas" board: straight-edged polygons
	// in a canvas of the given size, one or more per position.
	Canvas *CanvasDTO `json:"canvas,omitempty"`
	Areas  []AreaDTO  `json:"areas,omitempty"`
}

// CanvasDTO is the coordinate space of an area map.
type CanvasDTO struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// AreaDTO is one area of an area map: its outline(s), where its name and
// tokens go and its tags (ocean, land, glyph-dragon…).
type AreaDTO struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Polygons [][][2]float64 `json:"polygons"`
	Anchor   [2]float64     `json:"anchor"`
	Tags     []string       `json:"tags,omitempty"`
}

// PositionDTO is one board position with layout coordinates (y grows
// downwards).
type PositionDTO struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

// CounterDTO is a per-seat number kept in a reserve pool.
type CounterDTO struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// PieceDTO describes a piece type.
type PieceDTO struct {
	Type      string   `json:"type"`
	Label     string   `json:"label"`
	Code      string   `json:"code,omitempty"`
	Rules     string   `json:"rules,omitempty"`
	Shape     string   `json:"shape"`
	Color     string   `json:"color,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Pool      string   `json:"pool"`
	Coverable bool     `json:"coverable"`
	CanStep   bool     `json:"can_step,omitempty"`
}

// PoolDTO describes a reserve pool: a hand of pieces that placed piece
// types are drawn from.
type PoolDTO struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Pieces []string `json:"pieces"`
	Start  int      `json:"start"`
}

// ActionDTO describes an action a player can take.
type ActionDTO struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	Label      string   `json:"label"`
	Pieces     []string `json:"pieces,omitempty"`
	CarryLimit int      `json:"carry_limit,omitempty"`
	// Hint is a one-line tooltip (ruleset games). Source says where the
	// action comes from when its button should show it: "book" or
	// "requirement".
	Hint   string `json:"hint,omitempty"`
	Source string `json:"source,omitempty"`
}

// OpeningDTO describes the opening phase.
type OpeningDTO struct {
	Plies   int      `json:"plies"`
	Actions []string `json:"actions,omitempty"`
	Pieces  []string `json:"pieces,omitempty"`
	Owner   string   `json:"owner"`
}

// GameInfo describes the game definition registered under id.
func (m *Manager) GameInfo(id string) (GameInfo, error) {
	cg, ok := m.catalog.Get(id)
	if !ok {
		return GameInfo{}, fmt.Errorf("%w: %q", ErrGameDefNotFound, id)
	}
	return buildGameInfo(cg), nil
}

func buildGameInfo(cg *catalog.CompiledGame) GameInfo {
	def := cg.Definition
	notation := def.Notation
	if notation == "" {
		notation = "generic"
	}
	info := GameInfo{
		ID:          def.ID,
		Name:        def.Name,
		Description: def.Description,
		Rules:       def.Rules,
		Notation:    notation,
		NotationDoc: notationHelp(cg),
		Reserves:    def.Reserves,
	}

	for _, s := range def.Seats {
		label := s.Label
		if label == "" {
			label = strings.ToUpper(s.Color[:1]) + s.Color[1:]
		}
		info.Seats = append(info.Seats, SeatDefDTO{Seat: s.Seat, Color: s.Color, Label: label})
	}

	// Board.
	b := cg.Board
	style := def.Board.Style
	if style == "" {
		style = "squares"
	}
	bd := BoardDTO{Kind: b.Kind(), Style: style, Width: b.Width(), Height: b.Height()}
	if def.Board.Generator != nil {
		bd.Labels = def.Board.Generator.Labels
		if bd.Labels == "" {
			bd.Labels = "xy"
		}
	}
	explicitXY := map[string][2]float64{}
	for _, p := range def.Board.Positions {
		if p.X != nil && p.Y != nil {
			explicitXY[p.ID] = [2]float64{*p.X, *p.Y}
		}
	}
	n := b.Len()
	for i, id := range b.Positions() {
		pd := PositionDTO{ID: string(id)}
		c, hasCoord := b.Coord(id)
		xy, hasXY := explicitXY[string(id)]
		switch {
		case hasCoord && b.IsHex():
			// Pointy-top hexagons: each row is shifted half a cell right,
			// and rows are 3/4 of a cell height apart (in units where a
			// cell is one wide).
			pd.X, pd.Y = float64(c.X)+float64(c.Y)/2, float64(c.Y)*0.8660254
		case hasCoord:
			pd.X, pd.Y = float64(c.X), float64(c.Y)
		case hasXY:
			pd.X, pd.Y = xy[0], xy[1]
		default:
			// No layout given: spread positions over a square-ish block so
			// the board is at least visible.
			cols := ceilSqrt(n)
			pd.X, pd.Y = float64(i%cols), float64(i/cols)
		}
		bd.Positions = append(bd.Positions, pd)
	}
	if !b.IsGrid() && !b.IsHex() {
		seen := map[[2]string]bool{}
		for _, id := range b.Positions() {
			for _, nb := range b.Neighbors(id) {
				a, c := string(id), string(nb)
				if a > c {
					a, c = c, a
				}
				if !seen[[2]string{a, c}] {
					seen[[2]string{a, c}] = true
					bd.Edges = append(bd.Edges, [2]string{a, c})
				}
			}
		}
	}
	if names := b.RegionNames(); len(names) > 0 {
		bd.Regions = map[string][]string{}
		for _, name := range names {
			ids, _ := b.Region(name)
			out := make([]string, len(ids))
			for i, id := range ids {
				out[i] = string(id)
			}
			bd.Regions[name] = out
		}
	}
	if len(def.Board.Borders) > 0 {
		bd.Borders = def.Board.Borders
	}
	if style == "areas" {
		bd.Canvas, bd.Areas = areasOf(def)
		// Areas are positioned by their anchors so generic code that reads
		// position coordinates still places them sensibly.
		at := map[string][2]float64{}
		for _, a := range bd.Areas {
			at[a.ID] = a.Anchor
		}
		for i := range bd.Positions {
			if xy, ok := at[bd.Positions[i].ID]; ok {
				bd.Positions[i].X, bd.Positions[i].Y = xy[0], xy[1]
			}
		}
	}
	info.Board = bd
	if def.Guide != nil {
		for _, t := range def.Guide.Tabs {
			info.Guide = append(info.Guide, GuideTabInfo{ID: t.ID, Title: t.Title})
		}
	}

	// Pieces and pools.
	poolPieces := map[string][]string{}
	for _, name := range cg.PieceTypes {
		pd := def.Pieces[name]
		rp := cg.Pieces[name]
		label := pd.Label
		if label == "" {
			label = name
		}
		shape := pd.Shape
		if shape == "" {
			shape = "disc"
		}
		info.Pieces = append(info.Pieces, PieceDTO{
			Type:      name,
			Label:     label,
			Code:      pd.Code,
			Rules:     pd.Rules,
			Shape:     shape,
			Color:     pd.Color,
			Tags:      pd.Tags,
			Pool:      rp.Pool,
			Coverable: rp.Coverable || def.Engine == "ruleset",
			CanStep:   rp.Primitive != nil,
		})
		poolPieces[rp.Pool] = append(poolPieces[rp.Pool], name)
	}
	var poolIDs []string
	for id := range poolPieces {
		if _, hasReserve := def.Reserves[id]; hasReserve {
			poolIDs = append(poolIDs, id)
		}
	}
	sort.Strings(poolIDs)
	// Reserve pools no piece draws from are plain counters (Pente's captured
	// stones): shown next to each seat, never as a hand.
	var counterIDs []string
	for id := range def.Reserves {
		if _, used := poolPieces[id]; !used {
			counterIDs = append(counterIDs, id)
		}
	}
	sort.Strings(counterIDs)
	for _, id := range counterIDs {
		label := strings.ToUpper(id[:1]) + id[1:]
		info.Counters = append(info.Counters, CounterDTO{ID: id, Label: label})
	}
	for _, id := range poolIDs {
		label := id
		if p, ok := def.Pieces[id]; ok && p.Label != "" {
			label = p.Label
		} else if len(poolPieces[id]) == 1 {
			if p := def.Pieces[poolPieces[id][0]]; p.Label != "" {
				label = p.Label
			}
		} else if len(label) > 0 {
			label = strings.ToUpper(label[:1]) + label[1:] + "s"
		}
		info.Pools = append(info.Pools, PoolDTO{ID: id, Label: label, Pieces: poolPieces[id], Start: def.Reserves[id]})
	}

	// Actions.
	for _, a := range cg.Actions {
		info.Actions = append(info.Actions, ActionDTO{
			ID: a.ID, Type: a.Type, Label: a.Label, Pieces: a.Pieces, CarryLimit: a.CarryLimit,
		})
	}
	if ri, ok := state.DescribeRuleset(cg); ok {
		info.Engine = "ruleset"
		info.Display = def.Display
		info.CountLabels = ri.CountLabels
		info.Params = def.Params
		info.NotationDoc = ri.NotationHelp
		for _, a := range ri.Actions {
			info.Actions = append(info.Actions, ActionDTO{ID: a.ID, Type: "ruleset", Label: a.Label, Hint: a.Hint, Source: a.Source})
		}
	}

	if op := def.TurnOrder.Opening; op != nil {
		owner := op.Owner
		if owner == "" {
			owner = "mover"
		}
		info.Opening = &OpeningDTO{Plies: op.Plies, Actions: op.Actions, Pieces: op.Pieces, Owner: owner}
	}

	info.Cards = cardsDTO(cg)
	info.WinRules = describeWinRules(cg)
	info.Family = familyOf(def.Family, def.ID)
	info.Options = def.Options

	start := state.InitialState(cg)
	info.Start = StartDTO{Cells: map[string][]PieceOnBoardDTO{}, Reserves: start.Reserves()}
	for _, id := range start.Occupied() {
		for _, o := range start.Stack(id) {
			info.Start.Cells[string(id)] = append(info.Start.Cells[string(id)], PieceOnBoardDTO{Piece: o.Piece, Color: o.Color})
		}
	}
	for _, e := range def.Preview {
		if !cg.Board.HasPosition(board.PositionID(e.Position)) {
			continue // a smaller variant of the game that lacks the square
		}
		if info.Preview == nil {
			info.Preview = map[string][]PieceOnBoardDTO{}
		}
		info.Preview[e.Position] = append(info.Preview[e.Position], PieceOnBoardDTO{Piece: e.Piece, Color: e.Color})
	}
	for _, r := range def.WinCondition.Rules {
		if r.Type == "end" && r.Decide != nil && r.Decide.Type == "most" {
			label := r.Decide.Tag
			if label == "" {
				label = r.Decide.Piece
			}
			if label != "" {
				label = strings.ToUpper(label[:1]) + label[1:] + "s on top"
			}
			info.Scores = append(info.Scores, ScoreDTO{Label: label, Tag: r.Decide.Tag, Piece: r.Decide.Piece})
		}
	}
	return info
}

func describeWinRules(cg *catalog.CompiledGame) []string {
	wc := cg.Definition.WinCondition
	switch wc.Type {
	case "none":
		return []string{"This game has no win condition; play is free-form."}
	case "callback":
		return []string{"The game's custom rules decide the winner."}
	case "ruleset":
		if ri, ok := state.DescribeRuleset(cg); ok && len(ri.WinRules) > 0 {
			return ri.WinRules
		}
		return []string{"The game's own rules decide the winner (see its rules)."}
	}
	var out []string
	for _, r := range wc.Rules {
		who := "Win"
		if r.Result == "lose" {
			who = "Lose"
		}
		what := "pieces"
		if r.Tag != "" {
			what = "'" + r.Tag + "' pieces"
		} else if r.Piece != "" {
			what = "'" + r.Piece + "' pieces"
		}
		switch r.Type {
		case "connect":
			if len(r.Of) > 0 {
				out = append(out, fmt.Sprintf("%s by building one connected group of your %s that touches at least %d of: %s.", who, what, r.Touch, strings.Join(r.Of, ", ")))
				break
			}
			var pairs []string
			for _, p := range r.Regions {
				pairs = append(pairs, p[0]+" to "+p[1])
			}
			out = append(out, fmt.Sprintf("%s by connecting %s with a chain of your %s (orthogonally/adjacent). If both players connect on the same move, the mover wins.", who, strings.Join(pairs, " or "), what))
		case "line":
			if r.Result == "lose" {
				out = append(out, fmt.Sprintf("Lose by making a line of %d or more of your pieces (unless the same move wins another way).", r.Length))
			} else {
				out = append(out, fmt.Sprintf("Win by getting %d of your pieces in a row.", r.Length))
			}
		case "ring":
			out = append(out, fmt.Sprintf("%s by enclosing at least one square with a closed loop of your %s.", who, what))
		case "reach":
			out = append(out, fmt.Sprintf("%s by getting one of your %s onto the %s.", who, what, r.Region))
		case "end":
			var when []string
			for _, w := range r.When {
				switch w {
				case "board-full":
					when = append(when, "the board is full")
				case "reserve-empty":
					when = append(when, "a player has no pieces left in hand")
				case "no-legal-moves":
					when = append(when, "the player to move has no legal move")
				case "ply-limit":
					when = append(when, fmt.Sprintf("%d plies have been played", r.Plies))
				case "passes":
					when = append(when, fmt.Sprintf("%d passes happen in a row", r.Count))
				}
			}
			outcome := ""
			if r.Decide != nil {
				switch r.Decide.Type {
				case "draw":
					outcome = "the game is a draw"
				case "mover-wins":
					outcome = "the player who just moved wins"
				case "mover-loses":
					outcome = "the player who just moved loses"
				case "score":
					outcome = "the higher score wins (stones plus surrounded territory, with komi)"
				case "most":
					f := r.Decide.Tag
					if f == "" {
						f = r.Decide.Piece
					}
					if f == "" {
						f = "pieces"
					} else {
						f = "'" + f + "' pieces"
					}
					outcome = "the player with the most " + f + " on top wins (a tie is a draw)"
				}
			}
			out = append(out, fmt.Sprintf("The game ends when %s; %s.", strings.Join(when, " or "), outcome))
		}
	}
	return out
}

func notationHelp(cg *catalog.CompiledGame) []string {
	if ri, ok := state.DescribeRuleset(cg); ok {
		return ri.NotationHelp
	}
	if cg.Definition.Notation == "ptn" {
		return []string{
			"Moves use Portable Tak Notation (PTN). Squares are file letter + rank number, a1 = bottom-left.",
			"Placement: 'a1' places a flat stone; prefix 'S' for a standing stone (wall) or 'C' for a capstone: 'Sc3', 'Cd4'.",
			"Stack move: [count]<square><direction>[drops], direction is + (up/north), - (down/south), < (west), > (east). Count defaults to 1; drops is one digit per square entered, bottom pieces first, and may be omitted when everything is dropped on the first square. Example: '3c3>111' picks up 3 from c3 and drops 1 on each of d3, e3, f3; 'c3+' moves the top stone one square up; '2d2<' moves 2 stones one square west.",
		}
	}
	var out []string
	has := map[string]bool{}
	multiPiece := false
	for _, a := range cg.Actions {
		has[a.Type] = true
		if a.Type == catalog.ActionPlace && len(a.Pieces) > 1 {
			multiPiece = true
		}
	}
	if has[catalog.ActionStep] {
		out = append(out, "Move a piece: '<from>-<to>'.")
	}
	if has[catalog.ActionSlide] {
		thenAim := false
		for _, a := range cg.Actions {
			if a.Type == catalog.ActionSlide && a.Then != nil {
				thenAim = true
			}
		}
		if thenAim {
			out = append(out, "Slide a piece and shoot: '<from>-<to>/<target>' (the target is where the shot lands after the slide).")
		} else {
			out = append(out, "Slide a piece: '<from>-<to>'.")
		}
	}
	if has[catalog.ActionLeap] {
		out = append(out, "Clone or jump a piece: '<from>-<to>'.")
	}
	if has[catalog.ActionJump] {
		out = append(out, "Jump (and keep jumping): '<from>x<landing>[x<landing>...]'.")
	}
	if has[catalog.ActionPlace] {
		if multiPiece {
			out = append(out, "Place a piece: '<piece>@<square>'.")
		} else {
			out = append(out, "Place a piece: just name the square.")
		}
	}
	if has[catalog.ActionMoveStack] {
		out = append(out, "Move a stack: '<from>/<count>><square>:<drops>[><square>:<drops>...]'.")
	}
	if has[catalog.ActionPass] {
		out = append(out, "Pass: 'pass'.")
	}
	if has[catalog.ActionSwap] {
		out = append(out, "Swap sides (pie rule): 'swap'.")
	}
	// A concrete example from the opening position.
	if first := state.FirstSeat(cg); true {
		if moves := state.LegalMoves(cg, state.InitialState(cg), first); len(moves) > 0 {
			out = append(out, "Example of a legal first move: '"+state.FormatMove(cg, moves[0])+"'.")
		}
	}
	return out
}

func ceilSqrt(n int) int {
	r := 1
	for r*r < n {
		r++
	}
	return r
}

// CardsDTO is what a client needs to draw the cards of a game: the suits and
// every card type (the position names cards by id only), what a back looks
// like and the zones.
type CardsDTO struct {
	Suits     map[string]dsl.SuitDef `json:"suits"`
	SuitOrder []string               `json:"suit_order"`
	Types     []CardTypeDTO          `json:"types"`
	Back      dsl.CardBackDef        `json:"back"`
	Zones     []dsl.ZoneDef          `json:"zones"`
}

// CardTypeDTO is one kind of card.
type CardTypeDTO struct {
	ID      string   `json:"id"`
	Suit    string   `json:"suit,omitempty"`
	Rank    string   `json:"rank,omitempty"`
	Label   string   `json:"label"`
	Value   int      `json:"value"`
	Text    string   `json:"text,omitempty"`
	Symbols []string `json:"symbols,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

func cardsDTO(cg *catalog.CompiledGame) *CardsDTO {
	cc := cg.Cards
	if cc == nil {
		return nil
	}
	out := &CardsDTO{Suits: cc.Def.Suits, SuitOrder: cc.Suits, Back: cc.Def.Back, Zones: cc.Def.Zones}
	if out.Suits == nil {
		out.Suits = map[string]dsl.SuitDef{}
	}
	ids := make([]string, 0, len(cc.Types))
	for id := range cc.Types {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t := cc.Types[id]
		out.Types = append(out.Types, CardTypeDTO{ID: t.ID, Suit: t.Suit, Rank: t.Rank, Label: t.Label, Value: t.Value, Text: t.Text, Symbols: t.Symbols, Tags: t.Tags})
	}
	return out
}
