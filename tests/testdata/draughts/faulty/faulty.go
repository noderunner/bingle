// Package faulty is the Draughts fixture's rules made to misbehave when a
// call asks them to, for the tests of what the host does about a module that
// panics, never returns, wants more memory than it may have or answers with
// nonsense (ADR 0147). It is only ever run as a module: the same faults
// linked in would take the test down with them.
package faulty

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

// The faults a call can ask for.
const (
	// Panic panics, Loop never returns and Grow takes memory until there is
	// no more.
	Panic = 1
	Loop  = 2
	Grow  = 3
	// BadSeat makes Apply name a next seat the game does not have.
	BadSeat = 4
	// Tire breaks the rule that a guest keeps nothing between calls: the
	// instance that answers it is tired from then on. IfTired panics on a
	// tired instance and does nothing on a fresh one, so a call that asks for
	// it fails once and succeeds when it is asked again of a new instance.
	Tire    = 5
	IfTired = 6
	// Churn is no fault: it makes two megabytes of garbage and answers as
	// usual, so a few hundred calls put the instance's collector through as
	// many cycles as a long life of ordinary ones.
	Churn = 7
)

// How each call is asked: Start by its seed, a call that takes a position by
// the position's Counter, one that takes a selection by the selection's
// Count, and Parse by ParseText followed by the fault's number.
const (
	Counter   = "fault"
	ParseText = "fault "
)

// PanicText is what a panic says.
const PanicText = "the fixture was asked to panic"

type rules struct{ guest.Rules }

// New makes the draughts rules of g, with the faults.
func New(g *guest.Game) (guest.Rules, error) {
	r, err := draughts.New(g)
	if err != nil {
		return nil, err
	}
	return &rules{r}, nil
}

var (
	tired bool
	spin  uint64
	hog   [][]byte
)

func fault(kind int) {
	switch kind {
	case Panic:
		panic(PanicText)
	case Loop:
		for {
			spin++
		}
	case Grow:
		for {
			block := make([]byte, 4<<20)
			for i := 0; i < len(block); i += 4096 {
				block[i] = 1
			}
			hog = append(hog, block)
		}
	case Churn:
		for range 32 {
			hog = append(hog, make([]byte, 64<<10))
		}
		hog = nil
	case Tire:
		tired = true
	case IfTired:
		if tired {
			panic(PanicText)
		}
	}
}

// clean is pos without the counter that asked for a fault.
func clean(pos *guest.Position) *guest.Position {
	if pos.Counters[Counter] == 0 {
		return pos
	}
	c := pos.Clone()
	delete(c.Counters, Counter)
	return c
}

func (r *rules) Start(seed uint64) (*guest.Position, error) {
	if seed <= Churn {
		fault(int(seed))
	}
	return r.Rules.Start(seed)
}

func (r *rules) ImpulseSeat(pos *guest.Position) int {
	fault(pos.Counters[Counter])
	return r.Rules.ImpulseSeat(clean(pos))
}

func (r *rules) Legal(pos *guest.Position, seat int) []guest.Selection {
	fault(pos.Counters[Counter])
	return r.Rules.Legal(clean(pos), seat)
}

func (r *rules) Apply(pos *guest.Position, seat int, sel guest.Selection) (*guest.Result, error) {
	fault(pos.Counters[Counter])
	kind := sel.Count
	fault(kind)
	sel.Count = 0
	res, err := r.Rules.Apply(clean(pos), seat, sel)
	if err == nil && kind == BadSeat {
		res.NextSeat = 9
	}
	return res, err
}

func (r *rules) Format(sel guest.Selection) string {
	fault(sel.Count)
	return r.Rules.Format(sel)
}

func (r *rules) Parse(text string) (guest.Selection, error) {
	if rest, ok := strings.CutPrefix(text, ParseText); ok {
		kind, err := strconv.Atoi(rest)
		if err != nil {
			return guest.Selection{}, fmt.Errorf("faulty: no fault %q", rest)
		}
		fault(kind)
		return guest.Selection{Count: kind}, nil
	}
	return r.Rules.Parse(text)
}

func (r *rules) Gestures(sel guest.Selection) []guest.Gesture {
	fault(sel.Count)
	return r.Rules.Gestures(sel)
}

func (r *rules) Validate(pos *guest.Position) error {
	fault(pos.Counters[Counter])
	return r.Rules.Validate(clean(pos))
}

func (r *rules) Outcome(pos *guest.Position) guest.Outcome {
	fault(pos.Counters[Counter])
	return r.Rules.Outcome(clean(pos))
}
