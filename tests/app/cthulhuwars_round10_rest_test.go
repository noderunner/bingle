package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
)

// The playtest of game TQQJ96 started with Black Goat as the first player
// though Great Cthulhu sat in the game. A table's first player is Great
// Cthulhu wherever it sits, else the first cult by name; the platform asks
// that player (after the Windwalker, who sets up before anyone plays).

func TestNewTablesStartWithTheRightFirstPlayerChoosingTheDirection(t *testing.T) {
	baseURL := newRESTTestServer(t)
	cases := []struct {
		name    string
		def     string
		factors []string
		first   int // the seat that is the first player and chooses the direction
		setup   int // the Windwalker's seat, who sets up first (0: none)
	}{
		{"the standard table", "cthulhu-wars", nil, 1, 0},
		{"Great Cthulhu in seat 3", "cthulhu-wars", []string{"bg", "cc", "gc", "ys"}, 3, 0},
		{"Great Cthulhu last", "cthulhu-wars", []string{"ys", "bg", "cc", "gc"}, 4, 0},
		{"the TQQJ96 table", "cthulhu-wars-5p", []string{"bg", "cc", "gc", "ys", "ww"}, 3, 5},
		{"a Windwalker in seat 1", "cthulhu-wars-5p", []string{"ww", "ys", "cc", "gc", "bg"}, 4, 1},
		{"no Great Cthulhu: Black Goat is before Yellow Sign", "cthulhu-wars", []string{"ys", "cc", "bg", "sl"}, 3, 0},
		{"no Great Cthulhu or Black Goat: Crawling Chaos", "cthulhu-wars", []string{"ys", "sl", "cc", "ww"}, 3, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Every seat is a person, so that nothing plays by itself.
			body := map[string]any{"game_def_id": c.def, "player_name": "P1"}
			if c.factors != nil {
				body["options"] = map[string]any{"factions": c.factors}
			}
			var created struct {
				RoomCode string `json:"room_code"`
				Status   string `json:"status"`
			}
			if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
				t.Fatalf("create: %d", code)
			}
			var info struct {
				Seats []struct{ Seat int } `json:"seats"`
			}
			doJSON(t, "GET", baseURL+"/api/game-defs/"+gameDefOf(t, baseURL, created.RoomCode), nil, &info)
			names := map[int]string{1: "P1"}
			for seat := 2; seat <= len(info.Seats); seat++ {
				names[seat] = fmt.Sprintf("P%d", seat)
				joinRoom(t, baseURL, created.RoomCode, names[seat])
			}
			var st struct {
				Status   string `json:"status"`
				TurnSeat int    `json:"turn_seat"`
			}
			doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &st)
			wantFirst := c.first
			if c.setup != 0 {
				wantFirst = c.setup
			}
			if st.Status != "active" || st.TurnSeat != wantFirst {
				t.Fatalf("status %q, seat %d to move, want %d", st.Status, st.TurnSeat, wantFirst)
			}
			if c.setup != 0 {
				var legal struct {
					Moves []struct {
						Notation string `json:"notation"`
					} `json:"moves"`
				}
				doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/moves/legal", nil, &legal)
				if len(legal.Moves) == 0 {
					t.Fatal("the Windwalker has nowhere to begin")
				}
				if code, out := moveNotation(t, baseURL, created.RoomCode, names[c.setup], legal.Moves[0].Notation); code != 200 {
					t.Fatalf("setup %s: %d %v", legal.Moves[0].Notation, code, out)
				}
			}
			// The first player of the game chooses the direction of play.
			var state struct {
				TurnSeat int `json:"turn_seat"`
				State    struct {
					Ext struct {
						Stage string `json:"stage"`
						First int    `json:"first"`
						View  struct {
							Prompt string `json:"prompt"`
						} `json:"view"`
					} `json:"ext"`
				} `json:"state"`
			}
			doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/state", nil, &state)
			if state.TurnSeat != c.first || state.State.Ext.First != c.first || state.State.Ext.Stage != "direction" {
				t.Fatalf("after the setup: seat %d to move, first %d, stage %q", state.TurnSeat, state.State.Ext.First, state.State.Ext.Stage)
			}
			if p := state.State.Ext.View.Prompt; p == "" {
				t.Error("the first player is told what to choose")
			}
			// Another player may not choose for them.
			other := names[c.first%len(names)+1]
			if code, _ := moveNotation(t, baseURL, created.RoomCode, other, "cw"); code == 200 {
				t.Errorf("%s chose the direction for the first player", other)
			}
			if code, out := moveNotation(t, baseURL, created.RoomCode, names[c.first], "ccw"); code != 200 {
				t.Fatalf("direction: %d %v", code, out)
			}
			doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/state", nil, &state)
			if state.TurnSeat != c.first || state.State.Ext.Stage != "turn" {
				t.Errorf("then the first player acts: seat %d, stage %q", state.TurnSeat, state.State.Ext.Stage)
			}
		})
	}
}

