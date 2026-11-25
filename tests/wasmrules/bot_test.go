package wasmrules_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/tests/testdata"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

// searcherOf is a game's rules as a searcher, which a module with the bot
// call is.
func searcherOf(t *testing.T, game *catalog.CompiledGame) state.Searcher {
	t.Helper()
	s, ok := state.SearcherOf(game)
	if !ok {
		t.Fatalf("%s's rules do not search", game.Definition.ID)
	}
	return s
}

// quick is a search at each level that is bounded by its work, not by time.
var quick = []state.SearchRequest{
	{Level: "tactical"},
	{Level: "minimax", Nodes: 150},
	{Level: "mcts", Iterations: 10, Plies: 40},
}

func listedIn(legal []state.Selection, mv state.Selection) bool {
	for _, l := range legal {
		if l.Key() == mv.Key() {
			return true
		}
	}
	return false
}

// The search inside the module answers with one of the seat's legal
// selections, at every level, all through games played by its answers.
func TestTheSearchInsideTheModuleChoosesLegalSelections(t *testing.T) {
	for id, game := range games(t, draughtsModule(t).Rules()) {
		s := searcherOf(t, game)
		for _, req := range quick {
			t.Run(id+"/"+req.Level, func(t *testing.T) {
				st, err := state.InitialStateSeeded(game, 3)
				if err != nil {
					t.Fatal(err)
				}
				seat := state.StartSeat(game, st)
				for ply := 0; ply < 40; ply++ {
					legal := state.LegalSelections(game, st, seat)
					req.Seed = uint64(ply + 1)
					res, err := s.Search(context.Background(), game, st, seat, req)
					if err != nil {
						t.Fatalf("selection %d: %v", ply, err)
					}
					if !listedIn(legal, res.Move) {
						t.Fatalf("selection %d: %+v is not one of the %d legal ones", ply, res.Move, len(legal))
					}
					r, err := state.Apply(game, st, seat, res.Move)
					if err != nil {
						t.Fatal(err)
					}
					if r.GameEnded {
						return
					}
					st, seat = r.NewState, r.NextImpulseSeat
				}
			})
		}
	}
}

// The same request gives the same answer: twice on one module, on a new
// module, and through the guest's dispatch with no module at all.
func TestTheSearchInsideTheModuleIsDeterministic(t *testing.T) {
	st, seat, _ := midgame(t, nativeGames(t)[draughts.ID])
	sides := map[string]*catalog.CompiledGame{
		"the module":     games(t, draughtsModule(t).Rules())[draughts.ID],
		"a new module":   games(t, load(t, newRuntime(t, wasmrules.Options{Instances: 1}), testdata.DraughtsWasm).Rules())[draughts.ID],
		"in process too": games(t, wasmrules.Encoded(draughts.New))[draughts.ID],
	}
	for _, req := range quick {
		req.Seed = 1<<63 + 99
		first, err := searcherOf(t, sides["the module"]).Search(context.Background(), sides["the module"], st, seat, req)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"the module", "a new module", "in process too"} {
			got, err := searcherOf(t, sides[name]).Search(context.Background(), sides[name], st, seat, req)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, first) {
				t.Errorf("%s at %s: %+v, first %+v", name, req.Level, got, first)
			}
		}
	}
}

// A search keeps to its budget and to its context's deadline, and answers.
// Cancelled, it stops at once, and the module plays on without counting a
// fault.
//
// A module built with TinyGo is asked too, where there is a TinyGo: its
// runtime reads the wall clock where Go's reads the monotonic one, so both
// run while a search does.
func TestTheSearchInsideTheModuleStopsInTime(t *testing.T) {
	builds := map[string]func(testing.TB, string, ...string) []byte{"Go": testdata.BuildWasm, "TinyGo": testdata.BuildWasmTinyGo}
	for name, build := range builds {
		t.Run(name, func(t *testing.T) { searchStopsInTime(t, build(t, testdata.DraughtsWasm)) })
	}
}

