// Package botladder is the bot ladder (ADR 0160): a standing ranking of the
// built-in bots in each game family, apart from people's ratings. Once a day,
// at a time of day in UTC, the bots that think play each other a few games in
// every family that two bots can play alone, and each bot's Glicko-2 rating
// there moves once for the run, as one rating period.
//
// The ratings are kept in a table of their own (store.BotRating), with no
// account in it: no query for people's ratings or leaderboards can reach a
// bot, and no game a bot plays here can move a person's rating.
//
// A run plays its games the way a batch does (batch.PlayGame), on as many
// workers as a batch takes, and only while no batch plays (batch.Manager.Hold).
// It is bounded by a number of games a pairing and a time limit for the whole
// run, and a run that is due while another goes on is skipped, not queued.
package botladder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/rating"
	"github.com/noderunner/bingle/internal/store"
)

// Bots are the bots on the ladder, weakest first: the three that think, at the
// strength players meet them in a game. Random is left out.
var Bots = []string{"tactical", "minimax", "mcts"}

// MaxPlies ends a ladder game that runs this long; it counts as a draw. It is
// far below a batch's ceiling because a ladder game is played at the bots' full
// strength, a second a move for MCTS.
const MaxPlies = 300

// ErrBusy is the answer to a run asked for while one goes on.
var ErrBusy = errors.New("botladder: a run is already going on")

// Options are when a run starts and how much it may play.
type Options struct {
	// Off turns the scheduled runs off.
	Off bool
	// At is the time of day a run starts, in minutes after midnight UTC.
	At int
	// Games is how many games each pairing plays in each family a run.
	Games int
	// Limit bounds a whole run.
	Limit time.Duration
	// Strength replaces the bots' own strength, for tests; zero is the
	// strength players meet.
	Strength bots.Config
}

// Ladder runs the bot ladder.
type Ladder struct {
	st   *store.Store
	cat  *catalog.Catalog
	hold func(context.Context) (func(), error)
	opts Options
	log  *slog.Logger
	busy atomic.Bool
}

// New makes the ladder. hold, when not nil, is the batches' turn
// (batch.Manager.Hold): a run takes it before it plays.
func New(st *store.Store, cat *catalog.Catalog, hold func(context.Context) (func(), error), opts Options, log *slog.Logger) *Ladder {
	if opts.Games < 1 {
		opts.Games = 2
	}
	if opts.Limit <= 0 {
		opts.Limit = time.Hour
	}
	if log == nil {
		log = slog.Default()
	}
	return &Ladder{st: st, cat: cat, hold: hold, opts: opts, log: log}
}

// Next is the first moment after now that a run at minutes after midnight UTC
// is due.
func Next(now time.Time, at int) time.Time {
	now = now.UTC()
	t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(time.Duration(at) * time.Minute)
	if !t.After(now) {
		t = t.Add(24 * time.Hour)
	}
	return t
}

// Run starts a run at each scheduled time until ctx ends, and does nothing when
// the runs are off. A run is played to its end before the next time is worked
// out, so a run that overruns the next one's time makes that one skipped.
func (l *Ladder) Run(ctx context.Context) {
	if l.opts.Off {
		l.log.Info("bot ladder: off")
		return
	}
	for {
		next := Next(time.Now(), l.opts.At)
		l.log.Info("bot ladder: next run", "at", next)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if _, err := l.RunOnce(ctx); err != nil && ctx.Err() == nil {
			l.log.Warn("bot ladder: the run was skipped", "err", err)
		}
	}
}

// Game is one family on the ladder and the game its bots play there.
type Game struct {
	Family string
	Name   string
	GameID string
}

