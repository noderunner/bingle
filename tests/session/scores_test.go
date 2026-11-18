// A game of patience is scored by the moves it was won in (ADR 0135): the
// results worker keeps a winner's score, the best is the fewest, and a badge
// can ask for a win within so many moves.

package session_test

import (
	"context"
	"testing"

	"github.com/noderunner/bingle/internal/games/solitaire"
)

// patienceMoves plays n moves of turning the stock over and over.
func (w *ratedWorld) patienceMoves(room string, p player, n int) {
	w.t.Helper()
	for range n {
		if _, err := w.m.PlayNotation(p.ctx, room, p.handle, "draw"); err != nil {
			if _, err := w.m.PlayNotation(p.ctx, room, p.handle, "recycle"); err != nil {
				w.t.Fatalf("neither draw nor recycle: %v", err)
			}
		}
	}
}

func (w *ratedWorld) patience(id string, p player) string {
	w.t.Helper()
	res, err := w.m.CreateGame(p.ctx, id, p.handle)
	if err != nil {
		w.t.Fatal(err)
	}
	return res.RoomCode
}

func TestAWonGameOfPatienceKeepsItsScore(t *testing.T) {
	w := newRatedWorld(t)
	if err := w.cat.RegisterModule(solitaire.Module()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ana := w.account("ana")
	best := func(id string) (int, bool) {
		t.Helper()
		sc, ok, err := w.st.BestScore(ctx, ana.id, id)
		if err != nil {
			t.Fatal(err)
		}
		return sc.Score, ok
	}
	if _, ok := best("solitaire"); ok {
		t.Fatal("a best score before any game")
	}

	// Won in 30 moves, then in 5: the best is the fewer, and it earns the badge.
	for _, n := range []int{30, 5} {
		room := w.patience("solitaire", ana)
		w.patienceMoves(room, ana, n)
		w.finish(room, 1)
	}
	if n, ok := best("solitaire"); !ok || n != 5 {
		t.Errorf("the best Klondike score is %d (%v), want 5", n, ok)
	}
	if got := w.badgeIDs(ana); !has(got, "solitaire.quick-klondike") || has(got, "solitaire.quick-draw-three") {
		t.Errorf("ana's badges %v, want the quick Klondike only", got)
	}

	// A game lost keeps no score; one won in too many moves keeps it, and earns no badge.
	room := w.patience("solitaire-draw-3", ana)
	w.patienceMoves(room, ana, 3)
	w.finish(room, 0)
	if _, ok := best("solitaire-draw-3"); ok {
		t.Error("a lost game kept a score")
	}
	room = w.patience("solitaire-draw-3", ana)
	w.patienceMoves(room, ana, 101)
	w.finish(room, 1)
	if n, ok := best("solitaire-draw-3"); !ok || n != 101 {
		t.Errorf("the best draw-three score is %d (%v), want 101", n, ok)
	}
	if has(w.badgeIDs(ana), "solitaire.quick-draw-three") {
		t.Error("101 moves earned the badge for 100 or fewer")
	}
}
