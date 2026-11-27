// Tests of the Draughts fixture's own rules (tests/testdata/draughts), linked
// in through wasmrules.Native: the tests that play it as a WebAssembly module
// (tests/wasmrules, tests/difftest) lean on these.
package draughts_test

import (
	"encoding/json"
	"errors"
	"math/rand"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/tests/testdata"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

func games(t *testing.T) map[string]*catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(testdata.Draughts()); err != nil {
		t.Fatal(err)
	}
	out := map[string]*catalog.CompiledGame{}
	for _, g := range cat.List() {
		out[g.Definition.ID] = g
	}
	return out
}

// position sets pieces on an empty board: "b" and "w" are men, "B" and "W"
// kings, each followed by its square.
func position(t *testing.T, game *catalog.CompiledGame, turn int, extra string, pieces ...string) *state.BoardState {
	t.Helper()
	st := state.NewBoardState()
	for _, p := range pieces {
		occ := state.Occupant{Piece: "man", Color: "black"}
		if strings.ToLower(p[:1]) == "w" {
			occ.Color = "white"
		}
		if p[:1] == strings.ToUpper(p[:1]) {
			occ.Piece = "king"
		}
		st.Set(board.PositionID(p[1:]), occ)
	}
	ext := map[string]any{"turn": turn}
	if extra != "" {
		if err := json.Unmarshal([]byte(extra), &ext); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(ext)
	st.SetExt(json.RawMessage(raw))
	if err := state.Validate(game, st); err != nil {
		t.Fatalf("the test's position is not one of the game's: %v", err)
	}
	return st
}

func notations(game *catalog.CompiledGame, st *state.BoardState, seat int) string {
	var out []string
	for _, mv := range state.LegalSelections(game, st, seat) {
		out = append(out, state.FormatMove(game, mv))
	}
	return strings.Join(out, " ")
}

func play(t *testing.T, game *catalog.CompiledGame, st *state.BoardState, seat int, text string) *state.MoveResult {
	t.Helper()
	mv, err := state.ParseMove(game, text)
	if err != nil {
		t.Fatal(err)
	}
	res, err := state.Apply(game, st, seat, mv)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return res
}

func TestTheStart(t *testing.T) {
	all := games(t)
	for id, men := range map[string]int{draughts.ID: 12, draughts.SmallID: 6, draughts.BallotID: 11} {
		game := all[id]
		st, err := state.InitialStateSeeded(game, 5)
		if err != nil {
			t.Fatal(err)
		}
		count := map[string]int{}
		for _, p := range st.Occupied() {
			occ, _ := st.Get(p)
			if occ.Piece != "man" {
				t.Errorf("%s: a %s on %s at the start", id, occ.Piece, p)
			}
			count[occ.Color]++
		}
		if count["black"] != men || count["white"] != men {
			t.Errorf("%s starts with %v, want %d men a side", id, count, men)
		}
		if seat := state.StartSeat(game, st); seat != 1 {
			t.Errorf("%s: seat %d moves first, want Black", id, seat)
		}
		if err := state.Validate(game, st); err != nil {
			t.Errorf("%s: its own start does not validate: %v", id, err)
		}
		if !state.NeedsStartRecord(game) {
			t.Errorf("%s: the start is not recorded with its game", id)
		}
	}
	// Black's men stand on the dark squares of the bottom rows and move up.
	st, _ := state.InitialStateSeeded(all[draughts.ID], 5)
	if got, want := notations(all[draughts.ID], st, 1), "a3-b4 c3-b4 c3-d4 e3-d4 e3-f4 g3-f4 g3-h4"; got != want {
		t.Errorf("Black's first moves are %q, want %q", got, want)
	}
	if got := notations(all[draughts.ID], st, 2); got != "" {
		t.Errorf("White may move first: %q", got)
	}
}

// The ballot draws a man of each side off by lot: the same seed the same men,
// another seed (soon) others, and the plain game no lot at all.
func TestTheBallotIsDrawnFromTheSeed(t *testing.T) {
	all := games(t)
	doc := func(id string, seed uint64) string {
		st, err := state.InitialStateSeeded(all[id], seed)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := st.MarshalJSON()
		return string(raw)
	}
	if once, again := doc(draughts.BallotID, 3), doc(draughts.BallotID, 3); once != again {
		t.Error("one seed drew two starts")
	}
	starts := map[string]bool{}
	for seed := uint64(0); seed < 50; seed++ {
		starts[doc(draughts.BallotID, seed)] = true
	}
	if len(starts) < 25 {
		t.Errorf("50 seeds drew %d starts", len(starts))
	}
	if doc(draughts.ID, 1) != doc(draughts.ID, 2) {
		t.Error("the plain game's start depends on its seed")
	}
}

// A capture must be made, and each jump of a chain is a selection of its own:
// the seat keeps the impulse until its piece can jump no further.
func TestACaptureMustBeMadeAndAChainIsOneImpulse(t *testing.T) {
	game := games(t)[draughts.ID]
	// Black's man on c3 can jump d4 and then f6; its man on a1 can only move.
	st := position(t, game, 1, "", "bc3", "ba1", "wd4", "wf6", "wh8")
	if got := notations(game, st, 1); got != "c3xe5" {
		t.Fatalf("with a capture to make, Black may play %q", got)
	}
	if _, err := state.Apply(game, st, 1, state.Selection{Action: "move", From: "a1", To: "b2"}); err == nil {
		t.Fatal("a move was allowed with a capture to make")
	}

	first := play(t, game, st, 1, "c3xe5")
	if first.NextImpulseSeat != 1 || first.GameEnded || state.ImpulseEnds(game, st, 1, first.Move, first) {
		t.Fatalf("the first jump ended the impulse: %+v", first)
	}
	if len(first.Captures) != 1 || first.Captures[0] != "d4" || *first.Captured != "d4" {
		t.Fatalf("the first jump captured %v, want d4", first.Captures)
	}
	if _, held := first.NewState.Get("d4"); held {
		t.Fatal("the man jumped over is still on the board")
	}
	if got := notations(game, first.NewState, 1); got != "e5xg7" {
		t.Fatalf("in the chain Black may play %q, want only the next jump", got)
	}
	second := play(t, game, first.NewState, 1, "e5xg7")
	if second.NextImpulseSeat != 2 || !state.ImpulseEnds(game, first.NewState, 1, second.Move, second) {
		t.Fatalf("the last jump did not end the impulse: %+v", second)
	}

	// The engine takes the two as one move, and neither alone.
	jumps, err := state.ParseImpulse(game, "c3xe5; e5xg7")
	if err != nil {
		t.Fatal(err)
	}
	whole, err := state.ApplyImpulse(game, st, 1, jumps)
	if err != nil || len(whole.Selections) != 2 || whole.Last().NextImpulseSeat != 2 {
		t.Fatalf("the chain as one impulse: %+v, %v", whole, err)
	}
	if _, err := state.ApplyImpulse(game, st, 1, jumps[:1]); !errors.Is(err, state.ErrImpulseOpen) {
		t.Fatalf("half a chain was taken as a move: %v", err)
	}
	next, err := state.NextSelections(game, st, 1, jumps[:1])
	if err != nil || next.Complete || len(next.Options) != 1 || next.Options[0].To != "g7" {
		t.Fatalf("after the first jump the selections are %+v, %v", next, err)
	}
}

// A man that reaches the far row is crowned and its move ends, even where the
// king could jump on; a king moves and jumps backward as well.
func TestCrowning(t *testing.T) {
	game := games(t)[draughts.ID]
	// A man jumps forward only: from e7 it cannot take the man behind it on f6.
	st := position(t, game, 1, "", "bc5", "wd6", "wf6", "wh8")
	res := play(t, game, st, 1, "c5xe7")
	if occ, _ := res.NewState.Get("e7"); occ.Piece != "man" || res.NextImpulseSeat != 2 {
		t.Fatalf("a man that jumped to e7 is a %s, and seat %d moves", occ.Piece, res.NextImpulseSeat)
	}

	st = position(t, game, 1, "", "bd6", "we7", "wg7", "wh8")
	res = play(t, game, st, 1, "d6xf8")
	if occ, _ := res.NewState.Get("f8"); occ.Piece != "king" {
		t.Fatalf("the man on the far row is a %s", occ.Piece)
	}
	if res.NextImpulseSeat != 2 {
		t.Fatal("the crowned man jumped on in the same move")
	}

	st = position(t, game, 1, "", "Bf8", "wg7", "we5", "wh8")
	if got := notations(game, st, 1); got != "f8xh6" {
		t.Fatalf("a king on f8 may play %q, want its jump backward", got)
	}
	st = position(t, game, 1, "", "Bd4", "wb8", "wh8")
	if got := notations(game, st, 1); got != "d4-c5 d4-e5 d4-c3 d4-e3" {
		t.Fatalf("a king on d4 may play %q", got)
	}
}

// A side that cannot move has lost, and a long quiet game is a draw.
func TestTheEnd(t *testing.T) {
	game := games(t)[draughts.ID]
	// Black captures White's last man.
	st := position(t, game, 1, "", "bc3", "wd4")
	res := play(t, game, st, 1, "c3xe5")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != draughts.ReasonNoMoves {
		t.Fatalf("capturing the last man: %+v", res)
	}
	if out, _ := state.Evaluate(game, res.NewState, 1); !out.Ended || *out.Winner != 1 {
		t.Fatalf("the outcome of the last position is %+v", out)
	}
	if seat := state.StartSeat(game, res.NewState); seat != 1 {
		// Over: nobody is on impulse, and the engine names the first seat.
		t.Fatalf("a finished game waits for seat %d", seat)
	}
	if got := notations(game, res.NewState, 2); got != "" {
		t.Fatalf("the loser may still play %q", got)
	}
	if _, err := state.Apply(game, res.NewState, 1, state.Selection{Action: "move", From: "e5", To: "d6"}); err == nil {
		t.Fatal("a move was applied to a finished game")
	}

	// White's man is blocked: it has a man, and no move.
	st = position(t, game, 1, "", "ba3", "bc3", "bd2", "ba5", "bc5", "wb4", "Bh8")
	res = play(t, game, st, 1, "h8-g7")
	if !res.GameEnded || *res.WinnerSeat != 1 {
		t.Fatalf("a blocked side has not lost: %+v", res)
	}

	// Kings shuffling: the eightieth quiet selection draws the game.
	st = position(t, game, 1, `{"quiet":79}`, "Ba1", "Wh8")
	res = play(t, game, st, 1, "a1-b2")
	if !res.GameEnded || res.WinnerSeat != nil || res.EndReason != draughts.ReasonQuiet {
		t.Fatalf("the eightieth quiet selection: %+v", res)
	}
	// A man's move starts the count again.
	st = position(t, game, 1, `{"quiet":79}`, "ba1", "Wh8")
	if res = play(t, game, st, 1, "a1-b2"); res.GameEnded {
		t.Fatal("a man's move did not start the count again")
	}
}

func TestPositionsThatAreNotTheGames(t *testing.T) {
	game := games(t)[draughts.ID]
	good := position(t, game, 1, "", "bc3", "wd6")
	for name, edit := range map[string]func(st *state.BoardState){
		"a light square":     func(st *state.BoardState) { st.Set("a2", state.Occupant{Piece: "man", Color: "black"}) },
		"a color nobody has": func(st *state.BoardState) { st.Set("a1", state.Occupant{Piece: "man", Color: "red"}) },
		"an unknown piece":   func(st *state.BoardState) { st.Set("a1", state.Occupant{Piece: "rook", Color: "black"}) },
		"an uncrowned man":   func(st *state.BoardState) { st.Set("b8", state.Occupant{Piece: "man", Color: "black"}) },
		"two on a square":    func(st *state.BoardState) { st.Push("c3", state.Occupant{Piece: "man", Color: "black"}) },
		"no data":            func(st *state.BoardState) { st.SetExt(nil) },
		"a seat too many":    func(st *state.BoardState) { st.SetExt(json.RawMessage(`{"turn":3}`)) },
		"a chain of nothing": func(st *state.BoardState) { st.SetExt(json.RawMessage(`{"turn":1,"chain":"e5"}`)) },
		"nothing to move":    func(st *state.BoardState) { st.Remove("c3") },
	} {
		st := good.Clone()
		edit(st)
		if err := state.Validate(game, st); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// Random games: every listed selection applies and reads back from its
// notation, a selection changed into one that is not listed is refused, the
// men on the board never grow in number, and every game ends.
func TestRandomGamesKeepTheContract(t *testing.T) {
	for id, game := range games(t) {
		rng := rand.New(rand.NewSource(11))
		for g := 0; g < 20; g++ {
			st, err := state.InitialStateSeeded(game, uint64(g))
			if err != nil {
				t.Fatal(err)
			}
			seat, pieces, ended := state.StartSeat(game, st), len(st.Occupied()), false
			for ply := 0; ply < 2000 && !ended; ply++ {
				legal := state.LegalSelections(game, st, seat)
				if len(legal) == 0 {
					t.Fatalf("%s game %d: seat %d cannot move and the game goes on", id, g, seat)
				}
				listed := map[string]bool{}
				for _, mv := range legal {
					listed[mv.Key()] = true
					back, err := state.ParseMove(game, state.FormatMove(game, mv))
					if err != nil || back.Key() != mv.Key() {
						t.Fatalf("%s: %+v reads back as %+v, %v", id, mv, back, err)
					}
				}
				// One square off: refused unless it is itself listed.
				odd := legal[rng.Intn(len(legal))]
				odd.To = game.Board.Positions()[rng.Intn(game.Board.Len())]
				if _, err := state.Apply(game, st, seat, odd); (err == nil) != listed[odd.Key()] {
					t.Fatalf("%s: %+v is listed: %v, applied: %v", id, odd, listed[odd.Key()], err == nil)
				}
				res, err := state.Apply(game, st, seat, legal[rng.Intn(len(legal))])
				if err != nil {
					t.Fatalf("%s: a listed selection was refused: %v", id, err)
				}
				if n := len(res.NewState.Occupied()); n > pieces || n < pieces-1 || (n < pieces) != (len(res.Captures) == 1) {
					t.Fatalf("%s: %d pieces became %d with captures %v", id, pieces, n, res.Captures)
				} else {
					pieces = n
				}
				st, seat, ended = res.NewState, res.NextImpulseSeat, res.GameEnded
			}
			if !ended {
				t.Fatalf("%s game %d did not end in 2000 selections", id, g)
			}
		}
	}
}