// Games lists the families on the ladder, by family: every family of two seats
// that is not a prototype, played as the game of the family's own id when
// there is one (hex-11 for Hex) and otherwise as its first game, and only
// where a batch can set the game up with nobody choosing anything.
func Games(cat *catalog.Catalog) []Game {
	byFamily := map[string]*catalog.CompiledGame{}
	for _, cg := range cat.List() {
		d := cg.Definition
		if d.Prototype || len(d.Seats) != 2 {
			continue
		}
		family := d.Family
		if family == "" {
			family = d.ID
		}
		if have, ok := byFamily[family]; ok && (have.Definition.ID == family || d.ID != family) {
			continue
		}
		byFamily[family] = cg
	}
	out := make([]Game, 0, len(byFamily))
	for family, cg := range byFamily {
		spec := pairing(cg.Definition.ID, Bots[0], Bots[1], 1, 0, bots.Config{})
		if err := spec.Validate(cat); err != nil {
			continue
		}
		name := cg.Definition.Name
		if base, ok := cat.Get(family); ok {
			name = base.Definition.Name
		}
		out = append(out, Game{Family: family, Name: store.FamilyNameOf(name), GameID: cg.Definition.ID})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Family < out[j].Family })
	return out
}

// pairing is the batch spec of two bots in a game: games of them, moved round
// the seats, so that each plays first as often as the other.
func pairing(gameID, a, b string, games int, seed int64, strength bots.Config) batch.Spec {
	p := func(id string) batch.Player {
		return batch.Player{Bot: id, MCTSIterations: strength.MCTSIterations, MinimaxNodes: strength.MinimaxNodes}
	}
	return batch.Spec{Base: gameID, Players: []batch.Player{p(a), p(b)}, Games: games, Seed: seed, Rotate: true, MaxPlies: MaxPlies, Workers: 1}
}

// Summary is what a run did.
type Summary struct {
	Families int
	Games    int
	Took     time.Duration
	// Cut says the time limit ended the run before every game was played.
	Cut bool
	// Order is the families in the order the run played them.
	Order []string
}

// job is one game of a run.
type job struct {
	family int
	spec   batch.Spec
	index  int
}

// outcome is one game of a run as the ratings see it: the bot in each seat,
// and the first seat's score.
type outcome struct {
	family int
	bots   [2]string
	score  float64
}

// RunOnce plays a run now, unless one goes on (ErrBusy), and writes the new
// ratings. It waits for a batch to finish, within the run's time limit.
func (l *Ladder) RunOnce(ctx context.Context) (Summary, error) {
	if !l.busy.CompareAndSwap(false, true) {
		return Summary{}, ErrBusy
	}
	defer l.busy.Store(false)
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, l.opts.Limit)
	defer cancel()
	if l.hold != nil {
		release, err := l.hold(ctx)
		if err != nil {
			return Summary{}, fmt.Errorf("botladder: a batch was still playing: %w", err)
		}
		defer release()
	}
	games := Games(l.cat)
	if len(games) == 0 {
		return Summary{}, nil
	}
	// The families whose ratings are oldest go first, those never played
	// before all: a run its time limit cuts short is followed by one that
	// starts where it stopped.
	order, err := l.oldestFirst(ctx, games)
	if err != nil {
		return Summary{}, err
	}
	day := start.UTC().Unix() / 86400
	var jobs []job
	for _, f := range order {
		p := 0
		for i := 0; i < len(Bots); i++ {
			for j := i + 1; j < len(Bots); j++ {
				spec := pairing(games[f].GameID, Bots[i], Bots[j], l.opts.Games, day*1_000_000+int64(f)*1000+int64(p)*100, l.opts.Strength)
				if err := spec.Validate(l.cat); err != nil {
					continue
				}
				for g := 0; g < l.opts.Games; g++ {
					jobs = append(jobs, job{family: f, spec: spec, index: g})
				}
				p++
			}
		}
	}
	l.log.Info("bot ladder: run started", "families", len(games), "games", len(jobs))
	outcomes := l.play(ctx, jobs)
	sum := Summary{Families: len(games), Games: len(outcomes), Took: time.Since(start), Cut: len(outcomes) < len(jobs) && ctx.Err() != nil}
	for _, f := range order {
		sum.Order = append(sum.Order, games[f].Family)
	}
	// The ratings are written whatever ended the run: what was played counts.
	if err := l.record(context.WithoutCancel(ctx), games, outcomes, time.Now()); err != nil {
		return sum, err
	}
	l.log.Info("bot ladder: run finished", "games", sum.Games, "of", len(jobs), "took", sum.Took.Round(time.Second), "cut_short", sum.Cut)
	return sum, nil
}