// gameDefOf is the definition id of a room.
func gameDefOf(t *testing.T, baseURL, room string) string {
	t.Helper()
	var g struct {
		GameDefID string `json:"game_def_id"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &g)
	return g.GameDefID
}

// Before the game starts, anyone at the table may change the cult of an open
// seat; a cult played from another open seat changes places with it, and
// seats a player or bot holds keep theirs.
func TestTheCultOfAnOpenSeatCanBeChangedInTheLobby(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "lobby": true, "seat_bots": map[string]string{"2": "tactical"}}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	type seats struct {
		GameDefID string `json:"game_def_id"`
		Seats     []struct {
			Seat  int    `json:"seat"`
			Color string `json:"color"`
			Label string `json:"label"`
			Name  string `json:"name"`
		} `json:"seats"`
	}
	labels := func() (string, string) {
		var g seats
		doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &g)
		var out string
		for _, s := range g.Seats {
			out += s.Label + "/" + s.Color + ","
		}
		return g.GameDefID, out
	}
	change := func(who string, seat int, cult string) (int, map[string]any) {
		var out map[string]any
		code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/seat-cult", map[string]any{"player_name": who, "seat": seat, "cult": cult}, &out)
		return code, out
	}
	id, got := labels()
	if id != "cthulhu-wars" || got != "Great Cthulhu/green,Crawling Chaos/blue,Black Goat/red,Yellow Sign/yellow," {
		t.Fatalf("at the start: %s %s", id, got)
	}

	// Seat 3 (open) becomes the Sleeper: a cult nobody plays.
	if code, out := change("Alice", 3, "sl"); code != 200 {
		t.Fatalf("change: %d %v", code, out)
	}
	id, got = labels()
	if id != "cthulhu-wars~gc-cc-sl-ys" || got != "Great Cthulhu/green,Crawling Chaos/blue,Sleeper/orange,Yellow Sign/yellow," {
		t.Fatalf("after the change: %s %s", id, got)
	}
	// Seat 4 (open) takes Great Cthulhu's cult... which Alice holds: refused.
	if code, _ := change("Alice", 4, "gc"); code != 409 {
		t.Errorf("a cult a player holds cannot be taken: %d", code)
	}
	// The bot's seat keeps its cult.
	if code, _ := change("Alice", 2, "bg"); code != 409 {
		t.Errorf("a bot's seat cannot change: %d", code)
	}
	// Two open seats swap: seat 4 asks for the Sleeper, which seat 3 plays.
	if code, out := change("Alice", 4, "sl"); code != 200 {
		t.Fatalf("swap: %d %v", code, out)
	}
	id, got = labels()
	if id != "cthulhu-wars~gc-cc-ys-sl" || got != "Great Cthulhu/green,Crawling Chaos/blue,Yellow Sign/yellow,Sleeper/orange," {
		t.Fatalf("after the swap: %s %s", id, got)
	}
	// A stranger may not; an unknown cult is refused; so is a seat that does not exist.
	if code, _ := change("Mallory", 3, "bg"); code != 403 {
		t.Errorf("a stranger changed a seat: %d", code)
	}
	if code, _ := change("Alice", 3, "xx"); code != 400 {
		t.Errorf("an unknown cult: %d", code)
	}
	if code, _ := change("Alice", 9, "bg"); code == 200 {
		t.Error("a seat that does not exist")
	}

	// The player at the table sits in the seat of the new cult, with its colour.
	joinRoom(t, baseURL, room, "Bob")
	var g seats
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &g)
	if g.Seats[2].Name != "Bob" || g.Seats[2].Label != "Yellow Sign" {
		t.Errorf("Bob sits at %+v", g.Seats[2])
	}
	// Seated, Bob may no longer change his own seat's cult.
	if code, _ := change("Alice", 3, "bg"); code != 409 {
		t.Errorf("a seat Bob holds changed: %d", code)
	}
	// The last seat takes the changed cult; the table starts with the new cults.
	joinRoom(t, baseURL, room, "Cleo")
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/start", map[string]any{"player_name": "Alice"}, nil); code != 200 {
		t.Fatalf("start: %d", code)
	}
	var st struct {
		GameDefID string `json:"game_def_id"`
		Status    string `json:"status"`
		TurnSeat  int    `json:"turn_seat"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &st)
	if st.Status != "active" || st.GameDefID != "cthulhu-wars~gc-cc-ys-sl" || st.TurnSeat != 1 {
		t.Fatalf("started: %+v", st)
	}
	// Once the game has started the cults are fixed.
	if code, _ := change("Alice", 3, "bg"); code != 409 {
		t.Errorf("a started game changed a cult: %d", code)
	}
	// Great Cthulhu (Alice) chooses the direction, as the first player.
	if code, out := moveNotation(t, baseURL, room, "Alice", "cw"); code != 200 {
		t.Fatalf("direction: %d %v", code, out)
	}
}

