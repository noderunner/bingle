package envs

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// MatchOptions configures PlayMatch.
type MatchOptions struct {
	GameDefID string
	// BotA and BotB are built-in bot IDs. Games alternate which bot has the
	// first seat, so a match is fair for asymmetric games.
	BotA, BotB string
	Games      int
	Seed       int64
	// MaxPlies truncates a game (default 400); a truncated game is
	// reported as unfinished.
	MaxPlies int
	// KeepMoves includes every game's move list in the result.
	KeepMoves bool
	// BotIterations limits MCTS bots to that many iterations per move
	// (reproducible and fast) instead of ~1 s of wall-clock time.
	BotIterations int
	// Record keeps every position of every game as a training example:
	// the position document, the move played, its fixed action ID and the
	// game's outcome from the mover's side. RecordPlanes adds the
	// mover-relative planes of each position (PlaneDepth levels of stack).
	Record       bool
	RecordPlanes bool
	PlaneDepth   int
}

// MaxRecordedGames bounds Games when Record is set (the result can get big).
const MaxRecordedGames = 50

// PositionRecord is one training example from a recorded game.
type PositionRecord struct {
	Ply    int `json:"ply"`
	ToMove int `json:"to_move"`
	// State is the position document before the move.
	State json.RawMessage `json:"state"`
	// Move is the move played, in notation, and ActionID its fixed action ID
	// (-1 when the game has no fixed action space).
	Move     string `json:"move"`
	ActionID int    `json:"action_id"`
	// Result is the game's outcome for the player who moved here: +1 win,
	// -1 loss, 0 draw or unfinished.
	Result int `json:"result"`
	// Planes encodes State relative to the mover (RecordPlanes only).
	Planes *state.Planes `json:"planes,omitempty"`
}

// MatchGame is the record of one game in a match.
type MatchGame struct {
	// FirstSeatBot is "a" or "b": which bot played seat 1 (the first seat).
	FirstSeatBot string `json:"first_seat_bot"`
	// Winner is "a", "b", "draw" or "unfinished".
	Winner    string   `json:"winner"`
	EndReason string   `json:"end_reason,omitempty"`
	Plies     int      `json:"plies"`
	Moves     []string `json:"moves,omitempty"`
	// Positions are the game's training examples (Record only).
	Positions []PositionRecord `json:"positions,omitempty"`
}

// MatchResult summarises a match between two bots.
type MatchResult struct {
	GameDefID  string      `json:"game_def_id"`
	BotA       string      `json:"bot_a"`
	BotB       string      `json:"bot_b"`
	Played     int         `json:"played"`
	AWins      int         `json:"a_wins"`
	BWins      int         `json:"b_wins"`
	Draws      int         `json:"draws"`
	Unfinished int         `json:"unfinished"`
	AvgPlies   float64     `json:"avg_plies"`
	Games      []MatchGame `json:"games,omitempty"`
	// Truncated is true when the time budget ran out before all requested
	// games were played.
	Truncated bool `json:"truncated"`
}

// matchSlots limits concurrent matches to the number of CPUs; a match is
// seconds of CPU and callers just queue.
var matchSlots = make(chan struct{}, runtime.NumCPU())

