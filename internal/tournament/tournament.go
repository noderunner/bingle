// Package tournament holds the rules of a tournament that need no database:
// who plays whom in a round, and how the players stand. It is pure and
// deterministic given a seed, so a tournament can be explained and tested from
// its results alone (the tournaments plan, ADR 0082).
package tournament

import (
	"encoding/binary"
	"hash/fnv"
	"math/bits"
	"sort"
)

// The formats.
const (
	RoundRobin = "round_robin"
	Swiss      = "swiss"
)

// Pair is a pairing by player id, with A taking the first seat of the pairing's
// first game. B is 0 for a bye.
type Pair struct {
	A, B int64
}

// Shuffled is ids in an order that depends only on them and on seed.
func Shuffled(ids []int64, seed int64) []int64 {
	out := append([]int64(nil), ids...)
	sort.Slice(out, func(i, j int) bool {
		hi, hj := tieHash(out[i], seed), tieHash(out[j], seed)
		if hi != hj {
			return hi < hj
		}
		return out[i] < out[j]
	})
	return out
}

func tieHash(id, seed int64) uint64 {
	var b [16]byte
	binary.LittleEndian.PutUint64(b[:8], uint64(id))
	binary.LittleEndian.PutUint64(b[8:], uint64(seed))
	h := fnv.New64a()
	_, _ = h.Write(b[:])
	return h.Sum64()
}

// RoundRobinRounds is how many rounds a round robin of n players takes.
func RoundRobinRounds(n int) int {
	if n < 2 {
		return 0
	}
	if n%2 == 1 {
		return n
	}
	return n - 1
}

// RoundRobinRound is round (1 up to RoundRobinRounds) of a Berger table over ids,
// in the order given: everybody meets everybody once, and each player's first and
// second seats come out within one of each other at an even table. An odd table
// gives one player a bye each round.
func RoundRobinRound(ids []int64, round int) []Pair {
	players := append([]int64(nil), ids...)
	if len(players)%2 == 1 {
		players = append(players, 0)
	}
	n := len(players)
	if n < 2 || round < 1 || round > n-1 {
		return nil
	}
	r := round - 1
	mod := n - 1
	out := make([]Pair, 0, n/2)
	// The last player is the pivot, and takes turns at each side.
	if r%2 == 0 {
		out = append(out, Pair{A: players[r], B: players[n-1]})
	} else {
		out = append(out, Pair{A: players[n-1], B: players[r]})
	}
	for i := 1; i < n/2; i++ {
		x, y := players[(r+i)%mod], players[(r-i+mod)%mod]
		if i%2 == 1 {
			out = append(out, Pair{A: y, B: x})
		} else {
			out = append(out, Pair{A: x, B: y})
		}
	}
	// A bye is always written with its player as A.
	for i, p := range out {
		if p.A == 0 {
			out[i] = Pair{A: p.B}
		}
	}
	return out
}

// DefaultSwissRounds is how many rounds a Swiss tournament of n players plays:
// ceil(log2(n)), which is enough to separate them.
func DefaultSwissRounds(n int) int {
	if n < 2 {
		return 0
	}
	return bits.Len(uint(n - 1))
}

// SwissPlayer is what pairing a Swiss round needs to know about a player.
type SwissPlayer struct {
	ID     int64
	Score  float64
	Rating float64
	// FirstSeats is how many more games the player has started first than
	// second: negative when they are owed the first seat.
	FirstSeats int
	HadBye     bool
	Met        map[int64]bool
}

// swissBudget bounds the search for a pairing with no rematch. It is far more
// than any real table needs, and what it protects against is a late round of a
// small field, where no such pairing exists and proving that takes forever.
const swissBudget = 200000

// SwissRound pairs a round: within score groups, the upper half of a group
// against its lower half, seeded by rating, avoiding rematches and balancing who
// goes first. The bye goes to the lowest-ranked player without one. When no
// pairing without a rematch exists, rematches are allowed rather than leaving
// anyone out. seed settles equal ratings, and round alternates who goes first
// when two players are owed the same.
func SwissRound(players []SwissPlayer, seed int64, round int) []Pair {
	list := append([]SwissPlayer(nil), players...)
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Rating != b.Rating {
			return a.Rating > b.Rating
		}
		return tieHash(a.ID, seed) < tieHash(b.ID, seed)
	})
	var out []Pair
	if len(list)%2 == 1 {
		at := len(list) - 1
		for i := len(list) - 1; i >= 0; i-- {
			if !list[i].HadBye {
				at = i
				break
			}
		}
		out = append(out, Pair{A: list[at].ID})
		list = append(list[:at:at], list[at+1:]...)
	}
	if len(list) == 0 {
		return out
	}
	pairs, ok := pairSwiss(list, false)
	if !ok {
		pairs, _ = pairSwiss(list, true)
	}
	for _, p := range pairs {
		out = append(out, sides(list[p[0]], list[p[1]], round))
	}
	return out
}

