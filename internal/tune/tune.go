// Package tune searches for guide tunings (cthulhuwars.GuideTuning) that make
// one cult's bot win more often. It is a (1+λ) evolution strategy over the
// tuning's knobs: each generation plays the current tuning and a few mutated
// ones on the same games, keeps the best mutation if it beats the parent on
// fresh confirmation games, and finishes by playing the result against the
// shipped tuning on games it has never seen.
//
// Every game is a bot-only game played in memory by internal/batch; the tuned
// cult sits at a table of the other cults (played with their shipped tunings),
// with the seats rotated, and its fitness is its win rate plus a little of the
// Doom it reached, since a win is a noisy signal.
package tune

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// Config describes a tuning run.
type Config struct {
	// Base is the layout; Lineup the cults at the table (one per seat, Faction
	// among them); HighPriests adds the expansion.
	Base        string
	Lineup      []string
	HighPriests bool
	// Faction is the cult being tuned.
	Faction string
	// Bot is the bot every cult plays (default tactical: the tuning only
	// reaches the guide, which the tactical bot plays straight).
	Bot string
	// Generations, Population (mutations per generation), GamesPerEval (games
	// each candidate plays), ConfirmGames (to accept a change) and
	// ValidateGames (the final comparison with the shipped tuning).
	Generations   int
	Population    int
	GamesPerEval  int
	ConfirmGames  int
	ValidateGames int
	// Sigma is the size of a mutation as a fraction of a knob's range; Knobs
	// names the knobs to search (default: all).
	Sigma float64
	Knobs []string
	// DoomWeight is how much of the fitness the Doom reached carries (default
	// 0.25; a win is worth 1).
	DoomWeight float64
	// BookWeight is how much of the fitness the spellbooks held carry (books
	// out of six; default 0). A cult that cannot win yet needs it: nothing else
	// tells a search which way to go.
	BookWeight float64
	Seed       int64
	Workers    int
	// Init is where the search starts (default: the shipped tuning).
	Init *cthulhuwars.GuideTuning
	// MaxPlies caps a game (default batch.DefaultMaxPlies).
	MaxPlies int
}

// Eval is how a tuning played over a set of games.
type Eval struct {
	Games   int     `json:"games"`
	Wins    int     `json:"wins"`
	Rate    float64 `json:"rate"`
	Doom    float64 `json:"doom"`  // average final Doom
	Books   float64 `json:"books"` // average spellbooks held at the end
	Fitness float64 `json:"fitness"`
}

// Step is one generation.
type Step struct {
	Generation int                     `json:"generation"`
	Parent     cthulhuwars.GuideTuning `json:"parent"`
	ParentEval Eval                    `json:"parent_eval"`
	Best       cthulhuwars.GuideTuning `json:"best"`
	BestEval   Eval                    `json:"best_eval"`
	// Confirmed evaluations of the best mutation and the parent on fresh games
	// (zero when the mutation did not beat the parent to begin with).
	ConfirmBest   Eval    `json:"confirm_best"`
	ConfirmParent Eval    `json:"confirm_parent"`
	Accepted      bool    `json:"accepted"`
	Sigma         float64 `json:"sigma"`
}

// Validation compares the final tuning with the shipped one on fresh games.
type Validation struct {
	Tuned   Eval `json:"tuned"`
	Shipped Eval `json:"shipped"`
	// Z is the z-score of the difference of the win rates; positive favours
	// the tuned.
	Z float64 `json:"z"`
}

// Result is what a run found.
type Result struct {
	Faction    string                  `json:"faction"`
	Lineup     []string                `json:"lineup"`
	Best       cthulhuwars.GuideTuning `json:"best"`
	Steps      []Step                  `json:"steps"`
	Validation *Validation             `json:"validation,omitempty"`
}

func (c *Config) defaults() error {
	if c.Bot == "" {
		c.Bot = "tactical"
	}
	if c.Generations <= 0 {
		c.Generations = 10
	}
	if c.Population <= 0 {
		c.Population = 6
	}
	if c.GamesPerEval <= 0 {
		c.GamesPerEval = 120
	}
	if c.ConfirmGames <= 0 {
		c.ConfirmGames = 2 * c.GamesPerEval
	}
	if c.ValidateGames <= 0 {
		c.ValidateGames = 400
	}
	if c.Sigma <= 0 {
		c.Sigma = 0.15
	}
	if c.DoomWeight == 0 {
		c.DoomWeight = 0.25
	}
	found := false
	for _, f := range c.Lineup {
		found = found || f == c.Faction
	}
	if !found {
		return fmt.Errorf("tune: the line-up %v does not include %q", c.Lineup, c.Faction)
	}
	return nil
}

// evaluate plays games games with the tuned cult using t (nil: the shipped
// tuning) and scores it. The same seed replays the same games for another
// tuning, which makes two tunings comparable with less noise.
func evaluate(ctx context.Context, cat *catalog.Catalog, c Config, t *cthulhuwars.GuideTuning, games int, seed int64) (Eval, error) {
	spec := batch.Spec{Base: c.Base, HighPriests: c.HighPriests, Games: games, Seed: seed, Rotate: true, MaxPlies: c.MaxPlies, Workers: c.Workers}
	for _, f := range c.Lineup {
		p := batch.Player{Faction: f, Bot: c.Bot}
		if f == c.Faction {
			p.Tuning = t
		}
		spec.Players = append(spec.Players, p)
	}
	ev := Eval{}
	doom, books := 0.0, 0.0
	err := batch.Run(ctx, cat, spec, func(r batch.Record) {
		for _, s := range r.Metrics.Seats {
			if s.Faction != c.Faction {
				continue
			}
			ev.Games++
			doom += float64(s.Doom)
			books += float64(s.BooksTotal)
			if s.Won {
				ev.Wins++
			}
		}
	}, nil)
	if err != nil {
		return ev, err
	}
	if ev.Games > 0 {
		ev.Rate = float64(ev.Wins) / float64(ev.Games)
		ev.Doom = doom / float64(ev.Games)
		ev.Books = books / float64(ev.Games)
		ev.Fitness = ev.Rate + c.DoomWeight*ev.Doom/30 + c.BookWeight*ev.Books/6
	}
	return ev, nil
}

