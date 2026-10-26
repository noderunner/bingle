//go:build ignore

// Command tune searches for a guide tuning that makes one cult's bot win more
// often (see internal/tune):
//
//	go run scripts/tune/main.go -faction gc -generations 10 -out /tmp/tune-gc.json
//
// It prints every generation, saves the result after each one (and resumes from
// it with -resume), and ends by comparing the tuning with the shipped one on
// fresh games. -ship merges the result into
// internal/games/cthulhuwars/tuned.json, which every bot of that cult then
// plays with — only do that when the validation is convincing.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
	"github.com/noderunner/bingle/internal/tune"
)

func main() {
	var (
		base     = flag.String("base", "cthulhu-wars", "layout")
		lineup   = flag.String("lineup", "gc,cc,bg,ys", "cults at the table, comma separated")
		hp       = flag.Bool("hp", false, "High Priests expansion")
		faction  = flag.String("faction", "gc", "the cult to tune")
		bot      = flag.String("bot", "tactical", "bot every cult plays")
		gens     = flag.Int("generations", 10, "generations")
		pop      = flag.Int("pop", 6, "mutations per generation")
		games    = flag.Int("games", 120, "games per candidate")
		confirm  = flag.Int("confirm", 0, "games to confirm a change (default 2x games)")
		validate = flag.Int("validate", 400, "games of the final comparison")
		sigma    = flag.Float64("sigma", 0.15, "mutation size as a fraction of a knob's range")
		knobs    = flag.String("knobs", "", "knobs to search, comma separated (default all)")
		bookW    = flag.Float64("book-weight", 0, "weight of the spellbooks held (out of 6) in the fitness; use for cults that rarely win")
		doomW    = flag.Float64("doom-weight", 0.25, "weight of the Doom reached in the fitness")
		seed     = flag.Int64("seed", 1, "seed")
		workers  = flag.Int("workers", 0, "games at once (default 3/4 of the CPUs)")
		out      = flag.String("out", "", "file to save the result in")
		resume   = flag.Bool("resume", false, "start from the best tuning saved in -out")
		ship     = flag.Bool("ship", false, "merge the result into tuned.json")
	)
	flag.Parse()

	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			log.Fatal(err)
		}
	}
	cfg := tune.Config{
		Base: *base, Lineup: strings.Split(*lineup, ","), HighPriests: *hp, Faction: *faction, Bot: *bot,
		Generations: *gens, Population: *pop, GamesPerEval: *games, ConfirmGames: *confirm, ValidateGames: *validate,
		Sigma: *sigma, DoomWeight: *doomW, BookWeight: *bookW, Seed: *seed, Workers: *workers,
	}
	if *knobs != "" {
		cfg.Knobs = strings.Split(*knobs, ",")
	}
	var prior tune.Result
	if *resume && *out != "" {
		if raw, err := os.ReadFile(*out); err == nil {
			if json.Unmarshal(raw, &prior) == nil {
				t := prior.Best
				cfg.Init = &t
				fmt.Printf("resuming from %s: %+v\n", *out, t)
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
	fmt.Printf("\nbest tuning for %s: %s\n", res.Faction, raw)
	if v := res.Validation; v != nil {
		fmt.Printf("validation on %d fresh games: tuned wins %.1f%% (doom %.1f, books %.2f), shipped %.1f%% (doom %.1f, books %.2f), z = %+.2f\n",
			v.Tuned.Games, v.Tuned.Rate*100, v.Tuned.Doom, v.Tuned.Books, v.Shipped.Rate*100, v.Shipped.Doom, v.Shipped.Books, v.Z)
		if v.Z < 1.64 {
			fmt.Println("not convincing (z < 1.64): do not ship this one; run more games or generations.")
		}
	}
	if *ship && res.Validation != nil {
		const path = "internal/games/cthulhuwars/tuned.json"
		all := map[string]cthulhuwars.GuideTuning{}
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &all)
		}
		all[res.Faction] = res.Best
		b, _ := json.MarshalIndent(all, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println("shipped into", path)
	}
}
