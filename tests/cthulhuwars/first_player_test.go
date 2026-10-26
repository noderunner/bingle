package cthulhuwars_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The first player of the game (rule book, "Determine First Player Phase"):
// "On the first turn of the game, Cthulhu is the First Player. If you are in a
// game without the Cthulhu Faction, choose the first player by any means you
// wish (we recommend ... alphabetic order per Faction name — i.e., Black Goat
// is before Yellow Sign)." The first player chooses the direction of play
// ("clockwise" or "counter-clockwise") for the whole round, the first Action
// phase included, and the Doom phase is skipped in the first round.

type firstCase struct {
	name string
	base string
	keys []string
	want string // the cult that is the first player
}

var firstCases = []firstCase{
	{"the standard table", "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, "gc"},
	{"Great Cthulhu seated third", "cthulhu-wars", []string{"ys", "bg", "gc", "cc"}, "gc"},
	{"Great Cthulhu seated last", "cthulhu-wars", []string{"cc", "ys", "bg", "gc"}, "gc"},
	{"three players, Great Cthulhu last", "cthulhu-wars-3p", []string{"bg", "ys", "gc"}, "gc"},
	{"five players with a Windwalker leading the seats", "cthulhu-wars-5p", []string{"ww", "ys", "cc", "gc", "bg"}, "gc"},
	{"five players with the Sleeper", "cthulhu-wars-5p", []string{"sl", "ys", "gc", "ww", "cc"}, "gc"},
	{"no Great Cthulhu: Black Goat is before Yellow Sign", "cthulhu-wars", []string{"ys", "cc", "bg", "sl"}, "bg"},
	{"no Great Cthulhu or Black Goat: Crawling Chaos", "cthulhu-wars", []string{"ys", "sl", "cc", "ww"}, "cc"},
	{"no Great Cthulhu with a Windwalker setting up", "cthulhu-wars-5p", []string{"ww", "ys", "sl", "cc", "bg"}, "bg"},
	{"the Sleeper before the Windwalker and Yellow Sign", "cthulhu-wars-3p", []string{"ys", "ww", "sl"}, "sl"},
}

func TestTheFirstPlayerOfTheGameIsGreatCthulhuElseTheFirstCultByName(t *testing.T) {
	for _, c := range firstCases {
		t.Run(c.name, func(t *testing.T) {
			tb := newTable(t, c.base, c.keys, false)
			want := tb.seat(c.want)
			st := tb.start(11)
			d := tb.data(st)
			if d.First != want {
				t.Fatalf("first player is seat %d (%s), want seat %d (%s)", d.First, c.keys[d.First-1], want, c.want)
			}
			// The platform asks the right player first: the Windwalker to set
			// up, when it is at the table, else the first player.
			mover := want
			if i := indexOfKey(c.keys, "ww"); i >= 0 {
				mover = i + 1
				if d.Stage != "setup" || d.Setup != mover {
					t.Fatalf("stage %q, setup seat %d: the Windwalker sets up before anyone plays", d.Stage, d.Setup)
				}
			}
			if got := state.FirstSeat(tb.g); got != mover {
				t.Fatalf("the platform starts with seat %d, want seat %d", got, mover)
			}
			if got := toMove(tb.g, st); got != mover {
				t.Fatalf("the position waits for seat %d, want seat %d", got, mover)
			}
			if mover != want {
				st = play(t, tb.g, st, mover, "start@"+firstSigil(t, tb, st))
			}
			d = tb.data(st)
			if d.Stage != "direction" || !d.Opening || d.Turn != nil || d.Phase != "action" || d.Round != 1 {
				t.Fatalf("stage %q opening %v turn %+v phase %q round %d: the first player has yet to choose the direction", d.Stage, d.Opening, d.Turn, d.Phase, d.Round)
			}
			// Only the first player decides, between the two directions.
			for seat := 1; seat <= len(c.keys); seat++ {
				got := notations(tb.g, st, seat)
				if seat != want {
					if len(got) != 0 {
						t.Errorf("seat %d may move before the first player has chosen: %v", seat, got)
					}
					continue
				}
				if len(got) != 2 || !has(got, "cw") || !has(got, "ccw") {
					t.Errorf("the first player's choices: %v", got)
				}
			}
			tb.valid(st)

			// Clockwise: down the list of seats, the first player starting.
			cw := play(t, tb.g, st, want, "cw")
			d = tb.data(cw)
			if d.Stage != "turn" || d.Opening || d.Turn == nil || d.Turn.Seat != want || d.Dir != 1 || d.Phase != "action" || d.Round != 1 {
				t.Fatalf("after cw: stage %q opening %v turn %+v dir %d phase %q round %d", d.Stage, d.Opening, d.Turn, d.Dir, d.Phase, d.Round)
			}
			if got := toMove(tb.g, cw); got != want {
				t.Fatalf("the first player takes the first Action: seat %d is to move", got)
			}
			lines := lastLines(d)
			if !strings.Contains(lines, "is the first player") || !strings.Contains(lines, "chooses to play clockwise") || !strings.Contains(lines, "— Round 1 —") {
				t.Errorf("the log does not say who begins and how: %q", lines)
			}
			if strings.Contains(lines, "Doom") {
				t.Errorf("the Doom phase is skipped in the first round: %q", lines)
			}

			// Counter-clockwise: up the list.
			ccw := play(t, tb.g, st, want, "ccw")
			if d := tb.data(ccw); d.Dir != -1 || d.Turn == nil || d.Turn.Seat != want {
				t.Errorf("after ccw: dir %d turn %+v", d.Dir, d.Turn)
			}
			tb.valid(cw)
			tb.valid(ccw)
		})
	}
}

// The order of play follows the direction from the first player: after the
// first player's Action the next seat down (clockwise) or up (counter-
// clockwise) the list of seats acts.
func TestTheDirectionOfTheFirstRoundIsFollowedFromTheFirstPlayer(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"ys", "bg", "gc", "cc"}, false)
	for _, c := range []struct {
		dir  string
		next int
	}{{"cw", 4}, {"ccw", 2}} {
		st := tb.start(5)
		st = play(t, tb.g, st, 3, c.dir)
		// Great Cthulhu (seat 3) spends its whole turn: one Move, then done.
		moves := notations(tb.g, st, 3)
		var mv string
		for _, n := range moves {
			if strings.HasPrefix(n, "go:cultist@") {
				mv = n
				break
			}
		}
		if mv == "" {
			t.Fatalf("no Move to start with: %v", moves)
		}
		st = play(t, tb.g, st, 3, mv)
		st = play(t, tb.g, st, 3, "done")
		if got := toMove(tb.g, st); got != c.next {
			t.Errorf("%s: after Great Cthulhu seat %d acts, want seat %d", c.dir, got, c.next)
		}
	}
}

