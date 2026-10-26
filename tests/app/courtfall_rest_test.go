package app_test

import (
	"math/rand"
	"strings"
	"testing"
)

// cfState is the slice of a Courtfall position the REST tests look at.
type cfState struct {
	TurnSeat *int   `json:"turn_seat"`
	Status   string `json:"status"`
	MoveSeq  int    `json:"move_seq"`
	State    struct {
		Cells map[string][]struct{ Piece, Color string } `json:"cells"`
		Ext   struct {
			Phase  string `json:"phase"`
			Stage  string `json:"stage"`
			RNG    uint64 `json:"rng"`
			Events []string
			Con    *struct {
				AAtk   int  `json:"a_atk"`
				Rolled bool `json:"rolled"`
			} `json:"contest"`
			Last *struct {
				Kind string `json:"kind"`
			} `json:"last"`
			View struct {
				Prompt string `json:"prompt"`
				Seats  map[string]struct {
					Counters []struct {
						ID    string `json:"id"`
						Value int    `json:"value"`
					} `json:"counters"`
				} `json:"seats"`
				Tints map[string]string `json:"tints"`
			} `json:"view"`
		} `json:"ext"`
	} `json:"state"`
	StartState struct {
		Ext struct {
			RNG uint64 `json:"rng"`
		} `json:"ext"`
	} `json:"start_state"`
}

func cfGet(t *testing.T, base, room string) cfState {
	t.Helper()
	var st cfState
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/state", nil, &st); code != 200 {
		t.Fatalf("state: %d", code)
	}
	return st
}

type cfLegal struct {
	Seat  int `json:"seat"`
	Moves []struct {
		Notation string `json:"notation"`
		Action   string `json:"action"`
		Steps    []struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		} `json:"steps"`
	} `json:"moves"`
}

func cfLegalMoves(t *testing.T, base, room string) cfLegal {
	t.Helper()
	var l cfLegal
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/moves/legal?steps=1", nil, &l); code != 200 {
		t.Fatalf("legal: %d", code)
	}
	return l
}

var cfNames = map[int]string{1: "Alice", 2: "Bob"}

// cfPlay plays one random legal decision as whoever is to move, avoiding
// moves whose notation has one of the given prefixes.
func cfPlay(t *testing.T, base, room string, rng *rand.Rand, avoid ...string) string {
	t.Helper()
	l := cfLegalMoves(t, base, room)
	var pick []string
	for _, m := range l.Moves {
		skip := false
		for _, a := range avoid {
			if strings.HasPrefix(m.Notation, a) || strings.Contains(m.Notation, a) {
				skip = true
			}
		}
		if !skip {
			pick = append(pick, m.Notation)
		}
	}
	if len(pick) == 0 {
		for _, m := range l.Moves {
			pick = append(pick, m.Notation)
		}
	}
	n := pick[rng.Intn(len(pick))]
	if code, out := moveNotation(t, base, room, cfNames[l.Seat], n); code != 200 {
		t.Fatalf("%s by %s: %d %v", n, cfNames[l.Seat], code, out)
	}
	return n
}

// cfToStrike plays random non-contest moves until a strike is on offer,
// returning the attacker's seat and the strike notation (2 attack dice).
func cfToStrike(t *testing.T, base, room string, rng *rand.Rand) (int, string) {
	t.Helper()
	for i := 0; i < 600; i++ {
		l := cfLegalMoves(t, base, room)
		for _, m := range l.Moves {
			if m.Action == "strike" && strings.HasSuffix(m.Notation, "/2") {
				return l.Seat, m.Notation
			}
		}
		cfPlay(t, base, room, rng, "x", "*", "^", "clear")
	}
	t.Fatal("no strike became available")
	return 0, ""
}

