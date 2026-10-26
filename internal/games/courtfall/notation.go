package courtfall

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation (no spaces, so lists of moves split on whitespace):
//
//	tile:green@d2      lay a tile (green Overgrowth, blue Mire, red Hazard)
//	court:bastion      choose a Court
//	deploy:Q@c6        deploy a unit (P N B R Q)
//	recover:P          free recovery on that unit
//	activate:Q         activate a Fresh unit      exert:Q   exert a Fatigued one
//	surge:N>d3         spend 1 Momentum to shift a unit one hex
//	rouse:R            spend 2 Momentum to remove a black stone
//	tiles:c3<>d4       Stone Court: swap two tiles
//	clear / clear+2    Clear Turn, buying 0 / 2 VP
//	Qd4                the active unit moves to d4
//	Qxd4/2             strike the unit at d4 with 2 attack dice (the rest defend)
//	B*e5               shoot the unit at e5       R^d4   shove the unit at d4
//	end                end the activation
//	respond/2          answer a strike with 2 attack dice
//	reroll/3           spend 1 Momentum to reroll your 3rd die (attack first)
//	keep               decline to reroll
//	push>NE            push the unit in a direction (E NE NW W SW SE)
//	shift>d4  stay     Hit and Run
//	seize  done        take another turn / end the turn

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	code := unitCode[mv.Piece]
	switch mv.Action {
	case actTile:
		return fmt.Sprintf("tile:%s@%s", mv.Piece, mv.To)
	case actCourt:
		return "court:" + mv.Piece
	case actDeploy:
		return fmt.Sprintf("deploy:%s@%s", code, mv.To)
	case actRecover, actActivate, actExert, actRouse:
		return mv.Action + ":" + code
	case actClear:
		if mv.Count <= 0 {
			return "clear"
		}
		return fmt.Sprintf("clear+%d", mv.Count)
	case actSurge:
		return fmt.Sprintf("surge:%s>%s", code, mv.To)
	case actTiles:
		a, b := string(mv.From), string(mv.To)
		if a > b {
			a, b = b, a
		}
		return fmt.Sprintf("tiles:%s<>%s", a, b)
	case actMove:
		return code + string(mv.To)
	case actStrike:
		if mv.Count < 0 {
			return fmt.Sprintf("%sx%s/?", code, mv.To)
		}
		return fmt.Sprintf("%sx%s/%d", code, mv.To, mv.Count)
	case actShoot:
		return fmt.Sprintf("%s*%s", code, mv.To)
	case actShove:
		return fmt.Sprintf("%s^%s", code, mv.To)
	case actEnd, actKeep, actStay, actSeize, actDone:
		return mv.Action
	case actRespond:
		return fmt.Sprintf("respond/%d", mv.Count)
	case actReroll:
		return fmt.Sprintf("reroll/%d", mv.Count)
	case actPush:
		if mv.Count >= 1 && mv.Count <= 6 {
			return "push>" + dirNames[mv.Count-1]
		}
		return "push"
	case actShift:
		return "shift>" + string(mv.To)
	}
	return mv.Action
}

