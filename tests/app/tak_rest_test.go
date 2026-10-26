package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// doJSON sends method path with an optional JSON body and decodes the JSON
// response into out (if non-nil), returning the status code.
func doJSON(t *testing.T, method, url string, body any, out any) int {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, url, raw, err)
		}
	}
	return resp.StatusCode
}

type restState struct {
	BoardState map[string]struct{ Piece, Color string } `json:"board_state"`
	State      struct {
		V        int                                        `json:"v"`
		Cells    map[string][]struct{ Piece, Color string } `json:"cells"`
		Reserves map[string]map[string]int                  `json:"reserves"`
		Ply      int                                        `json:"ply"`
	} `json:"state"`
	TurnSeat   *int   `json:"turn_seat"`
	Status     string `json:"status"`
	MoveSeq    int    `json:"move_seq"`
	WinnerSeat *int   `json:"winner_seat"`
	EndReason  string `json:"end_reason"`
}

func newTakRoom(t *testing.T, baseURL, def, name string) string {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": def, "player_name": name}, &created); code != 200 {
		t.Fatalf("create %s: status %d", def, code)
	}
	return created.RoomCode
}

func joinRoom(t *testing.T, baseURL, room, name string) {
	t.Helper()
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/join", map[string]any{"player_name": name}, nil); code != 200 {
		t.Fatalf("join %s as %s: status %d", room, name, code)
	}
}

func moveNotation(t *testing.T, baseURL, room, name, notation string) (int, map[string]any) {
	t.Helper()
	var out map[string]any
	code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/moves", map[string]any{"player_name": name, "notation": notation}, &out)
	return code, out
}

func TestTakRegisteredWithVariants(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var defs []struct {
		ID          string `json:"id"`
		BoardWidth  int    `json:"board_width"`
		Description string `json:"description"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/game-defs", nil, &defs); code != 200 {
		t.Fatalf("status %d", code)
	}
	have := map[string]int{}
	for _, d := range defs {
		have[d.ID] = d.BoardWidth
	}
	for id, w := range map[string]int{"test-freeplay": 9, "tak-3x3": 3, "tak-4x4": 4, "tak-5x5": 5, "tak-6x6": 6, "tak-7x7": 7, "tak-8x8": 8} {
		if have[id] != w {
			t.Errorf("game def %q width = %d, want %d (defs: %v)", id, have[id], w, have)
		}
	}
}

func TestGameDefInfo(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var info struct {
		ID       string `json:"id"`
		Notation string `json:"notation"`
		Rules    []string
		Board    struct {
			Kind      string `json:"kind"`
			Width     int    `json:"width"`
			Labels    string `json:"labels"`
			Positions []struct {
				ID   string
				X, Y float64
			} `json:"positions"`
			Regions map[string][]string `json:"regions"`
		} `json:"board"`
		Pieces []struct {
			Type, Shape, Pool string
			Coverable         bool
		} `json:"pieces"`
		Pools []struct {
			ID     string
			Pieces []string
			Start  int
		} `json:"pools"`
		Actions []struct{ ID, Type string } `json:"actions"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/game-defs/tak-5x5", nil, &info); code != 200 {
		t.Fatalf("status %d", code)
	}
	if info.Notation != "ptn" || info.Board.Kind != "grid" || info.Board.Width != 5 || info.Board.Labels != "algebraic" {
		t.Errorf("info = %+v", info)
	}
	if len(info.Board.Positions) != 25 {
		t.Errorf("positions = %d, want 25", len(info.Board.Positions))
	}
	if len(info.Rules) == 0 {
		t.Error("no rules text")
	}
	// a1 must be bottom-left: x=0, y=4.
	for _, p := range info.Board.Positions {
		if p.ID == "a1" && (p.X != 0 || p.Y != 4) {
			t.Errorf("a1 at (%v,%v), want (0,4)", p.X, p.Y)
		}
	}
	pools := map[string]int{}
	for _, p := range info.Pools {
		pools[p.ID] = p.Start
	}
	if pools["stone"] != 21 || pools["cap"] != 1 {
		t.Errorf("pools = %v, want stone 21 cap 1", pools)
	}
	if len(info.Board.Regions["left"]) != 5 {
		t.Errorf("left region = %v", info.Board.Regions["left"])
	}
	if code := doJSON(t, "GET", baseURL+"/api/game-defs/nope", nil, nil); code != 404 {
		t.Errorf("unknown def status = %d, want 404", code)
	}
}

