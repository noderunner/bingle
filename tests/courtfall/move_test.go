package courtfall_test

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// A reference implementation of movement, written independently of the
// engine, on axial coordinates read from hex names.

type hexPos struct{ q, r int }

func (h hexPos) String() string { return fmt.Sprintf("%c%d", 'a'+h.q, h.r+1) }

func onBoard(h hexPos) bool {
	return h.q >= 0 && h.r >= 0 && h.q <= 6 && h.r <= 6 && iabs(h.q-3) <= 3 && iabs(h.r-3) <= 3 && iabs(h.q+h.r-6) <= 3
}

func iabs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

var refDirs = []hexPos{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

func hexDist(a, b hexPos) int {
	dq, dr := a.q-b.q, a.r-b.r
	m := iabs(dq)
	if v := iabs(dr); v > m {
		m = v
	}
	if v := iabs(dq + dr); v > m {
		m = v
	}
	return m
}

// refReach lists where piece may move from `from`, given occupied hexes and
// stop tiles (Mire/Hazard).
func refReach(piece string, from hexPos, occupied map[hexPos]bool, stops map[hexPos]bool) []string {
	var out []hexPos
	switch piece {
	case "knight":
		for q := 0; q < 7; q++ {
			for r := 0; r < 7; r++ {
				h := hexPos{q, r}
				if onBoard(h) && h != from && !occupied[h] && hexDist(from, h) <= 4 {
					out = append(out, h)
				}
			}
		}
	case "rook":
		for _, d := range refDirs {
			cur := from
			for i := 0; i < 3; i++ {
				cur = hexPos{cur.q + d.q, cur.r + d.r}
				if !onBoard(cur) || occupied[cur] {
					break
				}
				out = append(out, cur)
				if stops[cur] {
					break
				}
			}
		}
	default:
		m := map[string]int{"pawn": 3, "bishop": 3, "queen": 4}[piece]
		seen := map[hexPos]int{from: 0}
		frontier := []hexPos{from}
		for step := 1; step <= m; step++ {
			var next []hexPos
			for _, c := range frontier {
				if c != from && stops[c] {
					continue
				}
				for _, d := range refDirs {
					n := hexPos{c.q + d.q, c.r + d.r}
					if !onBoard(n) || occupied[n] {
						continue
					}
					if _, ok := seen[n]; ok {
						continue
					}
					seen[n] = step
					next = append(next, n)
					out = append(out, n)
				}
			}
			frontier = next
		}
	}
	names := make([]string, len(out))
	for i, h := range out {
		names[i] = h.String()
	}
	sort.Strings(names)
	return names
}

func TestMovementMatchesAReferenceOnRandomBoards(t *testing.T) {
	g := game(t)
	rng := rand.New(rand.NewSource(11))
	var all []hexPos
	for q := 0; q < 7; q++ {
		for r := 0; r < 7; r++ {
			if h := (hexPos{q, r}); onBoard(h) {
				all = append(all, h)
			}
		}
	}
	pieces := []string{"pawn", "knight", "bishop", "rook", "queen"}
	codes := map[string]string{"pawn": "P", "knight": "N", "bishop": "B", "rook": "R", "queen": "Q"}
	for trial := 0; trial < 300; trial++ {
		perm := rng.Perm(len(all))
		units := map[string]string{}
		occupied := map[hexPos]bool{}
		piece := pieces[trial%5]
		from := all[perm[0]]
		units["white/"+piece] = from.String()
		occupied[from] = true
		k := 0
		for _, p2 := range pieces {
			if p2 == piece || k >= rng.Intn(9) {
				continue
			}
			k++
			h := all[perm[k]]
			units["black/"+p2] = h.String()
			occupied[h] = true
		}
		tiles := map[string]string{}
		stops := map[hexPos]bool{}
		for i := 0; i < rng.Intn(8); i++ {
			h := all[perm[20+i]]
			col := []string{"blue", "red", "green"}[rng.Intn(3)]
			tiles[h.String()] = col
			if col != "green" {
				stops[h] = true
			}
		}
		p := pos{stage: "act", units: units, tiles: tiles, act: actFor(piece)}
		want := refReach(piece, from, occupied, stops)
		var got []string
		for _, n := range notations(g, p.build(), light) {
			if n[0] == codes[piece][0] && len(n) == 3 && n[1] >= 'a' && n[1] <= 'g' {
				got = append(got, n[1:])
			}
		}
		sort.Strings(got)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("trial %d (%s from %s, tiles %v, units %v):\n got  %v\n want %v", trial, piece, from, tiles, units, got, want)
		}
	}
}