// Parse reads notation produced by Format (case-insensitively).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		return state.Move{}, fmt.Errorf("courtfall: empty move")
	}
	low := strings.ToLower(t)
	bad := func(why string) (state.Move, error) {
		return state.Move{}, fmt.Errorf("courtfall: cannot read %q as a move (%s)", text, why)
	}
	cell := func(s string) (board.PositionID, bool) {
		id := board.PositionID(strings.ToLower(s))
		return id, game.Board.HasPosition(id)
	}
	unit := func(s string) (string, bool) {
		p, ok := unitFromCode[strings.ToUpper(s)]
		return p, ok
	}

	switch low {
	case "end", "keep", "stay", "seize", "done", "clear":
		if low == "clear" {
			return state.Move{Action: actClear}, nil
		}
		return state.Move{Action: low}, nil
	}
	switch {
	case strings.HasPrefix(low, "clear+"):
		n, err := strconv.Atoi(low[len("clear+"):])
		if err != nil || n < 0 {
			return bad("clear+N needs a number of VP")
		}
		return state.Move{Action: actClear, Count: n}, nil
	case strings.HasPrefix(low, "tile:"):
		parts := strings.SplitN(low[len("tile:"):], "@", 2)
		if len(parts) != 2 || tileIndex(parts[0]) < 0 {
			return bad("expected tile:green|blue|red@hex")
		}
		id, ok := cell(parts[1])
		if !ok {
			return bad("unknown hex " + parts[1])
		}
		return state.Move{Action: actTile, Piece: parts[0], To: id}, nil
	case strings.HasPrefix(low, "court:"):
		c := low[len("court:"):]
		if !validCourt(c) {
			return bad("unknown court " + c)
		}
		return state.Move{Action: actCourt, Piece: c}, nil
	case strings.HasPrefix(low, "deploy:"):
		parts := strings.SplitN(low[len("deploy:"):], "@", 2)
		if len(parts) != 2 {
			return bad("expected deploy:Q@hex")
		}
		p, ok := unit(parts[0])
		if !ok {
			return bad("unknown unit " + parts[0])
		}
		id, ok := cell(parts[1])
		if !ok {
			return bad("unknown hex " + parts[1])
		}
		return state.Move{Action: actDeploy, Piece: p, To: id}, nil
	case strings.HasPrefix(low, "recover:"), strings.HasPrefix(low, "activate:"), strings.HasPrefix(low, "exert:"), strings.HasPrefix(low, "rouse:"):
		i := strings.IndexByte(low, ':')
		p, ok := unit(low[i+1:])
		if !ok {
			return bad("unknown unit " + low[i+1:])
		}
		return state.Move{Action: low[:i], Piece: p}, nil
	case strings.HasPrefix(low, "surge:"):
		parts := strings.SplitN(low[len("surge:"):], ">", 2)
		if len(parts) != 2 {
			return bad("expected surge:N>hex")
		}
		p, ok := unit(parts[0])
		if !ok {
			return bad("unknown unit " + parts[0])
		}
		id, ok := cell(parts[1])
		if !ok {
			return bad("unknown hex " + parts[1])
		}
		return state.Move{Action: actSurge, Piece: p, To: id}, nil
	case strings.HasPrefix(low, "tiles:"):
		parts := strings.SplitN(low[len("tiles:"):], "<>", 2)
		if len(parts) != 2 {
			return bad("expected tiles:hex<>hex")
		}
		a, ok1 := cell(parts[0])
		b, ok2 := cell(parts[1])
		if !ok1 || !ok2 {
			return bad("unknown hex")
		}
		return state.Move{Action: actTiles, From: a, To: b}, nil
	case strings.HasPrefix(low, "respond/"), strings.HasPrefix(low, "reroll/"):
		i := strings.IndexByte(low, '/')
		n, err := strconv.Atoi(low[i+1:])
		if err != nil || n < 0 {
			return bad("expected a number")
		}
		return state.Move{Action: low[:i], Count: n}, nil
	case strings.HasPrefix(low, "push>"):
		d := dirIndex(strings.ToUpper(low[len("push>"):]))
		if d < 0 {
			return bad("direction must be one of E NE NW W SW SE")
		}
		return state.Move{Action: actPush, Count: d + 1}, nil
	case strings.HasPrefix(low, "shift>"):
		id, ok := cell(low[len("shift>"):])
		if !ok {
			return bad("unknown hex")
		}
		return state.Move{Action: actShift, To: id}, nil
	}

	// A unit letter then a destination: Qd4, Qxd4/2, B*e5, R^d4.
	if p, ok := unit(t[:1]); ok && len(t) > 1 {
		rest := low[1:]
		switch rest[0] {
		case 'x':
			parts := strings.SplitN(rest[1:], "/", 2)
			if len(parts) != 2 {
				return bad("expected Qxhex/dice")
			}
			id, ok := cell(parts[0])
			if !ok {
				return bad("unknown hex " + parts[0])
			}
			n, err := strconv.Atoi(parts[1])
			if err != nil || n < 0 {
				return bad("dice must be a number")
			}
			return state.Move{Action: actStrike, Piece: p, To: id, Count: n}, nil
		case '*':
			id, ok := cell(rest[1:])
			if !ok {
				return bad("unknown hex " + rest[1:])
			}
			return state.Move{Action: actShoot, Piece: p, To: id}, nil
		case '^':
			id, ok := cell(rest[1:])
			if !ok {
				return bad("unknown hex " + rest[1:])
			}
			return state.Move{Action: actShove, Piece: p, To: id}, nil
		default:
			id, ok := cell(rest)
			if !ok {
				return bad("unknown hex " + rest)
			}
			return state.Move{Action: actMove, Piece: p, To: id}, nil
		}
	}
	return bad("not a Courtfall move")
}