func TestTakFullGameOverREST(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	// Opening: Alice (white) places BLACK's flat; Bob places WHITE's.
	if code, _ := moveNotation(t, baseURL, room, "Alice", "a3"); code != 200 {
		t.Fatalf("opening a3: %d", code)
	}
	if code, _ := moveNotation(t, baseURL, room, "Bob", "c1"); code != 200 {
		t.Fatalf("opening c1: %d", code)
	}

	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if st.State.V != 2 || st.State.Ply != 2 {
		t.Errorf("state v=%d ply=%d, want v2 ply2", st.State.V, st.State.Ply)
	}
	if a3 := st.BoardState["a3"]; a3.Color != "black" {
		t.Errorf("legacy board_state a3 = %+v, want black", a3)
	}
	if st.State.Reserves["white"]["stone"] != 9 || st.State.Reserves["black"]["stone"] != 9 {
		t.Errorf("reserves = %v", st.State.Reserves)
	}

	// White builds the bottom row: c1 is white (placed by Bob). b1 then a1.
	for i, step := range []struct{ who, mv string }{
		{"Alice", "b1"}, {"Bob", "b3"},
		{"Alice", "a1"}, // a1,b1,c1 all white: a road left-right on rank 1
	} {
		code, out := moveNotation(t, baseURL, room, step.who, step.mv)
		if code != 200 {
			t.Fatalf("step %d %s %s: %d %v", i, step.who, step.mv, code, out)
		}
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if st.Status != "finished" || st.WinnerSeat == nil || *st.WinnerSeat != 1 || st.EndReason != "road" {
		t.Fatalf("state = status %q winner %v reason %q, want finished/1/road", st.Status, st.WinnerSeat, st.EndReason)
	}

	// History carries notation.
	var hist []struct {
		Seq      int    `json:"seq"`
		Notation string `json:"notation"`
		Action   string `json:"action"`
		Piece    string `json:"piece"`
		To       string `json:"to"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves", nil, &hist)
	if len(hist) != 5 || hist[0].Notation != "a3" || hist[4].Notation != "a1" || hist[0].Action != "place" || hist[0].Piece != "flat" {
		t.Errorf("history = %+v", hist)
	}

	// at_seq=0 is the starting position now.
	var start restState
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/state?at_seq=0", nil, &start); code != 200 {
		t.Fatalf("at_seq=0 status %d", code)
	}
	if len(start.State.Cells) != 0 || start.Status != "active" {
		t.Errorf("at_seq=0 state = %+v", start)
	}

	// Export.
	var rec struct {
		Notation []string `json:"notation"`
		PTN      string   `json:"ptn"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/export", nil, &rec)
	if len(rec.Notation) != 5 || !strings.Contains(rec.PTN, "[Size \"3\"]") || !strings.Contains(rec.PTN, "R-0") {
		t.Errorf("export = %+v", rec)
	}
}

func TestIllegalTakMoveReturns400WithReason(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	code, out := moveNotation(t, baseURL, room, "Alice", "Sc3") // wall during the opening
	if code != 400 {
		t.Fatalf("status = %d, want 400 (%v)", code, out)
	}
	msg, _ := out["error"].(string)
	if !strings.Contains(msg, "opening") {
		t.Errorf("error = %q, want it to explain the opening restriction", msg)
	}
}

func TestStructuredMoveFields(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	var out map[string]any
	code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/moves",
		map[string]any{"player_name": "Alice", "action": "place", "piece": "flat", "to": "c3"}, &out)
	if code != 200 {
		t.Fatalf("structured place: %d %v", code, out)
	}
	mv := out["move"].(map[string]any)
	if mv["notation"] != "c3" || mv["piece"] != "flat" {
		t.Errorf("move = %v", mv)
	}
}

func TestLegalMovesAllWithSteps(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	var lm struct {
		Seat  int `json:"seat"`
		Moves []struct {
			Notation string `json:"notation"`
			Action   string `json:"action"`
			Steps    []struct{ Kind, Value string }
		} `json:"moves"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves/legal?steps=1", nil, &lm); code != 200 {
		t.Fatalf("status %d", code)
	}
	if lm.Seat != 1 || len(lm.Moves) != 25 {
		t.Fatalf("seat=%d moves=%d, want 1/25", lm.Seat, len(lm.Moves))
	}
	first := lm.Moves[0]
	if first.Action != "place" || len(first.Steps) != 2 || first.Steps[0].Kind != "piece" || first.Steps[1].Kind != "pos" {
		t.Errorf("first move = %+v", first)
	}
}

func TestUndoRestoresPreviousPosition(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	moveNotation(t, baseURL, room, "Alice", "a1")
	moveNotation(t, baseURL, room, "Bob", "e5")

	// Alice cannot undo: Bob has moved since her move.
	var errBody map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/undo", map[string]any{"player_name": "Alice"}, &errBody); code != 409 {
		t.Fatalf("undo after opponent moved: %d %v, want 409", code, errBody)
	}

	// Bob can take back his move.
	var out struct {
		Removed []int     `json:"removed"`
		State   restState `json:"state"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/undo", map[string]any{"player_name": "Bob"}, &out); code != 200 {
		t.Fatalf("undo status %d", code)
	}
	if len(out.Removed) != 1 || out.Removed[0] != 2 || out.State.MoveSeq != 1 || out.State.TurnSeat == nil || *out.State.TurnSeat != 2 {
		t.Errorf("undo result = %+v", out)
	}
	if _, ok := out.State.BoardState["e5"]; ok {
		t.Error("e5 still occupied after undo")
	}
	if out.State.State.Reserves["white"]["stone"] != 21 {
		t.Errorf("white reserve after undo = %v, want restored to 21", out.State.State.Reserves)
	}

	// Moves history shrank and Bob can play again.
	var hist []map[string]any
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves", nil, &hist)
	if len(hist) != 1 {
		t.Errorf("history length = %d, want 1", len(hist))
	}
	if code, _ := moveNotation(t, baseURL, room, "Bob", "d4"); code != 200 {
		t.Errorf("replaying after undo: %d", code)
	}

	// Nothing to undo for a player who hasn't moved... after Alice undoes
	// everything of hers there is nothing left.
	moveNotation(t, baseURL, room, "Alice", "Sc3")
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/undo", map[string]any{"player_name": "Alice"}, nil); code != 200 {
		t.Errorf("undo own last move: %d", code)
	}
}

func TestResignEndsGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	var st restState
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/resign", map[string]any{"player_name": "Alice"}, &st); code != 200 {
		t.Fatalf("resign: %d", code)
	}
	if st.Status != "finished" || st.WinnerSeat == nil || *st.WinnerSeat != 2 || st.EndReason != "resignation" {
		t.Errorf("after resign: %+v", st)
	}
	if code, _ := moveNotation(t, baseURL, room, "Bob", "a1"); code != 409 {
		t.Errorf("move after resign: %d, want 409", code)
	}
}

func TestSimulateDoesNotChangeGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	var sim struct {
		Applied   []string                    `json:"applied"`
		GameEnded bool                        `json:"game_ended"`
		TurnSeat  *int                        `json:"turn_seat"`
		Legal     []struct{ Notation string } `json:"legal_moves"`
	}
	code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/simulate",
		map[string]any{"moves": []string{"a1", "c3", "b2"}}, &sim)
	if code != 200 {
		t.Fatalf("simulate: %d", code)
	}
	if len(sim.Applied) != 3 || sim.GameEnded || sim.TurnSeat == nil || *sim.TurnSeat != 2 || len(sim.Legal) == 0 {
		t.Errorf("simulation = %+v", sim)
	}

	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if st.MoveSeq != 0 || len(st.State.Cells) != 0 {
		t.Errorf("simulate mutated the game: %+v", st)
	}

	var bad map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/simulate",
		map[string]any{"moves": []string{"a1", "a1"}}, &bad); code != 400 {
		t.Errorf("simulating an illegal move: %d, want 400", code)
	}
}