// PlayMatch plays a series of games between two built-in bots and returns
// the tally (and, optionally, the games). It stops early, reporting what it
// has, when ctx ends. It is the engine behind the match_bots MCP tool: a
// quick way to baseline opponents and to generate move records.
func PlayMatch(ctx context.Context, cat *catalog.Catalog, opts MatchOptions) (MatchResult, error) {
	game, ok := cat.Get(opts.GameDefID)
	if !ok {
		return MatchResult{}, fmt.Errorf("%w: %q", ErrUnknownGame, opts.GameDefID)
	}
	if opts.Games < 1 {
		return MatchResult{}, fmt.Errorf("envs: games must be at least 1")
	}
	maxPlies := opts.MaxPlies
	if maxPlies <= 0 {
		maxPlies = 400
	}
	if opts.Record && opts.Games > MaxRecordedGames {
		return MatchResult{}, fmt.Errorf("envs: at most %d games can be recorded in one call", MaxRecordedGames)
	}
	if opts.RecordPlanes {
		if _, _, _, err := state.PlaneChannels(game, state.PlaneOptions{Depth: opts.PlaneDepth}); err != nil {
			return MatchResult{}, err
		}
	}

	select {
	case matchSlots <- struct{}{}:
		defer func() { <-matchSlots }()
	case <-ctx.Done():
		return MatchResult{}, ctx.Err()
	}

	res := MatchResult{GameDefID: opts.GameDefID, BotA: opts.BotA, BotB: opts.BotB}
	totalPlies := 0

	for i := 0; i < opts.Games; i++ {
		if ctx.Err() != nil {
			res.Truncated = true
			break
		}
		cfg := bots.Config{MCTSIterations: opts.BotIterations}
		a, okA := bots.MakeConfigured(opts.BotA, opts.Seed*1000+int64(2*i)+1, cfg)
		b, okB := bots.MakeConfigured(opts.BotB, opts.Seed*1000+int64(2*i)+2, cfg)
		if !okA {
			return MatchResult{}, fmt.Errorf("%w: %q", ErrUnknownBot, opts.BotA)
		}
		if !okB {
			return MatchResult{}, fmt.Errorf("%w: %q", ErrUnknownBot, opts.BotB)
		}

		// Alternate who gets the first seat.
		seats := game.Definition.TurnOrder.Seats
		botForSeat := map[int]bots.Bot{}
		whoForSeat := map[int]string{}
		for j, seat := range seats {
			if (i+j)%2 == 0 {
				botForSeat[seat], whoForSeat[seat] = a, "a"
			} else {
				botForSeat[seat], whoForSeat[seat] = b, "b"
			}
		}

		// Games with chance start from a seed derived from the match seed,
		// so a seeded match replays exactly.
		var start *state.BoardState
		if state.IsRuleset(game) && opts.Seed != 0 {
			start, _ = state.InitialStateSeeded(game, uint64(opts.Seed)*1000003+uint64(i)+1)
		}
		if start == nil {
			start = state.InitialState(game)
		}
		g := playOne(ctx, game, start, botForSeat, whoForSeat, maxPlies, opts)
		g.FirstSeatBot = whoForSeat[state.FirstSeat(game)]
		res.Played++
		totalPlies += g.Plies
		switch g.Winner {
		case "a":
			res.AWins++
		case "b":
			res.BWins++
		case "draw":
			res.Draws++
		default:
			res.Unfinished++
		}
		if opts.KeepMoves || opts.Record {
			if !opts.KeepMoves {
				g.Moves = nil
			}
			res.Games = append(res.Games, g)
		} else {
			g.Moves = nil
		}
	}
	if res.Played > 0 {
		res.AvgPlies = float64(totalPlies) / float64(res.Played)
	}
	return res, nil
}

func playOne(ctx context.Context, game *catalog.CompiledGame, start *state.BoardState, botForSeat map[int]bots.Bot, whoForSeat map[int]string, maxPlies int, opts MatchOptions) MatchGame {
	var out MatchGame
	st, seat := start, state.FirstSeat(game)
	keep := opts.KeepMoves
	var space *state.ActionSpace
	if opts.Record {
		space = state.ActionSpaceOf(game)
	}
	// finish stamps every recorded position with the outcome for its mover.
	finish := func(winner *int) {
		for i := range out.Positions {
			switch {
			case winner == nil:
				out.Positions[i].Result = 0
			case *winner == out.Positions[i].ToMove:
				out.Positions[i].Result = 1
			default:
				out.Positions[i].Result = -1
			}
		}
	}

	for ply := 0; ply < maxPlies; ply++ {
		if ctx.Err() != nil {
			out.Winner = "unfinished"
			finish(nil)
			return out
		}
		legal := state.LegalMoves(game, st, seat)
		if len(legal) == 0 {
			out.Winner, out.EndReason = "draw", "no-legal-moves"
			finish(nil)
			return out
		}
		// Bots play within the game's strategy guide, as they do at a table.
		mv, err := botForSeat[seat].Choose(ctx, game, st, seat, state.GuidedMoves(game, st, seat, legal))
		if err != nil {
			mv = legal[0]
		}
		r, err := state.Apply(game, st, seat, mv)
		if err != nil {
			out.Winner = "unfinished"
			finish(nil)
			return out
		}
		out.Plies++
		if keep {
			out.Moves = append(out.Moves, r.Notation)
		}
		if opts.Record {
			rec := PositionRecord{Ply: ply, ToMove: seat, Move: r.Notation, ActionID: -1}
			if doc, err := state.Public(game, st, seat).MarshalJSON(); err == nil {
				rec.State = json.RawMessage(doc)
			}
			if space != nil && space.Fixed {
				if id, ok := space.ID(r.Notation); ok {
					rec.ActionID = id
				}
			}
			if opts.RecordPlanes {
				if p, err := state.EncodePlanes(game, st, seat, seat, state.PlaneOptions{Depth: opts.PlaneDepth}); err == nil {
					rec.Planes = p
				}
			}
			out.Positions = append(out.Positions, rec)
		}
		if r.GameEnded {
			out.EndReason = r.EndReason
			if r.WinnerSeat == nil {
				out.Winner = "draw"
			} else {
				out.Winner = whoForSeat[*r.WinnerSeat]
			}
			finish(r.WinnerSeat)
			return out
		}
		st, seat = r.NewState, r.NextTurnSeat
	}
	out.Winner = "unfinished"
	finish(nil)
	return out
}

// DefaultMatchBudget bounds how long one match call may run.
const DefaultMatchBudget = 25 * time.Second
