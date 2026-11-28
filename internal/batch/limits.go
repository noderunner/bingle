package batch

import (
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/noderunner/bingle/internal/store"
)

// Ceilings are what one batch may ask for and use. A spec over one is refused
// before it is queued, and a batch that reaches its Minutes is ended (ADR
// 0161). The JSON names are those of a spec's "limits" (what an administrator
// may raise for one batch) and of GET /api/batches' "limits".
type Ceilings struct {
	// Workers is how many games one batch may play at once.
	Workers int `json:"workers"`
	// MCTSIterations and MinimaxNodes bound each bot's search a move: what
	// a spec may set for a player.
	MCTSIterations int `json:"mcts_iterations"`
	MinimaxNodes   int `json:"minimax_nodes"`
	// MaxPlies bounds a game's length, Games the games of a batch and
	// EventGames the games that keep their decisions.
	MaxPlies   int `json:"max_plies"`
	Games      int `json:"games"`
	EventGames int `json:"event_games"`
	// Minutes is the wall-clock time of a batch from the moment it starts
	// playing, its wait in the queue not counted.
	Minutes int `json:"minutes"`
}

// Limits are the ceilings on a batch and the allowances of a player, set by
// the operator (ADR 0161).
type Limits struct {
	Ceilings
	// Queue is how many batches may wait for their turn, the whole server's.
	Queue int `json:"queue"`
	// QueuedPerPlayer is how many batches one player may have queued or
	// running, StoredPerPlayer how many they may keep, and GamesPerDay how
	// many games they may start in 24 hours.
	QueuedPerPlayer int `json:"queued_per_player"`
	StoredPerPlayer int `json:"stored_per_player"`
	GamesPerDay     int `json:"games_per_day"`
}

// The defaults: a little above what a batch a person would start has used (see
// ADR 0161 for the measurements).
const (
	// MaxGames is the default ceiling on a batch's games.
	MaxGames            = 5000
	DefaultMCTSCeiling  = 5000
	DefaultMinimaxNodes = 100000
	// PlyCeiling is the default ceiling on a game's ply cap.
	PlyCeiling        = 5000
	DefaultEventGames = 100
	DefaultMinutes    = 720
	// NameRunes bounds a batch's name.
	NameRunes = 200
)

// DefaultLimits are the limits of a server whose operator set none.
func DefaultLimits() Limits {
	return Limits{
		Ceilings: Ceilings{
			Workers:        max(runtime.NumCPU(), 1),
			MCTSIterations: DefaultMCTSCeiling,
			MinimaxNodes:   DefaultMinimaxNodes,
			MaxPlies:       PlyCeiling,
			Games:          MaxGames,
			EventGames:     DefaultEventGames,
			Minutes:        DefaultMinutes,
		},
		Queue:           256,
		QueuedPerPlayer: 16,
		StoredPerPlayer: 100,
		GamesPerDay:     20000,
	}
}

// Wall is the wall-clock limit of a batch.
func (c Ceilings) Wall() time.Duration { return time.Duration(c.Minutes) * time.Minute }

// over returns each non-zero field of raised in place of c's.
func (c Ceilings) over(raised *Ceilings) Ceilings {
	if raised == nil {
		return c
	}
	for _, f := range []struct{ to, from *int }{
		{&c.Workers, &raised.Workers}, {&c.MCTSIterations, &raised.MCTSIterations}, {&c.MinimaxNodes, &raised.MinimaxNodes},
		{&c.MaxPlies, &raised.MaxPlies}, {&c.Games, &raised.Games}, {&c.EventGames, &raised.EventGames}, {&c.Minutes, &raised.Minutes},
	} {
		if *f.from > 0 {
			*f.to = *f.from
		}
	}
	return c
}

// LimitTime is what Info.Limit says of a batch ended at its wall-clock limit.
const LimitTime = "time"

var errWallClock = errors.New("batch: the time limit")

// Errors of starting a batch over a limit. A LimitError is a spec asking for
// too much (400); an AllowanceError is a player or the server having no room
// (429); ErrNotAdmin is a spec that raises its limits sent by anybody else
// (403).
var ErrNotAdmin = errors.New("batch: only an administrator may raise a batch's limits")

// LimitError says which ceiling a spec is over.
type LimitError struct {
	// Limit is the name of the ceiling (a field of Ceilings, or "name"), Max
	// what it is and Asked what the spec wanted.
	Limit string
	Max   int
	Asked int
}

func (e *LimitError) Error() string {
	switch e.Limit {
	case "name":
		return fmt.Sprintf("batch: a name may have at most %d characters (limit name)", e.Max)
	}
	return fmt.Sprintf("batch: %s is at most %d, the spec asks for %d (limit %s)", e.Limit, e.Max, e.Asked, e.Limit)
}

// AllowanceError says which allowance has no room left.
type AllowanceError struct {
	// Limit is queue, queued_per_player, stored_per_player or games_per_day;
	// Max is the allowance.
	Limit string
	Max   int
}

func (e *AllowanceError) Error() string {
	switch e.Limit {
	case "queue":
		return fmt.Sprintf("batch: the queue is full (%d batches wait: limit queue)", e.Max)
	case "queued_per_player":
		return fmt.Sprintf("batch: you have %d batches queued or running, the most there may be (limit queued_per_player)", e.Max)
	case "stored_per_player":
		return fmt.Sprintf("batch: you keep %d batches, the most there may be: delete one to start another (limit stored_per_player)", e.Max)
	case "games_per_day":
		return fmt.Sprintf("batch: this would pass %d games in 24 hours, a player's allowance (limit games_per_day)", e.Max)
	}
	return "batch: no room: limit " + e.Limit
}

// ErrQueueFull is the answer to a batch started while the queue is full.
var ErrQueueFull = &AllowanceError{Limit: "queue"}

// checkCeilings refuses a spec, as the player wrote it (a zero is the
// default), that is over a ceiling.
func checkCeilings(name string, spec Spec, c Ceilings) error {
	if n := len([]rune(name)); n > NameRunes {
		return &LimitError{Limit: "name", Max: NameRunes, Asked: n}
	}
	over := func(limit string, asked, max int) error {
		if asked > max {
			return &LimitError{Limit: limit, Max: max, Asked: asked}
		}
		return nil
	}
	for _, e := range []error{
		over("games", spec.Games, c.Games),
		over("workers", spec.Workers, c.Workers),
		over("max_plies", spec.MaxPlies, c.MaxPlies),
		over("event_games", spec.EventGames, c.EventGames),
	} {
		if e != nil {
			return e
		}
	}
	for _, p := range spec.Players {
		if e := over("mcts_iterations", p.MCTSIterations, c.MCTSIterations); e != nil {
			return e
		}
		if e := over("minimax_nodes", p.MinimaxNodes, c.MinimaxNodes); e != nil {
			return e
		}
	}
	return nil
}

// checkAllowances refuses a batch of games that its owner has no room for.
func (l Limits) checkAllowances(u store.BatchUsage, games int) error {
	switch {
	case u.Active >= l.QueuedPerPlayer:
		return &AllowanceError{Limit: "queued_per_player", Max: l.QueuedPerPlayer}
	case u.Stored >= l.StoredPerPlayer:
		return &AllowanceError{Limit: "stored_per_player", Max: l.StoredPerPlayer}
	case u.GamesToday+games > l.GamesPerDay:
		return &AllowanceError{Limit: "games_per_day", Max: l.GamesPerDay}
	}
	return nil
}
