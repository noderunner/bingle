package catalog

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/primitives"
)

// compileRules validates and compiles everything beyond board and pieces:
// seats, reserves, setup, actions, opening, win rules and notation.
func compileRules(cg *CompiledGame) error {
	def := cg.Definition

	seatColors, err := checkSeats(def)
	if err != nil {
		return err
	}
	if err := checkReserves(cg); err != nil {
		return err
	}
	if err := checkSetup(cg, seatColors); err != nil {
		return err
	}
	if err := compileCards(cg); err != nil {
		return err
	}
	if err := compileDeal(cg); err != nil {
		return err
	}

	if def.Engine == "ruleset" {
		// A ruleset owns moves, opening and scoring; the document only
		// describes the board, seats, pieces and display.
		if def.TurnOrder.Type != "round-robin" {
			return fmt.Errorf("catalog: a ruleset game needs turn_order.type \"round-robin\" (seat order); got %q", def.TurnOrder.Type)
		}
		return nil
	}

	actionIDs, err := compileActions(cg)
	if err != nil {
		return err
	}
	if err := compileDirections(cg); err != nil {
		return err
	}
	if err := compileOpening(cg, actionIDs); err != nil {
		return err
	}
	if err := compileTurns(cg, actionIDs); err != nil {
		return err
	}
	if err := compileEndOfTurn(cg); err != nil {
		return err
	}
	if err := compilePatience(cg); err != nil {
		return err
	}
	if err := compileWinRules(cg); err != nil {
		return err
	}
	return checkNotation(cg)
}

// checkSeats rejects duplicate or colourless seats and returns the colours in play.
func checkSeats(def *dsl.GameDefinition) (map[string]bool, error) {
	seatNums := map[int]bool{}
	seatColors := map[string]bool{}
	for _, s := range def.Seats {
		if seatNums[s.Seat] {
			return nil, fmt.Errorf("catalog: duplicate seat number %d", s.Seat)
		}
		seatNums[s.Seat] = true
		if strings.TrimSpace(s.Color) == "" {
			return nil, fmt.Errorf("catalog: seat %d has no color", s.Seat)
		}
		if len(s.Colors) > 0 && s.Colors[0] != s.Color {
			return nil, fmt.Errorf("catalog: seat %d's colors must begin with its color %q", s.Seat, s.Color)
		}
		for _, c := range s.Palette() {
			if strings.TrimSpace(c) == "" {
				return nil, fmt.Errorf("catalog: seat %d lists an empty colour", s.Seat)
			}
			if seatColors[c] {
				return nil, fmt.Errorf("catalog: duplicate seat color %q", c)
			}
			seatColors[c] = true
		}
	}
	for _, c := range def.ExtraColors {
		if seatColors[c] {
			return nil, fmt.Errorf("catalog: extra colour %q is a seat's colour", c)
		}
	}
	return seatColors, nil
}

// checkReserves checks that every reserve counts a pool some piece draws from.
func checkReserves(cg *CompiledGame) error {
	def := cg.Definition
	pools := map[string]bool{}
	for _, p := range cg.Pieces {
		pools[p.Pool] = true
	}
	// A pool may also be a plain counter that captures are credited to.
	for _, ad := range def.Actions {
		if ad.Custodian != nil && ad.Custodian.CountIn != "" {
			pools[ad.Custodian.CountIn] = true
		}
	}
	for _, e := range def.EndOfTurn {
		if e.CountIn != "" {
			pools[e.CountIn] = true
		}
	}
	for pool, n := range def.Reserves {
		if !pools[pool] {
			return fmt.Errorf("catalog: reserves reference unknown pool %q (no piece draws from it)", pool)
		}
		if n < 0 {
			return fmt.Errorf("catalog: reserve of %q is negative", pool)
		}
	}
	return nil
}

// checkSetup checks that every starting piece is on the board, is a piece of
// the game and belongs to a seat's colour.
func checkSetup(cg *CompiledGame, seatColors map[string]bool) error {
	def := cg.Definition
	for i, entry := range def.Setup {
		if !cg.Board.HasPosition(board.PositionID(entry.Position)) {
			return fmt.Errorf("catalog: setup entry %d references unknown position %q", i, entry.Position)
		}
		if _, ok := def.Pieces[entry.Piece]; !ok {
			return fmt.Errorf("catalog: setup entry %d references unknown piece %q", i, entry.Piece)
		}
		if !seatColors[entry.Color] {
			return fmt.Errorf("catalog: setup entry %d uses color %q that no seat plays", i, entry.Color)
		}
	}
	return nil
}

