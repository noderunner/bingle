package solitaire

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/cards"
)

// Bowling Solitaire (Sid Sackson, A Gamut of Games, 1969). Ten pins in a triangle,
// three piles of balls. A play is a face-up ball and the pins it knocks down, written
// e+h@2: pins e and h, with the ball on pile 2. "end" ends the ball.

const moveEnd = "end"

const (
	pinLetters = "abcdefghij"
	backRow    = "abcd"
	centrePin  = "f"
)

// pinNeighbours says which pins touch: a b c d at the back, then e f g, h i and j.
var pinNeighbours = map[byte]string{
	'a': "be",
	'b': "acef",
	'c': "bdfg",
	'd': "cg",
	'e': "abfh",
	'f': "bcegih",
	'g': "cdfi",
	'h': "efij",
	'i': "fghj",
	'j': "hi",
}

func pinZone(letter byte) string { return "pin" + string(letter) }
func ballZone(i int) string      { return "ball" + strconv.Itoa(i) }

func touches(a, b byte) bool { return strings.IndexByte(pinNeighbours[a], b) >= 0 }

var ballSizes = [3]int{5, 3, 2}

func (e *eng) bowlingStart() {
	e.d.Frame, e.d.Ball = 1, 1
	e.d.Rolls = [][]int{{}}
	e.zone(zoneGone)
	e.rack()
	e.settleBall()
}

// rack shuffles all twenty cards and sets them out again: ten pins, and three piles
// of balls with their top cards turned up.
func (e *eng) rack() {
	for _, z := range e.d.Table.Zones {
		z.Cards = nil
	}
	deck := e.shuffled()
	for i := 0; i < len(pinLetters); i++ {
		c := deck[i]
		c.Up = true
		e.zone(pinZone(pinLetters[i])).Cards = []cards.Card{c}
	}
	at := len(pinLetters)
	for i, size := range ballSizes {
		z := e.zone(ballZone(i + 1))
		z.Cards = append([]cards.Card(nil), deck[at:at+size]...)
		at += size
		z.Cards[size-1].Up = true
	}
	e.d.Fresh = true
	e.d.Knocked = 0
}

// standing lists the letters of the pins still up, in order.
func (e *eng) standing() string {
	var b strings.Builder
	for i := 0; i < len(pinLetters); i++ {
		if e.zone(pinZone(pinLetters[i])).Len() > 0 {
			b.WriteByte(pinLetters[i])
		}
	}
	return b.String()
}

// groups lists every set of one, two or three pins that touch one another in a chain.
func groups(up string) []string {
	var out []string
	for i := 0; i < len(up); i++ {
		out = append(out, up[i:i+1])
		for j := i + 1; j < len(up); j++ {
			ab := touches(up[i], up[j])
			if ab {
				out = append(out, up[i:i+1]+up[j:j+1])
			}
			for k := j + 1; k < len(up); k++ {
				ac, bc := touches(up[i], up[k]), touches(up[j], up[k])
				if (ab && ac) || (ab && bc) || (ac && bc) {
					out = append(out, up[i:i+1]+up[j:j+1]+up[k:k+1])
				}
			}
		}
	}
	return out
}

// allowedGroup says whether a group may be knocked down now. The first play on a rack
// cannot reach the back row or take the centre pin alone; every later one must touch
// a pin already down.
func (e *eng) allowedGroup(group, up string) bool {
	if e.d.Fresh {
		return !strings.ContainsAny(group, backRow) && group != centrePin
	}
	for i := 0; i < len(group); i++ {
		for _, n := range []byte(pinNeighbours[group[i]]) {
			if strings.IndexByte(up, n) < 0 {
				return true
			}
		}
	}
	return false
}

// plays lists the plays that can be made with the balls showing.
func (e *eng) plays() []string {
	up := e.standing()
	var ok []string
	sums := map[string]int{}
	for _, g := range groups(up) {
		if !e.allowedGroup(g, up) {
			continue
		}
		sum := 0
		for i := 0; i < len(g); i++ {
			sum += e.value(e.zone(pinZone(g[i])).Cards[0])
		}
		sums[g] = sum
		ok = append(ok, g)
	}
	var out []string
	for b := 1; b <= len(ballSizes); b++ {
		top, has := e.zone(ballZone(b)).Top()
		if !has {
			continue
		}
		for _, g := range ok {
			if sums[g]%10 == e.value(top)%10 {
				out = append(out, playNotation(g, b))
			}
		}
	}
	return out
}

func playNotation(group string, ball int) string {
	return strings.Join(strings.Split(group, ""), "+") + "@" + strconv.Itoa(ball)
}

// parsePlay reads e+h@2 into the pins and the pile.
func parsePlay(s string) (string, int, bool) {
	pins, ball, ok := strings.Cut(s, "@")
	if !ok {
		return "", 0, false
	}
	n, err := strconv.Atoi(ball)
	if err != nil || n < 1 || n > len(ballSizes) {
		return "", 0, false
	}
	group := strings.ReplaceAll(pins, "+", "")
	if group == "" || len(group) > 3 {
		return "", 0, false
	}
	for i := 0; i < len(group); i++ {
		if strings.IndexByte(pinLetters, group[i]) < 0 {
			return "", 0, false
		}
	}
	return group, n, true
}

func (e *eng) bowlingLegal() []string {
	out := e.plays()
	if len(out) > 0 {
		out = append(out, moveEnd)
	}
	return out
}

