package catalog

import (
	"fmt"
)

func compileWinRules(cg *CompiledGame) error {
	def := cg.Definition
	wc := def.WinCondition

	switch wc.Type {
	case "ruleset":
		return nil
	case "none", "callback":
		if len(wc.Rules) > 0 {
			return fmt.Errorf("catalog: win_condition.type %q does not take rules", wc.Type)
		}
		return nil
	case "rules":
		if len(wc.Rules) == 0 {
			return fmt.Errorf("catalog: win_condition.type \"rules\" needs at least one rule")
		}
	default:
		// Unknown types are rejected when a move is applied, as before;
		// keep Register permissive so v1 behaviour is unchanged.
		return nil
	}

	tagKnown := func(tag string) bool {
		for _, p := range def.Pieces {
			for _, t := range p.Tags {
				if t == tag {
					return true
				}
			}
		}
		return false
	}
	checkFilter := func(i int, tag, piece string) error {
		if tag != "" && !tagKnown(tag) {
			return fmt.Errorf("catalog: win rule %d references tag %q that no piece carries", i, tag)
		}
		if piece != "" {
			if _, ok := def.Pieces[piece]; !ok {
				return fmt.Errorf("catalog: win rule %d references unknown piece %q", i, piece)
			}
		}
		return nil
	}

	for i, r := range wc.Rules {
		if r.Seat != 0 {
			if _, ok := cg.Seat(r.Seat); !ok {
				return fmt.Errorf("catalog: win rule %d references unknown seat %d", i, r.Seat)
			}
		}
		switch r.Result {
		case "", "win", "lose":
		default:
			return fmt.Errorf("catalog: win rule %d has unknown result %q (use \"win\" or \"lose\")", i, r.Result)
		}
		if r.Result == "lose" && r.Type == "end" {
			return fmt.Errorf("catalog: win rule %d: \"result\" does not apply to end rules (use decide)", i)
		}
		switch r.Type {
		case "ring":
			if !cg.Board.IsHex() {
				return fmt.Errorf("catalog: win rule %d (ring) requires a hex board", i)
			}
			if err := checkFilter(i, r.Tag, r.Piece); err != nil {
				return err
			}
		case "connect":
			if len(r.Of) > 0 {
				if len(r.Regions) > 0 {
					return fmt.Errorf("catalog: win rule %d (connect) uses either regions or of/touch, not both", i)
				}
				if len(r.Of) > 32 {
					return fmt.Errorf("catalog: win rule %d (connect) lists too many regions (at most 32)", i)
				}
				if r.Touch < 2 || r.Touch > len(r.Of) {
					return fmt.Errorf("catalog: win rule %d (connect) needs 2 <= touch <= %d (the number of listed regions)", i, len(r.Of))
				}
				for _, name := range r.Of {
					if _, ok := cg.Board.Region(name); !ok {
						return fmt.Errorf("catalog: win rule %d references unknown region %q", i, name)
					}
				}
				if err := checkFilter(i, r.Tag, r.Piece); err != nil {
					return err
				}
				continue
			}
			if len(r.Regions) == 0 {
				return fmt.Errorf("catalog: win rule %d (connect) needs regions", i)
			}
			for _, pair := range r.Regions {
				if len(pair) != 2 {
					return fmt.Errorf("catalog: win rule %d (connect) region groups must be pairs", i)
				}
				for _, name := range pair {
					if _, ok := cg.Board.Region(name); !ok {
						return fmt.Errorf("catalog: win rule %d references unknown region %q", i, name)
					}
				}
			}
			if err := checkFilter(i, r.Tag, r.Piece); err != nil {
				return err
			}
		case "line":
			if !cg.Board.IsGrid() && !cg.Board.IsHex() {
				return fmt.Errorf("catalog: win rule %d (line) requires a grid or hex board", i)
			}
			if r.Length < 2 {
				return fmt.Errorf("catalog: win rule %d (line) needs length >= 2", i)
			}
			if err := checkFilter(i, r.Tag, r.Piece); err != nil {
				return err
			}
		case "absent":
			if _, ok := def.Pieces[r.Piece]; !ok {
				return fmt.Errorf("catalog: win rule %d (absent) references unknown piece %q", i, r.Piece)
			}
			if r.Seat == 0 {
				return fmt.Errorf("catalog: win rule %d (absent) needs the seat that wins", i)
			}
		case "pool":
			if _, ok := def.Reserves[r.Pool]; !ok {
				return fmt.Errorf("catalog: win rule %d references unknown pool %q", i, r.Pool)
			}
			if r.AtLeast < 1 {
				return fmt.Errorf("catalog: win rule %d (pool) needs at_least >= 1", i)
			}
		case "reach":
			if _, ok := cg.Board.Region(r.Region); !ok {
				return fmt.Errorf("catalog: win rule %d references unknown region %q", i, r.Region)
			}
			if err := checkFilter(i, r.Tag, r.Piece); err != nil {
				return err
			}
		case "end":
			if len(r.When) == 0 {
				return fmt.Errorf("catalog: win rule %d (end) needs a \"when\" trigger", i)
			}
			for _, w := range r.When {
				switch w {
				case "board-full", "reserve-empty", "no-legal-moves":
				case "ply-limit":
					if r.Plies < 1 {
						return fmt.Errorf("catalog: win rule %d (ply-limit) needs plies >= 1", i)
					}
				case "passes":
					if r.Count < 1 {
						return fmt.Errorf("catalog: win rule %d (passes) needs count >= 1", i)
					}
					hasPass := false
					for _, a := range cg.Actions {
						if a.Type == ActionPass {
							hasPass = true
						}
					}
					if !hasPass {
						return fmt.Errorf("catalog: win rule %d uses the passes trigger but the game has no pass action", i)
					}
				default:
					return fmt.Errorf("catalog: win rule %d has unknown trigger %q", i, w)
				}
			}
			if r.Decide == nil {
				return fmt.Errorf("catalog: win rule %d (end) needs \"decide\"", i)
			}
			switch r.Decide.Type {
			case "draw", "mover-wins", "mover-loses":
			case "score":
				if !cg.Board.IsGrid() && !cg.Board.IsHex() && cg.Board.Len() == 0 {
					return fmt.Errorf("catalog: win rule %d (score) needs a board", i)
				}
				for k := range r.Decide.Komi {
					seat := 0
					if _, err := fmt.Sscanf(k, "%d", &seat); err != nil {
						return fmt.Errorf("catalog: win rule %d komi key %q is not a seat number", i, k)
					}
					if _, ok := cg.Seat(seat); !ok {
						return fmt.Errorf("catalog: win rule %d komi references unknown seat %d", i, seat)
					}
				}
			case "most":
				if err := checkFilter(i, r.Decide.Tag, r.Decide.Piece); err != nil {
					return err
				}
			default:
				return fmt.Errorf("catalog: win rule %d has unknown decide type %q", i, r.Decide.Type)
			}
		default:
			return fmt.Errorf("catalog: win rule %d has unknown type %q", i, r.Type)
		}
	}
	return nil
}
