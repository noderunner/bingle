// A game of patience is dealt from a seed, and the same seed deals the same
// cards (ADR 0136): the seed is shown once the game is over, a player may
// give one to play a deal again, and such a game keeps no best score and earns
// no quick-win badge.

package session_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/noderunner/bingle/internal/games/solitaire"
	"github.com/noderunner/bingle/internal/session"
)

func (w *ratedWorld) seeded(id string, p player, seed string) (string, error) {
	w.t.Helper()
	res, err := w.m.CreateGameWithOptions(p.ctx, session.CreateOptions{GameDefID: id, CreatorName: p.handle, Seed: seed})
	return res.RoomCode, err
}

func TestTheSameSeedDealsTheSameCards(t *testing.T) {
	w := newRatedWorld(t)
	if err := w.cat.RegisterModule(solitaire.Module()); err != nil {
		t.Fatal(err)
	}
	ana := w.account("ana")
	bo := w.account("bo")

	// A new deal's seed is kept, and is not told while the game is played:
	// it says where every face-down card lies.
	first := w.patience("solitaire", ana)
	snap, err := w.m.GetState(ana.ctx, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Seed != "" || snap.StartChosen {
		t.Fatalf("a game being played shows seed %q (chosen %v)", snap.Seed, snap.StartChosen)
	}
	w.patienceMoves(first, ana, 2)
	w.finish(first, 0)
	snap, err = w.m.GetState(ana.ctx, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	seed := snap.Seed
	if seed == "" {
		t.Fatal("a finished game of patience shows no seed")
	}

	// The seed deals the same cards again, to anybody, and is shown from the start.
	again, err := w.seeded("solitaire", bo, seed)
	if err != nil {
		t.Fatal(err)
	}
	if snap, err := w.m.GetState(bo.ctx, again, nil); err != nil || snap.Seed != seed || !snap.StartChosen {
		t.Errorf("the replayed deal shows seed %q (chosen %v, %v), want %s, chosen", snap.Seed, snap.StartChosen, err, seed)
	}
	// Compare the opening deals: the first game's start, and the second's now.
	if a, b := startTable(t, w, first), startTable(t, w, again); a != b {
		t.Error("the same seed dealt different cards")
	}
	other, err := w.seeded("solitaire", bo, "12345")
	if err != nil {
		t.Fatal(err)
	}
	if startTable(t, w, other) == startTable(t, w, again) {
		t.Error("two seeds dealt the same cards")
	}

	// A seed is a number, and only a game of patience takes one.
	if _, err := w.seeded("solitaire", bo, "red"); !errors.Is(err, session.ErrInvalidSeed) {
		t.Errorf("seed \"red\": %v, want ErrInvalidSeed", err)
	}
	if _, err := w.seeded("breakthrough", bo, "7"); !errors.Is(err, session.ErrInvalidSeed) {
		t.Errorf("a seed for Breakthrough: %v, want ErrInvalidSeed", err)
	}
}

// startTable is the cards a game was dealt, from its recorded start.
func startTable(t *testing.T, w *ratedWorld, room string) string {
	t.Helper()
	g, err := w.st.GetGame(context.Background(), room)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cards json.RawMessage `json:"cards"`
	}
	if err := json.Unmarshal([]byte(g.InitialState), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cards) == 0 {
		t.Fatalf("room %s recorded no table: %.200s", room, g.InitialState)
	}
	return string(doc.Cards)
}

func TestADealPlayedAgainKeepsNoScore(t *testing.T) {
	w := newRatedWorld(t)
	if err := w.cat.RegisterModule(solitaire.Module()); err != nil {
		t.Fatal(err)
	}
	ana := w.account("ana")
	room, err := w.seeded("solitaire", ana, "42")
	if err != nil {
		t.Fatal(err)
	}
	w.patienceMoves(room, ana, 5)
	w.finish(room, 1)
	if _, ok, err := w.st.BestScore(context.Background(), ana.id, "solitaire"); err != nil || ok {
		t.Errorf("a chosen deal kept a best score (%v, %v)", ok, err)
	}
	if has(w.badgeIDs(ana), "solitaire.quick-klondike") {
		t.Error("a chosen deal earned the quick-win badge")
	}
}