// A game whose seats are not cults has nothing to choose.
func TestTheCultOfASeatCannotBeChangedInAGameWithoutCults(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice", "lobby": true}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/seat-cult", map[string]any{"player_name": "Alice", "seat": 2, "cult": "gc"}, nil); code != 409 {
		t.Errorf("seat-cult in Tak: %d", code)
	}
}

// The Auto-Play Timeout: when it is a person's turn to act and they have not
// within the time they chose, a tactical bot plays the turn for them, and the
// moves say so. The wait counts from the last move in the game.
func TestAutoPlayTimeoutPlaysTheTurnOfAPersonWhoTookTooLong(t *testing.T) {
	cfg := newTestConfig(t)
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	registerFixtures(t, a)
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	base := srv.URL
	mgr := a.Manager()
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// age pretends the last move of the game was d ago.
	age := func(room string, d time.Duration) {
		t.Helper()
		if _, err := db.Exec(`UPDATE games SET updated_at = datetime('now', ?) WHERE room_code = ?`, fmt.Sprintf("-%d seconds", int(d.Seconds())), room); err != nil {
			t.Fatal(err)
		}
	}
	due := func() []string { return mgr.AutoPlayDue(context.Background()) }

	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat_bots": map[string]string{"2": "random", "3": "random", "4": "random"}}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	openingDirection(t, base, room, "Alice")

	type settings struct {
		Seat                  int    `json:"seat"`
		Enabled               bool   `json:"enabled"`
		TimeoutSeconds        int    `json:"timeout_seconds"`
		DefaultTimeoutSeconds int    `json:"default_timeout_seconds"`
		Deadline              string `json:"deadline"`
	}
	get := func(who string) (settings, int) {
		var s settings
		code := doJSON(t, "GET", base+"/api/games/"+room+"/autoplay?player_name="+who, nil, &s)
		return s, code
	}
	put := func(who string, body map[string]any) (settings, int) {
		body["player_name"] = who
		var s settings
		code := doJSON(t, "PUT", base+"/api/games/"+room+"/autoplay", body, &s)
		return s, code
	}

	// Off by default, 24 hours when on; bots have none.
	s, code := get("Alice")
	if code != 200 || s.Enabled || s.TimeoutSeconds != 86400 || s.DefaultTimeoutSeconds != 86400 || s.Deadline != "" {
		t.Fatalf("defaults: %d %+v", code, s)
	}
	if _, code := get("Crawling%20Chaos%20(Random)"); code != 403 {
		t.Errorf("a bot has an Auto-Play Timeout: %d", code)
	}
	if _, code := get("Mallory"); code != 403 {
		t.Errorf("a stranger has an Auto-Play Timeout: %d", code)
	}
	// The wait is a minute at least and a year at most.
	for _, bad := range []int{0, 59, 366 * 86400} {
		if _, code := put("Alice", map[string]any{"timeout_seconds": bad}); code != 400 {
			t.Errorf("timeout %d: %d", bad, code)
		}
	}
	// Disabled: nothing happens, however long the wait.
	age(room, 48*time.Hour)
	if got := due(); len(got) != 0 {
		t.Fatalf("the option is off: %v", got)
	}

	// Enabled for two minutes: the deadline is two minutes after the last move.
	age(room, 30*time.Second)
	s, code = put("Alice", map[string]any{"enabled": true, "timeout_seconds": 120})
	if code != 200 || !s.Enabled || s.TimeoutSeconds != 120 || s.Deadline == "" {
		t.Fatalf("enabled: %d %+v", code, s)
	}
	if dl, err := time.Parse(time.RFC3339, s.Deadline); err != nil || time.Until(dl) < 80*time.Second || time.Until(dl) > 100*time.Second {
		t.Fatalf("deadline %q (%v)", s.Deadline, err)
	}
	if got := due(); len(got) != 0 {
		t.Fatalf("30 seconds into two minutes: %v", got)
	}
	var before []struct{ Seq int }
	doJSON(t, "GET", base+"/api/games/"+room+"/moves", nil, &before)

	// Three minutes without a move: the bot plays Alice's turn.
	age(room, 3*time.Minute)
	if got := due(); len(got) != 1 || got[0] != room {
		t.Fatalf("due: %v", got)
	}
	type move struct {
		Seq      int    `json:"seq"`
		Seat     int    `json:"seat"`
		Notation string `json:"notation"`
		Auto     bool   `json:"auto"`
		AutoPlay bool   `json:"auto_play"`
	}
	var moves []move
	doJSON(t, "GET", base+"/api/games/"+room+"/moves", nil, &moves)
	played := moves[len(before):]
	if len(played) == 0 || played[0].Seat != 1 || !played[0].AutoPlay {
		t.Fatalf("the bot's moves: %+v", played)
	}
	// The whole turn is the bot's, in Alice's seat and tagged; whoever plays
	// after her is not.
	i := 0
	for i < len(played) && played[i].Seat == 1 {
		if !played[i].AutoPlay {
			t.Errorf("move %d (%s) is Alice's, not tagged: %+v", played[i].Seq, played[i].Notation, played[i])
		}
		i++
	}
	if i == len(played) {
		t.Fatalf("play never passed on: %+v", played)
	}
	for _, m := range played[i:] {
		if m.Seat == 1 && !m.AutoPlay {
			continue // Alice's next turn is hers
		}
		if m.AutoPlay {
			t.Errorf("a bot's own move is marked as auto-play: %+v", m)
		}
	}
	// The count started afresh after the bot's moves: nobody is due.
	if got := due(); len(got) != 0 {
		t.Fatalf("right after the bot played: %v", got)
	}

	// A move by the person starts it afresh too.
	var st struct {
		TurnSeat int `json:"turn_seat"`
	}
	doJSON(t, "GET", base+"/api/games/"+room, nil, &st)
	if st.TurnSeat != 1 {
		t.Skipf("the game moved on to seat %d", st.TurnSeat)
	}
	age(room, 3*time.Minute)
	if code, out := moveNotation(t, base, room, "Alice", "pass"); code != 200 {
		t.Fatalf("pass: %d %v", code, out)
	}
	if got := due(); len(got) != 0 {
		t.Fatalf("after Alice's own move: %v", got)
	}
	var after []move
	doJSON(t, "GET", base+"/api/games/"+room+"/moves", nil, &after)
	for _, m := range after[len(moves):] {
		if m.Seat == 1 && m.AutoPlay {
			t.Errorf("Alice's own pass is tagged auto-play: %+v", m)
		}
	}
	// Off again.
	if s, code = put("Alice", map[string]any{"enabled": false}); code != 200 || s.Enabled || s.TimeoutSeconds != 120 {
		t.Fatalf("disabled: %d %+v", code, s)
	}
}