// checkPreview checks that the pieces of the sample position are pieces of
// the game, in colours that are played. (Squares the board lacks are skipped
// when the preview is drawn, so a smaller variant can inherit it.)
func checkPreview(cg *CompiledGame) error {
	def := cg.Definition
	colors := map[string]bool{}
	for _, s := range def.Seats {
		for _, c := range s.Palette() {
			colors[c] = true
		}
	}
	for _, c := range def.ExtraColors {
		colors[c] = true
	}
	for i, entry := range def.Preview {
		if _, ok := def.Pieces[entry.Piece]; !ok {
			return fmt.Errorf("catalog: preview entry %d references unknown piece %q", i, entry.Piece)
		}
		if !colors[entry.Color] {
			return fmt.Errorf("catalog: preview entry %d uses color %q that nobody plays", i, entry.Color)
		}
	}
	return nil
}

// compileActions resolves the declared actions (plus the implicit step of
// pieces that have a movement primitive) into cg.Actions and returns the set
// of action ids.
func compileActions(cg *CompiledGame) (map[string]bool, error) {
	def := cg.Definition
	anyStep := false
	for _, p := range cg.Pieces {
		if p.Primitive != nil {
			anyStep = true
		}
	}
	if anyStep {
		cg.Actions = append(cg.Actions, ResolvedAction{ID: ActionStep, Type: ActionStep, Label: "Move"})
	}

	seen := map[string]bool{ActionStep: anyStep}
	for _, ad := range def.Actions {
		if strings.TrimSpace(ad.ID) == "" {
			return nil, fmt.Errorf("catalog: action missing id")
		}
		if seen[ad.ID] {
			return nil, fmt.Errorf("catalog: duplicate action id %q", ad.ID)
		}
		seen[ad.ID] = true

		ra := ResolvedAction{ID: ad.ID, Type: ad.Type, Label: ad.Label}
		if ra.Label == "" {
			ra.Label = ad.ID
		}
		if err := compileAction(cg, ad, &ra); err != nil {
			return nil, err
		}
		cg.Actions = append(cg.Actions, ra)
	}
	return seen, nil
}

// compileAction fills ra from the declaration ad, by the action's type.
func compileAction(cg *CompiledGame, ad dsl.ActionDef, ra *ResolvedAction) error {
	switch ad.Type {
	case ActionPlace:
		return compilePlace(cg, ad, ra)
	case ActionMoveStack:
		return compileMoveStack(cg, ad, ra)
	case ActionJump:
		if !cg.Board.IsGrid() {
			return fmt.Errorf("catalog: action %q (jump) requires a grid board", ad.ID)
		}
		ra.Mandatory = ad.Mandatory
		ra.Chain = ad.Chain
		return nil
	case ActionSlide:
		return compileSlide(cg, ad, ra)
	case ActionLeap:
		return compileLeap(cg, ad, ra)
	case ActionSwap:
		return compileSwap(cg, ad, ra)
	case ActionPass:
		return nil // a pass takes no parameters
	case ActionStep:
		return compileStep(cg, ad, ra)
	case ActionCardMove:
		return compileCardMove(cg, ad, ra)
	}
	return fmt.Errorf("catalog: action %q has unknown type %q", ad.ID, ad.Type)
}

// listedPieces checks that an action of the given kind names its pieces and
// that each one is a piece of the game.
func listedPieces(cg *CompiledGame, ad dsl.ActionDef, kind string) error {
	if len(ad.Pieces) == 0 {
		return fmt.Errorf("catalog: %s action %q must list its pieces", kind, ad.ID)
	}
	return knownPieces(cg, ad, kind)
}

// knownPieces checks that each piece an action names is a piece of the game.
func knownPieces(cg *CompiledGame, ad dsl.ActionDef, kind string) error {
	for _, p := range ad.Pieces {
		if _, ok := cg.Definition.Pieces[p]; !ok {
			return fmt.Errorf("catalog: %s action %q references unknown piece %q", kind, ad.ID, p)
		}
	}
	return nil
}

