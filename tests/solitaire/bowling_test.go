package solitaire_test

import (
	"encoding/json"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// rack is a position part way into a game: pins by letter ("" for one that is down),
// ball piles from the bottom up with the top face up.
func rack(t *testing.T, g *catalog.CompiledGame, pins map[string]string, balls [3]string, fresh bool, frame, ball int, rolls [][]int) *state.BoardState {
	t.Helper()
	piles := map[string]string{"ball1": balls[0], "ball2": balls[1], "ball3": balls[2]}
	for letter, c := range pins {
		piles["pin"+letter] = c
	}
	return position(t, g, piles, func(ext *map[string]any) {
		e := *ext
		e["fresh"] = fresh
		e["frame"] = frame
		e["ball"] = ball
		e["knocked"] = 0
		e["rolls"] = rolls
	})
}

func fullRack() map[string]string {
	return map[string]string{
		"a": "hearts-2", "b": "hearts-5", "c": "hearts-9", "d": "hearts-4",
		"e": "hearts-A", "f": "spades-2", "g": "hearts-3",
		"h": "hearts-6", "i": "hearts-7", "j": "spades-4",
	}
}

func TestBowling_SetUp(t *testing.T) {
	g := game(t, "bowling-solitaire")
	st, err := state.InitialStateSeeded(g, 9)
	if err != nil {
		t.Fatal(err)
	}
	d := read(t, st)
	for _, l := range "abcdefghij" {
		if count(d, "pin"+string(l)) != 1 {
			t.Errorf("pin %c: %d cards", l, count(d, "pin"+string(l)))
		}
	}
	for i, want := range []int{5, 3, 2} {
		z := "ball" + string(rune('1'+i))
		if count(d, z) != want {
			t.Errorf("%s: %d cards", z, count(d, z))
		}
		cards := d.Table.Zones[z].Cards
		if !cards[want-1].Up || (want > 1 && cards[want-2].Up) {
			t.Errorf("%s: only the top card is up: %+v", z, cards)
		}
	}
}

// The first play cannot reach the back row or take the centre pin alone.
func TestBowling_TheFirstPlay(t *testing.T) {
	g := game(t, "bowling-solitaire")
	st := rack(t, g, fullRack(), [3]string{"?hearts-8 spades-A", "?hearts-10 spades-2", "?spades-9 spades-5"}, true, 1, 1, [][]int{{}})
	wantMoves(t, g, st,
		[]string{"e@1", "f+g@3"},
		[]string{
			"f@2",   // the centre pin alone
			"a@2",   // the back row
			"b@3",   // the back row, the sum right
			"a+e@2", // the back row in a group
		})
}

// After the first play, a group must include a pin next to a gap. Here e is down.
func TestBowling_LaterPlaysTouchAGap(t *testing.T) {
	g := game(t, "bowling-solitaire")
	pins := fullRack()
	pins["e"] = ""
	st := rack(t, g, pins, [3]string{"?hearts-8 spades-6", "?hearts-10 spades-2", "?spades-9 spades-3"}, false, 1, 1, [][]int{{}})
	wantMoves(t, g, st,
		[]string{"h@1", "a@2", "f@2"},
		[]string{
			"g@3",   // 3 is right, but g touches no gap
			"c+d@3", // 13 ends in 3, but neither touches a gap
		})
}

func TestBowling_TheLastDigitOfTheSum(t *testing.T) {
	g := game(t, "bowling-solitaire")
	// 7 + 9 = 16: a 6 takes them. A 10 counts as 0: 4 + 6 = 10.
	pins := map[string]string{"e": "hearts-7", "f": "spades-9", "g": "hearts-4", "h": "hearts-6", "i": "hearts-A"}
	st := rack(t, g, pins, [3]string{"?hearts-8 spades-6", "?hearts-2 spades-10", "?spades-3 hearts-3"}, true, 1, 1, [][]int{{}})
	// g and h come to 10 as well, but they do not touch.
	wantMoves(t, g, st, []string{"e+f@1"}, []string{"e+g@1", "g@2", "g+h@2"})
	st2 := rack(t, g, map[string]string{"h": "hearts-4", "i": "spades-6", "j": "hearts-A"}, [3]string{"", "?hearts-2 spades-10", ""}, false, 1, 1, [][]int{{}})
	wantMoves(t, g, st2, []string{"h+i@2"}, nil)
}

func TestBowling_NotAllThreeNeedTouchEachOther(t *testing.T) {
	g := game(t, "bowling-solitaire")
	// a touches b, b touches c, but a and c do not touch: a chain will do.
	pins := fullRack()
	pins["f"] = ""
	st := rack(t, g, pins, [3]string{"?hearts-8 spades-6", "?hearts-10 spades-8", "?spades-9 spades-10"}, false, 1, 1, [][]int{{}})
	// 2 + 5 + 9 = 16.
	wantMoves(t, g, st, []string{"a+b+c@1"}, []string{"a+c@1"})
}

// A ball that clears the pins is a strike, and it goes straight to the next frame.
func TestBowling_Strike(t *testing.T) {
	g := game(t, "bowling-solitaire")
	st := rack(t, g, map[string]string{"h": "hearts-4"}, [3]string{"?hearts-8 spades-4", "?hearts-10", "?spades-9"}, false, 1, 1, [][]int{{}})
	// The other nine went on the first ball, already.
	st = withKnocked(t, st, 9)
	st = play(t, g, st, "h@1")
	d := read(t, st)
	if d.Frame != 2 || d.Ball != 1 || len(d.Rolls[0]) != 1 || d.Rolls[0][0] != 10 {
		t.Errorf("frame %d, ball %d, rolls %v", d.Frame, d.Ball, d.Rolls)
	}
	for _, l := range "abcdefghij" {
		if count(d, "pin"+string(l)) != 1 {
			t.Errorf("the new rack has no pin %c", l)
		}
	}
}

func withKnocked(t *testing.T, st *state.BoardState, n int) *state.BoardState {
	t.Helper()
	b, err := st.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var whole map[string]any
	if err := json.Unmarshal(b, &whole); err != nil {
		t.Fatal(err)
	}
	whole["ext"].(map[string]any)["knocked"] = n
	b, err = json.Marshal(whole)
	if err != nil {
		t.Fatal(err)
	}
	out := state.NewBoardState()
	if err := out.UnmarshalJSON(b); err != nil {
		t.Fatal(err)
	}
	return out
}

// When no play is left the ball ends by itself: the top of each pile is thrown away
// and the next card turned up.
func TestBowling_SecondBall(t *testing.T) {
	g := game(t, "bowling-solitaire")
	st := rack(t, g, map[string]string{"h": "hearts-4", "i": "hearts-8"}, [3]string{"?hearts-3 ?hearts-6 spades-4", "?spades-8 spades-7", "?spades-A spades-9"}, false, 1, 1, [][]int{{}})
	st = withKnocked(t, st, 8)
	wantMoves(t, g, st, []string{"h@1", "end"}, nil)
	st = play(t, g, st, "h@1")
	d := read(t, st)
	if d.Ball != 2 || d.Rolls[0][0] != 9 {
		t.Fatalf("ball %d, rolls %v", d.Ball, d.Rolls)
	}
	// Pile 1 lost the 4 to the play and the 6 to the end of the ball.
	if top := d.Table.Zones["ball1"].Cards[0]; count(d, "ball1") != 1 || top.ID != "hearts-3" || !top.Up {
		t.Errorf("pile 1: %+v", d.Table.Zones["ball1"])
	}
	if top := d.Table.Zones["ball2"].Cards[0]; count(d, "ball2") != 1 || top.ID != "spades-8" || !top.Up {
		t.Errorf("pile 2: %+v", d.Table.Zones["ball2"])
	}
}

// You can end a ball while a play is left; a second ball with nothing to do ends the frame.
func TestBowling_EndingABallYourself(t *testing.T) {
	g := game(t, "bowling-solitaire")
	st := rack(t, g, map[string]string{"h": "hearts-4", "i": "hearts-8"}, [3]string{"?spades-3 spades-4", "?spades-5 spades-7", ""}, false, 1, 1, [][]int{{}})
	st = play(t, g, st, "end")
	d := read(t, st)
	if d.Frame != 2 || len(d.Rolls[0]) != 2 {
		t.Errorf("after ending: frame %d, rolls %v", d.Frame, d.Rolls)
	}
}

func TestBowling_Scoring(t *testing.T) {
	g := game(t, "bowling-solitaire")
	strike := func(n int) [][]int {
		out := [][]int{}
		for i := 0; i < n; i++ {
			out = append(out, []int{10})
		}
		return out
	}
	cases := []struct {
		name  string
		rolls [][]int
		want  string
	}{
		{"a perfect game", append(strike(9), []int{10, 10, 10}), "300 points"},
		{"all gutter", [][]int{{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}}, "0 points"},
		{"all nines", [][]int{{9, 0}, {9, 0}, {9, 0}, {9, 0}, {9, 0}, {9, 0}, {9, 0}, {9, 0}, {9, 0}, {9, 0}}, "90 points"},
		{"all spares of five", [][]int{{5, 5}, {5, 5}, {5, 5}, {5, 5}, {5, 5}, {5, 5}, {5, 5}, {5, 5}, {5, 5}, {5, 5, 5}}, "150 points"},
		{"a strike and a spare", [][]int{{10}, {7, 3}, {4, 2}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}}, "40 points"},
		{"the last frame open", [][]int{{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {3, 4}}, "7 points"},
	}
	outcome := func(st *state.BoardState) state.Outcome { out, _ := state.Evaluate(g, st, 1); return out }
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := position(t, g, nil, func(ext *map[string]any) {
				e := *ext
				e["rolls"] = c.rolls
				e["frame"] = 10
				e["turn"] = 0
				e["reason"] = "score"
			})
			if out := outcome(st); !out.Ended || out.Detail != c.want || out.Winner != nil {
				t.Errorf("outcome %+v, want %s", out, c.want)
			}
		})
	}
}

