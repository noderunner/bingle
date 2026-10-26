//go:build ignore

// Command genpuzzles regenerates internal/puzzles/puzzles.json: it plays
// bot games, takes the positions shortly before the winner's decisive moves,
// proves with the exhaustive solver that the side to move wins by force, and
// keeps the most instructive ones (few winning first moves, many pieces).
//
//	go run scripts/genpuzzles/main.go -out internal/puzzles/puzzles.json
package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/puzzles"
)

type plan struct {
	game   string
	games  int   // bot games to mine
	depths []int // solver depths in plies (1 = win in one move)
	keep   int   // puzzles to keep per depth
	budget time.Duration
}

var plans = []plan{
	{"tak-4x4", 500, []int{1, 3}, 4, 10 * time.Second},
	{"tak-5x5", 600, []int{1, 3}, 5, 15 * time.Second},
	{"tak-6x6", 300, []int{1, 3}, 3, 25 * time.Second},
	{"connect-four", 500, []int{1, 3, 5}, 4, 10 * time.Second},
	{"yavalath", 500, []int{1, 3}, 3, 10 * time.Second},
	{"havannah-4", 400, []int{1, 3}, 3, 10 * time.Second},
	{"hex-7", 400, []int{1, 3}, 3, 10 * time.Second},
	{"breakthrough", 400, []int{1, 3}, 3, 10 * time.Second},
	{"draughts", 400, []int{1, 3}, 3, 10 * time.Second},
	{"brandubh", 600, []int{1, 3}, 4, 10 * time.Second},
	{"pente", 300, []int{1}, 3, 10 * time.Second},
}

type candidate struct {
	p      puzzles.Puzzle
	pieces int
}

func main() {
	out := flag.String("out", "internal/puzzles/puzzles.json", "output file")
	seed := flag.Int64("seed", 1, "random seed")
	only := flag.String("game", "", "only this game")
	flag.Parse()

	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			log.Fatal(err)
		}
	}

	var all []puzzles.Puzzle
	for _, pl := range plans {
		if *only != "" && pl.game != *only {
			continue
		}
		game, ok := cat.Get(pl.game)
		if !ok {
			log.Fatalf("unknown game %s", pl.game)
		}
		start := time.Now()
		cands := mine(game, pl, *seed)
		for _, depth := range pl.depths {
			var list []candidate
			for _, c := range cands {
				if c.p.Plies == depth {
					list = append(list, c)
				}
			}
			// Most instructive first: few winning first moves, then more
			// pieces on the board, then ID for determinism.
			sort.Slice(list, func(i, j int) bool {
				a, b := list[i], list[j]
				if len(a.p.FirstMoves) != len(b.p.FirstMoves) {
					return len(a.p.FirstMoves) < len(b.p.FirstMoves)
				}
				if a.pieces != b.pieces {
					return a.pieces > b.pieces
				}
				return a.p.ID < b.p.ID
			})
			n := pl.keep
			if n > len(list) {
				n = len(list)
			}
			for _, c := range list[:n] {
				all = append(all, c.p)
			}
			fmt.Printf("%-13s plies=%d candidates=%d kept=%d\n", pl.game, depth, len(list), n)
		}
		fmt.Printf("%-13s done in %v\n", pl.game, time.Since(start).Round(time.Second))
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].GameDefID != all[j].GameDefID {
			return all[i].GameDefID < all[j].GameDefID
		}
		if all[i].Plies != all[j].Plies {
			return all[i].Plies < all[j].Plies
		}
		return all[i].ID < all[j].ID
	})
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d puzzles to %s\n", len(all), *out)
}

// mine plays games and collects proven puzzles from their final plies.
func mine(game *catalog.CompiledGame, pl plan, seed int64) []candidate {
	var mu sync.Mutex
	seen := map[string]bool{}
	var out []candidate

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				for _, c := range candidatesFromGame(game, pl, seed+int64(i)) {
					mu.Lock()
					if !seen[c.p.ID] {
						seen[c.p.ID] = true
						out = append(out, c)
					}
					mu.Unlock()
				}
			}
		}()
	}
	for i := 0; i < pl.games; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

