package app_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/tests/testdata/council"
)

// newServerWith builds a server whose configuration is changed by mod.
func newServerWith(t *testing.T, mod func(*config.Config)) string {
	t.Helper()
	cfg := newTestConfig(t)
	mod(&cfg)
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	registerFixtures(t, a)
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return srv.URL
}

type lobbyGame struct {
	Status      string `json:"status"`
	Lobby       bool   `json:"lobby"`
	HostSeat    int    `json:"host_seat"`
	InviteURL   string `json:"invite_url"`
	ImpulseSeat *int   `json:"impulse_seat"`
	Seats       []struct {
		Seat int    `json:"seat"`
		Name string `json:"name"`
		Bot  string `json:"bot"`
	} `json:"seats"`
}

func getLobbyGame(t *testing.T, base, room string) lobbyGame {
	t.Helper()
	var g lobbyGame
	if code := doJSON(t, "GET", base+"/api/games/"+room, nil, &g); code != 200 {
		t.Fatalf("GET game: %d", code)
	}
	return g
}

func seatName(g lobbyGame, seat int) string {
	for _, s := range g.Seats {
		if s.Seat == seat {
			return s.Name
		}
	}
	return ""
}

func TestInviteLinksUseThePublicURLWhenOneIsConfigured(t *testing.T) {
	base := newServerWith(t, func(c *config.Config) { c.PublicURL = "https://games.example.com" })
	var created createGameResponse
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Alice"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if g := getLobbyGame(t, base, created.RoomCode); g.InviteURL != "https://games.example.com/game/"+created.RoomCode {
		t.Fatalf("invite url %q", g.InviteURL)
	}
	var cfg struct {
		PublicURL string `json:"public_url"`
	}
	if code := doJSON(t, "GET", base+"/api/config", nil, &cfg); code != 200 || cfg.PublicURL != "https://games.example.com" {
		t.Fatalf("config %d %+v", code, cfg)
	}

	// Without one the address the request came to is used.
	plain := newRESTTestServer(t)
	if code := doJSON(t, "POST", plain+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Alice"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if g := getLobbyGame(t, plain, created.RoomCode); g.InviteURL != plain+"/game/"+created.RoomCode {
		t.Fatalf("invite url %q, want the server's own address", g.InviteURL)
	}
}

func TestALobbyWaitsForItsHostToStartTheGame(t *testing.T) {
	base := newRESTTestServer(t)
	var created createGameResponse
	body := map[string]any{"game_def_id": "test-council", "player_name": "Alice", "lobby": true, "seat_bots": map[string]string{"2": "random"}}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 || created.Status != "waiting" {
		t.Fatalf("create: %d %+v", code, created)
	}
	room := created.RoomCode
	g := getLobbyGame(t, base, room)
	if !g.Lobby || g.HostSeat != 1 || seatName(g, 2) != "Stag (Random)" {
		t.Fatalf("game %+v", g)
	}

	// Bob picks the Ram.
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/join", map[string]any{"player_name": "Bob", "seat": 4}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	if g = getLobbyGame(t, base, room); seatName(g, 4) != "Bob" || seatName(g, 3) != "" {
		t.Fatalf("seats %+v", g.Seats)
	}
	// Seat 4 is taken now.
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/join", map[string]any{"player_name": "Carol", "seat": 4}, nil); code != 409 {
		t.Fatalf("a taken seat: %d", code)
	}

	post := func(path, name string, extra map[string]any) int {
		b := map[string]any{"player_name": name}
		for k, v := range extra {
			b[k] = v
		}
		return doJSON(t, "POST", base+"/api/games/"+room+path, b, nil)
	}
	// Starting needs the host and a full table.
	if code := post("/start", "Alice", nil); code != 409 {
		t.Fatalf("start with an open seat: %d", code)
	}
	if code := post("/bots", "Alice", map[string]any{"bot": "tactical", "seat": 3}); code != 200 {
		t.Fatalf("add bot: %d", code)
	}
	if g = getLobbyGame(t, base, room); g.Status != "waiting" || seatName(g, 3) != "Wolf (Tactical)" {
		t.Fatalf("a lobby stays open with every seat taken: %+v", g)
	}
	if code := post("/start", "Bob", nil); code != 403 {
		t.Fatalf("only the host may start: %d", code)
	}
	if code := post("/start", "Stranger", nil); code != 403 {
		t.Fatalf("a stranger may not start: %d", code)
	}
	if code := post("/start", "Alice", nil); code != 200 {
		t.Fatalf("start: %d", code)
	}
	if g = getLobbyGame(t, base, room); g.Status != "active" || g.ImpulseSeat == nil || *g.ImpulseSeat != 1 {
		t.Fatalf("started: %+v", g)
	}
	if code := post("/start", "Alice", nil); code != 409 {
		t.Fatalf("starting twice: %d", code)
	}
}