// The tenth frame: a strike earns two balls on a new rack; a second strike a new rack again.
func TestBowling_TheTenthFrame(t *testing.T) {
	g := game(t, "bowling-solitaire")
	nine := [][]int{{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}}
	frame := func(r []int) [][]int { return append(append([][]int{}, nine...), r) }
	last := map[string]string{"h": "hearts-4", "i": "hearts-8"}

	// The strike: one pin left and the ball to take it.
	strike := withKnocked(t, rack(t, g, map[string]string{"h": "hearts-4"}, [3]string{"?spades-8 spades-4", "?hearts-A", ""}, false, 10, 1, frame([]int{})), 9)
	d := read(t, play(t, g, strike, "h@1"))
	if d.Frame != 10 || d.Ball != 2 || d.Turn == 0 || !d.Fresh && len(d.Rolls[9]) != 1 {
		t.Errorf("after a strike: frame %d ball %d turn %d rolls %v", d.Frame, d.Ball, d.Turn, d.Rolls[9])
	}
	for _, l := range "abcdefghij" {
		if count(d, "pin"+string(l)) != 1 {
			t.Errorf("a new rack after a strike has no pin %c", l)
		}
	}

	// An open frame ends the game after the second ball.
	open := withKnocked(t, rack(t, g, last, [3]string{"?spades-8 spades-4", "?hearts-A", ""}, false, 10, 2, frame([]int{3})), 2)
	end := read(t, play(t, g, open, "end"))
	if end.Turn != 0 || end.Reason != "score" {
		t.Errorf("an open tenth: turn %d, reason %q", end.Turn, end.Reason)
	}

	// A spare on the second ball earns exactly one more, on a new rack.
	spare := withKnocked(t, rack(t, g, map[string]string{"h": "hearts-4"}, [3]string{"?spades-8 spades-4", "?hearts-A", ""}, false, 10, 2, frame([]int{3})), 6)
	d = read(t, play(t, g, spare, "h@1"))
	if d.Turn == 0 || d.Ball != 3 || d.Rolls[9][0]+d.Rolls[9][1] != 10 {
		t.Errorf("a spare in the tenth: turn %d ball %d rolls %v", d.Turn, d.Ball, d.Rolls[9])
	}
	for _, l := range "abcdefghij" {
		if count(d, "pin"+string(l)) != 1 {
			t.Errorf("a new rack after a spare has no pin %c", l)
		}
	}

	// The extra ball ends the game, however it goes.
	extra := withKnocked(t, rack(t, g, last, [3]string{"?spades-8 spades-4", "?hearts-A", ""}, false, 10, 3, frame([]int{3, 7})), 2)
	end = read(t, play(t, g, extra, "end"))
	if end.Turn != 0 {
		t.Errorf("the extra ball did not end the game: ball %d", end.Ball)
	}
}