// A game that is not new (a position without the opening flag) is played as
// before: nothing about the order of play changes for it.
func TestAPositionBeyondTheOpeningKeepsItsOrder(t *testing.T) {
	g := game(t)
	p := pos{first: 3, dir: -1, power: powers(8, 8, 8, 8), turn: 3}
	st := p.build()
	if d := data(t, st); d.Opening || d.First != 3 || d.Dir != -1 {
		t.Fatalf("opening %v first %d dir %d", d.Opening, d.First, d.Dir)
	}
	if got := notations(g, st, 3); has(got, "cw") || has(got, "ccw") {
		t.Errorf("the direction is only chosen at the start of a round: %v", got)
	}
}

func indexOfKey(keys []string, k string) int {
	for i, x := range keys {
		if x == k {
			return i
		}
	}
	return -1
}

// firstSigil is a sigil area the Windwalker may begin in.
func firstSigil(t testing.TB, tb *table, st *state.BoardState) string {
	t.Helper()
	for _, n := range notations(tb.g, st, tb.seat("ww")) {
		if strings.HasPrefix(n, "start@") {
			return strings.TrimPrefix(n, "start@")
		}
	}
	t.Fatalf("the Windwalker has no place to begin")
	return ""
}

// lastLines is the log of the latest decision, in one string.
func lastLines(d *cthulhuwars.Data) string {
	if len(d.Log) == 0 {
		return ""
	}
	return strings.Join(d.Log[len(d.Log)-1].Lines, "\n")
}

// toMove is the seat a position waits for.
func toMove(g *catalog.CompiledGame, st *state.BoardState) int {
	rs, ok := state.RulesetOf(g)
	if !ok {
		return 0
	}
	return rs.ToMove(g, st)
}
