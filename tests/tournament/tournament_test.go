package tournament_test

import (
	"fmt"
	"testing"

	"github.com/noderunner/bingle/internal/tournament"
)

func ids(n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = int64(i + 1)
	}
	return out
}

func TestRoundRobinEveryoneMeetsEveryoneOnce(t *testing.T) {
	for n := 2; n <= 21; n++ {
		players := ids(n)
		met := map[[2]int64]int{}
		first := map[int64]int{}
		second := map[int64]int{}
		byes := map[int64]int{}
		for round := 1; round <= tournament.RoundRobinRounds(n); round++ {
			seen := map[int64]bool{}
			pairs := tournament.RoundRobinRound(players, round)
			if want := (n + 1) / 2; len(pairs) != want {
				t.Fatalf("%d players, round %d: %d pairs, want %d", n, round, len(pairs), want)
			}
			for _, p := range pairs {
				if seen[p.A] || (p.B != 0 && seen[p.B]) {
					t.Fatalf("%d players, round %d: somebody plays twice: %v", n, round, pairs)
				}
				seen[p.A], seen[p.B] = true, true
				if p.B == 0 {
					byes[p.A]++
					continue
				}
				met[[2]int64{min(p.A, p.B), max(p.A, p.B)}]++
				first[p.A]++
				second[p.B]++
			}
		}
		if want := n * (n - 1) / 2; len(met) != want {
			t.Errorf("%d players: %d pairings, want %d", n, len(met), want)
		}
		for pair, count := range met {
			if count != 1 {
				t.Errorf("%d players: %v met %d times", n, pair, count)
			}
		}
		for _, id := range players {
			if n%2 == 1 && byes[id] != 1 {
				t.Errorf("%d players: %d had %d byes", n, id, byes[id])
			}
			if d := first[id] - second[id]; n%2 == 0 && (d < -1 || d > 1) || d < -2 || d > 2 {
				t.Errorf("%d players: %d went first %d times and second %d times", n, id, first[id], second[id])
			}
		}
	}
}

func TestDefaultSwissRounds(t *testing.T) {
	for n, want := range map[int]int{1: 0, 2: 1, 3: 2, 4: 2, 5: 3, 8: 3, 9: 4, 16: 4, 17: 5} {
		if got := tournament.DefaultSwissRounds(n); got != want {
			t.Errorf("%d players: %d rounds, want %d", n, got, want)
		}
	}
}

// swissField plays out a Swiss tournament, the higher id always winning, and
// checks each round: nobody plays twice, no rematch, a bye only for who needs one.
func TestSwissPlaysOutWithoutRematches(t *testing.T) {
	for _, n := range []int{2, 3, 4, 5, 6, 7, 8, 9, 15, 16, 33, 64} {
		rounds := tournament.DefaultSwissRounds(n)
		players := make(map[int64]*tournament.SwissPlayer, n)
		list := ids(n)
		for _, id := range list {
			players[id] = &tournament.SwissPlayer{ID: id, Rating: float64(1500 + id), Met: map[int64]bool{}}
		}
		for round := 1; round <= rounds; round++ {
			var field []tournament.SwissPlayer
			for _, id := range list {
				field = append(field, *players[id])
			}
			pairs := tournament.SwissRound(field, 7, round)
			if want := (n + 1) / 2; len(pairs) != want {
				t.Fatalf("%d players, round %d: %d pairs, want %d", n, round, len(pairs), want)
			}
			seen := map[int64]bool{}
			for _, p := range pairs {
				if seen[p.A] || seen[p.B] {
					t.Fatalf("%d players, round %d: somebody plays twice: %v", n, round, pairs)
				}
				seen[p.A] = true
				if p.B == 0 {
					if players[p.A].HadBye {
						t.Errorf("%d players, round %d: %d has a second bye", n, round, p.A)
					}
					players[p.A].HadBye = true
					players[p.A].Score++
					continue
				}
				seen[p.B] = true
				if players[p.A].Met[p.B] {
					t.Errorf("%d players, round %d: a rematch, %d and %d", n, round, p.A, p.B)
				}
				players[p.A].Met[p.B], players[p.B].Met[p.A] = true, true
				players[p.A].FirstSeats++
				players[p.B].FirstSeats--
				if p.A > p.B {
					players[p.A].Score++
				} else {
					players[p.B].Score++
				}
			}
		}
		for _, id := range list {
			if d := players[id].FirstSeats; d < -2 || d > 2 {
				t.Errorf("%d players: %d is %d out on the first seat", n, id, d)
			}
		}
	}
}

func TestSwissPairsWithinScoreGroupsTopHalfAgainstBottomHalf(t *testing.T) {
	field := []tournament.SwissPlayer{
		{ID: 1, Score: 1, Rating: 1800, Met: map[int64]bool{}},
		{ID: 2, Score: 1, Rating: 1700, Met: map[int64]bool{}},
		{ID: 3, Score: 1, Rating: 1600, Met: map[int64]bool{}},
		{ID: 4, Score: 1, Rating: 1500, Met: map[int64]bool{}},
		{ID: 5, Score: 0, Rating: 1400, Met: map[int64]bool{}},
		{ID: 6, Score: 0, Rating: 1300, Met: map[int64]bool{}},
	}
	got := map[int64]int64{}
	for _, p := range tournament.SwissRound(field, 1, 1) {
		got[p.A], got[p.B] = p.B, p.A
	}
	for a, b := range map[int64]int64{1: 3, 2: 4, 5: 6} {
		if got[a] != b {
			t.Errorf("%d plays %d, want %d (all: %v)", a, got[a], b, got)
		}
	}
}

