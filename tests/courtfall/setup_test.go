package courtfall_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

func TestStartIsTheEmptyArenaAwaitingTheDraft(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	d := data(t, st)
	if d.Phase != "terrain" || d.Left != [3]int{4, 3, 3} {
		t.Fatalf("start data = %+v", d)
	}
	if state.FirstSeat(g) != light {
		t.Fatalf("first seat = %d, want Light to draft first", state.FirstSeat(g))
	}
	if g.Board.Len() != 37 {
		t.Fatalf("arena has %d hexes, want 37", g.Board.Len())
	}
	for _, seat := range []string{"white", "black"} {
		if n := st.ReserveTotal(seat); n != 5 {
			t.Errorf("%s reserve = %d, want the five units", seat, n)
		}
	}
	legal := notations(g, st, light)
	if len(legal) == 0 {
		t.Fatal("no draft moves")
	}
	if legal2 := notations(g, st, dark); len(legal2) != 0 {
		t.Fatalf("Dark has %d moves before Light drafts", len(legal2))
	}
}

func TestDraftKeepsTilesOutOfZonesAndTheEye(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	for _, n := range notations(g, st, light) {
		for _, bad := range []string{"@d4", "@a6", "@e6", "@a7", "@d7", "@d1", "@g1", "@c2", "@g2"} {
			if len(n) >= len(bad) && n[len(n)-len(bad):] == bad {
				t.Fatalf("draft offers %s, which is the Eye or in a deployment zone", n)
			}
		}
	}
	// 18 hexes × 3 colours.
	if got := len(notations(g, st, light)); got != 54 {
		t.Fatalf("opening draft moves = %d, want 54", got)
	}
}

func TestDraftForbidsAdjacentHazards(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	st, _ = play(t, g, st, light, "tile:red@d3")
	// d3's neighbours are e3, c3, c4 (d4 is the Eye, d2 and e2 are Dark's zone).
	legal := notations(g, st, dark)
	for _, hex := range []string{"e3", "c3", "c4"} {
		if has(legal, "tile:red@"+hex) {
			t.Errorf("a Hazard may not sit next to another: tile:red@%s offered", hex)
		}
		if !has(legal, "tile:green@"+hex) {
			t.Errorf("a non-Hazard tile may sit next to a Hazard: tile:green@%s missing", hex)
		}
	}
	if has(legal, "tile:red@d3") || has(legal, "tile:green@d3") {
		t.Error("an occupied tile slot was offered again")
	}
}

func TestDraftLimitsSpecialTilesTouchingEachOther(t *testing.T) {
	g := game(t)
	// Three special tiles around c4: the fourth special tile touching one of
	// them would give it a third special neighbour… build the crowd directly.
	p := pos{phase: "terrain", tiles: map[string]string{"c3": "green", "b4": "green", "d3": "blue"}}
	st := p.build()
	d := data(t, st)
	d.Left = [3]int{2, 2, 3}
	// c4 touches c3, b4 and d3: a special tile there would have three.
	if has(notations(g, st, light), "tile:green@c4") || has(notations(g, st, light), "tile:blue@c4") {
		t.Fatalf("a special tile touching three others was offered: %v", notations(g, st, light))
	}
}

func TestEveryRandomDraftPlacesAllTenTiles(t *testing.T) {
	g := game(t)
	for n := 0; n < 60; n++ {
		rng := rand.New(rand.NewSource(int64(n)))
		st, _ := state.InitialStateSeeded(g, uint64(n+1))
		seat := light
		placed := 0
		for data(t, st).Phase == "terrain" {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("draft %d dead-ended after %d tiles", n, placed)
			}
			res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
			if err != nil {
				t.Fatal(err)
			}
			st, seat = res.NewState, res.NextTurnSeat
			placed++
		}
		if placed != 10 {
			t.Fatalf("draft %d placed %d tiles, want 10", n, placed)
		}
		d := data(t, st)
		counts := map[string]int{}
		for _, c := range d.Tiles {
			counts[c]++
		}
		if counts["green"] != 4 || counts["blue"] != 3 || counts["red"] != 3 {
			t.Fatalf("draft %d tiles = %v", n, counts)
		}
		if err := state.Validate(g, st); err != nil {
			t.Fatalf("draft %d invalid: %v", n, err)
		}
	}
}

func TestDraftAlternatesThenCourtsThenDeployment(t *testing.T) {
	g := game(t)
	rng := rand.New(rand.NewSource(5))
	st := state.InitialState(g)
	seat := light
	var order []int
	for data(t, st).Phase == "terrain" {
		order = append(order, seat)
		legal := state.LegalMoves(g, st, seat)
		res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
		if err != nil {
			t.Fatal(err)
		}
		st, seat = res.NewState, res.NextTurnSeat
	}
	for i, s := range order {
		if want := 1 + i%2; s != want {
			t.Fatalf("draft move %d by seat %d, want %d", i, s, want)
		}
	}
	// Courts: Dark chooses first.
	if seat != dark {
		t.Fatalf("after the draft seat %d to move, want Dark to pick a Court first", seat)
	}
	if !has(notations(g, st, dark), "court:bastion") || len(notations(g, st, dark)) != 5 {
		t.Fatalf("court choices = %v", notations(g, st, dark))
	}
	st, _ = play(t, g, st, dark, "court:veiled")
	st, _ = play(t, g, st, light, "court:gale")
	d := data(t, st)
	if d.Phase != "deploy" || d.Courts != [2]string{"gale", "veiled"} {
		t.Fatalf("after courts: phase %q courts %v", d.Phase, d.Courts)
	}
	// Light deploys first, only in its own back two rows (a6–e6, a7–d7).
	for _, n := range notations(g, st, light) {
		cell := n[len(n)-2:]
		if cell[1] != '6' && cell[1] != '7' {
			t.Fatalf("Light may deploy at %s", n)
		}
	}
	refuse(t, g, st, dark, "deploy:Q@d2")
	st, _ = play(t, g, st, light, "deploy:Q@c6")
	for _, n := range notations(g, st, dark) {
		cell := n[len(n)-2:]
		if cell[1] != '1' && cell[1] != '2' {
			t.Fatalf("Dark may deploy at %s", n)
		}
	}
	refuse(t, g, st, light, "deploy:P@a6")
	st, _ = play(t, g, st, dark, "deploy:Q@d2")
	refuse(t, g, st, light, "deploy:Q@b6") // already deployed
	deploys := []struct {
		seat int
		n    string
	}{
		{light, "deploy:P@d6"}, {dark, "deploy:P@e2"},
		{light, "deploy:N@b6"}, {dark, "deploy:N@f2"},
		{light, "deploy:B@a6"}, {dark, "deploy:B@c2"},
		{light, "deploy:R@d7"}, {dark, "deploy:R@g2"},
	}
	for i, m := range deploys {
		var res *state.MoveResult
		st, res = play(t, g, st, m.seat, m.n)
		if i < len(deploys)-1 && res.NextTurnSeat == m.seat {
			t.Fatalf("deployment did not alternate at %s", m.n)
		}
	}
	d = data(t, st)
	if d.Phase != "play" || d.Turn != dark || d.Stage != "main" {
		t.Fatalf("after deployment: %+v — Dark should take the first turn", d)
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatalf("deployed position invalid: %v", err)
	}
}
