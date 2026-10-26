package bots_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/envs"
	"github.com/noderunner/bingle/internal/games/classics"
	"github.com/noderunner/bingle/internal/games/tak"
)

// TestStrengthReport prints a bot-vs-bot results table. It is a development
// aid, not an assertion: run it with
//
//	BINGLE_STRENGTH=1 go test ./tests/bots -run StrengthReport -v
func TestStrengthReport(t *testing.T) {
	if os.Getenv("BINGLE_STRENGTH") == "" {
		t.Skip("set BINGLE_STRENGTH=1 to print a bot strength table")
	}
	cat := catalog.NewCatalog()
	for _, m := range append([]catalog.Module{tak.Module()}, classics.Modules()...) {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatal(err)
		}
	}
	games := map[string]int{"tak-3x3": 40, "tak-5x5": 12, "connect-four": 20, "hex-7": 12, "tic-tac-toe": 40}
	pairs := [][2]string{{"tactical", "random"}, {"minimax", "tactical"}, {"mcts", "random"}, {"mcts", "tactical"}, {"mcts", "minimax"}}
	for id, n := range games {
		for _, p := range pairs {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			start := time.Now()
			res, err := envs.PlayMatch(ctx, cat, envs.MatchOptions{GameDefID: id, BotA: p[0], BotB: p[1], Games: n, Seed: 1, BotIterations: 150})
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%-13s %-8s vs %-8s  %2d games: %2d-%2d-%2d (unfinished %d) avg %.0f plies, %s",
				id, p[0], p[1], res.Played, res.AWins, res.BWins, res.Draws, res.Unfinished, res.AvgPlies, time.Since(start).Round(time.Millisecond))
		}
	}
}
