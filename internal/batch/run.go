// Package batch plays many bot-versus-bot games in memory and records what
// happened in each, so that bots can be compared and tuned, and a game itself
// studied — how often the first player wins, how long a game runs, how often it
// is drawn. It touches neither the database nor the rooms: the caller decides
// where records go.
//
// Every game can be batched. A table of Cthulhu Wars also names a cult per seat
// and is watched by cthulhuwars.Observer, which measures far more than who won
// (Doom, Gates, spellbooks, battles); any other game is played the same way
// without that, and its report holds what any game has: the outcome per seat,
// the bots that played, the length and how the games ended.
package batch

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
	"github.com/noderunner/bingle/internal/metrics"
)

// Player is one seat of the table and the bot that plays it.
type Player struct {
	// Faction is the cult key of a Cthulhu Wars table: gc, cc, bg, ys, ww or
	// sl. Any other game leaves it empty: its seats are the game's own.
	Faction string `json:"faction,omitempty"`
	// Bot is a built-in bot id: random, tactical, minimax or mcts.
	Bot string `json:"bot"`
	// MCTSIterations bounds the mcts bot per move (it is reproducible and
	// much faster than wall-clock time); MinimaxNodes bounds minimax.
	MCTSIterations int `json:"mcts_iterations,omitempty"`
	MinimaxNodes   int `json:"minimax_nodes,omitempty"`
	// Tuning replaces the shipped guide tuning of this cult (see
	// cthulhuwars.GuideTuning); nil plays with the shipped one.
	Tuning *cthulhuwars.GuideTuning `json:"tuning,omitempty"`
}

// Spec describes a batch.
type Spec struct {
	// Base is the game, e.g. "cthulhu-wars-5p" or "hex-11"; the number of
	// Players must equal its number of seats.
	Base        string   `json:"base"`
	HighPriests bool     `json:"high_priests,omitempty"`
	Players     []Player `json:"players"`
	Games       int      `json:"games"`
	Seed        int64    `json:"seed"`
	// Rotate seats the players differently in every game (a cyclic shift), so
	// that the order of play does not favour a cult.
	Rotate bool `json:"rotate"`
	// MaxPlies ends a game that runs too long as unfinished (default 3000).
	MaxPlies int `json:"max_plies,omitempty"`
	// EventGames is how many games (the first ones) keep every decision.
	EventGames int `json:"event_games,omitempty"`
	// Workers is how many games are played at once (default: 3/4 of the CPUs).
	Workers int `json:"workers,omitempty"`
}

// Event is one decision of a game.
type Event struct {
	Ply   int    `json:"ply"`
	Seat  int    `json:"seat"`
	Round int    `json:"round"`
	Move  string `json:"move"`
}

// Record is one finished game.
type Record struct {
	Index int   `json:"index"`
	Seed  int64 `json:"seed"`
	// GameID is the game the table was played as (a derived one for a table of
	// cults); Seating lists the cult key in each seat, empty for a game whose
	// seats are its own, and Bots the bot that played each seat.
	GameID  string   `json:"game_id"`
	Seating []string `json:"seating"`
	Bots    []string `json:"bots,omitempty"`
	// The outcome every game has: who won (0 for a draw or an unfinished
	// game), why it ended, and how long it ran.
	Winner    int    `json:"winner"`
	EndReason string `json:"end_reason"`
	Plies     int    `json:"plies"`
	Rounds    int    `json:"rounds"`
	// Metrics is what the Cthulhu Wars observer measured; it is empty for
	// every other game.
	Metrics cthulhuwars.GameMetrics `json:"metrics"`
	Events  []Event                 `json:"events,omitempty"`
}

// DefaultWorkers is how many games run at once when a spec does not say: three
// quarters of the CPUs, so the live server stays responsive.
func DefaultWorkers() int {
	if n := runtime.NumCPU() * 3 / 4; n > 1 {
		return n
	}
	return 1
}

// DefaultMaxPlies is the ply cap of a game when a spec sets none.
const DefaultMaxPlies = 3000

// Validate checks a spec against the catalog and fills in its defaults.
func (s *Spec) Validate(cat *catalog.Catalog) error {
	base, ok := cat.Get(s.Base)
	if !ok {
		return fmt.Errorf("batch: unknown game %q", s.Base)
	}
	if n := len(base.Definition.Seats); len(s.Players) != n {
		return fmt.Errorf("batch: %s seats %d players, the spec has %d", s.Base, n, len(s.Players))
	}
	seen := map[string]bool{}
	cults := 0
	for _, p := range s.Players {
		if p.Faction != "" {
			cults++
			if seen[p.Faction] {
				return fmt.Errorf("batch: cult %q sits twice", p.Faction)
			}
			seen[p.Faction] = true
		}
		if _, ok := bots.Make(p.Bot, 1); !ok {
			return fmt.Errorf("batch: unknown bot %q", p.Bot)
		}
	}
	// A table either names a cult for every seat (Cthulhu Wars, whose seats are
	// a choice) or for none (every other game, whose seats are its own).
	if cults != 0 && cults != len(s.Players) {
		return fmt.Errorf("batch: %d of %d seats name a cult: name one for all of them or for none", cults, len(s.Players))
	}
	if cults == 0 && s.HighPriests {
		return fmt.Errorf("batch: high_priests is a Cthulhu Wars option")
	}
	if s.Games < 1 {
		return fmt.Errorf("batch: games must be at least 1")
	}
	if s.MaxPlies <= 0 {
		s.MaxPlies = DefaultMaxPlies
	}
	if s.Workers <= 0 {
		s.Workers = DefaultWorkers()
	}
	for i := range s.Players {
		if _, err := s.table(cat, i); err != nil {
			return err
		}
		if !s.Rotate {
			break
		}
	}
	return nil
}

