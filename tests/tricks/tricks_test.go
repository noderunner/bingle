// Package tricks_test checks the Tricks fixture (tests/testdata/tricks)
// itself, so the platform tests that lean on it lean on something sound.
package tricks_test

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/tests/testdata/tricks"
)

func games(t *testing.T) map[string]*catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(tricks.Module()); err != nil {
		t.Fatal(err)
	}
	out := map[string]*catalog.CompiledGame{}
	for _, id := range []string{"test-tricks-3", tricks.ID, "test-tricks-5"} {
		g, ok := cat.Get(id)
		if !ok {
			t.Fatalf("no game %q", id)
		}
		out[id] = g
	}
	return out
}

// Every table plays to an end: the board is crossed off, every card is played,
// and the result says each seat's points.
func TestTricks_EveryTablePlaysToAnEnd(t *testing.T) {
	for id, g := range games(t) {
		rs := g.Rules.(state.Ruleset)
		for seed := uint64(1); seed <= 20; seed++ {
			st, err := rs.Start(g, seed)
			if err != nil {
				t.Fatal(err)
			}
			rng := rand.New(rand.NewSource(int64(seed)))
			plies := 0
			for seat := rs.ImpulseSeat(g, st); seat != 0; seat = rs.ImpulseSeat(g, st) {
				legal := rs.Legal(g, st, seat)
				if len(legal) == 0 {
					t.Fatalf("%s seed %d: seat %d has no move at ply %d", id, seed, seat, plies)
				}
				res, err := rs.Apply(g, st, seat, legal[rng.Intn(len(legal))])
				if err != nil {
					t.Fatalf("%s seed %d: %v", id, seed, err)
				}
				if err := rs.Validate(g, res.NewState); err != nil {
					t.Fatalf("%s seed %d: %v", id, seed, err)
				}
				st, plies = res.NewState, plies+1
			}
			n, hand := len(g.Definition.Seats), g.ParamInt("hand", 8)
			if want := 5 + n*hand; plies != want {
				t.Errorf("%s seed %d: %d plies, want %d (five boxes, then every card)", id, seed, plies, want)
			}
			if out := rs.Outcome(g, st); !out.Ended || strings.Count(out.Detail, ",") != n-1 {
				t.Errorf("%s seed %d: outcome %+v", id, seed, out)
			}
		}
	}
}

// A hand is seen by its owner alone, and a determinized position keeps the
// viewer's own hand while dealing the others again.
func TestTricks_AHandIsItsOwnersAlone(t *testing.T) {
	g := games(t)[tricks.ID]
	rs := g.Rules.(state.HiddenInfo)
	st, err := rs.Start(g, 7)
	if err != nil {
		t.Fatal(err)
	}
	hands := func(st *state.BoardState) map[int]int {
		raw, err := json.Marshal(st.Ext())
		if err != nil {
			t.Fatal(err)
		}
		var ext struct {
			View struct {
				Table []struct {
					ID    string `json:"id"`
					Owner int    `json:"owner"`
					Cards []struct {
						ID string `json:"id"`
					} `json:"cards"`
				} `json:"table"`
			} `json:"view"`
		}
		if err := json.Unmarshal(raw, &ext); err != nil {
			t.Fatal(err)
		}
		seen := map[int]int{}
		for _, z := range ext.View.Table {
			if z.ID == "hand" {
				for _, c := range z.Cards {
					if c.ID != "" {
						seen[z.Owner]++
					}
				}
			}
		}
		return seen
	}
	if got := hands(rs.Redact(g, st, 2)); got[2] != 8 || len(got) != 1 {
		t.Errorf("seat 2 sees the cards of %v, want only its own 8", got)
	}
	if got := hands(rs.Redact(g, st, 0)); len(got) != 0 {
		t.Errorf("a spectator sees the cards of %v", got)
	}
	again := rs.Determinize(g, st, 1, rand.New(rand.NewSource(1)))
	if err := rs.Validate(g, again); err != nil {
		t.Fatal(err)
	}
	if a, b := hands(rs.Redact(g, st, 1)), hands(rs.Redact(g, again, 1)); a[1] != b[1] {
		t.Errorf("the viewer's hand changed: %v %v", a, b)
	}
}

// The deck is plain enough to wear a set of cards (catalog.CompiledCards.Standard).
func TestTricks_TheDeckCanWearASet(t *testing.T) {
	if !games(t)[tricks.ID].Cards.Standard() {
		t.Error("the deck cannot wear a set of cards")
	}
}