func TestPlayersChooseSeatsLeaveAndTheHostRemoves(t *testing.T) {
	base := newRESTTestServer(t)
	var created createGameResponse
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Alice", "lobby": true}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	post := func(path, name string, seat int) int {
		return doJSON(t, "POST", base+"/api/games/"+room+path, map[string]any{"player_name": name, "seat": seat}, nil)
	}
	if code := post("/join", "Bob", 2); code != 200 {
		t.Fatalf("join: %d", code)
	}
	// Bob prefers the Wolf; Alice cannot take it from under him.
	if code := post("/seat", "Bob", 3); code != 200 {
		t.Fatalf("move: %d", code)
	}
	if g := getLobbyGame(t, base, room); seatName(g, 3) != "Bob" || seatName(g, 2) != "" {
		t.Fatalf("seats %+v", g.Seats)
	}
	if code := post("/seat", "Alice", 3); code != 409 {
		t.Fatalf("a taken seat: %d", code)
	}
	if code := post("/seat", "Bob", 9); code != 400 {
		t.Fatalf("no such seat: %d", code)
	}
	// Only the host removes another player.
	if code := post("/remove", "Bob", 1); code != 403 {
		t.Fatalf("not the host: %d", code)
	}
	if code := post("/remove", "Alice", 3); code != 200 {
		t.Fatalf("remove: %d", code)
	}
	if g := getLobbyGame(t, base, room); seatName(g, 3) != "" {
		t.Fatalf("seats %+v", g.Seats)
	}
	// Leaving hands the host's role on.
	post("/join", "Carol", 2)
	if code := post("/leave", "Alice", 0); code != 200 {
		t.Fatalf("leave: %d", code)
	}
	if g := getLobbyGame(t, base, room); g.HostSeat != 2 || seatName(g, 1) != "" {
		t.Fatalf("host seat %d, seats %+v", g.HostSeat, g.Seats)
	}
}