func TestMireAndHazardEndAMove(t *testing.T) {
	g := game(t)
	// A Juggernaut on d4 with Mire on e4 and a Hazard on c4: it may step
	// onto them but not travel through.
	u := map[string]string{"white/rook": "d4", "black/queen": "d2"}
	p := pos{stage: "act", units: u, tiles: map[string]string{"e4": "blue", "c4": "red"}, act: actFor("rook")}
	got := notations(g, p.build(), light)
	if !has(got, "Re4") || !has(got, "Rc4") {
		t.Fatalf("Mire and Hazard hexes may be entered: %v", got)
	}
	if has(got, "Rf4") || has(got, "Rb4") {
		t.Fatalf("a move continued past a stopping tile: %v", got)
	}
	// Entering the Hazard is a Hit that earns nobody Momentum.
	st, _ := play(t, g, p.build(), light, "Rc4")
	d := data(t, st)
	if d.Cond["white/rook"] != 1 {
		t.Fatalf("the Hazard should Fatigue the Juggernaut: %v", d.Cond)
	}
	if d.Mom != [2]int{} {
		t.Fatalf("a Hazard's Hit earns nobody Momentum: %v", d.Mom)
	}
	// And Mire, the Warden's detour aside, ends a Warden's move too.
	u = map[string]string{"white/pawn": "a4", "black/queen": "d2"}
	p = pos{stage: "act", units: u, tiles: map[string]string{"b4": "blue", "a5": "blue", "b3": "blue"}, act: actFor("pawn")}
	got = notations(g, p.build(), light)
	for _, bad := range []string{"Pc4", "Pc3", "Pa6", "Pb5"} {
		if has(got, bad) {
			t.Fatalf("%s should be unreachable behind Mire: %v", bad, got)
		}
	}
}

func TestRookMovesOnlyInStraightLinesAndKnightLeaps(t *testing.T) {
	g := game(t)
	// Rook on d4 with a blocker at e4: it cannot pass through and the
	// Knight ignores it.
	u := map[string]string{"white/rook": "d4", "black/pawn": "e4"}
	p := pos{stage: "act", units: u, act: actFor("rook")}
	got := notations(g, p.build(), light)
	if has(got, "Re4") || has(got, "Rf4") || has(got, "Rg4") {
		t.Fatalf("rook went through a unit: %v", got)
	}
	if has(got, "Re5") {
		t.Fatalf("Re5 is not on a straight line from d4: %v", got)
	}
	u2 := map[string]string{"white/knight": "d4", "black/pawn": "e4", "black/rook": "c4", "black/queen": "d3"}
	p2 := pos{stage: "act", units: u2, act: actFor("knight")}
	got = notations(g, p2.build(), light)
	if !has(got, "Nf4") || !has(got, "Ng4") {
		t.Fatalf("the Outrider should leap over the unit at e4: %v", got)
	}
	if has(got, "Ne4") {
		t.Fatalf("the Outrider cannot land on a unit: %v", got)
	}
}

func TestIllegalDestinationsAreRefused(t *testing.T) {
	g := game(t)
	u := map[string]string{"white/pawn": "d4", "black/queen": "e4"}
	p := pos{stage: "act", units: u, act: actFor("pawn")}
	refuse(t, g, p.build(), light, "Pe4") // occupied
	refuse(t, g, p.build(), light, "Pd4") // not a move
	refuse(t, g, p.build(), light, "Pa1") // not on the board
	refuse(t, g, p.build(), light, "Qd5") // not the active unit
	far := pos{stage: "act", units: map[string]string{"white/pawn": "d1", "black/queen": "d7"}, act: actFor("pawn")}
	refuse(t, g, far.build(), light, "Pd5") // four hexes away
}