// occupancyOf is the occupancy an action declares, empty-only by default.
func occupancyOf(ad dsl.ActionDef, kind string) (primitives.Occupancy, error) {
	switch occ := primitives.Occupancy(ad.Occupancy); occ {
	case primitives.OccupancyEmptyOnly, primitives.OccupancyCaptureOnly, primitives.OccupancyCaptureOrEmpty:
		return occ, nil
	case "":
		return primitives.OccupancyEmptyOnly, nil
	}
	return "", fmt.Errorf("catalog: %s action %q has unknown occupancy %q", kind, ad.ID, ad.Occupancy)
}

// regionSet is the positions of a named region of the board as a set.
func regionSet(cg *CompiledGame, name string) (map[board.PositionID]bool, bool) {
	ids, ok := cg.Board.Region(name)
	if !ok {
		return nil, false
	}
	set := make(map[board.PositionID]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, true
}

func compilePlace(cg *CompiledGame, ad dsl.ActionDef, ra *ResolvedAction) error {
	ra.Pieces = ad.Pieces
	if len(ra.Pieces) == 0 {
		ra.Pieces = append([]string(nil), cg.PieceTypes...)
	}
	for _, p := range ra.Pieces {
		if _, ok := cg.Definition.Pieces[p]; !ok {
			return fmt.Errorf("catalog: action %q places unknown piece %q", ad.ID, p)
		}
	}
	switch ad.Source {
	case "", SourceReserve:
		ra.Source = SourceReserve
	case SourceSupply:
		ra.Source = SourceSupply
	default:
		return fmt.Errorf("catalog: action %q has unknown source %q", ad.ID, ad.Source)
	}
	if ad.Region != "" {
		region, ok := regionSet(cg, ad.Region)
		if !ok {
			return fmt.Errorf("catalog: action %q references unknown region %q", ad.ID, ad.Region)
		}
		ra.Region = region
	}
	if ad.Flips != nil {
		if !cg.Board.IsGrid() {
			return fmt.Errorf("catalog: action %q (flips) requires a grid board", ad.ID)
		}
		ra.Flips = true
		ra.FlipsRequired = ad.Flips.Require
	}
	if ad.Captures != nil {
		ra.Captures = true
		ra.AllowSuicide = ad.Captures.AllowSuicide
	}
	if err := resolveCustodian(cg, ad, ra); err != nil {
		return err
	}
	if ad.Ko {
		if ad.Captures == nil {
			return fmt.Errorf("catalog: action %q sets ko without liberty captures", ad.ID)
		}
		ra.Ko = true
		cg.KeepsHistory = true
	}
	if ad.Gravity != "" {
		found := false
		for _, d := range cg.Directions {
			if d.Name == ad.Gravity {
				ra.HasGravity, ra.GravityDX, ra.GravityDY = true, d.DX, d.DY
				found = true
			}
		}
		if !found {
			return fmt.Errorf("catalog: action %q has unknown gravity direction %q (grid boards only)", ad.ID, ad.Gravity)
		}
	}
	return nil
}

func compileMoveStack(cg *CompiledGame, ad dsl.ActionDef, ra *ResolvedAction) error {
	if !cg.Board.IsGrid() {
		return fmt.Errorf("catalog: action %q (move-stack) requires a grid board", ad.ID)
	}
	ra.CarryLimit = cg.Board.Size()
	if q := ad.CarryLimit; q != nil {
		switch q.Keyword {
		case "":
			if q.N < 1 {
				return fmt.Errorf("catalog: action %q carry_limit must be >= 1", ad.ID)
			}
			ra.CarryLimit = q.N
		case "board-size":
			ra.CarryLimit = cg.Board.Size()
		case "board-width":
			ra.CarryLimit = cg.Board.Width()
		case "board-height":
			ra.CarryLimit = cg.Board.Height()
		default:
			return fmt.Errorf("catalog: action %q has unknown carry_limit keyword %q", ad.ID, q.Keyword)
		}
	}
	if ad.Flatten != nil {
		ra.Flatten = *ad.Flatten
	}
	return nil
}

func compileSlide(cg *CompiledGame, ad dsl.ActionDef, ra *ResolvedAction) error {
	if !cg.Board.IsGrid() {
		return fmt.Errorf("catalog: action %q (slide) requires a grid board", ad.ID)
	}
	if err := listedPieces(cg, ad, "slide"); err != nil {
		return err
	}
	ra.Pieces = ad.Pieces
	occ, err := occupancyOf(ad, "slide")
	if err != nil {
		return err
	}
	ra.SlideOccupancy = occ
	if ad.MaxDistance < 0 {
		return fmt.Errorf("catalog: slide action %q has a negative max_distance", ad.ID)
	}
	ra.MaxDistance = ad.MaxDistance
	dirs, err := resolveSeatDirections(cg, ad.Directions)
	if err != nil {
		return fmt.Errorf("catalog: slide action %q: %w", ad.ID, err)
	}
	ra.SlideDirs = dirs
	if ad.Then != nil {
		if ra.Then, err = compileShot(cg, ad, ra); err != nil {
			return err
		}
	}
	if err := resolveCustodian(cg, ad, ra); err != nil {
		return err
	}
	if ad.ForbidRegion != "" {
		region, ok := regionSet(cg, ad.ForbidRegion)
		if !ok {
			return fmt.Errorf("catalog: slide action %q forbids unknown region %q", ad.ID, ad.ForbidRegion)
		}
		ra.ForbidLanding = region
	}
	return nil
}

// compileShot resolves the shot that follows a slide (Amazons' arrow): by
// default it goes the way and as far as the slide itself.
func compileShot(cg *CompiledGame, ad dsl.ActionDef, ra *ResolvedAction) (*ResolvedThen, error) {
	if ad.Then.Type != "shoot" {
		return nil, fmt.Errorf("catalog: slide action %q: unknown then type %q (use \"shoot\")", ad.ID, ad.Then.Type)
	}
	if _, ok := cg.Definition.Pieces[ad.Then.Piece]; !ok {
		return nil, fmt.Errorf("catalog: slide action %q shoots unknown piece %q", ad.ID, ad.Then.Piece)
	}
	if ad.Then.MaxDistance < 0 {
		return nil, fmt.Errorf("catalog: slide action %q has a negative then.max_distance", ad.ID)
	}
	th := &ResolvedThen{Piece: ad.Then.Piece, Dirs: ra.SlideDirs, MaxDistance: ra.MaxDistance}
	if ad.Then.MaxDistance > 0 {
		th.MaxDistance = ad.Then.MaxDistance
	}
	if len(ad.Then.Directions) > 0 {
		dirs, err := resolveSeatDirections(cg, ad.Then.Directions)
		if err != nil {
			return nil, fmt.Errorf("catalog: slide action %q: %w", ad.ID, err)
		}
		th.Dirs = dirs
	}
	return th, nil
}

// compileCardMove resolves a card-move action (ADR 0120): a hand that is a
// seat zone, a pass_on that is a table zone, cards with moves, and a forward
// on every seat to read them by.
func compileCardMove(cg *CompiledGame, ad dsl.ActionDef, ra *ResolvedAction) error {
	if err := listedPieces(cg, ad, "card-move"); err != nil {
		return err
	}
	cc := cg.Cards
	if cc == nil {
		return fmt.Errorf("catalog: card-move action %q needs cards", ad.ID)
	}
	if !cg.Board.IsGrid() {
		return fmt.Errorf("catalog: card-move action %q requires a grid board", ad.ID)
	}
	if z, ok := cc.Zones[ad.Hand]; !ok || z.Scope != "seat" {
		return fmt.Errorf("catalog: card-move action %q needs hand, a seat zone (got %q)", ad.ID, ad.Hand)
	}
	if ad.PassOn != "" {
		if z, ok := cc.Zones[ad.PassOn]; !ok || z.Scope != "table" {
			return fmt.Errorf("catalog: card-move action %q passes cards on to %q, which is not a table zone", ad.ID, ad.PassOn)
		}
	}
	switch ad.Stuck {
	case "":
	case "spend":
		ra.StuckSpend = true
	default:
		return fmt.Errorf("catalog: card-move action %q has stuck %q, not spend", ad.ID, ad.Stuck)
	}
	moves := false
	for _, t := range cc.Types {
		moves = moves || len(t.Moves) > 0
	}
	if !moves {
		return fmt.Errorf("catalog: card-move action %q, but no card lists moves", ad.ID)
	}
	for _, sd := range cg.Definition.Seats {
		if sd.Forward == "" {
			return fmt.Errorf("catalog: card-move action %q reads cards by each seat's forward, and seat %d has none", ad.ID, sd.Seat)
		}
	}
	ra.Pieces = ad.Pieces
	ra.Hand, ra.PassOn = ad.Hand, ad.PassOn
	return nil
}

func compileLeap(cg *CompiledGame, ad dsl.ActionDef, ra *ResolvedAction) error {
	if err := listedPieces(cg, ad, "leap"); err != nil {
		return err
	}
	if ad.MaxDistance < 1 {
		return fmt.Errorf("catalog: leap action %q needs max_distance >= 1", ad.ID)
	}
	if ad.CloneDistance < 0 || ad.CloneDistance > ad.MaxDistance {
		return fmt.Errorf("catalog: leap action %q needs 0 <= clone_distance <= max_distance", ad.ID)
	}
	ra.Pieces = ad.Pieces
	ra.MaxDistance = ad.MaxDistance
	ra.CloneDistance = ad.CloneDistance
	ra.Infect = ad.Infect
	return nil
}

func compileSwap(cg *CompiledGame, ad dsl.ActionDef, ra *ResolvedAction) error {
	if len(cg.Definition.Seats) != 2 {
		return fmt.Errorf("catalog: swap action %q requires exactly two seats", ad.ID)
	}
	ra.SwapPlies = map[int]bool{}
	plies := ad.Plies
	if len(plies) == 0 {
		plies = []int{1}
	}
	for _, p := range plies {
		if p < 0 {
			return fmt.Errorf("catalog: swap action %q has a negative ply", ad.ID)
		}
		ra.SwapPlies[p] = true
	}
	return nil
}

func compileStep(cg *CompiledGame, ad dsl.ActionDef, ra *ResolvedAction) error {
	if len(ad.Pieces) == 0 {
		return fmt.Errorf("catalog: step action %q must list its pieces", ad.ID)
	}
	if ad.ID == ActionStep {
		return fmt.Errorf("catalog: step action id %q is reserved for the implicit step", ad.ID)
	}
	if err := knownPieces(cg, ad, "step"); err != nil {
		return err
	}
	occ, err := occupancyOf(ad, "step")
	if err != nil {
		return err
	}
	ra.StepOccupancy = occ
	ra.Pieces = ad.Pieces
	ra.Explicit = true
	if len(ad.Directions) == 0 {
		return nil
	}
	if !cg.Board.IsGrid() {
		return fmt.Errorf("catalog: step action %q restricts directions but the board is not a grid", ad.ID)
	}
	ra.StepDirs = map[int][]board.Direction{}
	for _, sd := range cg.Definition.Seats {
		// "forward" and the other relative directions are read from the seat's own forward.
		var forward [2]int
		haveForward := false
		if v, ok := compassVectors[sd.Forward]; ok {
			forward, haveForward = v, true
		}
		for _, name := range ad.Directions {
			v, err := resolveDirection(name, forward, haveForward)
			if err != nil {
				return fmt.Errorf("catalog: step action %q: %w", ad.ID, err)
			}
			ra.StepDirs[sd.Seat] = append(ra.StepDirs[sd.Seat], board.Direction{Name: name, DX: v[0], DY: v[1]})
		}
	}
	return nil
}

// compileTurns resolves turns of several selections (ADR 0115): the game's
// own and the opening's. A game that declares one gets the done action.
func compileTurns(cg *CompiledGame, actionIDs map[string]bool) error {
	def := cg.Definition
	var err error
	if cg.Turn, err = compileTurn(def.TurnOrder.Turn, actionIDs, "turn_order.turn"); err != nil {
		return err
	}
	if op := def.TurnOrder.Opening; op != nil && cg.Opening != nil {
		if cg.Opening.Turn, err = compileTurn(op.Turn, actionIDs, "turn_order.opening.turn"); err != nil {
			return err
		}
	}
	if cg.Turn == nil && (cg.Opening == nil || cg.Opening.Turn == nil) {
		return nil
	}
	if actionIDs[ActionDone] {
		return fmt.Errorf("catalog: %q is the action that ends a turn early: a game with a turn may not declare an action of that id", ActionDone)
	}
	cg.Actions = append(cg.Actions, ResolvedAction{ID: ActionDone, Type: ActionDone, Label: "Done"})
	return nil
}

func compileTurn(parts []dsl.TurnPartDef, actionIDs map[string]bool, where string) ([]TurnPart, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	out := make([]TurnPart, len(parts))
	for i, p := range parts {
		if len(p.Actions) == 0 {
			return nil, fmt.Errorf("catalog: %s part %d names no actions", where, i+1)
		}
		tp := TurnPart{Actions: map[string]bool{}, Min: 1}
		for _, a := range p.Actions {
			if !actionIDs[a] {
				return nil, fmt.Errorf("catalog: %s part %d references unknown action %q", where, i+1, a)
			}
			tp.Actions[a] = true
		}
		if p.Min != nil {
			tp.Min = *p.Min
		}
		tp.Max = p.Max
		if tp.Max == 0 {
			tp.Max = max(tp.Min, 1)
		}
		if tp.Min < 0 || tp.Max < tp.Min {
			return nil, fmt.Errorf("catalog: %s part %d takes from %d to %d selections", where, i+1, tp.Min, tp.Max)
		}
		switch p.Distinct {
		case "":
		case "piece":
			tp.DistinctPiece = true
		default:
			return nil, fmt.Errorf("catalog: %s part %d: distinct %q is not known (only \"piece\")", where, i+1, p.Distinct)
		}
		out[i] = tp
	}
	return out, nil
}

// compileOpening resolves the opening phase, in which only some actions and
// pieces are allowed for the first plies.
func compileOpening(cg *CompiledGame, actionIDs map[string]bool) error {
	def := cg.Definition
	op := def.TurnOrder.Opening
	if op == nil {
		return nil
	}
	if op.Plies < 1 {
		return fmt.Errorf("catalog: opening plies must be >= 1")
	}
	ro := &ResolvedOpening{Plies: op.Plies}
	if len(op.Actions) > 0 {
		ro.Actions = map[string]bool{}
		for _, a := range op.Actions {
			if !actionIDs[a] {
				return fmt.Errorf("catalog: opening references unknown action %q", a)
			}
			ro.Actions[a] = true
		}
	}
	if len(op.Pieces) > 0 {
		ro.Pieces = map[string]bool{}
		for _, p := range op.Pieces {
			if _, ok := def.Pieces[p]; !ok {
				return fmt.Errorf("catalog: opening references unknown piece %q", p)
			}
			ro.Pieces[p] = true
		}
	}
	switch op.Owner {
	case "", "mover":
	case "opponent":
		if len(def.Seats) != 2 {
			return fmt.Errorf("catalog: opening owner \"opponent\" requires exactly two seats")
		}
		ro.Opponent = true
	default:
		return fmt.Errorf("catalog: opening has unknown owner %q", op.Owner)
	}
	cg.Opening = ro
	return nil
}

// checkNotation checks that the notation style the game asks for can be used on its board.
func checkNotation(cg *CompiledGame) error {
	def := cg.Definition
	switch def.Notation {
	case "", "generic":
	case "ptn":
		if !cg.Board.IsGrid() || def.Board.Generator.Labels != string(board.LabelAlgebraic) {
			return fmt.Errorf("catalog: notation \"ptn\" requires a grid board with algebraic labels")
		}
		if cg.Board.Width() > 9 || cg.Board.Height() > 9 {
			return fmt.Errorf("catalog: notation \"ptn\" supports boards up to 9x9")
		}
	default:
		return fmt.Errorf("catalog: unknown notation %q", def.Notation)
	}
	return nil
}

// compileEndOfTurn resolves the effects at the end of a turn (ADR 0116).
func compileEndOfTurn(cg *CompiledGame) error {
	def := cg.Definition
	for i, e := range def.EndOfTurn {
		if e.Effect != EffectRemoveGroups {
			return fmt.Errorf("catalog: end_of_turn %d: unknown effect %q", i, e.Effect)
		}
		if e.Condition != ConditionNoLiberties {
			return fmt.Errorf("catalog: end_of_turn %d: unknown condition %q", i, e.Condition)
		}
		whose := e.Whose
		switch whose {
		case "":
			whose = WhoseOpponents
		case WhoseOpponents, WhoseMover, WhoseAll:
		default:
			return fmt.Errorf("catalog: end_of_turn %d: whose is %q, not opponents, mover or all", i, e.Whose)
		}
		if _, ok := def.Reserves[e.CountIn]; e.CountIn != "" && !ok {
			return fmt.Errorf("catalog: end_of_turn %d counts in %q, which reserves does not declare", i, e.CountIn)
		}
		if e.Tint != "" {
			if _, ok := def.Display.TintOf(e.Tint); !ok {
				return fmt.Errorf("catalog: end_of_turn %d: display.tints declares no %q", i, e.Tint)
			}
		}
		cg.EndOfTurn = append(cg.EndOfTurn, TurnEffect{Effect: e.Effect, Condition: e.Condition, Whose: whose, CountIn: e.CountIn, Tint: e.Tint})
	}
	return nil
}

// compileDeal resolves the deal at the start (ADR 0119): every zone named is
// one of the game's, a shuffle fills a table zone from a deck, a seat zone
// gets `each` and a table zone `count`, and no deal takes more cards than its
// zone holds by then.
func compileDeal(cg *CompiledGame) error {
	def := cg.Definition
	if len(def.Deal) == 0 {
		return nil
	}
	if def.Engine == "ruleset" {
		return fmt.Errorf("catalog: a ruleset deals its own cards; deal is for declared games")
	}
	cc := cg.Cards
	if cc == nil {
		return fmt.Errorf("catalog: deal needs cards")
	}
	seats := len(def.Seats)
	held := map[string]int{} // cards in each table zone, and in one seat's zone
	zone := func(i int, id string) (dsl.ZoneDef, error) {
		z, ok := cc.Zones[id]
		if !ok {
			return z, fmt.Errorf("catalog: deal %d names unknown zone %q", i, id)
		}
		return z, nil
	}
	for i, d := range def.Deal {
		if d.Shuffle != "" {
			if d.From != "" || d.To != "" || d.Each != 0 || d.Count != 0 || d.Face != "" {
				return fmt.Errorf("catalog: deal %d shuffles and deals at once", i)
			}
			z, err := zone(i, d.Shuffle)
			if err != nil {
				return err
			}
			if z.Scope != "table" {
				return fmt.Errorf("catalog: deal %d shuffles %q, a seat's zone", i, d.Shuffle)
			}
			deck := d.Deck
			if deck == "" {
				if len(cc.Instances) != 1 {
					return fmt.Errorf("catalog: deal %d must name its deck (the game has %d)", i, len(cc.Instances))
				}
				for name := range cc.Instances {
					deck = name
				}
			}
			ids, ok := cc.Instances[deck]
			if !ok {
				return fmt.Errorf("catalog: deal %d names unknown deck %q", i, deck)
			}
			held[d.Shuffle] = len(ids)
			cg.Deal = append(cg.Deal, DealStep{Shuffle: d.Shuffle, Deck: deck})
			continue
		}
		if d.Deck != "" {
			return fmt.Errorf("catalog: deal %d names a deck without shuffling", i)
		}
		from, err := zone(i, d.From)
		if err != nil {
			return err
		}
		if from.Scope != "table" {
			return fmt.Errorf("catalog: deal %d deals from %q, a seat's zone", i, d.From)
		}
		to, err := zone(i, d.To)
		if err != nil {
			return err
		}
		n := d.Count
		if to.Scope == "seat" {
			if d.Count != 0 || d.Each <= 0 {
				return fmt.Errorf("catalog: deal %d to the seat zone %q needs each, not count", i, d.To)
			}
			n = d.Each * seats
		} else if d.Each != 0 || d.Count <= 0 {
			return fmt.Errorf("catalog: deal %d to the table zone %q needs count, not each", i, d.To)
		}
		if n > held[d.From] {
			return fmt.Errorf("catalog: deal %d takes %d cards from %q, which holds %d", i, n, d.From, held[d.From])
		}
		up := false
		switch d.Face {
		case "", "down":
		case "up":
			up = true
		default:
			return fmt.Errorf("catalog: deal %d has face %q, not up or down", i, d.Face)
		}
		held[d.From] -= n
		if to.Scope == "table" {
			held[d.To] += n
		}
		cg.Deal = append(cg.Deal, DealStep{From: d.From, To: d.To, Each: d.Each, Count: d.Count, Up: up, ToSeats: to.Scope == "seat"})
	}
	return nil
}