// Steps decomposes a move into clicks. Positions come first where a click
// on the board can name the decision; verbs are actions; numbers are counts.
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	pos := func(id board.PositionID) state.Step { return state.Step{Kind: state.StepPos, Value: string(id)} }
	act := func(id string) state.Step { return state.Step{Kind: state.StepAction, Value: id} }
	cnt := func(n int) state.Step { return state.Step{Kind: state.StepCount, Value: strconv.Itoa(n)} }
	switch mv.Action {
	case actTile:
		return []state.Step{pos(mv.To), act("tile-" + mv.Piece)}
	case actCourt:
		return []state.Step{act("court-" + mv.Piece)}
	case actDeploy:
		return []state.Step{{Kind: state.StepPiece, Value: mv.Piece}, pos(mv.To)}
	case actRecover, actActivate, actExert, actRouse:
		return []state.Step{pos(mv.From), act(mv.Action)}
	case actSurge:
		return []state.Step{pos(mv.From), act(actSurge), pos(mv.To)}
	case actTiles:
		return []state.Step{pos(mv.From), act(actTiles), pos(mv.To)}
	case actClear:
		return []state.Step{act(actClear), cnt(mv.Count)}
	case actMove, actShoot, actShove:
		return []state.Step{pos(mv.To), act(mv.Action)}
	case actStrike:
		return []state.Step{pos(mv.To), act(actStrike), cnt(mv.Count)}
	case actRespond, actReroll:
		return []state.Step{act(mv.Action), cnt(mv.Count)}
	case actPush:
		if mv.To != "" {
			return []state.Step{pos(mv.To), act(actPush)}
		}
		if mv.Count >= 1 && mv.Count <= 6 {
			return []state.Step{act("edge-" + dirNames[mv.Count-1])}
		}
		return []state.Step{act(actPush)}
	case actShift:
		return []state.Step{pos(mv.To), act(actShift)}
	default:
		return []state.Step{act(mv.Action)}
	}
}

