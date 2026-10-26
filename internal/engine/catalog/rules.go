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
		if seatColors[s.Color] {
			return nil, fmt.Errorf("catalog: duplicate seat color %q", s.Color)
		}
		seatColors[s.Color] = true
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
		colors[s.Color] = true
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