func searchStopsInTime(t *testing.T, wasm []byte) {
	mod, err := newRuntime(t, wasmrules.Options{Instances: 1}).Load(context.Background(), "fixture", wasm)
	if err != nil {
		t.Fatal(err)
	}
	game := games(t, mod.Rules())[draughts.ID]
	s := searcherOf(t, game)
	st, seat, _ := midgame(t, nativeGames(t)[draughts.ID])
	for _, level := range []string{"minimax", "mcts"} {
		t.Run(level, func(t *testing.T) {
			req := state.SearchRequest{Level: level, Budget: 300 * time.Millisecond, Nodes: 1 << 30, Depth: 64, Seed: 1}
			start := time.Now()
			if _, err := s.Search(context.Background(), game, st, seat, req); err != nil {
				t.Fatal(err)
			}
			if took := time.Since(start); took < 250*time.Millisecond || took > time.Second {
				t.Errorf("a search with 300 ms took %s", took)
			}

			req.Budget = time.Minute
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			start = time.Now()
			_, err := s.Search(ctx, game, st, seat, req)
			cancel()
			if err != nil {
				t.Fatalf("a search within its context's second: %v", err)
			}
			if took := time.Since(start); took > time.Second {
				t.Errorf("a search within its context's second took %s", took)
			}

			ctx, cancel = context.WithCancel(context.Background())
			time.AfterFunc(100*time.Millisecond, cancel)
			start = time.Now()
			_, err = s.Search(ctx, game, st, seat, req)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("a cancelled search answered %v", err)
			}
			if took := time.Since(start); took > time.Second {
				t.Errorf("a search cancelled at 100 ms took %s", took)
			}
			if faults, _ := mod.Faults(); faults != 0 {
				t.Errorf("%d faults counted", faults)
			}
			if _, err := s.Search(context.Background(), game, st, seat, quick[2]); err != nil {
				t.Errorf("the module does not search after a cancelled search: %v", err)
			}
		})
	}
}

// Every built-in bot plays a pack's game to its end with the search inside
// the module, and a module built without the bot call plays it through the
// host's own search.
func TestBotsPlayAModuleAtEveryLevel(t *testing.T) {
	without, err := newRuntime(t, wasmrules.Options{}).Load(context.Background(), "fixture", testdata.BuildWasm(t, testdata.DraughtsWasm, testdata.NoBot))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := without.Rules().(state.Searcher); ok {
		t.Fatal("a module without bingle_bot is a searcher")
	}
	for name, rules := range map[string]state.Ruleset{"inside": draughtsModule(t).Rules(), "without bot": without.Rules()} {
		game := games(t, rules)[draughts.SmallID]
		for _, level := range []string{"random", "tactical", "minimax", "mcts"} {
			t.Run(fmt.Sprintf("%s/%s", name, level), func(t *testing.T) {
				before := bots.NodeCounts()[level]
				bot, _ := bots.MakeConfigured(level, 7, bots.Config{MCTSIterations: 20, MinimaxNodes: 200})
				other := bots.NewRandom(8)
				st, err := state.InitialStateSeeded(game, 5)
				if err != nil {
					t.Fatal(err)
				}
				seat := state.StartSeat(game, st)
				for ply := 0; ; ply++ {
					if ply == 300 {
						t.Fatal("the game did not end in 300 selections")
					}
					legal := state.LegalSelections(game, st, seat)
					player := bot
					if seat == 2 {
						player = other
					}
					mv, err := player.Choose(context.Background(), game, st, seat, legal)
					if err != nil {
						t.Fatal(err)
					}
					r, err := state.Apply(game, st, seat, mv)
					if err != nil {
						t.Fatal(err)
					}
					if r.GameEnded {
						break
					}
					st, seat = r.NewState, r.NextImpulseSeat
				}
				if level == "minimax" || level == "mcts" {
					if bots.NodeCounts()[level] == before {
						t.Errorf("%s counted no positions", level)
					}
				}
			})
		}
	}
}
