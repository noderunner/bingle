// Package allgames_test runs one battery of engine invariants over every
// game the server ships, variants included, so a new game (or a change to the
// engine) is checked against all of them without writing per-game tests.
package allgames_test

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

func allGames(t testing.TB) []*catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatalf("RegisterModule(%s): %v", m.ID, err)
		}
	}
	games := cat.List()
	if len(games) < 25 {
		t.Fatalf("only %d games registered (expected the full catalog)", len(games))
	}
	return games
}

// For every game, along random playouts: every enumerated move applies and
// round-trips through notation to the identical resulting position; the
// position document survives marshal/unmarshal; planes keep their shape and
// every move has an action ID when the space is fixed.
func TestEveryGameStaysConsistentInRandomPlay(t *testing.T) {
	plies := 70
	if testing.Short() {
		plies = 30
	}
	for _, g := range allGames(t) {
		g := g
		t.Run(g.Definition.ID, func(t *testing.T) {
			// Big boards make every ply expensive; trim their budget.
			budget := plies
			if g.Board.Len() > 150 {
				budget = 25
			}
			rng := rand.New(rand.NewSource(int64(len(g.Definition.ID)) * 31))
			space := state.ActionSpaceOf(g)
			channels, h, w, planesErr := state.PlaneChannels(g, state.PlaneOptions{})

			for game := 0; game < 2; game++ {
				st, seat := state.InitialState(g), state.FirstSeat(g)
				for ply := 0; ply < budget; ply++ {
					legal := state.LegalMoves(g, st, seat)
					if len(legal) == 0 {
						break
					}
					// Check a bounded sample of the legal moves each ply.
					sample := legal
					if len(sample) > 12 {
						sample = make([]state.Move, 12)
						for i := range sample {
							sample[i] = legal[rng.Intn(len(legal))]
						}
					}
					for _, mv := range sample {
						res, err := state.Apply(g, st, seat, mv)
						if err != nil {
							t.Fatalf("ply %d: enumerated move %+v rejected: %v", ply, mv, err)
						}
						text := state.FormatMove(g, mv)
						back, err := state.ParseMove(g, text)
						if err != nil {
							t.Fatalf("ply %d: notation %q does not parse: %v", ply, text, err)
						}
						res2, err := state.Apply(g, st, seat, back)
						if err != nil {
							t.Fatalf("ply %d: parsed %q rejected: %v", ply, text, err)
						}
						a, _ := res.NewState.MarshalJSON()
						b, _ := res2.NewState.MarshalJSON()
						if string(a) != string(b) {
							t.Fatalf("ply %d: %q parsed to a different move (positions differ)", ply, text)
						}
						if space.Fixed {
							if _, ok := space.ID(text); !ok {
								t.Fatalf("ply %d: %q has no fixed action id", ply, text)
							}
						}
					}

					// Position document round trip.
					doc, err := st.MarshalJSON()
					if err != nil {
						t.Fatal(err)
					}
					var back state.BoardState
					if err := json.Unmarshal(doc, &back); err != nil {
						t.Fatalf("ply %d: state does not unmarshal: %v", ply, err)
					}
					doc2, _ := back.MarshalJSON()
					if string(doc) != string(doc2) {
						t.Fatalf("ply %d: state document changed across a round trip", ply)
					}
					if err := state.Validate(g, st); err != nil {
						t.Fatalf("ply %d: a reachable position fails validation: %v", ply, err)
					}

					if planesErr == nil {
						p, err := state.EncodePlanes(g, st, seat, seat, state.PlaneOptions{})
						if err != nil || p.Shape != [3]int{len(channels), h, w} || len(p.Data) != len(channels)*h*w {
							t.Fatalf("ply %d: planes shape %v vs %d channels %dx%d (err %v)", ply, p.Shape, len(channels), h, w, err)
						}
					}

					res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
					if err != nil {
						t.Fatal(err)
					}
					if res.GameEnded {
						break
					}
					st, seat = res.NewState, res.NextTurnSeat
				}
			}
		})
	}
}

// Every game must be able to describe its starting position and have a
// legal opening move for the first seat.
func TestEveryGameHasAPlayableStart(t *testing.T) {
	for _, g := range allGames(t) {
		st, seat := state.InitialState(g), state.FirstSeat(g)
		if len(state.LegalMoves(g, st, seat)) == 0 {
			t.Errorf("%s: no legal first move for seat %d", g.Definition.ID, seat)
		}
		if out, err := state.Evaluate(g, st, seat); err == nil && out.Ended && g.Definition.WinCondition.Type == "rules" {
			t.Errorf("%s: the starting position is already decided (%s)", g.Definition.ID, out.Reason)
		}
		if state.RenderText(g, st) == "" {
			t.Errorf("%s: empty text rendering", g.Definition.ID)
		}
	}
}

// FuzzApplyAnyMoveToAnyGame throws arbitrary structured moves — including
// the compound fields (aim) — at a mid-game position of every shipped game;
// the engine may reject them but must never panic, and anything it accepts
// must be a move it also enumerates.
func FuzzApplyAnyMoveToAnyGame(f *testing.F) {
	f.Add(uint8(0), "move", "", "d1", "d5", "f5", 0)
	f.Add(uint8(3), "", "stone", "", "c3", "", 0)
	f.Add(uint8(7), "swap", "", "", "", "", 0)
	f.Add(uint8(9), "move", "", "a1", "b2", "", 3)
	f.Add(uint8(11), "x", "king", "zz", "", "a1", -4)

	games := allGames(f)
	// One mid-game position per game, from a seeded playout.
	type pos struct {
		st   *state.BoardState
		seat int
	}
	positions := make([]pos, len(games))
	for i, g := range games {
		rng := rand.New(rand.NewSource(int64(i) + 1))
		st, seat := state.InitialState(g), state.FirstSeat(g)
		for ply := 0; ply < 12; ply++ {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				break
			}
			res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
			if err != nil || res.GameEnded {
				break
			}
			st, seat = res.NewState, res.NextTurnSeat
		}
		positions[i] = pos{st, seat}
	}

	f.Fuzz(func(t *testing.T, idx uint8, action, piece, from, to, aim string, count int) {
		i := int(idx) % len(games)
		g, p := games[i], positions[i]
		mv := state.Move{
			Action: action, Piece: piece, From: board.PositionID(from), To: board.PositionID(to),
			Aim: board.PositionID(aim), Count: count,
		}
		res, err := state.Apply(g, p.st, p.seat, mv)
		if err != nil {
			return
		}
		// Accepted: it must appear in the enumeration under its notation.
		want := state.FormatMove(g, res.Move)
		for _, lm := range state.LegalMoves(g, p.st, p.seat) {
			if state.FormatMove(g, lm) == want {
				return
			}
		}
		t.Fatalf("%s: Apply accepted %q (%+v) but LegalMoves does not list it", g.Definition.ID, want, mv)
	})
}
