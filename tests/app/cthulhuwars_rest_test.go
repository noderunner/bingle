package app_test

import (
	"io"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"
)

type cwInfo struct {
	ID     string `json:"id"`
	Engine string `json:"engine"`
	Guide  []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"guide"`
	Seats []struct {
		Seat  int    `json:"seat"`
		Color string `json:"color"`
		Label string `json:"label"`
	} `json:"seats"`
	Board struct {
		Style  string `json:"style"`
		Canvas *struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"canvas"`
		Areas []struct {
			ID       string         `json:"id"`
			Name     string         `json:"name"`
			Polygons [][][2]float64 `json:"polygons"`
			Anchor   [2]float64     `json:"anchor"`
			Tags     []string       `json:"tags"`
		} `json:"areas"`
		Edges [][2]string `json:"edges"`
	} `json:"board"`
}

func TestCthulhuWarsDescribesItsAreaMapAndGuide(t *testing.T) {
	base := newRESTTestServer(t)
	var info cwInfo
	if code := doJSON(t, "GET", base+"/api/game-defs/cthulhu-wars", nil, &info); code != 200 {
		t.Fatalf("game def: %d", code)
	}
	if info.Engine != "ruleset" || info.Board.Style != "areas" {
		t.Fatalf("engine %q style %q", info.Engine, info.Board.Style)
	}
	if len(info.Seats) != 4 {
		t.Fatalf("seats %+v", info.Seats)
	}
	if info.Board.Canvas == nil || info.Board.Canvas.Width <= 0 || len(info.Board.Areas) != 17 || len(info.Board.Edges) != 42 {
		t.Fatalf("canvas %+v, %d areas, %d edges", info.Board.Canvas, len(info.Board.Areas), len(info.Board.Edges))
	}
	for _, a := range info.Board.Areas {
		if len(a.Polygons) == 0 || a.Name == "" {
			t.Errorf("area %+v", a)
		}
		for _, poly := range a.Polygons {
			if len(poly) < 3 {
				t.Errorf("area %s has a degenerate polygon", a.ID)
			}
			for _, p := range poly {
				if p[0] < 0 || p[1] < 0 || p[0] > info.Board.Canvas.Width || p[1] > info.Board.Canvas.Height {
					t.Errorf("area %s corner %v is off the canvas", a.ID, p)
				}
			}
		}
	}
	if len(info.Guide) < 6 {
		t.Fatalf("a rich game ships a tabbed guide: %+v", info.Guide)
	}

	var guide struct {
		Game      string `json:"game"`
		Generated bool   `json:"generated"`
		Tabs      []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Markdown string `json:"markdown"`
		} `json:"tabs"`
	}
	if code := doJSON(t, "GET", base+"/api/game-defs/cthulhu-wars/guide", nil, &guide); code != 200 {
		t.Fatalf("guide: %d", code)
	}
	if guide.Generated || len(guide.Tabs) != len(info.Guide) {
		t.Fatalf("guide %+v", guide)
	}
	for _, tab := range guide.Tabs {
		if strings.TrimSpace(tab.Markdown) == "" || strings.Contains(tab.Markdown, "(stub)") {
			t.Errorf("tab %q has no content", tab.ID)
		}
	}
}

func TestGuideAssetsAreServedInertAndSafely(t *testing.T) {
	base := newRESTTestServer(t)
	resp, raw := getJSON(t, base, "/api/game-defs/cthulhu-wars/assets/board.svg")
	if resp.StatusCode != 200 {
		t.Fatalf("board.svg: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("content type %q", ct)
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("assets must be served inert: %v", resp.Header)
	}
	svg := string(raw)
	if n := strings.Count(svg, "<polygon"); n < 17 {
		t.Errorf("the generated map has %d polygons", n)
	}
	for _, name := range []string{"North America", "Antarctica", "Arctic Ocean"} {
		if !strings.Contains(svg, name) {
			t.Errorf("map is missing %s", name)
		}
	}

	for _, path := range []string{
		"/api/game-defs/cthulhu-wars/assets/nope.png",
		"/api/game-defs/cthulhu-wars/assets/../cthulhuwars.json",
		"/api/game-defs/cthulhu-wars/assets/..%2Fcthulhuwars.json",
		"/api/game-defs/cthulhu-wars/assets/%2e%2e/%2e%2e/etc/passwd",
		"/api/game-defs/tak/assets/board.svg",
		"/api/game-defs/no-such-game/assets/board.svg",
	} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode < 400 || strings.Contains(string(body), `"id": "cthulhu-wars"`) {
			t.Errorf("%s: status %d body %.80s", path, resp.StatusCode, body)
		}
	}
}

