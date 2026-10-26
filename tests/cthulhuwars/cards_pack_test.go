package cthulhuwars_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// packTable derives a game with the cults and options given.
func packTable(t testing.TB, base string, keys []string, opts map[string]any) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	var l []any
	for _, k := range keys {
		l = append(l, k)
	}
	o := map[string]any{"factions": l}
	for k, v := range opts {
		o[k] = v
	}
	id, err := cat.Derive(base, o)
	if err != nil {
		t.Fatalf("derive %s %v %v: %v", base, keys, opts, err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("derived game %q is not registered", id)
	}
	return g
}

var allCards = map[string]any{
	"azathoth": true, "dreamlands": true, "beyond": true,
	"igoo-abhoth": true, "igoo-chaugnar": true, "igoo-cthugha": true, "igoo-hydra": true, "igoo-yig": true,
}

// Random games with every pack of Loyalty Cards in the box stay valid and end.
func TestRandomGamesWithEveryPack(t *testing.T) {
	tables := []struct {
		base string
		keys []string
	}{
		{"cthulhu-wars", []string{"gc", "cc", "bg", "ys"}},
		{"cthulhu-wars-5p", []string{"gc", "ww", "sl", "ow", "tt"}},
		{"cthulhu-wars-3p", []string{"bg", "ys", "gc"}},
	}
	seeds := 6
	if testing.Short() {
		seeds = 2
	}
	for _, c := range tables {
		g := packTable(t, c.base, c.keys, allCards)
		for seed := int64(1); seed <= int64(seeds); seed++ {
			name := fmt.Sprintf("%s seed %d", g.Definition.ID, seed)
			st, ply, done := randomGame(t, g, seed, 12000, func(ply int, st *state.BoardState, seat int, mv state.Move) {
				if err := state.Validate(g, st); err != nil {
					t.Fatalf("%s ply %d after %s: %v\n%s", name, ply, state.FormatMove(g, mv), err, state.RenderText(g, st))
				}
			})
			if !done {
				t.Errorf("%s: no end after %d plies\n%s", name, ply, state.RenderText(g, st))
			}
		}
	}
}

// Guided games (the bots' own filter) with every pack in the box are legal,
// never give a Gate up, and reach an end.
func TestGuidedGamesWithEveryPack(t *testing.T) {
	for _, keys := range [][]string{{"gc", "cc", "bg", "ys"}, {"ww", "sl", "ow", "tt"}} {
		g := packTable(t, "cthulhu-wars", keys, allCards)
		finished := 0
		for seed := int64(1); seed <= 6; seed++ {
			s := playGuided(t, g, seed, 9000)
			if s.abandons != 0 {
				t.Errorf("%v seed %d: bots abandoned %d Gates", keys, seed, s.abandons)
			}
			if s.ended {
				finished++
			}
		}
		if finished < 3 {
			t.Errorf("%v: only %d of 6 guided games finished", keys, finished)
		}
	}
}

// Every kind of bot plays on with every pack in the box: no decision is left
// unanswered or rejected, and the positions stay valid.
func TestEveryKindOfBotPlaysWithEveryPack(t *testing.T) {
	g := packTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, allCards)
	kinds := []string{"tactical", "mcts", "minimax", "random"}
	plies := 140
	if testing.Short() {
		plies = 60
	}
	for round := 0; round < 1; round++ {
		bs := make([]bots.Bot, 4)
		for i := range bs {
			b, ok := bots.MakeConfigured(kinds[(i+round)%len(kinds)], int64(10*round+i+1), bots.Config{MCTSIterations: 15, MinimaxNodes: 400})
			if !ok {
				t.Fatalf("no bot %s", kinds[(i+round)%len(kinds)])
			}
			bs[i] = b
		}
		st, err := state.InitialStateSeeded(g, uint64(500+round))
		if err != nil {
			t.Fatal(err)
		}
		seat := state.FirstSeat(g)
		for ply := 0; ply < plies; ply++ {
			legal := state.LegalMoves(g, st, seat)
			mv, err := bs[seat-1].Choose(context.Background(), g, st, seat, legal)
			if err != nil {
				t.Fatalf("round %d ply %d: %s: %v", round, ply, bs[seat-1].ID(), err)
			}
			res, err := state.Apply(g, st, seat, mv)
			if err != nil {
				t.Fatalf("round %d ply %d: %s chose %s: %v", round, ply, bs[seat-1].ID(), state.FormatMove(g, mv), err)
			}
			if err := state.Validate(g, res.NewState); err != nil {
				t.Fatalf("round %d ply %d after %s: %v", round, ply, state.FormatMove(g, mv), err)
			}
			if res.GameEnded {
				break
			}
			st, seat = res.NewState, res.NextTurnSeat
		}
	}
}
