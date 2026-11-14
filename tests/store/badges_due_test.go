// The watermark of the badges checked during play (ADR 0130): a move that
// reports facts makes its game due, a check moves the watermark up to it, and
// an undo moves both back.

package store_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/store"
)

func TestBadgesDueFollowTheMovesThatReportFacts(t *testing.T) {
	s := newRecordMoveTestStore(t)
	ctx := t.Context()
	game, _, err := s.CreateGame(ctx, "ROOM01", "test-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatal(err)
	}
	due := func() []store.BadgesDue {
		t.Helper()
		d, err := s.GamesWithBadgesDue(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	move := func(facts string) {
		t.Helper()
		if _, err := s.RecordMove(ctx, game.ID, store.MoveInput{Seat: 1, Color: "black", To: "a1", ResultingBoardState: `{}`, NextImpulseSeat: 1, Facts: facts}); err != nil {
			t.Fatal(err)
		}
	}

	move("")
	if d := due(); len(d) != 0 {
		t.Fatalf("due after a move without facts = %+v, want none", d)
	}
	move(`[{"name":"placed","seat":1}]`)
	move("")
	d := due()
	if len(d) != 1 || d[0].Game.ID != game.ID || d[0].Seq != 2 {
		t.Fatalf("due = %+v, want the game up to move 2", d)
	}
	if _, err := s.RecordDuring(ctx, game.ID, 2, nil); err != nil {
		t.Fatal(err)
	}
	if d := due(); len(d) != 0 {
		t.Fatalf("due after the check = %+v, want none", d)
	}

	// An undo past the watermark takes it back, so the moves played again
	// are looked at.
	if err := s.UndoToSeq(ctx, game.ID, 1, `{}`, 1); err != nil {
		t.Fatal(err)
	}
	move(`[{"name":"placed","seat":1}]`)
	if d := due(); len(d) != 1 || d[0].Seq != 2 {
		t.Fatalf("due after an undo and a move = %+v, want the game up to move 2", d)
	}
}
