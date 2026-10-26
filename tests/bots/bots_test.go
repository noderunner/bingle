// Package bots_test checks the built-in opponents against positions whose
// best move is forced, using Tak as the test game.
package bots_test

import (
	"context"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/tak"
)

func tak5(t testing.TB) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(tak.Module()); err != nil {
		t.Fatal(err)
	}
	g, _ := cat.Get("tak-5x5")
	return g
}

func position(t testing.TB, g *catalog.CompiledGame, tps string) (*state.BoardState, int) {
	t.Helper()
	st, seat, err := state.ParseTPS(g, tps)
	if err != nil {
		t.Fatal(err)
	}
	return st, seat
}

func choose(t testing.TB, b bots.Bot, g *catalog.CompiledGame, st *state.BoardState, seat int) string {
	t.Helper()
	legal := state.LegalMoves(g, st, seat)
	mv, err := b.Choose(context.Background(), g, st, seat, legal)
	if err != nil {
		t.Fatalf("%s.Choose: %v", b.ID(), err)
	}
	return state.FormatMove(g, mv)
}

// White has a1-d1 on rank 1 and e1 empty: e1 wins the road. Each of the
// stronger bots must find it; a Tak mistake costs the game.
const winInOne = "x5/x5/x5/x5/1,1,1,1,x 1 5"

// Black threatens e1 to complete its own road; white must block there.
// (A flat on e1 blocks; so would a wall or capstone.)
const mustBlock = "x5/x5/x5/x5/2,2,2,2,x 1 6"

func TestTacticalAndMCTSTakeAnImmediateWin(t *testing.T) {
	g := tak5(t)
	st, seat := position(t, g, winInOne)
	for _, b := range []bots.Bot{bots.NewTactical(1), bots.NewMinimax(bots.MinimaxOptions{Seed: 1}), bots.NewMCTS(bots.MCTSOptions{Seed: 1, Budget: 200 * time.Millisecond})} {
		text := choose(t, b, g, st, seat)
		mv, err := state.ParseMove(g, text)
		if err != nil {
			t.Fatal(err)
		}
		res, err := state.Apply(g, st, seat, mv)
		if err != nil {
			t.Fatal(err)
		}
		// e1 and Ce1 both complete the road; any winning move is fine.
		if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != seat {
			t.Errorf("%s played %q, which does not win", b.ID(), text)
		}
	}
}

func TestTacticalAndMCTSBlockAnImmediateLoss(t *testing.T) {
	g := tak5(t)
	st, seat := position(t, g, mustBlock)
	for _, b := range []bots.Bot{bots.NewTactical(2), bots.NewMinimax(bots.MinimaxOptions{Seed: 2}), bots.NewMCTS(bots.MCTSOptions{Seed: 2, Budget: 300 * time.Millisecond})} {
		for i := 0; i < 5; i++ {
			mv := choose(t, b, g, st, seat)
			parsed, _ := state.ParseMove(g, mv)
			res, err := state.Apply(g, st, seat, parsed)
			if err != nil {
				t.Fatalf("%s chose illegal %q: %v", b.ID(), mv, err)
			}
			// After the bot's move black must have no road-completing reply.
			for _, reply := range state.LegalMoves(g, res.NewState, res.NextTurnSeat) {
				r2, err := state.Apply(g, res.NewState, res.NextTurnSeat, reply)
				if err == nil && r2.GameEnded && r2.WinnerSeat != nil && *r2.WinnerSeat == res.NextTurnSeat {
					t.Fatalf("%s played %q and allowed black to win with %s", b.ID(), mv, state.FormatMove(g, reply))
				}
			}
		}
	}
}

func TestRandomIsReproducibleWithASeed(t *testing.T) {
	g := tak5(t)
	st := state.InitialState(g)
	a, b := bots.NewRandom(42), bots.NewRandom(42)
	for i := 0; i < 10; i++ {
		if x, y := choose(t, a, g, st, 1), choose(t, b, g, st, 1); x != y {
			t.Fatalf("seeded random bots diverged at pick %d: %q vs %q", i, x, y)
		}
	}
}

func TestRegistryListsBuiltinsWeakestFirst(t *testing.T) {
	r := bots.NewRegistry()
	var ids []string
	for _, info := range r.List() {
		ids = append(ids, info.ID)
		if info.Label == "" || info.Description == "" {
			t.Errorf("bot %q lacks a label/description", info.ID)
		}
	}
	want := []string{"random", "tactical", "minimax", "mcts"}
	if len(ids) != len(want) {
		t.Fatalf("bots = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("bots[%d] = %q, want %q", i, ids[i], want[i])
		}
	}
	if _, ok := bots.Make("nonsense", 1); ok {
		t.Error("Make accepted an unknown bot id")
	}
}