func TestGamesWithoutAGuideGetAGeneratedOne(t *testing.T) {
	base := newRESTTestServer(t)
	var guide struct {
		Generated bool `json:"generated"`
		Tabs      []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Markdown string `json:"markdown"`
		} `json:"tabs"`
	}
	// A game without a guide of its own gets one page, Rules, made from its definition.
	if code := doJSON(t, "GET", base+"/api/game-defs/test-freeplay/guide", nil, &guide); code != 200 {
		t.Fatalf("guide: %d", code)
	}
	if !guide.Generated || len(guide.Tabs) != 1 || guide.Tabs[0].ID != "rules" || guide.Tabs[0].Title != "Rules" {
		t.Fatalf("guide %+v", guide)
	}
	md := guide.Tabs[0].Markdown
	for _, want := range []string{"# Test", "Move notation"} {
		if !strings.Contains(md, want) {
			t.Errorf("the generated rules page lacks %q: %q", want, md)
		}
	}
}

func TestCthulhuWarsPlaysOverRESTWithFourPlayers(t *testing.T) {
	base := newRESTTestServer(t)
	names := map[int]string{1: "Alice", 2: "Bob", 3: "Carol", 4: "Dave"}
	room := newTakRoom(t, base, "cthulhu-wars", "Alice")
	for _, n := range []string{"Bob", "Carol", "Dave"} {
		joinRoom(t, base, room, n)
	}
	rng := rand.New(rand.NewSource(11))
	type legal struct {
		Seat  int `json:"seat"`
		Moves []struct {
			Notation string `json:"notation"`
			Action   string `json:"action"`
		} `json:"moves"`
	}
	var st struct {
		Status   string `json:"status"`
		TurnSeat *int   `json:"turn_seat"`
		State    struct {
			Ext struct {
				RNG   uint64  `json:"rng"`
				Signs [][]int `json:"signs"`
				View  struct {
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
	}
	for i := 0; i < 240; i++ {
		var l legal
		code := doJSON(t, "GET", base+"/api/games/"+room+"/moves/legal", nil, &l)
		if code == 409 {
			// The random game ended: the state says so.
			doJSON(t, "GET", base+"/api/games/"+room+"/state", nil, &st)
			if st.Status != "finished" {
				t.Fatalf("legal moves refused (409) in a %q game", st.Status)
			}
			break
		}
		if code != 200 {
			t.Fatalf("legal moves: %d", code)
		}
		if l.Seat < 1 || l.Seat > 4 || len(l.Moves) == 0 {
			t.Fatalf("decision %d: seat %d with %d moves", i, l.Seat, len(l.Moves))
		}
		n := l.Moves[rng.Intn(len(l.Moves))].Notation
		// A player cannot move for another (checked before the real move, so
		// the next seat is not yet legitimately to move).
		if i%40 == 0 {
			other := names[l.Seat%4+1]
			if code, _ := moveNotation(t, base, room, other, n); code == 200 {
				t.Fatalf("%s moved out of turn", other)
			}
		}
		if code, out := moveNotation(t, base, room, names[l.Seat], n); code != 200 {
			t.Fatalf("%s by %s: %d %v", n, names[l.Seat], code, out)
		}
	}
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/state", nil, &st); code != 200 {
		t.Fatalf("state: %d", code)
	}
	if st.State.Ext.RNG != 0 {
		t.Error("the dice generator leaked")
	}
	if st.State.Ext.View.Prompt == "" && st.Status != "finished" {
		t.Error("a live game shows what is being asked")
	}
	if len(st.State.Ext.View.Seats) != 4 {
		t.Errorf("seat panels: %v", st.State.Ext.View.Seats)
	}
	if len(st.State.Ext.View.Tints) == 0 {
		t.Error("the view should tint ocean and desecrated areas")
	}

	// The whole story: every narrated move, in order, from the first. A
	// position keeps only the latest few.
	var hist []struct {
		Ply   int      `json:"ply"`
		Seat  int      `json:"seat"`
		Lines []string `json:"lines"`
	}
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/history", nil, &hist); code != 200 {
		t.Fatalf("history: %d", code)
	}
	if len(hist) < 40 {
		t.Fatalf("history has %d entries after a long game", len(hist))
	}
	for i, e := range hist {
		if len(e.Lines) == 0 || e.Seat < 1 || (i > 0 && e.Ply <= hist[i-1].Ply) {
			t.Fatalf("history entry %d out of order or empty: %+v", i, e)
		}
	}
	if hist[0].Ply > 3 {
		t.Errorf("history should start at the beginning, not ply %d", hist[0].Ply)
	}
}

func TestBotsCanFillTheOpenSeatsOfAWaitingGame(t *testing.T) {
	base := newRESTTestServer(t)
	room := newTakRoom(t, base, "cthulhu-wars", "Alice")
	joinRoom(t, base, room, "Bob")

	var out struct {
		Seat   int    `json:"seat"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	// Strangers cannot fill someone else's table.
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/bots", map[string]any{"player_name": "Mallory", "bot": "random"}, nil); code != 403 {
		t.Fatalf("a non-player adding a bot: status %d, want 403", code)
	}
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/bots", map[string]any{"player_name": "Alice", "bot": "no-such-bot"}, nil); code != 400 {
		t.Fatalf("unknown bot: status %d, want 400", code)
	}
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/bots", map[string]any{"player_name": "Alice", "bot": "random"}, &out); code != 200 {
		t.Fatalf("add bot: status %d", code)
	}
	if out.Seat != 3 || out.Status != "waiting" || out.Name != "Black Goat (Random)" {
		t.Fatalf("first bot: %+v", out)
	}
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/bots", map[string]any{"player_name": "Bob", "bot": "random"}, &out); code != 200 {
		t.Fatalf("add second bot: status %d", code)
	}
	if out.Seat != 4 || out.Status != "active" {
		t.Fatalf("the table is full and the game starts: %+v", out)
	}
	// Full: nothing more to add.
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/bots", map[string]any{"player_name": "Alice", "bot": "random"}, nil); code != 409 {
		t.Fatalf("a full table: status %d, want 409", code)
	}
	// Alice (seat 1) moves first.
	var l struct {
		Seat int `json:"seat"`
	}
	doJSON(t, "GET", base+"/api/games/"+room+"/moves/legal", nil, &l)
	if l.Seat != 1 {
		t.Fatalf("seat %d to move", l.Seat)
	}
}

func TestSeatBotsArrangeATableAndVariantsAreRegistered(t *testing.T) {
	base := newRESTTestServer(t)
	// Seats 2 and 4 get bots, seat 3 stays open for a friend.
	var created struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
		Seat     int    `json:"seat"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars-3-5", "player_name": "Alice", "seat": 1, "seat_bots": map[string]string{"2": "random", "4": "tactical"}}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if created.Status != "waiting" {
		t.Fatalf("seat 3 is open: %+v", created)
	}
	var g struct {
		Seats []struct {
			Seat int    `json:"seat"`
			Name string `json:"name"`
			Bot  string `json:"bot"`
		} `json:"seats"`
	}
	doJSON(t, "GET", base+"/api/games/"+created.RoomCode, nil, &g)
	names := map[int]string{}
	for _, s := range g.Seats {
		names[s.Seat] = s.Name
	}
	if names[2] != "Crawling Chaos (Random)" || names[4] != "Yellow Sign (Tactical)" || names[3] != "" {
		t.Fatalf("seats %+v", g.Seats)
	}
	joinRoom(t, base, created.RoomCode, "Bob")
	var info cwInfo
	doJSON(t, "GET", base+"/api/game-defs/cthulhu-wars-3p", nil, &info)
	if len(info.Seats) != 3 || len(info.Board.Areas) != 13 {
		t.Fatalf("3p: %d seats, %d areas", len(info.Seats), len(info.Board.Areas))
	}
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "A", "seat_bots": map[string]string{"x": "random"}}, nil); code != 400 {
		t.Fatalf("a bad seat key: %d", code)
	}
}