func TestSwissAvoidsARematchInTheGroup(t *testing.T) {
	field := []tournament.SwissPlayer{
		{ID: 1, Score: 1, Rating: 1800, Met: map[int64]bool{3: true}},
		{ID: 2, Score: 1, Rating: 1700, Met: map[int64]bool{}},
		{ID: 3, Score: 1, Rating: 1600, Met: map[int64]bool{1: true}},
		{ID: 4, Score: 1, Rating: 1500, Met: map[int64]bool{}},
	}
	for _, p := range tournament.SwissRound(field, 1, 2) {
		if (p.A == 1 && p.B == 3) || (p.A == 3 && p.B == 1) {
			t.Errorf("paired a rematch: %+v", p)
		}
	}
}

func TestSwissByeGoesToTheLowestWithoutOne(t *testing.T) {
	field := []tournament.SwissPlayer{
		{ID: 1, Score: 2, Rating: 1800, Met: map[int64]bool{}},
		{ID: 2, Score: 1, Rating: 1700, Met: map[int64]bool{}},
		{ID: 3, Score: 0, Rating: 1600, HadBye: true, Met: map[int64]bool{}},
	}
	for _, p := range tournament.SwissRound(field, 1, 3) {
		if p.B == 0 && p.A != 2 {
			t.Errorf("the bye went to %d, want 2", p.A)
		}
	}
}

// Nobody can be left out of a late round where a rematch cannot be avoided.
func TestSwissRematchesRatherThanLeavingAnyoneOut(t *testing.T) {
	field := []tournament.SwissPlayer{
		{ID: 1, Met: map[int64]bool{2: true}},
		{ID: 2, Met: map[int64]bool{1: true}},
	}
	got := tournament.SwissRound(field, 1, 2)
	if len(got) != 1 || got[0].B == 0 {
		t.Fatalf("got %+v, want the rematch", got)
	}
}

func TestPairingIsDeterministicGivenTheSeed(t *testing.T) {
	field := make([]tournament.SwissPlayer, 8)
	for i := range field {
		field[i] = tournament.SwissPlayer{ID: int64(i + 1), Rating: 1500, Met: map[int64]bool{}}
	}
	first := fmt.Sprint(tournament.SwissRound(field, 42, 1))
	for range 5 {
		if got := fmt.Sprint(tournament.SwissRound(field, 42, 1)); got != first {
			t.Fatalf("%s, then %s", first, got)
		}
	}
	once, again := fmt.Sprint(tournament.Shuffled(ids(8), 1)), fmt.Sprint(tournament.Shuffled(ids(8), 1))
	if once != again {
		t.Error("one seed gave two orders")
	}
	if once == fmt.Sprint(tournament.Shuffled(ids(8), 2)) {
		t.Error("two seeds gave one order")
	}
}

func TestStandingsScoresAndSonnebornBerger(t *testing.T) {
	// 1 beats 2, 2 beats 3, 3 beats 1: all on one point, so all share first.
	cycle := []tournament.Result{
		{Round: 1, A: 1, B: 2, ScoreA: 1},
		{Round: 2, A: 2, B: 3, ScoreA: 1},
		{Round: 3, A: 3, B: 1, ScoreA: 1},
	}
	for _, s := range tournament.Standings(tournament.RoundRobin, []int64{1, 2, 3}, cycle) {
		if s.Place != 1 || s.Score != 1 {
			t.Errorf("%+v, want a share of first on one point", s)
		}
	}
	// 1 and 2 tie on 1.5, and 1 beat the stronger player.
	games := []tournament.Result{
		{Round: 1, A: 1, B: 2, ScoreA: 0.5, ScoreB: 0.5},
		{Round: 1, A: 3, B: 4, ScoreA: 1},
		{Round: 2, A: 1, B: 3, ScoreA: 1},
		{Round: 2, A: 2, B: 4, ScoreA: 1},
	}
	got := tournament.Standings(tournament.RoundRobin, []int64{2, 1, 3, 4}, games)
	if got[0].ID != 1 || got[0].Place != 1 || got[1].ID != 2 || got[1].Place != 2 {
		t.Errorf("%+v, want 1 ahead of 2 on Sonneborn-Berger", got)
	}
	if got[0].SonnebornBerger != 0.75+1 {
		t.Errorf("Sonneborn-Berger %v, want 1.75", got[0].SonnebornBerger)
	}
}

func TestStandingsBuchholzCountsAPairingOnce(t *testing.T) {
	games := []tournament.Result{
		{Round: 1, A: 1, B: 2, ScoreA: 1},
		{Round: 1, A: 2, B: 1, ScoreB: 1},
		{Round: 1, A: 3, B: 4, ScoreA: 1},
		{Round: 1, A: 4, B: 3, ScoreA: 1},
	}
	got := tournament.Standings(tournament.Swiss, []int64{1, 2, 3, 4}, games)
	if got[0].ID != 1 || got[0].Score != 2 || got[0].Buchholz != 0 {
		t.Errorf("%+v, want 1 first on two points with a Buchholz of 0", got[0])
	}
	if got[1].Buchholz != 1 || got[2].Buchholz != 1 {
		t.Errorf("%+v: 3 and 4 have each other's single point once, not twice", got)
	}
}

func TestStandingsByeAndDoubleForfeit(t *testing.T) {
	games := []tournament.Result{
		{Round: 1, A: 1, ScoreA: 2},
		{Round: 1, A: 2, B: 3},
	}
	got := tournament.Standings(tournament.Swiss, []int64{1, 2, 3}, games)
	if got[0].ID != 1 || got[0].Score != 2 || got[0].Games != 0 {
		t.Errorf("%+v, want two points and no game for the bye", got[0])
	}
	if got[1].Lost+got[1].Won+got[1].Drawn != 0 || got[1].Place != 2 || got[2].Place != 2 {
		t.Errorf("%+v, want a shared place with nothing counted", got)
	}
}