// A bot that is the first player chooses the direction by itself, and the
// table plays on until it needs a person.
func TestABotThatIsTheFirstPlayerChoosesTheDirection(t *testing.T) {
	baseURL := newRESTTestServer(t)
	for _, kind := range []string{"random", "tactical", "minimax"} {
		var created struct {
			RoomCode string `json:"room_code"`
		}
		body := map[string]any{
			"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat": 2,
			"seat_bots": map[string]string{"1": kind, "3": kind, "4": kind},
		}
		if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
			t.Fatalf("%s: create: %d", kind, code)
		}
		var moves []struct {
			Seat     int    `json:"seat"`
			Notation string `json:"notation"`
		}
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			var st struct {
				TurnSeat int `json:"turn_seat"`
			}
			doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &st)
			doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/moves", nil, &moves)
			if st.TurnSeat == 2 && len(moves) > 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if len(moves) == 0 || moves[0].Seat != 1 || (moves[0].Notation != "cw" && moves[0].Notation != "ccw") {
			t.Fatalf("%s: the first move is %+v, want Great Cthulhu's choice of direction", kind, moves)
		}
	}
}

// The wait does not count the time the server was down: when the watcher
// comes up, a game last touched long ago starts its count from that moment.
func TestAutoPlayDoesNotCountTheTimeTheServerWasDown(t *testing.T) {
	cfg := newTestConfig(t)
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	registerFixtures(t, a)
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	base := srv.URL
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat_bots": map[string]string{"2": "random", "3": "random", "4": "random"}}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	openingDirection(t, base, room, "Alice")
	if code := doJSON(t, "PUT", base+"/api/games/"+room+"/autoplay", map[string]any{"player_name": "Alice", "enabled": true, "timeout_seconds": 120}, nil); code != 200 {
		t.Fatalf("enable: %d", code)
	}
	// The game was last touched two days ago, while the server was down.
	if _, err := db.Exec(`UPDATE games SET updated_at = datetime('now', '-172800 seconds') WHERE room_code = ?`, room); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Manager().RunAutoPlay(ctx, 30*time.Millisecond)
	time.Sleep(400 * time.Millisecond)
	var moves []struct{ Seq int }
	doJSON(t, "GET", base+"/api/games/"+room+"/moves", nil, &moves)
	if len(moves) != 1 {
		t.Fatalf("a bot played for Alice at once, though the server was only just up: %d moves", len(moves))
	}
	var s struct {
		Deadline string `json:"deadline"`
	}
	doJSON(t, "GET", base+"/api/games/"+room+"/autoplay?player_name=Alice", nil, &s)
	if dl, err := time.Parse(time.RFC3339, s.Deadline); err != nil || time.Until(dl) < 60*time.Second || time.Until(dl) > 125*time.Second {
		t.Fatalf("the deadline counts from the moment the server came up: %q (%v)", s.Deadline, err)
	}
}