func TestSlowBotsPlayOnInTheBackgroundAndRequestsDoNotWaitForThem(t *testing.T) {
	base := newRESTTestServer(t)
	room := ""
	{
		var created struct {
			RoomCode string `json:"room_code"`
		}
		if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "cthulhu-wars-3p", "player_name": "Alice", "opponent": "minimax"}, &created); code != 200 {
			t.Fatalf("create: %d", code)
		}
		room = created.RoomCode
	}
	// Great Cthulhu, Alice, is the first player of the game: she chooses the direction.
	if code, out := moveNotation(t, base, room, "Alice", "cw"); code != 200 {
		t.Fatalf("direction: %d %v", code, out)
	}
	started := time.Now()
	if code, out := moveNotation(t, base, room, "Alice", "pass"); code != 200 {
		t.Fatalf("pass: %d %v", code, out)
	}
	if took := time.Since(started); took > 10*time.Second {
		t.Fatalf("the request waited %v for the bots: slow bots must carry on in the background", took)
	}
	// Whatever the bots had not finished by then arrives on its own: the
	// table plays on until it needs Alice again (guided bots decide quickly,
	// so after her pass they finish the Action phase and the first Doom
	// phase until Alice's spellbook choice) or is well under way.
	deadline := time.Now().Add(90 * time.Second)
	n := 0
	for time.Now().Before(deadline) {
		var ms []struct {
			Seat int `json:"seat"`
		}
		doJSON(t, "GET", base+"/api/games/"+room+"/moves", nil, &ms)
		n = len(ms)
		var st struct {
			TurnSeat *int `json:"turn_seat"`
		}
		doJSON(t, "GET", base+"/api/games/"+room+"/state", nil, &st)
		if n >= 14 || (n >= 8 && st.TurnSeat != nil && *st.TurnSeat == 1) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("only %d moves were played in %v; the bots should play their turns by themselves", n, time.Since(started))
}

