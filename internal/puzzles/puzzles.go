package puzzles

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Puzzle is one "win in N" position.
type Puzzle struct {
	ID        string `json:"id"`
	GameDefID string `json:"game_def_id"`
	Title     string `json:"title"`
	// ToMove is the seat that has the forced win.
	ToMove int `json:"to_move"`
	// WinIn is the number of the solver's own moves it takes (1 = mate in
	// one), Plies the total plies including the defender's replies.
	WinIn int `json:"win_in"`
	Plies int `json:"plies"`
	// TPS is the position in TPS for PTN games; State is the position
	// document (every game).
	TPS   string          `json:"tps,omitempty"`
	State json.RawMessage `json:"state"`
	// FirstMoves lists every winning first move; Solution is one winning
	// line (the attacker's moves and the defender's replies, notation).
	FirstMoves []string `json:"first_moves"`
	Solution   []string `json:"solution"`
}

//go:embed puzzles.json
var puzzlesJSON []byte

var (
	loadOnce sync.Once
	loaded   []Puzzle
	byID     map[string]Puzzle
	loadErr  error
)

func load() {
	loadOnce.Do(func() {
		if err := json.Unmarshal(puzzlesJSON, &loaded); err != nil {
			loadErr = fmt.Errorf("puzzles: parsing puzzles.json: %w", err)
			return
		}
		byID = make(map[string]Puzzle, len(loaded))
		for _, p := range loaded {
			byID[p.ID] = p
		}
	})
}

// All returns every puzzle, ordered by game, difficulty then ID.
func All() ([]Puzzle, error) {
	load()
	if loadErr != nil {
		return nil, loadErr
	}
	out := append([]Puzzle(nil), loaded...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].GameDefID != out[j].GameDefID {
			return out[i].GameDefID < out[j].GameDefID
		}
		if out[i].WinIn != out[j].WinIn {
			return out[i].WinIn < out[j].WinIn
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Get returns the puzzle with the given ID.
func Get(id string) (Puzzle, bool) {
	load()
	if loadErr != nil {
		return Puzzle{}, false
	}
	p, ok := byID[id]
	return p, ok
}

// Filter returns the puzzles of one game (empty = all) with at most maxWinIn
// moves (0 = no limit).
func Filter(gameDefID string, maxWinIn int) ([]Puzzle, error) {
	all, err := All()
	if err != nil {
		return nil, err
	}
	var out []Puzzle
	for _, p := range all {
		if gameDefID != "" && p.GameDefID != gameDefID {
			continue
		}
		if maxWinIn > 0 && p.WinIn > maxWinIn {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