// A whole game, played by always taking the first play: it reaches its end, the
// score adds up, and every rack keeps all twenty cards.
func TestBowling_AWholeGame(t *testing.T) {
	g := game(t, "bowling-solitaire")
	outcome := func(st *state.BoardState) state.Outcome { out, _ := state.Evaluate(g, st, 1); return out }
	for seed := uint64(1); seed <= 25; seed++ {
		st, err := state.InitialStateSeeded(g, seed)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 400; i++ {
			moves := state.LegalSelections(g, st, 1)
			if len(moves) == 0 {
				break
			}
			res, err := state.Apply(g, st, 1, moves[0])
			if err != nil {
				t.Fatal(err)
			}
			st = res.NewState
			d := read(t, st)
			total := 0
			for _, z := range d.Table.Zones {
				total += len(z.Cards)
			}
			if total != 20 {
				t.Fatalf("seed %d: %d cards on the table", seed, total)
			}
			if d.Turn == 0 {
				break
			}
		}
		out := outcome(st)
		if !out.Ended || out.Reason != "score" {
			t.Fatalf("seed %d: outcome %+v", seed, out)
		}
		if d := read(t, st); len(d.Rolls) != 10 {
			t.Errorf("seed %d: %d frames", seed, len(d.Rolls))
		}
	}
}