// mutate returns a copy of t with a few knobs moved by a Gaussian step of
// sigma times their range, clipped to it.
func mutate(t cthulhuwars.GuideTuning, knobs []cthulhuwars.Knob, sigma float64, rng *rand.Rand) cthulhuwars.GuideTuning {
	out := t
	changed := false
	for !changed {
		for _, k := range knobs {
			// About three knobs per mutation: a noisy fitness cannot tell
			// which of fourteen changes helped.
			if rng.Float64() > 3/float64(len(knobs)) {
				continue
			}
			p := k.Ptr(&out)
			v := *p + rng.NormFloat64()*sigma*(k.Max-k.Min)
			*p = math.Round(math.Max(k.Min, math.Min(k.Max, v))*2) / 2
			changed = changed || *p != *k.Ptr(&t)
		}
	}
	return out
}

func selectKnobs(names []string) ([]cthulhuwars.Knob, error) {
	if len(names) == 0 {
		return cthulhuwars.GuideKnobs, nil
	}
	var out []cthulhuwars.Knob
	for _, n := range names {
		found := false
		for _, k := range cthulhuwars.GuideKnobs {
			if k.Name == n {
				out = append(out, k)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("tune: unknown knob %q", n)
		}
	}
	return out, nil
}

// Run searches. onStep, if set, is called after every generation with the
// result so far (to print or save it). It returns the result so far with ctx's
// error when ctx ends.
func Run(ctx context.Context, cat *catalog.Catalog, c Config, onStep func(Step, Result)) (Result, error) {
	if err := c.defaults(); err != nil {
		return Result{}, err
	}
	knobs, err := selectKnobs(c.Knobs)
	if err != nil {
		return Result{}, err
	}
	parent := cthulhuwars.ShippedTuning(c.Faction)
	if c.Init != nil {
		parent = *c.Init
	}
	res := Result{Faction: c.Faction, Lineup: c.Lineup, Best: parent}
	rng := rand.New(rand.NewSource(c.Seed + 7))
	sigma := c.Sigma
	seedFor := func(gen, slot int) int64 { return c.Seed + int64(gen)*1_000_003 + int64(slot)*97_001 + 1 }

	for gen := 1; gen <= c.Generations; gen++ {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		seed := seedFor(gen, 0)
		pt := parent
		pe, err := evaluate(ctx, cat, c, &pt, c.GamesPerEval, seed)
		if err != nil {
			return res, err
		}
		step := Step{Generation: gen, Parent: parent, ParentEval: pe, Sigma: sigma}
		type cand struct {
			t cthulhuwars.GuideTuning
			e Eval
		}
		var cands []cand
		for i := 0; i < c.Population; i++ {
			t := mutate(parent, knobs, sigma, rng)
			e, err := evaluate(ctx, cat, c, &t, c.GamesPerEval, seed)
			if err != nil {
				return res, err
			}
			cands = append(cands, cand{t, e})
		}
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].e.Fitness > cands[j].e.Fitness })
		best := cands[0]
		step.Best, step.BestEval = best.t, best.e
		if best.e.Fitness > pe.Fitness {
			// The best of several noisy tries is flattered by luck: it must beat
			// the parent again on games neither has played.
			cseed := seedFor(gen, 1)
			bt, ptc := best.t, parent
			ce, err := evaluate(ctx, cat, c, &bt, c.ConfirmGames, cseed)
			if err != nil {
				return res, err
			}
			cp, err := evaluate(ctx, cat, c, &ptc, c.ConfirmGames, cseed)
			if err != nil {
				return res, err
			}
			step.ConfirmBest, step.ConfirmParent = ce, cp
			if ce.Fitness > cp.Fitness {
				step.Accepted = true
				parent = best.t
				res.Best = parent
			}
		}
		if step.Accepted {
			sigma = math.Min(0.4, sigma*1.15)
		} else {
			sigma = math.Max(0.04, sigma*0.9)
		}
		res.Steps = append(res.Steps, step)
		if onStep != nil {
			onStep(step, res)
		}
	}

	// A last comparison on games nothing was fitted to.
	vseed := seedFor(c.Generations+1, 2)
	bt := res.Best
	te, err := evaluate(ctx, cat, c, &bt, c.ValidateGames, vseed)
	if err != nil {
		return res, err
	}
	se, err := evaluate(ctx, cat, c, nil, c.ValidateGames, vseed)
	if err != nil {
		return res, err
	}
	res.Validation = &Validation{Tuned: te, Shipped: se, Z: zDiff(te, se)}
	return res, nil
}

// zDiff is the z-score of the difference of two win rates.
func zDiff(a, b Eval) float64 {
	if a.Games == 0 || b.Games == 0 {
		return 0
	}
	p := float64(a.Wins+b.Wins) / float64(a.Games+b.Games)
	se := math.Sqrt(p * (1 - p) * (1/float64(a.Games) + 1/float64(b.Games)))
	if se == 0 {
		return 0
	}
	return (a.Rate - b.Rate) / se
}