// ActionNotations lists every move notation in a stable order: the fixed
// action space of the game.
func (rs *Ruleset) ActionNotations(game *catalog.CompiledGame) []string {
	g := geoOf(game)
	var out []string
	add := func(mv state.Move) { out = append(out, rs.Format(game, mv)) }
	var drafting, zoned []board.PositionID
	for c, id := range g.ids {
		if g.zone[c] == 0 && c != g.eye {
			drafting = append(drafting, id)
		}
		if g.zone[c] != 0 {
			zoned = append(zoned, id)
		}
	}
	for _, color := range tileColors {
		for _, id := range drafting {
			add(state.Move{Action: actTile, Piece: color, To: id})
		}
	}
	for _, c := range courts {
		add(state.Move{Action: actCourt, Piece: c})
	}
	for _, p := range unitTypes {
		for _, id := range zoned {
			add(state.Move{Action: actDeploy, Piece: p, To: id})
		}
	}
	for _, a := range []string{actRecover, actActivate, actExert, actRouse} {
		for _, p := range unitTypes {
			add(state.Move{Action: a, Piece: p})
		}
	}
	for n := 0; priceOf(n) <= game.ParamInt("momentum_cap", 5); n++ {
		add(state.Move{Action: actClear, Count: n})
	}
	for _, p := range unitTypes {
		for _, id := range g.ids {
			add(state.Move{Action: actSurge, Piece: p, To: id})
		}
	}
	for ai, a := range drafting {
		for _, b := range drafting[ai+1:] {
			if g.dist[g.index[a]][g.index[b]] <= 2 {
				add(state.Move{Action: actTiles, From: a, To: b})
			}
		}
	}
	for _, p := range unitTypes {
		for _, id := range g.ids {
			add(state.Move{Action: actMove, Piece: p, To: id})
		}
	}
	for _, p := range unitTypes {
		for _, id := range g.ids {
			for n := 0; n <= 4; n++ {
				add(state.Move{Action: actStrike, Piece: p, To: id, Count: n})
			}
		}
	}
	for _, id := range g.ids {
		add(state.Move{Action: actShoot, Piece: "bishop", To: id})
	}
	for _, p := range unitTypes {
		for _, id := range g.ids {
			add(state.Move{Action: actShove, Piece: p, To: id})
		}
	}
	for _, a := range []string{actEnd, actKeep, actStay, actSeize, actDone} {
		add(state.Move{Action: a})
	}
	for n := 0; n <= 4; n++ {
		add(state.Move{Action: actRespond, Count: n})
	}
	for n := 1; n <= 4; n++ {
		add(state.Move{Action: actReroll, Count: n})
	}
	for d := 1; d <= 6; d++ {
		add(state.Move{Action: actPush, Count: d})
	}
	for _, id := range g.ids {
		add(state.Move{Action: actShift, Piece: "knight", To: id})
	}
	return out
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			fmt.Sprintf("Be the first to %d victory points: VP are bought with Momentum during your own Clear Turn (2 for the first, 3 for the second, 4 for the third).", game.ParamInt("vp_to_win", 5)),
			fmt.Sprintf("After %d plies the player with more VP wins (equal VP is a draw).", game.ParamInt("max_plies", 2000)),
		},
		CountLabels: map[string]string{
			actStrike:  "Attack dice (the rest defend)",
			actRespond: "Attack dice (the rest defend)",
			actReroll:  "Reroll which die?",
			actClear:   "Buy VP",
			"":         "Choose",
		},
		NotationHelp: []string{
			"Units are P (Warden), N (Outrider), B (Arbalest), R (Juggernaut), Q (Champion); hexes are a1–g7.",
			"tile:green@d2 · court:bastion · deploy:Q@c6 — setup (green Overgrowth, blue Mire, red Hazard).",
			"activate:Q · exert:Q · clear / clear+2 (buy 2 VP) · surge:N>d3 · rouse:R · tiles:c3<>d4 — start of a turn.",
			"Qd4 move · Qxd4/2 strike with 2 attack dice · B*e5 shoot · R^d4 shove · end the activation.",
			"respond/2 answer a strike with 2 attack dice · reroll/3 reroll your 3rd die · keep · push>NE · shift>d4 · stay · seize · done.",
		},
	}
	if st, err := rs.Start(game, 1); err == nil {
		if moves := rs.Legal(game, st, 1); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"' (lay a tile); later decisions look like 'activate:Q', then 'Qd4'.")
		}
	}
	add := func(id, label, hint string) {
		info.Actions = append(info.Actions, state.RulesetAction{ID: id, Label: label, Hint: hint})
	}
	add("tile-green", "Overgrowth", "Lay an Overgrowth tile: blocks shots, grants a defensive Edge against them.")
	add("tile-blue", "Mire", "Lay a Mire tile: entering ends a move; −1 die in every contest.")
	add("tile-red", "Hazard", "Lay a Hazard tile: entering or being pushed in is a Hit.")
	for _, c := range courts {
		term := ""
		if game.Definition.Display != nil {
			term = game.Definition.Display.Terms[c]
		}
		add("court-"+c, courtLabel(c), term)
	}
	add(actRecover, "Recover", "Remove this unit's stone (free).")
	add(actActivate, "Activate", "Two actions, then it gains a white stone.")
	add(actExert, "Exert", "One action, then its white stone turns black.")
	add(actRouse, "Rouse (2)", "Spend 2 Momentum: remove a black stone.")
	add(actSurge, "Surge (1)", "Spend 1 Momentum: shift a non-Exhausted unit one hex.")
	add(actTiles, "Swap tiles", "Stone Court: swap two unoccupied tiles within 2 hexes.")
	add(actClear, "Clear Turn", "No unit acts: collect the Eye, buy VP, remove every stone.")
	add(actMove, "Move", "Move the active unit here.")
	add(actStrike, "Strike", "Secret exchange: split your dice between attack and defense.")
	add(actShoot, "Shoot", "Arbalest: 3 dice at 6+ against a target 2–4 hexes away.")
	add(actShove, "Shove", "Opposed roll of full CV; push the target directly away.")
	add(actEnd, "End activation", "Stop acting with this unit.")
	add(actRespond, "Allocate", "Choose how many of your dice attack.")
	add(actReroll, "Reroll a die", "Spend 1 Momentum to reroll one failed die.")
	add(actKeep, "Keep the roll", "Do not reroll.")
	add(actPush, "Push", "Push the unit to this hex.")
	for _, d := range dirNames {
		add("edge-"+d, "Off the edge ("+d+")", "Push it off the edge: it suffers a Hit and stays put.")
	}
	add(actShift, "Shift", "Hit and Run: shift one hex.")
	add(actStay, "Stay", "Do not shift.")
	add(actSeize, "Seize the Tempo (2)", "Spend 2 Momentum to take another turn now.")
	add(actDone, "End turn", "Pass the turn to your opponent.")
	return info
}