// table is the derived game id and the players in seat order for game i.
func (s *Spec) table(cat *catalog.Catalog, i int) (string, error) {
	id, _ := s.layout(i)
	if _, ok := cat.Get(id); !ok {
		return "", fmt.Errorf("batch: cannot build the table %q", id)
	}
	return id, nil
}

func (s *Spec) layout(i int) (string, []Player) {
	n := len(s.Players)
	shift := 0
	if s.Rotate {
		shift = i % n
	}
	seated := make([]Player, n)
	keys := make([]string, n)
	cults := 0
	for j := range seated {
		seated[j] = s.Players[(j+shift)%n]
		keys[j] = seated[j].Faction
		if keys[j] != "" {
			cults++
		}
	}
	// Only a table whose seats are a choice derives a game of its own; every
	// other game is played as it is, with the bots moved round the seats.
	if cults != n {
		return s.Base, seated
	}
	suffix := strings.Join(keys, "-")
	if s.HighPriests {
		suffix += "-hp"
	}
	return s.Base + "~" + suffix, seated
}

// batchMetrics is the registry the worker count is reported to. It is a
// package-level handle, like the bots' node counters, so that Run's signature
// stays what every caller and test already passes; a nil one records nothing.
var batchMetrics *metrics.Metrics

// SetMetrics hands this package the registry to record into.
func SetMetrics(m *metrics.Metrics) { batchMetrics = m }

// Run plays the batch, calling sink with every finished game (from several
// goroutines, in no particular order) and progress with the number of games
// finished so far. It returns when all are played or ctx ends.
func Run(ctx context.Context, cat *catalog.Catalog, spec Spec, sink func(Record), progress func(done int)) error {
	if err := spec.Validate(cat); err != nil {
		return err
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	// How many games a batch plays at once, so a server that feels slow
	// while a batch runs says why.
	batchMetrics.SetBatchWorkers(spec.Workers)
	defer batchMetrics.SetBatchWorkers(0)
	for w := 0; w < spec.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				rec, err := PlayGame(ctx, cat, spec, i)
				if err != nil {
					continue
				}
				sink(rec)
				mu.Lock()
				done++
				d := done
				mu.Unlock()
				if progress != nil {
					progress(d)
				}
			}
		}()
	}
	for i := 0; i < spec.Games && ctx.Err() == nil; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return ctx.Err()
}

// PlayGame plays game number i of the batch.
func PlayGame(ctx context.Context, cat *catalog.Catalog, spec Spec, i int) (Record, error) {
	id, seated := spec.layout(i)
	game, ok := cat.Get(id)
	if !ok {
		return Record{}, fmt.Errorf("batch: unknown game %q", id)
	}
	seed := spec.Seed + int64(i)
	rec := Record{Index: i, Seed: seed, GameID: id}
	botFor := map[int]bots.Bot{}
	tuning := map[int]*cthulhuwars.GuideTuning{}
	rules, _ := state.RulesetOf(game)
	cw, _ := rules.(*cthulhuwars.Ruleset)
	for j, p := range seated {
		seat := j + 1
		b, ok := bots.MakeConfigured(p.Bot, seed*1000+int64(seat), bots.Config{MCTSIterations: p.MCTSIterations, MinimaxNodes: p.MinimaxNodes})
		if !ok {
			return rec, fmt.Errorf("batch: unknown bot %q", p.Bot)
		}
		botFor[seat] = b
		tuning[seat] = p.Tuning
		rec.Seating = append(rec.Seating, p.Faction)
		rec.Bots = append(rec.Bots, p.Bot)
	}
	st, err := state.InitialStateSeeded(game, uint64(seed)*1000003+1)
	if err != nil {
		return rec, err
	}
	// The observer measures a cult's play (Doom, Gates, battles): it belongs to
	// Cthulhu Wars, and any other game is watched only by its outcome.
	var obs *cthulhuwars.Observer
	if cw != nil {
		obs = cthulhuwars.NewObserver(game)
	}
	keep := i < spec.EventGames
	seat := state.FirstSeat(game)
	winner, reason := 0, "unfinished"
	for ply := 0; ply < spec.MaxPlies; ply++ {
		if ctx.Err() != nil {
			return rec, ctx.Err()
		}
		legal := state.LegalMoves(game, st, seat)
		if len(legal) == 0 {
			reason = "no-legal-moves"
			break
		}
		guided := legal
		switch {
		case cw != nil && tuning[seat] != nil && len(legal) > 1:
			if g := cw.GuideMovesTuned(game, st, seat, legal, tuning[seat]); len(g) > 0 {
				guided = g
			}
		default:
			guided = state.GuidedMoves(game, st, seat, legal)
		}
		mv, err := botFor[seat].Choose(ctx, game, st, seat, guided)
		if err != nil {
			mv = legal[0]
		}
		r, err := state.Apply(game, st, seat, mv)
		if err != nil {
			reason = "illegal-move"
			break
		}
		if obs != nil {
			obs.Observe(st, r.NewState, seat, mv)
		}
		rec.Plies = ply + 1
		if keep {
			rec.Events = append(rec.Events, Event{Ply: ply + 1, Seat: seat, Move: r.Notation})
		}
		if r.GameEnded {
			reason = r.EndReason
			if r.WinnerSeat != nil {
				winner = *r.WinnerSeat
			}
			st = r.NewState
			break
		}
		st, seat = r.NewState, r.NextTurnSeat
	}
	rec.Winner, rec.EndReason = winner, reason
	if obs != nil {
		rec.Metrics = obs.Finish(st, winner, reason)
		rec.Plies, rec.Rounds = rec.Metrics.Plies, rec.Metrics.Rounds
	}
	return rec, nil
}