func TestBotOpponentPlaysBack(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
		Seat     int    `json:"seat"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games",
		map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create vs bot: %d", code)
	}
	if created.Status != "active" || created.Seat != 1 {
		t.Fatalf("created = %+v, want active seat 1", created)
	}

	var out struct {
		Move     map[string]any   `json:"move"`
		State    restState        `json:"state"`
		BotMoves []map[string]any `json:"bot_moves"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/moves",
		map[string]any{"player_name": "Alice", "notation": "c3"}, &out); code != 200 {
		t.Fatalf("move: %d", code)
	}
	if len(out.BotMoves) != 1 || out.State.MoveSeq != 2 || out.State.TurnSeat == nil || *out.State.TurnSeat != 1 {
		t.Errorf("after move vs bot: bot_moves=%d move_seq=%d turn=%v", len(out.BotMoves), out.State.MoveSeq, out.State.TurnSeat)
	}

	// Undo takes back Alice's move AND the bot's reply.
	var undone struct {
		Removed []int `json:"removed"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/undo", map[string]any{"player_name": "Alice"}, &undone); code != 200 {
		t.Fatalf("undo vs bot: %d", code)
	}
	if len(undone.Removed) != 2 {
		t.Errorf("removed = %v, want both plies", undone.Removed)
	}

	// Playing second: the bot opens.
	var created2 struct {
		RoomCode string           `json:"room_code"`
		Seat     int              `json:"seat"`
		BotMoves []map[string]any `json:"bot_moves"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games",
		map[string]any{"game_def_id": "tak-5x5", "player_name": "Zed", "opponent": "tactical", "seat": 2}, &created2); code != 200 {
		t.Fatalf("create as seat 2: %d", code)
	}
	if created2.Seat != 2 || len(created2.BotMoves) != 1 {
		t.Errorf("seat-2 game = %+v, want seat 2 with the bot's opening move already played", created2)
	}

	if code := doJSON(t, "POST", baseURL+"/api/games",
		map[string]any{"game_def_id": "tak-5x5", "player_name": "Q", "opponent": "no-such-bot"}, nil); code != 400 {
		t.Errorf("unknown bot: %d, want 400", code)
	}
}

func TestObserveForAgents(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	moveNotation(t, baseURL, room, "Alice", "a1")

	var obs struct {
		Seat       int      `json:"seat"`
		YourTurn   bool     `json:"your_turn"`
		LegalMoves []string `json:"legal_moves"`
		BoardText  string   `json:"board_text"`
		TPS        string   `json:"tps"`
		Reward     float64  `json:"reward"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/observe?player_name=Bob", nil, &obs)
	if obs.Seat != 2 || !obs.YourTurn || len(obs.LegalMoves) != 24 {
		t.Errorf("Bob's observation = seat %d turn %v legal %d", obs.Seat, obs.YourTurn, len(obs.LegalMoves))
	}
	if !strings.Contains(obs.BoardText, "bF") || obs.TPS == "" {
		t.Errorf("board text/TPS missing: %q %q", obs.BoardText, obs.TPS)
	}

	doJSON(t, "GET", baseURL+"/api/games/"+room+"/observe?player_name=Alice", nil, &obs)
	if obs.YourTurn || len(obs.LegalMoves) != 0 {
		t.Errorf("Alice's observation = turn %v legal %d, want waiting", obs.YourTurn, len(obs.LegalMoves))
	}
}

func TestInputLimits(t *testing.T) {
	baseURL := newRESTTestServer(t)

	long := strings.Repeat("n", 41)
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tak-3x3", "player_name": long}, nil); code != 400 {
		t.Errorf("41-character name: %d, want 400", code)
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tak-3x3", "player_name": "bad\x07name"}, nil); code != 400 {
		t.Errorf("control character in name: %d, want 400", code)
	}

	huge := map[string]any{"game_def_id": "tak-3x3", "player_name": "x", "pad": strings.Repeat("A", 2<<20)}
	if code := doJSON(t, "POST", baseURL+"/api/games", huge, nil); code != 400 {
		t.Errorf("2 MiB body: %d, want 400", code)
	}

	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")
	moves := make([]string, 401)
	for i := range moves {
		moves[i] = "a1"
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/simulate", map[string]any{"moves": moves}, nil); code != 400 {
		t.Errorf("401 simulated moves: %d, want 400", code)
	}

	// Room codes are case-insensitive.
	var st restState
	if code := doJSON(t, "GET", baseURL+"/api/games/"+strings.ToLower(room)+"/state", nil, &st); code != 200 {
		t.Errorf("lower-case room code: %d, want 200", code)
	}
}

func TestTimedGameOverREST(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games",
		map[string]any{"game_def_id": "tak-3x3", "player_name": "Alice", "move_time_seconds": 2}, &created); code != 200 {
		t.Fatalf("create timed game: %d", code)
	}
	joinRoom(t, baseURL, created.RoomCode, "Bob")

	var game struct {
		Status          string `json:"status"`
		MoveTimeSeconds int    `json:"move_time_seconds"`
		TurnDeadline    string `json:"turn_deadline"`
		ServerTime      string `json:"server_time"`
		EndReason       string `json:"end_reason"`
		WinnerSeat      *int   `json:"winner_seat"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &game)
	if game.Status != "active" || game.MoveTimeSeconds != 2 || game.TurnDeadline == "" || game.ServerTime == "" {
		t.Fatalf("timed game = %+v", game)
	}

	// Nobody moves for longer than the limit: reading the game ends it.
	time.Sleep(3200 * time.Millisecond)
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &game)
	if game.Status != "finished" || game.EndReason != "timeout" || game.WinnerSeat == nil || *game.WinnerSeat != 2 {
		t.Errorf("after the limit: %+v, want finished by timeout with seat 2 winning", game)
	}
	if code, _ := moveNotation(t, baseURL, created.RoomCode, "Alice", "a1"); code != 409 {
		t.Errorf("late move: %d, want 409", code)
	}

	if code := doJSON(t, "POST", baseURL+"/api/games",
		map[string]any{"game_def_id": "tak-3x3", "player_name": "Alice", "move_time_seconds": -5}, nil); code != 400 {
		t.Errorf("negative limit: %d, want 400", code)
	}
}