// A table's view carries the round's phases in rule-book order, the order of
// play both ways, the Elder Sign bag and the log of recent decisions; a
// player sees their own Elder Signs and nobody else's.
func TestCthulhuWarsViewCarriesPhasesOrderBagLogAndPrivateSigns(t *testing.T) {
	base := newRESTTestServer(t)
	room := newTakRoom(t, base, "cthulhu-wars", "Alice")
	for _, n := range []string{"Bob", "Carol", "Dave"} {
		joinRoom(t, base, room, n)
	}
	type view struct {
		Phases []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"phases"`
		Order struct {
			CW  []int `json:"cw"`
			CCW []int `json:"ccw"`
		} `json:"order"`
		Bag struct {
			Left  int `json:"left"`
			Total int `json:"total"`
		} `json:"bag"`
		Seats map[string]struct {
			Hidden int   `json:"hidden_signs"`
			Signs  []int `json:"signs"`
			Units  []struct {
				ID       string `json:"id"`
				Pool     int    `json:"pool"`
				Enhanced bool   `json:"enhanced"`
			} `json:"units"`
		} `json:"seats"`
	}
	type doc struct {
		Status string `json:"status"`
		State  struct {
			Ext struct {
				View view `json:"view"`
				Log  []struct {
					Ply   int      `json:"ply"`
					Seat  int      `json:"seat"`
					Lines []string `json:"lines"`
				} `json:"log"`
			} `json:"ext"`
		} `json:"state"`
	}
	var st doc
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/state", nil, &st); code != 200 {
		t.Fatalf("state: %d", code)
	}
	v := st.State.Ext.View
	var ids []string
	for _, p := range v.Phases {
		ids = append(ids, p.ID)
	}
	if got := strings.Join(ids, ","); got != "action,gather,first,doom" {
		t.Fatalf("phases %s", got)
	}
	// The game starts at the choice of the direction of play, by the first
	// player; the Doom phase is skipped in the first round.
	if v.Phases[0].State != "todo" || v.Phases[1].State != "done" || v.Phases[2].State != "current" || v.Phases[3].State != "todo" {
		t.Errorf("the game starts at the first player's choice of direction: %+v", v.Phases)
	}
	if len(v.Order.CW) != 4 || v.Order.CW[0] != 1 || v.Order.CW[1] != 2 || v.Order.CCW[1] != 4 {
		t.Errorf("order %+v", v.Order)
	}
	if v.Bag.Left != 36 || v.Bag.Total != 36 {
		t.Errorf("bag %+v", v.Bag)
	}
	for seat, s := range v.Seats {
		if len(s.Units) != 5 {
			t.Errorf("seat %s has %d unit cards", seat, len(s.Units))
		}
	}

	// A decision shows up in the log with its seat.
	var l struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"moves"`
	}
	doJSON(t, "GET", base+"/api/games/"+room+"/moves/legal", nil, &l)
	if len(l.Moves) == 0 {
		t.Fatal("no legal moves")
	}
	if code, out := moveNotation(t, base, room, "Alice", l.Moves[0].Notation); code != 200 {
		t.Fatalf("%s: %d %v", l.Moves[0].Notation, code, out)
	}
	doJSON(t, "GET", base+"/api/games/"+room+"/state", nil, &st)
	if n := len(st.State.Ext.Log); n != 1 || st.State.Ext.Log[0].Seat != 1 || st.State.Ext.Log[0].Ply != 1 {
		t.Errorf("log after one decision: %+v", st.State.Ext.Log)
	}

	// Own Elder Signs: put some in the start position of a new table.
	var raw map[string]any
	doJSON(t, "GET", base+"/api/games/"+newTakRoom(t, base, "cthulhu-wars", "Zed")+"/state", nil, &raw)
	ext := raw["state"].(map[string]any)["ext"].(map[string]any)
	ext["signs"] = []any{nil, []any{3, 1}, []any{2}, nil, nil}
	ext["bag"] = []any{16, 11, 6}
	start := raw["state"]
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{
		"game_def_id": "cthulhu-wars", "player_name": "Alice", "start_state": start, "start_seat": 1,
	}, &created); code != 200 {
		t.Fatalf("create from a position: %d", code)
	}
	joinRoom(t, base, created.RoomCode, "Bob")
	var mine, theirs, anyone doc
	doJSON(t, "GET", base+"/api/games/"+created.RoomCode+"/state?player_name=Alice", nil, &mine)
	doJSON(t, "GET", base+"/api/games/"+created.RoomCode+"/state?player_name=Bob", nil, &theirs)
	doJSON(t, "GET", base+"/api/games/"+created.RoomCode+"/state", nil, &anyone)
	if got := mine.State.Ext.View.Seats["1"]; len(got.Signs) != 2 || got.Signs[0] != 3 || got.Signs[1] != 1 || got.Hidden != 2 {
		t.Errorf("Alice's own view: %+v", got)
	}
	for name, d := range map[string]doc{"Bob": theirs, "a spectator": anyone} {
		if got := d.State.Ext.View.Seats["1"]; len(got.Signs) != 0 || got.Hidden != 2 {
			t.Errorf("%s sees Alice's signs: %+v", name, got)
		}
	}
	if b := anyone.State.Ext.View.Bag; b.Left != 33 {
		t.Errorf("bag after those signs were drawn: %+v", b)
	}
}

// openingDirection plays the first decision of a new Cthulhu Wars table:
// Great Cthulhu, the first player of the game, chooses the direction of play
// before anybody acts.
func openingDirection(t *testing.T, baseURL, room, first string) {
	t.Helper()
	if code, out := moveNotation(t, baseURL, room, first, "cw"); code != 200 {
		t.Fatalf("the first player's choice of direction: %d %v", code, out)
	}
}