func candidatesFromGame(game *catalog.CompiledGame, pl plan, seed int64) []candidate {
	rng := rand.New(rand.NewSource(seed))
	botIDs := []string{"random", "tactical"}
	players := map[int]bots.Bot{}
	for _, sd := range game.Definition.Seats {
		b, _ := bots.MakeConfigured(botIDs[rng.Intn(len(botIDs))], rng.Int63(), bots.Config{})
		players[sd.Seat] = b
	}

	type snap struct {
		st   *state.BoardState
		seat int
	}
	st, seat := state.InitialState(game), state.FirstSeat(game)
	history := []snap{{st, seat}}
	var winner *int
	ctx := context.Background()
	for ply := 0; ply < 200; ply++ {
		legal := state.LegalMoves(game, st, seat)
		if len(legal) == 0 {
			return nil
		}
		mv, err := players[seat].Choose(ctx, game, st, seat, legal)
		if err != nil {
			return nil
		}
		res, err := state.Apply(game, st, seat, mv)
		if err != nil {
			return nil
		}
		if res.GameEnded {
			winner = res.WinnerSeat
			break
		}
		st, seat = res.NewState, res.NextTurnSeat
		history = append(history, snap{st, seat})
	}
	if winner == nil {
		return nil
	}

	var out []candidate
	last := len(history) - 1 // index of the position before the final move
	for _, depth := range pl.depths {
		idx := last - (depth - 1)
		if idx < 0 || history[idx].seat != *winner {
			continue
		}
		pos := history[idx]
		solver := puzzles.NewSolver(game)
		cctx, cancel := context.WithTimeout(ctx, pl.budget)
		// Not trivially shorter: nothing wins in fewer plies.
		if depth > 1 {
			if len(solver.WinningMoves(cctx, pos.st, pos.seat, depth-2)) > 0 && depth-2 >= 1 {
				cancel()
				continue
			}
		}
		wins := solver.WinningMoves(cctx, pos.st, pos.seat, depth)
		timedOut := cctx.Err() != nil
		cancel()
		if timedOut || len(wins) == 0 || len(wins) > 3 {
			continue
		}
		doc, err := pos.st.MarshalJSON()
		if err != nil {
			continue
		}
		sum := sha1.Sum(append([]byte(game.Definition.ID+fmt.Sprint(pos.seat, depth)), doc...))
		p := puzzles.Puzzle{
			ID:        fmt.Sprintf("%s-w%d-%s", game.Definition.ID, (depth+1)/2, hex.EncodeToString(sum[:])[:6]),
			GameDefID: game.Definition.ID,
			ToMove:    pos.seat,
			WinIn:     (depth + 1) / 2,
			Plies:     depth,
			State:     doc,
		}
		if sd, ok := game.Seat(pos.seat); ok {
			who := sd.Label
			if who == "" {
				who = sd.Color
			}
			if p.WinIn == 1 {
				p.Title = who + " to move: win at once"
			} else {
				p.Title = fmt.Sprintf("%s to move: win in %d", who, p.WinIn)
			}
		}
		if game.Definition.Notation == "ptn" {
			if tps, err := state.FormatTPS(game, pos.st, pos.seat); err == nil {
				p.TPS = tps
			}
		}
		for _, mv := range wins {
			p.FirstMoves = append(p.FirstMoves, state.FormatMove(game, mv))
		}
		sort.Strings(p.FirstMoves)
		lctx, lcancel := context.WithTimeout(ctx, pl.budget)
		p.Solution = solver.Line(lctx, pos.st, pos.seat, wins[0], depth)
		lcancel()
		out = append(out, candidate{p: p, pieces: len(pos.st.Occupied())})
	}
	return out
}
