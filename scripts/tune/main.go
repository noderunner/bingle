//go:build ignore

// Command tune searches for a guide tuning that makes one pick's bot win more
// often, in a game whose practice study has knobs (see internal/tune and
// batch.Study): a Cthulhu Wars cult in a private build (ADR 0127), for instance,
//
//	go run -tags private scripts/tune/main.go -base cthulhu-wars -lineup gc,cc,bg,ys -pick gc -generations 10 -out /tmp/tune-gc.json
//
// It prints every generation, saves the result after each one (and resumes from
// it with -resume), and ends by comparing the tuning with the shipped one on
// fresh games. -ship FILE merges the result into the game's file of shipped
// tunings (Cthulhu Wars' is internal/games/private/cthulhuwars/tuned.json), which every
// bot of that pick then plays with — only do that when the validation is
// convincing.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/tune"
)

func main() {
	var (
		base     = flag.String("base", "", "game (required)")
		lineup   = flag.String("lineup", "", "picks at the table, one per seat, comma separated (required)")
		options  = flag.String("options", "", "the study's options for the table, comma separated (Cthulhu Wars: hp, the High Priests)")
		pick     = flag.String("pick", "", "the pick to tune (required)")
		bot      = flag.String("bot", "tactical", "bot every cult plays")
		gens     = flag.Int("generations", 10, "generations")
		pop      = flag.Int("pop", 6, "mutations per generation")
		games    = flag.Int("games", 120, "games per candidate")
		confirm  = flag.Int("confirm", 0, "games to confirm a change (default 2x games)")
		validate = flag.Int("validate", 400, "games of the final comparison")
		sigma    = flag.Float64("sigma", 0.15, "mutation size as a fraction of a knob's range")
		knobs    = flag.String("knobs", "", "knobs to search, comma separated (default all)")
		weights  = flag.String("weights", "", "weights of the study's measures in the fitness, name=w comma separated (Cthulhu Wars: doom=0.25 by default, books=0; raise books for a cult that rarely wins)")
		seed     = flag.Int64("seed", 1, "seed")
		workers  = flag.Int("workers", 0, "games at once (default 3/4 of the CPUs)")
		out      = flag.String("out", "", "file to save the result in")
		resume   = flag.Bool("resume", false, "start from the best tuning saved in -out")
		ship     = flag.String("ship", "", "merge the result into this file of shipped tunings")
	)
	flag.Parse()
	if *base == "" || *lineup == "" || *pick == "" {
		log.Fatal("-base, -lineup and -pick are required")
	}

	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			log.Fatal(err)
		}
	}
	cfg := tune.Config{
		Base: *base, Lineup: strings.Split(*lineup, ","), Pick: *pick, Bot: *bot,
		Generations: *gens, Population: *pop, GamesPerEval: *games, ConfirmGames: *confirm, ValidateGames: *validate,
		Sigma: *sigma, Seed: *seed, Workers: *workers,
	}
	if *options != "" {
		cfg.Options = strings.Split(*options, ",")
	}
	if *knobs != "" {
		cfg.Knobs = strings.Split(*knobs, ",")
	}
	if *weights != "" {
		cfg.Weights = map[string]float64{}
		for _, kv := range strings.Split(*weights, ",") {
			name, val, ok := strings.Cut(kv, "=")
			w, err := strconv.ParseFloat(val, 64)
			if !ok || err != nil {
				log.Fatalf("-weights: %q is not name=number", kv)
			}
			cfg.Weights[name] = w
		}
	}
	var prior tune.Result
	if *resume && *out != "" {
		if raw, err := os.ReadFile(*out); err == nil {
			if json.Unmarshal(raw, &prior) == nil {
				cfg.Init = prior.Best
				fmt.Printf("resuming from %s: %v\n", *out, prior.Best)
			}
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	save := func(r tune.Result) {
		if *out == "" {
			return
		}
		raw, _ := json.MarshalIndent(r, "", "  ")
		_ = os.WriteFile(*out, raw, 0o644)
	}
	res, err := tune.Run(ctx, cat, cfg, func(s tune.Step, r tune.Result) {
		verdict := "kept the parent"
		if s.Accepted {
			verdict = "ACCEPTED"
		}
		fmt.Printf("gen %2d  parent %.3f (win %.0f%%)  best mutation %.3f (win %.0f%%)", s.Generation, s.ParentEval.Fitness, s.ParentEval.Rate*100, s.BestEval.Fitness, s.BestEval.Rate*100)
		if s.ConfirmBest.Games > 0 {
			fmt.Printf("  confirm %.3f vs %.3f", s.ConfirmBest.Fitness, s.ConfirmParent.Fitness)
		}
		fmt.Printf("  %s  sigma %.2f\n", verdict, s.Sigma)
		save(r)
	})
	if err != nil {
		fmt.Println("stopped:", err)
	}
	save(res)
	raw, _ := json.Marshal(res.Best)
	fmt.Printf("\nbest tuning for %s: %s\n", res.Pick, raw)
	if v := res.Validation; v != nil {
		fmt.Printf("validation on %d fresh games: tuned wins %.1f%% %v, shipped %.1f%% %v, z = %+.2f\n",
			v.Tuned.Games, v.Tuned.Rate*100, v.Tuned.Means, v.Shipped.Rate*100, v.Shipped.Means, v.Z)
		if v.Z < 1.64 {
			fmt.Println("not convincing (z < 1.64): do not ship this one; run more games or generations.")
		}
	}
	if *ship != "" && res.Validation != nil {
		all := map[string]batch.Tuning{}
		if b, err := os.ReadFile(*ship); err == nil {
			_ = json.Unmarshal(b, &all)
		}
		all[res.Pick] = res.Best
		b, _ := json.MarshalIndent(all, "", "  ")
		if err := os.WriteFile(*ship, append(b, '\n'), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println("shipped into", *ship)
	}
}