// oldestFirst is the order of games to play in: by the oldest rating of any
// of the ladder's bots in the family, a missing one older than all.
func (l *Ladder) oldestFirst(ctx context.Context, games []Game) ([]int, error) {
	have, err := l.st.BotRatings(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]map[string]time.Time{}
	for _, r := range have {
		if seen[r.Family] == nil {
			seen[r.Family] = map[string]time.Time{}
		}
		seen[r.Family][r.Bot] = r.UpdatedAt
	}
	oldest := make([]time.Time, len(games))
	for i, g := range games {
		for k, bot := range Bots {
			at, ok := seen[g.Family][bot]
			if !ok {
				at = time.Time{}
			}
			if k == 0 || at.Before(oldest[i]) {
				oldest[i] = at
			}
		}
	}
	order := make([]int, len(games))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return oldest[order[a]].Before(oldest[order[b]]) })
	return order, nil
}

// play plays the jobs on the batches' number of workers, until they are done
// or ctx ends, and returns the games that count.
func (l *Ladder) play(ctx context.Context, jobs []job) []outcome {
	workers := min(batch.DefaultWorkers(), len(jobs))
	feed := make(chan job)
	var mu sync.Mutex
	var out []outcome
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for jb := range feed {
				rec, err := batch.PlayGame(ctx, l.cat, jb.spec, jb.index)
				if err != nil || len(rec.Bots) != 2 {
					continue
				}
				score := 0.5 // a draw, or a game the ply cap ended
				switch {
				case rec.EndReason == "illegal-move" || rec.EndReason == "no-legal-moves":
					continue // a hole in the rules, which says nothing of the bots
				case rec.Winner == 1:
					score = 1
				case rec.Winner == 2:
					score = 0
				}
				mu.Lock()
				out = append(out, outcome{family: jb.family, bots: [2]string{rec.Bots[0], rec.Bots[1]}, score: score})
				mu.Unlock()
			}
		}()
	}
	for _, jb := range jobs {
		if ctx.Err() != nil {
			break
		}
		select {
		case feed <- jb:
		case <-ctx.Done():
		}
	}
	close(feed)
	wg.Wait()
	return out
}

// record moves each bot's rating in each family once, by every game it played
// there this run against its opponents as they stood before it (one Glicko-2
// rating period), and writes them together.
func (l *Ladder) record(ctx context.Context, games []Game, outcomes []outcome, at time.Time) error {
	if len(outcomes) == 0 {
		return nil
	}
	have, err := l.st.BotRatings(ctx)
	if err != nil {
		return err
	}
	type key struct{ bot, family string }
	before := map[key]store.BotRating{}
	for _, r := range have {
		before[key{r.Bot, r.Family}] = r
	}
	player := func(k key) rating.Player {
		if r, ok := before[k]; ok {
			return rating.Player{Rating: r.Rating, Deviation: r.Deviation, Volatility: r.Volatility}
		}
		return rating.New()
	}
	results := map[key][]rating.Result{}
	for _, o := range outcomes {
		family := games[o.family].Family
		a, b := key{o.bots[0], family}, key{o.bots[1], family}
		results[a] = append(results[a], rating.Result{Opponent: player(b), Score: o.score})
		results[b] = append(results[b], rating.Result{Opponent: player(a), Score: 1 - o.score})
	}
	names := map[string]string{}
	for _, g := range games {
		names[g.Family] = g.Name
	}
	list := make([]store.BotRating, 0, len(results))
	for k, rs := range results {
		p := rating.Update(player(k), rs)
		r := before[k]
		r.Bot, r.Family, r.FamilyName = k.bot, k.family, names[k.family]
		r.Rating, r.Deviation, r.Volatility = p.Rating, p.Deviation, p.Volatility
		r.Games += len(rs)
		for _, x := range rs {
			switch x.Score {
			case 1:
				r.Won++
			case 0:
				r.Lost++
			default:
				r.Drawn++
			}
		}
		r.UpdatedAt = at
		list = append(list, r)
	}
	return l.st.SetBotRatings(ctx, list)
}