// TestTacticalBeatsRandom plays seeded games on the small board: a bot that
// at least avoids one-move losses and grabs one-move wins must clearly beat
// uniform random play.
func TestTacticalBeatsRandom(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(tak.Module()); err != nil {
		t.Fatal(err)
	}
	g, _ := cat.Get("tak-3x3")

	const games = 24
	wins := 0
	for i := 0; i < games; i++ {
		tactical := bots.NewTactical(int64(100 + i))
		random := bots.NewRandom(int64(200 + i))
		tacticalSeat := 1 + i%2 // alternate sides

		st, seat := state.InitialState(g), state.FirstSeat(g)
		for ply := 0; ply < 200; ply++ {
			legal := state.LegalMoves(g, st, seat)
			b := bots.Bot(random)
			if seat == tacticalSeat {
				b = tactical
			}
			mv, err := b.Choose(context.Background(), g, st, seat, legal)
			if err != nil {
				t.Fatal(err)
			}
			res, err := state.Apply(g, st, seat, mv)
			if err != nil {
				t.Fatal(err)
			}
			st, seat = res.NewState, res.NextTurnSeat
			if res.GameEnded {
				if res.WinnerSeat != nil && *res.WinnerSeat == tacticalSeat {
					wins++
				}
				break
			}
		}
	}
	if wins < games*2/3 {
		t.Errorf("tactical won only %d of %d games against random, want at least %d", wins, games, games*2/3)
	}
	t.Logf("tactical beat random in %d/%d games", wins, games)
}

// Minimax is reproducible for a given seed and always plays legal moves.
func TestMinimaxIsDeterministicForASeed(t *testing.T) {
	g := tak5(t)
	st, seat := position(t, g, "x5/x5/x2,1,x2/x5/x5 2 3")
	a := choose(t, bots.NewMinimax(bots.MinimaxOptions{Seed: 7, NodeBudget: 5000}), g, st, seat)
	b := choose(t, bots.NewMinimax(bots.MinimaxOptions{Seed: 7, NodeBudget: 5000}), g, st, seat)
	if a != b {
		t.Errorf("seeded minimax bots diverged: %q vs %q", a, b)
	}
	if _, err := state.ParseMove(g, a); err != nil {
		t.Errorf("unparseable choice %q: %v", a, err)
	}
}

func TestMinimaxBeatsRandomAndTacticalOnSmallTak(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(tak.Module()); err != nil {
		t.Fatal(err)
	}
	g, _ := cat.Get("tak-3x3")
	play := func(a, b func(seed int64) bots.Bot, games int) int {
		wins := 0
		for i := 0; i < games; i++ {
			first := a(int64(10 + i))
			second := b(int64(500 + i))
			aSeat := 1 + i%2
			st, seat := state.InitialState(g), state.FirstSeat(g)
			for ply := 0; ply < 100; ply++ {
				legal := state.LegalMoves(g, st, seat)
				bot := second
				if seat == aSeat {
					bot = first
				}
				mv, err := bot.Choose(context.Background(), g, st, seat, legal)
				if err != nil {
					t.Fatal(err)
				}
				res, err := state.Apply(g, st, seat, mv)
				if err != nil {
					t.Fatal(err)
				}
				if res.GameEnded {
					if res.WinnerSeat != nil && *res.WinnerSeat == aSeat {
						wins++
					}
					break
				}
				st, seat = res.NewState, res.NextTurnSeat
			}
		}
		return wins
	}
	minimax := func(seed int64) bots.Bot { return bots.NewMinimax(bots.MinimaxOptions{Seed: seed, NodeBudget: 4000}) }
	random := func(seed int64) bots.Bot { return bots.NewRandom(seed) }
	tactical := func(seed int64) bots.Bot { return bots.NewTactical(seed) }

	if w := play(minimax, random, 14); w < 10 {
		t.Errorf("minimax won %d/14 against random, want at least 10", w)
	}
	if w := play(minimax, tactical, 14); w < 7 {
		t.Errorf("minimax won only %d/14 against tactical, want at least half", w)
	}
}

// The connection term of the heuristic is what lets minimax build roads: with
// it, the bot clearly beats the material-only variant on small Tak.
func TestMinimaxConnectivityTermHelpsOnTak(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(tak.Module()); err != nil {
		t.Fatal(err)
	}
	g, _ := cat.Get("tak-3x3")
	wins, losses := 0, 0
	for i := 0; i < 16; i++ {
		withConn := bots.NewMinimax(bots.MinimaxOptions{Seed: int64(i + 1), NodeBudget: 6000})
		without := bots.NewMinimax(bots.MinimaxOptions{Seed: int64(700 + i), NodeBudget: 6000, NoConnectivity: true})
		connSeat := 1 + i%2
		st, seat := state.InitialState(g), state.FirstSeat(g)
		for ply := 0; ply < 100; ply++ {
			legal := state.LegalMoves(g, st, seat)
			b := bots.Bot(without)
			if seat == connSeat {
				b = withConn
			}
			mv, err := b.Choose(context.Background(), g, st, seat, legal)
			if err != nil {
				t.Fatal(err)
			}
			res, err := state.Apply(g, st, seat, mv)
			if err != nil {
				t.Fatal(err)
			}
			if res.GameEnded {
				if res.WinnerSeat != nil {
					if *res.WinnerSeat == connSeat {
						wins++
					} else {
						losses++
					}
				}
				break
			}
			st, seat = res.NewState, res.NextTurnSeat
		}
	}
	if wins < losses+3 {
		t.Errorf("connectivity minimax went %d-%d against the material-only bot, want a clear edge", wins, losses)
	}
	t.Logf("connectivity vs material-only: %d wins, %d losses", wins, losses)
}