func TestCourtfallSecretsStayOffTheWire(t *testing.T) {
	base := newRESTTestServer(t)
	room := newTakRoom(t, base, "courtfall", "Alice")
	joinRoom(t, base, room, "Bob")
	rng := rand.New(rand.NewSource(7))

	st := cfGet(t, base, room)
	if st.State.Ext.Phase != "terrain" || st.State.Ext.View.Prompt == "" {
		t.Fatalf("fresh game: %+v", st.State.Ext)
	}
	if st.State.Ext.RNG != 0 || st.StartState.Ext.RNG != 0 {
		t.Fatalf("the dice generator leaked: state %d start %d", st.State.Ext.RNG, st.StartState.Ext.RNG)
	}

	seat, strike := cfToStrike(t, base, room, rng)
	attacker, defender := cfNames[seat], cfNames[3-seat]
	if code, out := moveNotation(t, base, room, attacker, strike); code != 200 {
		t.Fatalf("strike %s: %d %v", strike, code, out)
	} else if mv, _ := out["move"].(map[string]any); mv == nil || !strings.HasSuffix(mv["notation"].(string), "/?") {
		t.Errorf("the mover's own response should not echo the committed dice: %v", out["move"])
	}

	// Everyone reads the same masked position.
	st = cfGet(t, base, room)
	if st.State.Ext.Stage != "respond" || st.State.Ext.Con == nil || st.State.Ext.Con.AAtk != -1 {
		t.Fatalf("pending strike should be masked: %+v", st.State.Ext.Con)
	}
	if st.State.Ext.RNG != 0 {
		t.Error("RNG leaked while live")
	}
	var moves []struct {
		Notation string `json:"notation"`
		Count    int    `json:"count"`
	}
	doJSON(t, "GET", base+"/api/games/"+room+"/moves", nil, &moves)
	last := moves[len(moves)-1]
	if !strings.HasSuffix(last.Notation, "/?") {
		t.Errorf("move list reveals the commit: %q", last.Notation)
	}
	// The observation of the committing player carries their own split; the
	// opponent's does not.
	var mine, theirs cfState
	doJSON(t, "GET", base+"/api/games/"+room+"/observe?player_name="+attacker, nil, &mine)
	doJSON(t, "GET", base+"/api/games/"+room+"/observe?player_name="+defender, nil, &theirs)
	if mine.State.Ext.Con == nil || mine.State.Ext.Con.AAtk != 2 {
		t.Errorf("the attacker's observation should show their own split: %+v", mine.State.Ext.Con)
	}
	if theirs.State.Ext.Con == nil || theirs.State.Ext.Con.AAtk != -1 {
		t.Errorf("the defender's observation must not: %+v", theirs.State.Ext.Con)
	}
	// The same goes for the state endpoint with ?player_name=.
	var own, other cfState
	doJSON(t, "GET", base+"/api/games/"+room+"/state?player_name="+attacker, nil, &own)
	doJSON(t, "GET", base+"/api/games/"+room+"/state?player_name="+defender, nil, &other)
	if own.State.Ext.Con == nil || own.State.Ext.Con.AAtk != 2 || own.State.Ext.RNG != 0 {
		t.Errorf("the attacker's state should show their split but not the generator: %+v", own.State.Ext.Con)
	}
	if other.State.Ext.Con == nil || other.State.Ext.Con.AAtk != -1 {
		t.Errorf("the defender's state must not: %+v", other.State.Ext.Con)
	}
	// Export of a live game keeps the seed out.
	var rec struct {
		InitialState string `json:"initial_state"`
		FinalState   string `json:"final_state"`
	}
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/export", nil, &rec); code != 200 {
		t.Fatalf("export: %d", code)
	}
	for name, doc := range map[string]string{"initial": rec.InitialState, "final": rec.FinalState} {
		if !strings.Contains(doc, `"rng":0`) {
			t.Errorf("export of a live game leaks the %s generator: %.200s", name, doc)
		}
	}

	// Before anyone answers, the attacker may still undo: no dice rolled.
	if code, out := moveNotation(t, base, room, attacker, "end"); code == 200 {
		t.Errorf("the attacker moved out of turn: %v", out)
	}
	var undone map[string]any
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/undo", map[string]any{"player_name": attacker}, &undone); code != 200 {
		t.Fatalf("undo before the answer: %d %v", code, undone)
	}
	if cfGet(t, base, room).State.Ext.Stage != "act" {
		t.Error("undo should return to the activation")
	}
	// Strike again and let the defender answer: the dice are rolled.
	if code, out := moveNotation(t, base, room, attacker, strike); code != 200 {
		t.Fatalf("restrike: %d %v", code, out)
	}
	if code, out := moveNotation(t, base, room, defender, "respond/0"); code != 200 {
		t.Fatalf("respond: %d %v", code, out)
	}
	st = cfGet(t, base, room)
	if st.State.Ext.Last == nil || st.State.Ext.Last.Kind != "strike" {
		t.Fatalf("resolved contest missing: %+v", st.State.Ext.Last)
	}
	// Dice cannot be unrolled.
	var refused map[string]any
	for _, who := range []string{attacker, defender} {
		if code := doJSON(t, "POST", base+"/api/games/"+room+"/undo", map[string]any{"player_name": who}, &refused); code != 409 {
			t.Errorf("undo by %s across a roll: %d %v, want 409", who, code, refused)
		}
	}
	if s, _ := refused["error"].(string); !strings.Contains(s, "dice") {
		t.Errorf("the refusal should explain: %v", refused)
	}
	// Import of recorded moves is refused: replays would roll new dice.
	if code := doJSON(t, "POST", base+"/api/games/import", map[string]any{"game_def_id": "courtfall", "moves": "tile:green@d3"}, &refused); code == 200 {
		t.Error("importing a game with chance should be refused")
	}
}

func TestCourtfallAgainstABotPlaysThroughSetupAndTurns(t *testing.T) {
	base := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "courtfall", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 150; i++ {
		st := cfGet(t, base, room)
		if st.Status == "finished" {
			break
		}
		if st.TurnSeat == nil || *st.TurnSeat != 1 {
			t.Fatalf("ply %d: the bot should have played until it is Alice's decision again, turn %v", i, st.TurnSeat)
		}
		n := cfPlay(t, base, room, rng)
		_ = n
	}
	st := cfGet(t, base, room)
	if st.State.Ext.Phase != "play" {
		t.Errorf("after 150 decisions the game should be in play, phase %q", st.State.Ext.Phase)
	}
	if st.MoveSeq < 150 {
		t.Errorf("only %d moves recorded", st.MoveSeq)
	}
	if len(st.State.Cells) != 10 {
		t.Errorf("ten units should be on the board, got %d hexes occupied", len(st.State.Cells))
	}
	if len(st.State.Ext.View.Seats) != 2 {
		t.Errorf("scoreboards missing: %+v", st.State.Ext.View.Seats)
	}
}
