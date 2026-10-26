package envs

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/puzzles"
)

// ApplyPuzzle points opts at the puzzle with the given ID: the game, the
// position and its side to move become the episode's start, the agent plays
// the side with the forced win, and — unless MaxPlies is already set — the
// episode is truncated shortly after the proven line (so reward +1 means
// "solved", a truncated episode "not solved").
func ApplyPuzzle(opts *ResetOptions, puzzleID string) error {
	p, ok := puzzles.Get(puzzleID)
	if !ok {
		return fmt.Errorf("unknown puzzle %q (see list_puzzles)", puzzleID)
	}
	if opts.GameDefID != "" && opts.GameDefID != p.GameDefID {
		return fmt.Errorf("puzzle %q is for game %q, not %q", puzzleID, p.GameDefID, opts.GameDefID)
	}
	opts.GameDefID = p.GameDefID
	opts.StartState = string(p.State)
	opts.StartSeat = p.ToMove
	opts.StartTPS = ""
	if opts.Seat == 0 {
		opts.Seat = p.ToMove
	}
	if opts.MaxPlies == 0 {
		var doc struct {
			Ply int `json:"ply"`
		}
		_ = json.Unmarshal(p.State, &doc)
		opts.MaxPlies = doc.Ply + 2*p.Plies + 2
	}
	return nil
}
