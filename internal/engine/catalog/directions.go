package catalog

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

// compassVectors maps compass names to grid vectors.
var compassVectors = map[string][2]int{
	"N": {0, -1}, "NE": {1, -1}, "E": {1, 0}, "SE": {1, 1},
	"S": {0, 1}, "SW": {-1, 1}, "W": {-1, 0}, "NW": {-1, -1},
}

// resolveCustodian compiles an action's custodian block (placements and
// slides) into ra.
func resolveCustodian(cg *CompiledGame, ad dsl.ActionDef, ra *ResolvedAction) error {
	if ad.Custodian == nil {
		return nil
	}
	def := cg.Definition
	if !cg.Board.IsGrid() {
		return fmt.Errorf("catalog: action %q (custodian) requires a grid board", ad.ID)
	}
	lengths := ad.Custodian.Lengths
	if len(lengths) == 0 {
		lengths = []int{2}
	}
	for _, l := range lengths {
		if l < 1 || l > 8 {
			return fmt.Errorf("catalog: action %q custodian lengths must be between 1 and 8", ad.ID)
		}
	}
	ra.CustodianLengths = append([]int(nil), lengths...)
	for _, name := range ad.Custodian.Anvils {
		ids, ok := cg.Board.Region(name)
		if !ok {
			return fmt.Errorf("catalog: action %q custodian anvil region %q is unknown", ad.ID, name)
		}
		if ra.Anvils == nil {
			ra.Anvils = map[board.PositionID]bool{}
		}
		for _, id := range ids {
			ra.Anvils[id] = true
		}
	}
	if p := ad.Custodian.CountIn; p != "" {
		if _, ok := def.Reserves[p]; !ok {
			return fmt.Errorf("catalog: action %q counts captures in unknown pool %q (declare it in reserves)", ad.ID, p)
		}
		ra.CountPool = p
	}
	return nil
}

// resolveSeatDirections resolves direction names for every seat (relative
// names such as "forward" use the seat's orientation). An empty list means
// unrestricted and returns nil.
func resolveSeatDirections(cg *CompiledGame, names []string) (map[int][]board.Direction, error) {
	if len(names) == 0 {
		return nil, nil
	}
	out := map[int][]board.Direction{}
	for _, sd := range cg.Definition.Seats {
		var fv [2]int
		haveFwd := false
		if sd.Forward != "" {
			if v, ok := compassVectors[sd.Forward]; ok {
				fv, haveFwd = v, true
			}
		}
		for _, dn := range names {
			v, err := resolveDirection(dn, fv, haveFwd)
			if err != nil {
				return nil, err
			}
			out[sd.Seat] = append(out[sd.Seat], board.Direction{Name: dn, DX: v[0], DY: v[1]})
		}
	}
	return out, nil
}

// compileDirections resolves seat orientation, per-piece direction
// restrictions and promotion rules.
func compileDirections(cg *CompiledGame) error {
	def := cg.Definition

	for _, sd := range def.Seats {
		if sd.Forward == "" {
			continue
		}
		if !cg.Board.IsGrid() {
			return fmt.Errorf("catalog: seat %d declares a forward direction but the board is not a grid", sd.Seat)
		}
		v, ok := compassVectors[sd.Forward]
		if !ok || (v[0] != 0 && v[1] != 0) {
			return fmt.Errorf("catalog: seat %d has invalid forward direction %q (use N, E, S or W)", sd.Seat, sd.Forward)
		}
		if cg.Forward == nil {
			cg.Forward = map[int][2]int{}
		}
		cg.Forward[sd.Seat] = v
	}

	for _, name := range cg.PieceTypes {
		pd := def.Pieces[name]
		rp := cg.Pieces[name]

		if len(pd.Movement.Directions) > 0 {
			if !cg.Board.IsGrid() {
				return fmt.Errorf("catalog: piece %q restricts directions but the board is not a grid", name)
			}
			rp.Dirs = map[int][]board.Direction{}
			for _, sd := range def.Seats {
				var dirs []board.Direction
				for _, dn := range pd.Movement.Directions {
					v, err := resolveDirection(dn, cg.Forward[sd.Seat], cg.Forward[sd.Seat] != [2]int{})
					if err != nil {
						return fmt.Errorf("catalog: piece %q: %w", name, err)
					}
					dirs = append(dirs, board.Direction{Name: dn, DX: v[0], DY: v[1]})
				}
				rp.Dirs[sd.Seat] = dirs
			}
		}

		for i, pr := range pd.Promote {
			ids, ok := cg.Board.Region(pr.Region)
			if !ok {
				return fmt.Errorf("catalog: piece %q promotion %d references unknown region %q", name, i, pr.Region)
			}
			if _, ok := def.Pieces[pr.To]; !ok {
				return fmt.Errorf("catalog: piece %q promotes to unknown piece %q", name, pr.To)
			}
			if pr.Seat != 0 {
				if _, ok := cg.Seat(pr.Seat); !ok {
					return fmt.Errorf("catalog: piece %q promotion %d references unknown seat %d", name, i, pr.Seat)
				}
			}
			region := make(map[board.PositionID]bool, len(ids))
			for _, id := range ids {
				region[id] = true
			}
			rp.Promotions = append(rp.Promotions, ResolvedPromotion{Seat: pr.Seat, Region: region, To: pr.To})
		}
		cg.Pieces[name] = rp
	}
	return nil
}

// resolveDirection turns a direction name into a grid vector. Relative names
// need the seat's forward vector.
func resolveDirection(name string, forward [2]int, haveForward bool) ([2]int, error) {
	if v, ok := compassVectors[name]; ok {
		return v, nil
	}
	if !haveForward {
		return [2]int{}, fmt.Errorf("direction %q is relative but the seat declares no \"forward\" direction", name)
	}
	f := forward
	left := [2]int{f[1], -f[0]}
	right := [2]int{-f[1], f[0]}
	back := [2]int{-f[0], -f[1]}
	add := func(a, b [2]int) [2]int { return [2]int{a[0] + b[0], a[1] + b[1]} }
	switch name {
	case "forward":
		return f, nil
	case "backward":
		return back, nil
	case "left":
		return left, nil
	case "right":
		return right, nil
	case "forward-left":
		return add(f, left), nil
	case "forward-right":
		return add(f, right), nil
	case "backward-left":
		return add(back, left), nil
	case "backward-right":
		return add(back, right), nil
	}
	return [2]int{}, fmt.Errorf("unknown direction %q", name)
}