// pairSwiss searches for a pairing, best first, by index into list (which is in
// rank order).
func pairSwiss(list []SwissPlayer, rematches bool) ([][2]int, bool) {
	groupSize := map[float64]int{}
	groupStart := map[float64]int{}
	for i, p := range list {
		if groupSize[p.Score] == 0 {
			groupStart[p.Score] = i
		}
		groupSize[p.Score]++
	}
	used := make([]bool, len(list))
	pairs := make([][2]int, 0, len(list)/2)
	budget := swissBudget
	var search func() bool
	search = func() bool {
		i := 0
		for i < len(list) && used[i] {
			i++
		}
		if i == len(list) {
			return true
		}
		if budget--; budget < 0 {
			return false
		}
		used[i] = true
		// The ideal opponent is as far below in the group as half the group.
		p := list[i]
		ideal := i + groupSize[p.Score]/2
		if off := i - groupStart[p.Score]; off >= groupSize[p.Score]/2 {
			ideal = i + 1 // the upper half has gone: whoever is next
		}
		var candidates []int
		for j := i + 1; j < len(list); j++ {
			if used[j] || (!rematches && p.Met[list[j].ID]) {
				continue
			}
			candidates = append(candidates, j)
		}
		sort.SliceStable(candidates, func(a, b int) bool {
			ca, cb := candidates[a], candidates[b]
			da, db := scoreGap(p, list[ca]), scoreGap(p, list[cb])
			if da != db {
				return da < db
			}
			return absInt(ca-ideal) < absInt(cb-ideal)
		})
		for _, j := range candidates {
			used[j] = true
			pairs = append(pairs, [2]int{i, j})
			if search() {
				return true
			}
			pairs = pairs[:len(pairs)-1]
			used[j] = false
		}
		used[i] = false
		return false
	}
	return pairs, search()
}

func scoreGap(a, b SwissPlayer) float64 {
	if a.Score > b.Score {
		return a.Score - b.Score
	}
	return b.Score - a.Score
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// sides gives the first seat to whoever is owed it. When neither is owed more,
// the higher-ranked player (a) has it in odd rounds.
func sides(a, b SwissPlayer, round int) Pair {
	switch {
	case a.FirstSeats < b.FirstSeats:
		return Pair{A: a.ID, B: b.ID}
	case b.FirstSeats < a.FirstSeats:
		return Pair{A: b.ID, B: a.ID}
	case round%2 == 1:
		return Pair{A: a.ID, B: b.ID}
	}
	return Pair{A: b.ID, B: a.ID}
}

// Result is one finished game, forfeit or bye. B is 0 for a bye.
type Result struct {
	Round          int
	A, B           int64
	ScoreA, ScoreB float64
}

// Standing is a player's line in the standings.
type Standing struct {
	ID       int64
	Place    int
	Score    float64
	Games    int
	Won      int
	Drawn    int
	Lost     int
	Buchholz float64
	// SonnebornBerger is what the player scored against each opponent, weighted
	// by that opponent's total.
	SonnebornBerger float64
}

// Standings ranks the players by score, then by the format's tie-breaks:
// Sonneborn-Berger in a round robin; Buchholz and then Sonneborn-Berger in Swiss.
// Players who tie on all of it share a place, and there is no playoff. Equal
// places come out in the order of ids, which the caller may reorder.
func Standings(format string, ids []int64, results []Result) []Standing {
	byID := make(map[int64]*Standing, len(ids))
	for _, id := range ids {
		byID[id] = &Standing{ID: id}
	}
	for _, r := range results {
		if s := byID[r.A]; s != nil {
			s.Score += r.ScoreA
			if r.B != 0 {
				tally(s, r.ScoreA, r.ScoreB)
			}
		}
		if s := byID[r.B]; r.B != 0 && s != nil {
			s.Score += r.ScoreB
			tally(s, r.ScoreB, r.ScoreA)
		}
	}
	// Each opponent counts once a round towards Buchholz, whatever the games per
	// pairing, and a bye counts for nothing.
	type meeting struct {
		round int
		a, b  int64
	}
	met := map[meeting]bool{}
	for _, r := range results {
		if r.B == 0 {
			continue
		}
		a, b := byID[r.A], byID[r.B]
		if a != nil && b != nil {
			a.SonnebornBerger += r.ScoreA * b.Score
			b.SonnebornBerger += r.ScoreB * a.Score
			m := meeting{round: r.Round, a: min(r.A, r.B), b: max(r.A, r.B)}
			if !met[m] {
				met[m] = true
				a.Buchholz += b.Score
				b.Buchholz += a.Score
			}
		}
	}
	out := make([]Standing, 0, len(ids))
	for _, id := range ids {
		out = append(out, *byID[id])
	}
	key := func(s Standing) [3]float64 {
		if format == Swiss {
			return [3]float64{s.Score, s.Buchholz, s.SonnebornBerger}
		}
		return [3]float64{s.Score, s.SonnebornBerger, 0}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := key(out[i]), key(out[j])
		for k := range a {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return out[i].ID < out[j].ID
	})
	for i := range out {
		if i > 0 && key(out[i]) == key(out[i-1]) {
			out[i].Place = out[i-1].Place
		} else {
			out[i].Place = i + 1
		}
	}
	return out
}

func tally(s *Standing, mine, theirs float64) {
	s.Games++
	switch {
	case mine > theirs:
		s.Won++
	case mine < theirs:
		s.Lost++
	case mine > 0:
		s.Drawn++
	default:
		// Nobody scored (both had dropped out): not a win, a loss or a draw.
		s.Games--
	}
}