func TestChatIsSharedByPlayersAndSpectatorsAndIsLimited(t *testing.T) {
	base := newRESTTestServer(t)
	var created createGameResponse
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Alice", "lobby": true}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	conn := dialSpectator(t, base, room, "Watcher")

	say := func(name, text string) (int, map[string]any) {
		var out map[string]any
		code := doJSON(t, "POST", base+"/api/games/"+room+"/chat", map[string]any{"player_name": name, "text": text}, &out)
		return code, out
	}
	if code, out := say("Alice", "  hello table  "); code != 200 || out["text"] != "hello table" || out["seat"].(float64) != 1 || out["color"] != "red" {
		t.Fatalf("player says: %d %v", code, out)
	}
	if code, out := say("Watcher", "nice table"); code != 200 || out["seat"].(float64) != 0 {
		t.Fatalf("spectator says: %d %v", code, out)
	}
	// Everyone listening hears it.
	ev := readFirstEvent(t, conn)
	if ev.Type != "chat" {
		t.Fatalf("event %q: %s", ev.Type, ev.Data)
	}
	var first struct {
		Name string `json:"name"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(ev.Data, &first); err != nil || first.Name != "Alice" || first.Text != "hello table" {
		t.Fatalf("event data %s (%v)", ev.Data, err)
	}
	if code, _ := say("Alice", "   "); code != 400 {
		t.Fatalf("an empty message: %d", code)
	}
	if code, _ := say("Alice", strings.Repeat("x", 501)); code != 400 {
		t.Fatalf("a long message: %d", code)
	}
	var log []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Seat int    `json:"seat"`
	}
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/chat", nil, &log); code != 200 || len(log) != 2 || log[0].Name != "Alice" || log[1].Seat != 0 {
		t.Fatalf("log %d %+v", code, log)
	}
	var later []struct {
		Name string `json:"name"`
	}
	if code := doJSON(t, "GET", fmt.Sprintf("%s/api/games/%s/chat?after=%d", base, room, log[0].ID), nil, &later); code != 200 || len(later) != 1 || later[0].Name != "Watcher" {
		t.Fatalf("after: %d %+v", code, later)
	}
	// A flood is refused.
	refused := 0
	for i := 0; i < 20; i++ {
		if code, _ := say("Watcher", fmt.Sprintf("spam %d", i)); code == http.StatusTooManyRequests {
			refused++
		}
	}
	if refused == 0 {
		t.Fatal("a flood of messages must be refused")
	}
}

// councilDoc is a Council position document (tests/testdata/council) with d as
// its data.
func councilDoc(t *testing.T, d *council.Data) string {
	t.Helper()
	if d.Claims == nil {
		d.Claims = map[int][]string{}
	}
	st := state.NewBoardState()
	st.SetExt(d)
	raw, err := st.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// councilReactionTable is the table the reaction tests play at: the Crow (Alice, seat 1, who may
// Object), two bots, and the Ram (Bob, seat 4), with the Court in play, so the
// others list Object before they hold it. Bob moves first, and Alice next.
const councilReactionTable = "test-council~crow-wolf-stag-ram-court"

func reactionStart(t *testing.T) string {
	return councilDoc(t, &council.Data{Turn: 4, Envoys: map[int]string{1: "north", 2: "south", 3: "south", 4: "west"}})
}

// A person's reactions are passed for them unless they ask to be asked: when
// they are targeted (on by default), after any other player's action, or
// after the actions of chosen players.
func TestReactionsArePassedUnlessThePlayerAsksToBeAsked(t *testing.T) {
	base := newRESTTestServer(t)
	newGame := func() (room string) {
		var created createGameResponse
		body := map[string]any{
			"game_def_id": councilReactionTable, "player_name": "Alice", "seat_bots": map[string]string{"2": "random", "3": "random"},
			"start_state": json.RawMessage(reactionStart(t)), "start_seat": 4,
		}
		if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
			t.Fatalf("create: %d", code)
		}
		joinRoom(t, base, created.RoomCode, "Bob")
		return created.RoomCode
	}
	type settings struct {
		Seat      int `json:"seat"`
		Abilities []struct {
			Name string `json:"name"`
			Held bool   `json:"held"`
		} `json:"abilities"`
		Targeted bool                 `json:"targeted"`
		AskAll   bool                 `json:"ask_all"`
		AskSeats []int                `json:"ask_seats"`
		Others   []struct{ Seat int } `json:"others"`
		Pending  *struct {
			Ability  string `json:"ability"`
			Trigger  int    `json:"trigger"`
			Targeted bool   `json:"targeted"`
		} `json:"pending"`
	}
	get := func(room, who string) (settings, int) {
		var cur settings
		code := doJSON(t, "GET", base+"/api/games/"+room+"/reactions?player_name="+who, nil, &cur)
		return cur, code
	}
	asked := func(room string) bool {
		cur, code := get(room, "Alice")
		if code != 200 {
			t.Fatalf("reactions: %d", code)
		}
		return cur.Pending != nil
	}
	put := func(room string, body map[string]any) settings {
		body["player_name"] = "Alice"
		var set settings
		if code := doJSON(t, "PUT", base+"/api/games/"+room+"/reactions", body, &set); code != 200 {
			t.Fatalf("set %v: %d", body, code)
		}
		return set
	}

	// By default she is asked only when targeted: nothing is aimed at her
	// when Bob walks from the West into the Hall, so the reaction is passed for her.
	room := newGame()
	s, code := get(room, "Alice")
	if code != 200 || len(s.Abilities) != 1 || s.Abilities[0].Name != "Object" || !s.Abilities[0].Held || len(s.Others) != 3 {
		t.Fatalf("settings %d %+v", code, s)
	}
	if !s.Targeted || s.AskAll || len(s.AskSeats) != 0 {
		t.Fatalf("the defaults: targeted, and nothing else is asked: %+v", s)
	}
	if code, out := moveNotation(t, base, room, "Bob", "go@hall"); code != 200 {
		t.Fatalf("move: %d %v", code, out)
	}
	if asked(room) {
		t.Fatal("not targeted: the reaction should have been passed for Alice")
	}
	var moves []struct {
		Seat     int    `json:"seat"`
		Notation string `json:"notation"`
		Auto     bool   `json:"auto"`
	}
	doJSON(t, "GET", base+"/api/games/"+room+"/moves", nil, &moves)
	found := false
	for _, m := range moves {
		if m.Seat == 1 && m.Notation == "decline" && m.Auto {
			found = true
		}
	}
	if !found {
		t.Fatalf("the auto-passed move is recorded as such: %+v", moves)
	}
	// Bob's move handed the turn on, and what the server passed for Alice does
	// not change that.
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/undo", map[string]any{"player_name": "Bob"}, nil); code != 409 {
		t.Fatalf("undo after an automatic pass: %d, want 409", code)
	}

	// Targeted: Bob walks into the North, where Alice's envoy stands.
	room = newGame()
	if code, out := moveNotation(t, base, room, "Bob", "go@north"); code != 200 {
		t.Fatalf("move: %d %v", code, out)
	}
	cur, _ := get(room, "Alice")
	if cur.Pending == nil || cur.Pending.Ability != "Object" || cur.Pending.Trigger != 4 || !cur.Pending.Targeted {
		t.Fatalf("Alice is asked, as she is targeted: %+v", cur.Pending)
	}
	if code, _ := moveNotation(t, base, room, "Alice", "decline"); code != 200 {
		t.Fatalf("pass: %d", code)
	}
	// ... unless she asked not to be.
	room = newGame()
	if set := put(room, map[string]any{"targeted": false}); set.Targeted {
		t.Fatalf("set: %+v", set)
	}
	moveNotation(t, base, room, "Bob", "go@north")
	if asked(room) {
		t.Fatal("targeted asking is off: Alice is not asked")
	}

	// Asked after a chosen player's actions: Bob's.
	room = newGame()
	if set := put(room, map[string]any{"ask_seats": []int{4}}); len(set.AskSeats) != 1 || set.AskSeats[0] != 4 || !set.Targeted {
		t.Fatalf("set: %+v", set)
	}
	moveNotation(t, base, room, "Bob", "go@hall")
	cur, _ = get(room, "Alice")
	if cur.Pending == nil || cur.Pending.Trigger != 4 || cur.Pending.Targeted {
		t.Fatalf("asked after Bob, though not targeted: %+v", cur.Pending)
	}
	// Another player's actions do not ask: Alice names only the Stag.
	room = newGame()
	put(room, map[string]any{"ask_seats": []int{3}})
	moveNotation(t, base, room, "Bob", "go@hall")
	if asked(room) {
		t.Fatal("the Ram was not chosen: Alice is not asked")
	}
	// Everyone.
	room = newGame()
	if set := put(room, map[string]any{"ask_all": true}); !set.AskAll {
		t.Fatalf("set: %+v", set)
	}
	moveNotation(t, base, room, "Bob", "go@hall")
	if !asked(room) {
		t.Fatal("everyone was chosen: Alice is asked")
	}
	// Choosing while a question waits: the window that is open is passed at once.
	if set := put(room, map[string]any{"ask_all": false}); set.AskAll || set.Pending != nil {
		t.Fatalf("the open window should have been passed: %+v", set)
	}

	// At the Court, Bob's house may Object once it holds a claim: it is listed
	// as not held yet, so he can choose when to be asked before he has it.
	bobs, code := get(room, "Bob")
	if code != 200 || len(bobs.Abilities) != 1 || bobs.Abilities[0].Name != "Object" || bobs.Abilities[0].Held {
		t.Fatalf("bob's settings %d %+v", code, bobs)
	}
	if _, code := get(room, "Nobody"); code != 403 {
		t.Fatalf("a stranger has no settings: %d", code)
	}
	// Bots are never asked about settings: they are asked about every reaction.
	if _, code := get(room, "Wolf%20(Random)"); code != 403 {
		t.Fatalf("a bot has no settings: %d", code)
	}
}

// A choice saved before the reversal (what to pass) keeps its meaning.
func TestReactionChoicesSavedByTheEarlierWayStillMean(t *testing.T) {
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

	newGame := func() string {
		var created createGameResponse
		body := map[string]any{
			"game_def_id": councilReactionTable, "player_name": "Alice", "seat_bots": map[string]string{"2": "random", "3": "random"},
			"start_state": json.RawMessage(reactionStart(t)), "start_seat": 4,
		}
		if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
			t.Fatalf("create: %d", code)
		}
		joinRoom(t, base, created.RoomCode, "Bob")
		return created.RoomCode
	}
	legacy := func(room, raw string) {
		db, err := sql.Open("sqlite", cfg.DBPath)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if _, err := db.Exec(`INSERT INTO reaction_prefs (game_id, seat, prefs) SELECT id, 1, ? FROM games WHERE room_code = ?`, raw, room); err != nil {
			t.Fatal(err)
		}
	}
	type settings struct {
		Targeted bool  `json:"targeted"`
		AskAll   bool  `json:"ask_all"`
		AskSeats []int `json:"ask_seats"`
		Pending  any   `json:"pending"`
	}
	get := func(room string) settings {
		var s settings
		doJSON(t, "GET", base+"/api/games/"+room+"/reactions?player_name=Alice", nil, &s)
		return s
	}

	// Pass for everyone: never asked.
	room := newGame()
	legacy(room, `{"all":true,"seats":[]}`)
	if s := get(room); s.Targeted || s.AskAll || len(s.AskSeats) != 0 {
		t.Fatalf("passing everything is never being asked: %+v", s)
	}
	moveNotation(t, base, room, "Bob", "go@hall")
	if s := get(room); s.Pending != nil {
		t.Fatalf("passed for her: %+v", s)
	}

	// Pass for the Stag only: asked after the others (the Wolf and the Ram),
	// and when targeted.
	room = newGame()
	legacy(room, `{"all":false,"seats":[3]}`)
	if s := get(room); !s.Targeted || s.AskAll || len(s.AskSeats) != 2 || s.AskSeats[0] != 2 || s.AskSeats[1] != 4 {
		t.Fatalf("asked after the others: %+v", s)
	}
	moveNotation(t, base, room, "Bob", "go@hall")
	if s := get(room); s.Pending == nil {
		t.Fatalf("asked after Bob: %+v", s)
	}

	// Nothing passed (the old default): asked after everyone.
	room = newGame()
	legacy(room, `{"all":false,"seats":[]}`)
	if s := get(room); !s.AskAll {
		t.Fatalf("asked after everyone: %+v", s)
	}

	// A client that still sends the earlier choice is understood.
	room = newGame()
	put := func(body map[string]any) settings {
		body["player_name"] = "Alice"
		var s settings
		if code := doJSON(t, "PUT", base+"/api/games/"+room+"/reactions", body, &s); code != 200 {
			t.Fatalf("put %v: %d", body, code)
		}
		return s
	}
	if s := put(map[string]any{"all": true}); s.Targeted || s.AskAll || len(s.AskSeats) != 0 {
		t.Fatalf("all: %+v", s)
	}
	if s := put(map[string]any{"seats": []int{3}}); !s.Targeted || s.AskAll || len(s.AskSeats) != 2 {
		t.Fatalf("seats: %+v", s)
	}
	// The new fields win when both are sent.
	if s := put(map[string]any{"all": true, "ask_all": true}); !s.AskAll {
		t.Fatalf("new and old: %+v", s)
	}
}

// A seat's reactions are listed from the first move, flagged as not held:
// a player looks for a reaction's settings before they can use it.
func TestReactionsAreListedBeforeTheyAreHeld(t *testing.T) {
	base := newRESTTestServer(t)
	type settings struct {
		Seat      int `json:"seat"`
		Abilities []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Held bool   `json:"held"`
		} `json:"abilities"`
		Targeted bool  `json:"targeted"`
		AskAll   bool  `json:"ask_all"`
		AskSeats []int `json:"ask_seats"`
		Others   []struct {
			Seat int `json:"seat"`
		} `json:"others"`
	}
	create := func(id string, seat int, bots map[string]string) string {
		var created createGameResponse
		body := map[string]any{"game_def_id": id, "player_name": "Alice", "seat": seat, "seat_bots": bots}
		if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
			t.Fatalf("create: %d", code)
		}
		return created.RoomCode
	}

	// The Stag at the Court, at the very start: no claim, Object listed, not held.
	room := create("test-council~stag-crow-wolf-ram-court", 1, map[string]string{"2": "random", "3": "random", "4": "random"})
	var s settings
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/reactions?player_name=Alice", nil, &s); code != 200 {
		t.Fatalf("settings: %d", code)
	}
	if len(s.Abilities) != 1 || s.Abilities[0].ID != "object" || s.Abilities[0].Held {
		t.Fatalf("the Stag's reactions at the start = %+v", s.Abilities)
	}
	if len(s.Others) != 3 || !s.Targeted {
		t.Fatalf("others = %+v targeted = %v", s.Others, s.Targeted)
	}
	// The choice can be made already and is kept.
	var set settings
	if code := doJSON(t, "PUT", base+"/api/games/"+room+"/reactions", map[string]any{"player_name": "Alice", "ask_all": true}, &set); code != 200 || !set.AskAll || len(set.Abilities) != 1 {
		t.Fatalf("set before holding it: %d %+v", code, set)
	}
	doJSON(t, "GET", base+"/api/games/"+room+"/reactions?player_name=Alice", nil, &s)
	if !s.AskAll {
		t.Fatalf("the choice was lost: %+v", s)
	}

	// Without the Court the Stag has no reaction at all: nothing to set.
	room = create("test-council", 2, map[string]string{"1": "random", "3": "random", "4": "random"})
	var stag settings
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/reactions?player_name=Alice", nil, &stag); code != 200 || len(stag.Abilities) != 0 {
		t.Fatalf("the Stag's reactions %d %+v", code, stag.Abilities)
	}
}

// Each reaction has its own choice (the Owl's Heckle and Object can differ),
// and Heckle asks on every occasion until the player says otherwise.
func TestEveryReactionHasItsOwnChoiceAndOneAsksEveryTime(t *testing.T) {
	base := newRESTTestServer(t)
	type choice struct {
		Targeted bool  `json:"targeted"`
		AskAll   bool  `json:"ask_all"`
		AskSeats []int `json:"ask_seats"`
	}
	type settings struct {
		Abilities []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Held      bool   `json:"held"`
			AskAlways bool   `json:"ask_always"`
		} `json:"abilities"`
		Targeted bool              `json:"targeted"`
		AskAll   bool              `json:"ask_all"`
		AskSeats []int             `json:"ask_seats"`
		Choices  map[string]choice `json:"choices"`
		Pending  *struct {
			Ability string `json:"ability"`
		} `json:"pending"`
	}
	newGame := func() string { return heckleGame(t, base) }
	get := func(room string) settings {
		var cur settings
		if code := doJSON(t, "GET", base+"/api/games/"+room+"/reactions?player_name=Alice", nil, &cur); code != 200 {
			t.Fatalf("reactions: %d", code)
		}
		return cur
	}
	put := func(room string, body map[string]any) (settings, int) {
		body["player_name"] = "Alice"
		var set settings
		code := doJSON(t, "PUT", base+"/api/games/"+room+"/reactions", body, &set)
		return set, code
	}

	room := newGame()
	s := get(room)
	if len(s.Abilities) != 2 || s.Abilities[0].ID != "heckle" || !s.Abilities[0].AskAlways || s.Abilities[1].ID != "object" || s.Abilities[1].AskAlways {
		t.Fatalf("the Owl's reactions: %+v", s.Abilities)
	}
	// Heckle: every box ticked, so Alice is asked whoever acted.
	m := s.Choices["heckle"]
	if !m.Targeted || !m.AskAll || len(m.AskSeats) != 0 {
		t.Fatalf("Heckle by default: %+v", m)
	}
	// Object: only when targeted, as before.
	o := s.Choices["object"]
	if !o.Targeted || o.AskAll || len(o.AskSeats) != 0 {
		t.Fatalf("Object by default: %+v", o)
	}
	if s.Pending == nil || s.Pending.Ability != "Heckle" {
		t.Fatalf("Alice is asked about Heckle after a turn not aimed at her: %+v", s.Pending)
	}

	// Choosing for Object leaves Heckle alone, and the other way round.
	set, code := put(room, map[string]any{"ability": "object", "ask_all": true})
	if code != 200 || !set.Choices["object"].AskAll || !set.Choices["heckle"].AskAll {
		t.Fatalf("Object asks about everything: %d %+v", code, set.Choices)
	}
	if set.Pending == nil {
		t.Fatalf("the question about Heckle is still open: %+v", set.Pending)
	}
	// Heckle: never ask — the open window is passed for her at once.
	set, code = put(room, map[string]any{"ability": "heckle", "ask_all": false, "targeted": false, "ask_seats": []int{}})
	if code != 200 {
		t.Fatalf("set Heckle: %d", code)
	}
	if c := set.Choices["heckle"]; c.Targeted || c.AskAll || len(c.AskSeats) != 0 {
		t.Fatalf("Heckle is never asked: %+v", c)
	}
	if c := set.Choices["object"]; !c.AskAll || !c.Targeted {
		t.Fatalf("Object kept its choice: %+v", c)
	}
	if set.Pending != nil {
		t.Fatalf("Heckle is passed for her now: %+v", set.Pending)
	}
	// What was chosen is kept.
	if s = get(room); s.Choices["heckle"].AskAll || !s.Choices["object"].AskAll {
		t.Fatalf("saved: %+v", s.Choices)
	}

	// A choice for every reaction at once.
	room = newGame()
	set, code = put(room, map[string]any{"targeted": false, "ask_all": false})
	if code != 200 || set.Choices["heckle"].AskAll || set.Choices["heckle"].Targeted || set.Choices["object"].Targeted {
		t.Fatalf("both reactions: %d %+v", code, set.Choices)
	}
	// Only a reaction of the seat's own can be chosen for.
	if _, code := put(room, map[string]any{"ability": "summon", "ask_all": true}); code != 400 {
		t.Fatalf("summon is not the Owl's reaction: %d", code)
	}
}

// heckleGame creates a game in which Alice plays the Owl (Heckle and Object)
// in seat 2, and the Crow has just moved into the West, where she is not: a
// question about Heckle waits for her.
func heckleGame(t *testing.T, base string) string {
	t.Helper()
	doc := councilDoc(t, &council.Data{
		Turn:   2,
		Envoys: map[int]string{1: "west", 2: "east", 3: "south", 4: "south"},
		Window: &council.Window{Trigger: 1, Area: "west", Waiting: []council.Ask{{Seat: 2, Ability: "heckle"}}},
	})
	var created createGameResponse
	body := map[string]any{
		"game_def_id": "test-council~crow-owl-wolf-ram", "player_name": "Alice", "seat": 2,
		"seat_bots":   map[string]string{"1": "random", "3": "random", "4": "random"},
		"start_state": json.RawMessage(doc), "start_seat": 2,
	}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	return created.RoomCode
}