func (e *eng) bowlingPlay(move string) {
	if move == moveEnd {
		e.endBall()
		e.settleBall()
		return
	}
	group, ball, _ := parsePlay(move)
	gone := e.zone(zoneGone)
	for i := 0; i < len(group); i++ {
		z := e.zone(pinZone(group[i]))
		gone.Cards = append(gone.Cards, z.Cards...)
		z.Cards = nil
		e.d.Knocked++
	}
	e.takeBall(ball)
	e.d.Fresh = false
	e.settleBall()
}

// takeBall puts a pile's top card out of play and turns up the one under it.
func (e *eng) takeBall(i int) {
	z := e.zone(ballZone(i))
	for _, c := range z.Draw(1) {
		e.zone(zoneGone).Push(c)
	}
	if l := z.Len(); l > 0 {
		z.Cards[l-1].Up = true
	}
}

// settleBall ends the ball when it can do no more: every pin is down, or no play is left.
func (e *eng) settleBall() {
	for e.d.Turn != 0 && (e.standing() == "" || len(e.plays()) == 0) {
		e.endBall()
	}
}

// endBall writes down what the ball knocked over and says what comes next.
func (e *eng) endBall() {
	frame := e.d.Frame
	rolls := &e.d.Rolls[frame-1]
	*rolls = append(*rolls, e.d.Knocked)
	e.d.Knocked = 0
	clear := e.standing() == ""
	r := *rolls
	if frame < 10 {
		switch {
		case clear && e.d.Ball == 1:
			e.say("Frame %d: a strike!", frame)
			e.nextFrame()
		case clear:
			e.say("Frame %d: a spare!", frame)
			e.nextFrame()
		case e.d.Ball == 1:
			e.say("Frame %d: first ball, %d. Each ball pile loses its top card.", frame, r[0])
			e.secondBall()
		default:
			e.say("Frame %d: %d pins.", frame, r[0]+r[1])
			e.nextFrame()
		}
		return
	}
	switch e.d.Ball {
	case 1:
		if clear {
			e.say("Frame 10: a strike! Two more balls, on a new rack.")
			e.d.Ball = 2
			e.rack()
			return
		}
		e.say("Frame 10: first ball, %d. Each ball pile loses its top card.", r[0])
		e.secondBall()
	case 2:
		switch {
		case r[0] == 10 && clear:
			e.say("Frame 10: another strike! One more ball, on a new rack.")
			e.d.Ball = 3
			e.rack()
		case r[0] == 10:
			e.d.Ball = 3
			e.discardBallTops()
		case clear:
			e.say("Frame 10: a spare! One more ball, on a new rack.")
			e.d.Ball = 3
			e.rack()
		default:
			e.gameOver()
		}
	default:
		e.gameOver()
	}
}

func (e *eng) secondBall() {
	e.d.Ball = 2
	e.discardBallTops()
}

func (e *eng) discardBallTops() {
	for i := 1; i <= len(ballSizes); i++ {
		e.takeBall(i)
	}
}

func (e *eng) nextFrame() {
	e.d.Frame++
	e.d.Ball = 1
	e.d.Rolls = append(e.d.Rolls, []int{})
	e.rack()
}

func (e *eng) gameOver() {
	e.d.Turn, e.d.Reason = 0, reasonScore
	e.say("Ten frames: %d points.", bowlingTotal(e.d.Rolls))
}

// frameScores gives the running total after each frame, as far as it is known: a
// strike waits for two more balls and a spare for one.
func frameScores(frames [][]int) []*int {
	out := make([]*int, len(frames))
	total := 0
	for i, r := range frames {
		later := []int{}
		for _, f := range frames[i+1:] {
			later = append(later, f...)
		}
		sum, known := frameValue(i, r, later)
		if !known {
			return out
		}
		total += sum
		t := total
		out[i] = &t
	}
	return out
}

func frameValue(i int, r, later []int) (int, bool) {
	if i == 9 {
		if len(r) < 2 {
			return 0, false
		}
		need := 2
		if r[0] == 10 || r[0]+r[1] == 10 {
			need = 3
		}
		if len(r) < need {
			return 0, false
		}
		return sumOf(r), true
	}
	switch {
	case len(r) >= 1 && r[0] == 10:
		if len(later) < 2 {
			return 0, false
		}
		return 10 + later[0] + later[1], true
	case len(r) == 2 && r[0]+r[1] == 10:
		if len(later) < 1 {
			return 0, false
		}
		return 10 + later[0], true
	case len(r) == 2:
		return r[0] + r[1], true
	}
	return 0, false
}

func sumOf(r []int) int {
	n := 0
	for _, v := range r {
		n += v
	}
	return n
}

// bowlingTotal is the score so far: the last frame whose total is known.
func bowlingTotal(frames [][]int) int {
	total := 0
	for _, t := range frameScores(frames) {
		if t != nil {
			total = *t
		}
	}
	return total
}

// frameMarks writes a frame's balls as a score sheet does: X, / and - for nothing.
func frameMarks(r []int) []string {
	out := make([]string, 0, len(r))
	rack := 0
	for _, v := range r {
		switch {
		case v == 10 && rack == 0:
			out = append(out, "X")
		case v > 0 && rack+v == 10:
			out = append(out, "/")
		case v == 0:
			out = append(out, "-")
		default:
			out = append(out, fmt.Sprint(v))
		}
		rack += v
		if rack == 10 {
			rack = 0
		}
	}
	return out
}

// bowlingWorth orders plays for a hint: more pins are better, and ending the ball is last.
func (e *eng) bowlingWorth(move string) int {
	if move == moveEnd {
		return -1
	}
	group, _, _ := parsePlay(move)
	return len(group)
}

// sortedZones lists the table's zones in a fixed order.
func (d *Data) sortedZones() []string {
	keys := make([]string, 0, len(d.Table.Zones))
	for k := range d.Table.Zones {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