func TestDrawOfferFlow(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	draw := func(who, action string) (int, map[string]any) {
		var out map[string]any
		code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/draw", map[string]any{"player_name": who, "action": action}, &out)
		return code, out
	}
	offerSeat := func() any {
		var g map[string]any
		doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &g)
		return g["draw_offer_seat"]
	}

	if code, _ := draw("Alice", "accept"); code != 409 {
		t.Errorf("accepting with no offer: %d, want 409", code)
	}
	if code, out := draw("Alice", "offer"); code != 200 || out["draw_offer_seat"] != float64(1) {
		t.Fatalf("offer: %d %v", code, out)
	}
	if got := offerSeat(); got != float64(1) {
		t.Errorf("draw_offer_seat = %v, want 1", got)
	}
	if code, _ := draw("Alice", "offer"); code != 409 {
		t.Errorf("second offer: %d, want 409", code)
	}
	if code, _ := draw("Alice", "accept"); code != 409 {
		t.Errorf("accepting your own offer: %d, want 409", code)
	}

	// A move withdraws the offer (Bob playing on = declining).
	if code, out := moveNotation(t, baseURL, room, "Alice", "a1"); code != 200 {
		t.Fatalf("move: %d %v", code, out)
	}
	if got := offerSeat(); got != nil {
		t.Errorf("offer after a move = %v, want withdrawn", got)
	}

	// Offer, decline, offer again, accept.
	draw("Bob", "offer")
	if code, _ := draw("Alice", "decline"); code != 200 {
		t.Errorf("decline: %d", code)
	}
	if got := offerSeat(); got != nil {
		t.Errorf("offer after decline = %v", got)
	}
	draw("Bob", "offer")
	if code, _ := draw("Bob", "cancel"); code != 200 {
		t.Errorf("cancel: %d", code)
	}
	draw("Bob", "offer")
	var final struct {
		Status     string `json:"status"`
		EndReason  string `json:"end_reason"`
		WinnerSeat *int   `json:"winner_seat"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/draw", map[string]any{"player_name": "Alice", "action": "accept"}, &final); code != 200 {
		t.Fatalf("accept: %d", code)
	}
	if final.Status != "finished" || final.EndReason != "agreement" || final.WinnerSeat != nil {
		t.Errorf("after agreement: %+v, want a finished draw by agreement", final)
	}
}

func TestDrawOfferToABotIsRefused(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tak-3x3", "player_name": "Alice", "opponent": "random"}, &created)
	var out map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/draw", map[string]any{"player_name": "Alice", "action": "offer"}, &out); code != 409 {
		t.Errorf("offering a draw to a bot: %d %v, want 409", code, out)
	}
}

func TestHumansCannotTakeABotsSeatByName(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tak-3x3", "player_name": "Alice", "opponent": "random"}, &created)

	var out map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/join", map[string]any{"player_name": "bot (random)"}, &out); code != 409 {
		t.Errorf("claiming the bot's seat: %d %v, want 409", code, out)
	}
	if code, _ := moveNotation(t, baseURL, created.RoomCode, "Bot (Random)", "a1"); code != 403 {
		t.Errorf("moving as the bot: %d, want 403", code)
	}
}

func TestHistoricalTurnSeatAccountsForPasses(t *testing.T) {
	baseURL := newRESTTestServer(t)
	names := map[int]string{1: "Alice", 2: "Bob"}

	// Random Reversi games pass fairly often; play seeded games until one
	// does (the same seat moving twice in a row), then check the turn seat
	// reported for the position right after the move that was followed by
	// the pass.
	for attempt := 0; attempt < 25; attempt++ {
		rng := rand.New(rand.NewSource(int64(attempt)))
		room := newTakRoom(t, baseURL, "reversi", "Alice")
		joinRoom(t, baseURL, room, "Bob")

		var seats []int
		passedAt := 0
		for len(seats) < 70 && passedAt == 0 {
			var legal struct {
				Seat  int                         `json:"seat"`
				Moves []struct{ Notation string } `json:"moves"`
			}
			if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves/legal", nil, &legal); code != 200 {
				break // game over
			}
			mv := legal.Moves[rng.Intn(len(legal.Moves))].Notation
			if code, out := moveNotation(t, baseURL, room, names[legal.Seat], mv); code != 200 {
				t.Fatalf("move %s by seat %d: %d %v", mv, legal.Seat, code, out)
			}
			seats = append(seats, legal.Seat)
			if n := len(seats); n >= 2 && seats[n-1] == seats[n-2] {
				passedAt = n - 1
			}
		}
		if passedAt == 0 {
			continue
		}
		var st restState
		doJSON(t, "GET", baseURL+"/api/games/"+room+"/state?at_seq="+strconv.Itoa(passedAt), nil, &st)
		if st.TurnSeat == nil || *st.TurnSeat != seats[passedAt-1] {
			t.Errorf("turn seat after move %d = %v, want %d (the opponent had to pass)", passedAt, st.TurnSeat, seats[passedAt-1])
		}
		return
	}
	t.Fatal("no seeded Reversi game produced a pass")
}

// Every shipped game must describe how it ends and how to write a move, with
// a worked example, so the rules dialog and get_game_info are never empty.
func TestEveryGameDefDescribesRulesAndNotation(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var defs []struct {
		ID string `json:"id"`
	}
	doJSON(t, "GET", baseURL+"/api/game-defs", nil, &defs)
	if len(defs) < 20 {
		t.Fatalf("only %d game definitions", len(defs))
	}
	for _, d := range defs {
		var info struct {
			Rules        []string `json:"rules"`
			WinRules     []string `json:"win_rules"`
			NotationHelp []string `json:"notation_help"`
		}
		if code := doJSON(t, "GET", baseURL+"/api/game-defs/"+d.ID, nil, &info); code != 200 {
			t.Errorf("%s: status %d", d.ID, code)
			continue
		}
		if d.ID != "test-freeplay" && len(info.Rules) == 0 {
			t.Errorf("%s: no rules text", d.ID)
		}
		if len(info.WinRules) == 0 {
			t.Errorf("%s: no win rule description", d.ID)
		}
		hasExample := false
		for _, n := range info.NotationHelp {
			if strings.Contains(n, "Example") || strings.Contains(n, "Placement:") {
				hasExample = true
			}
		}
		if len(info.NotationHelp) == 0 || !hasExample {
			t.Errorf("%s: notation help %v lacks an example", d.ID, info.NotationHelp)
		}
		for _, w := range info.WinRules {
			if strings.Contains(w, "; .") || strings.HasSuffix(w, "; .") {
				t.Errorf("%s: empty outcome in %q", d.ID, w)
			}
		}
	}
}

func TestSimulateReportsTPSForTak(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	moveNotation(t, baseURL, room, "Alice", "a5")
	moveNotation(t, baseURL, room, "Bob", "e1")

	var sim struct {
		TPS string `json:"tps"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/simulate", map[string]any{"moves": []string{}}, &sim); code != 200 {
		t.Fatalf("simulate: %d", code)
	}
	if !strings.HasSuffix(sim.TPS, " 1 2") || strings.Count(sim.TPS, "/") != 4 {
		t.Errorf("TPS = %q, want a 5-row TPS with white to move on move 2", sim.TPS)
	}
	// The TPS must round-trip into a new game.
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tak-5x5", "player_name": "Eve", "start_tps": sim.TPS}, &created); code != 200 {
		t.Errorf("starting a game from the reported TPS: %d", code)
	}

	// Other games report none.
	other := newTakRoom(t, baseURL, "tic-tac-toe", "Zed")
	var ttt struct {
		TPS string `json:"tps"`
	}
	doJSON(t, "POST", baseURL+"/api/games/"+other+"/simulate", map[string]any{"moves": []string{}}, &ttt)
	if ttt.TPS != "" {
		t.Errorf("tic-tac-toe simulate returned TPS %q", ttt.TPS)
	}
}
