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
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
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
	Status    string `json:"status"`
	Lobby     bool   `json:"lobby"`
	HostSeat  int    `json:"host_seat"`
	InviteURL string `json:"invite_url"`
	TurnSeat  *int   `json:"turn_seat"`
	Seats     []struct {
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
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice"}, &created); code != 200 {
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
	if code := doJSON(t, "POST", plain+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if g := getLobbyGame(t, plain, created.RoomCode); g.InviteURL != plain+"/game/"+created.RoomCode {
		t.Fatalf("invite url %q, want the server's own address", g.InviteURL)
	}
}

func TestALobbyWaitsForItsHostToStartTheGame(t *testing.T) {
	base := newRESTTestServer(t)
	var created createGameResponse
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "lobby": true, "seat_bots": map[string]string{"2": "random"}}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 || created.Status != "waiting" {
		t.Fatalf("create: %d %+v", code, created)
	}
	room := created.RoomCode
	g := getLobbyGame(t, base, room)
	if !g.Lobby || g.HostSeat != 1 || seatName(g, 2) != "Crawling Chaos (Random)" {
		t.Fatalf("game %+v", g)
	}

	// Bob picks the Yellow Sign.
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
	if g = getLobbyGame(t, base, room); g.Status != "waiting" || seatName(g, 3) != "Black Goat (Tactical)" {
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
	if g = getLobbyGame(t, base, room); g.Status != "active" || g.TurnSeat == nil || *g.TurnSeat != 1 {
		t.Fatalf("started: %+v", g)
	}
	if code := post("/start", "Alice", nil); code != 409 {
		t.Fatalf("starting twice: %d", code)
	}
}

func TestPlayersChooseSeatsLeaveAndTheHostRemoves(t *testing.T) {
	base := newRESTTestServer(t)
	var created createGameResponse
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "lobby": true}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	post := func(path, name string, seat int) int {
		return doJSON(t, "POST", base+"/api/games/"+room+path, map[string]any{"player_name": name, "seat": seat}, nil)
	}
	if code := post("/join", "Bob", 2); code != 200 {
		t.Fatalf("join: %d", code)
	}
	// Bob prefers the Black Goat; Alice cannot take it from under him.
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
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "lobby": true}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	conn := dialSpectator(t, base, room, "Watcher")

	say := func(name, text string) (int, map[string]any) {
		var out map[string]any
		code := doJSON(t, "POST", base+"/api/games/"+room+"/chat", map[string]any{"player_name": name, "text": text}, &out)
		return code, out
	}
	if code, out := say("Alice", "  hello table  "); code != 200 || out["text"] != "hello table" || out["seat"].(float64) != 1 || out["color"] != "green" {
		t.Fatalf("player says: %d %v", code, out)
	}
	if code, out := say("Watcher", "nice map"); code != 200 || out["seat"].(float64) != 0 {
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

// A person's reactions are passed for them unless they ask to be asked: when
// they are targeted (on by default), after any other player's action, or
// after the actions of chosen players.
func TestReactionsArePassedUnlessThePlayerAsksToBeAsked(t *testing.T) {
	base := newRESTTestServer(t)
	// Great Cthulhu (Alice) holds Devolve; Crawling Chaos (Bob) acts first,
	// with Cultists in Asia (where Alice has nobody) and in the South Pacific
	// (where she has her Gate).
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}, {2, "cultist", "south-pacific", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 3, 3, 0, 0}, [5]uint8{0, 1 << 1, 0, 0, 0},
		func(d *cthulhuwars.Data) { d.Turn.Seat = 2 })
	newGame := func() (room string) {
		var created createGameResponse
		body := map[string]any{
			"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat_bots": map[string]string{"3": "random", "4": "random"},
			"start_state": json.RawMessage(doc), "start_seat": 2,
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
	// asked says whether Alice's answer to a reaction is awaited.
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
	// when Bob recruits in Asia, so the reaction is passed for her.
	room := newGame()
	s, code := get(room, "Alice")
	if code != 200 || len(s.Abilities) != 1 || s.Abilities[0].Name != "Devolve" || !s.Abilities[0].Held || len(s.Others) != 3 {
		t.Fatalf("settings %d %+v", code, s)
	}
	if !s.Targeted || s.AskAll || len(s.AskSeats) != 0 {
		t.Fatalf("the defaults: targeted, and nothing else is asked: %+v", s)
	}
	if code, out := moveNotation(t, base, room, "Bob", "recruit@asia"); code != 200 {
		t.Fatalf("recruit: %d %v", code, out)
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
		if m.Seat == 1 && m.Notation == "done" && m.Auto {
			found = true
		}
	}
	if !found {
		t.Fatalf("the auto-passed move is recorded as such: %+v", moves)
	}
	// What the server passed for Alice does not stop Bob from taking his move back.
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/undo", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("undo after an automatic pass: %d", code)
	}

	// Targeted: Bob recruits in the South Pacific, where Alice has her Gate.
	room = newGame()
	if code, out := moveNotation(t, base, room, "Bob", "recruit@south-pacific"); code != 200 {
		t.Fatalf("recruit: %d %v", code, out)
	}
	cur, _ := get(room, "Alice")
	if cur.Pending == nil || cur.Pending.Ability != "Devolve" || cur.Pending.Trigger != 2 || !cur.Pending.Targeted {
		t.Fatalf("Alice is asked, as she is targeted: %+v", cur.Pending)
	}
	if code, _ := moveNotation(t, base, room, "Alice", "done"); code != 200 {
		t.Fatalf("pass: %d", code)
	}
	// ... unless she asked not to be.
	room = newGame()
	if set := put(room, map[string]any{"targeted": false}); set.Targeted {
		t.Fatalf("set: %+v", set)
	}
	moveNotation(t, base, room, "Bob", "recruit@south-pacific")
	if asked(room) {
		t.Fatal("targeted asking is off: Alice is not asked")
	}

	// Asked after a chosen player's actions: Bob's.
	room = newGame()
	if set := put(room, map[string]any{"ask_seats": []int{2}}); len(set.AskSeats) != 1 || set.AskSeats[0] != 2 || !set.Targeted {
		t.Fatalf("set: %+v", set)
	}
	moveNotation(t, base, room, "Bob", "recruit@asia")
	cur, _ = get(room, "Alice")
	if cur.Pending == nil || cur.Pending.Trigger != 2 || cur.Pending.Targeted {
		t.Fatalf("asked after Bob, though not targeted: %+v", cur.Pending)
	}
	// Another player's actions do not ask: Alice names only Black Goat.
	room = newGame()
	put(room, map[string]any{"ask_seats": []int{3}})
	moveNotation(t, base, room, "Bob", "recruit@asia")
	if asked(room) {
		t.Fatal("Crawling Chaos was not chosen: Alice is not asked")
	}
	// Everyone.
	room = newGame()
	if set := put(room, map[string]any{"ask_all": true}); !set.AskAll {
		t.Fatalf("set: %+v", set)
	}
	moveNotation(t, base, room, "Bob", "recruit@asia")
	if !asked(room) {
		t.Fatal("everyone was chosen: Alice is asked")
	}
	// Choosing while a question waits: the window that is open is passed at once.
	if set := put(room, map[string]any{"ask_all": false}); set.AskAll || set.Pending != nil {
		t.Fatalf("the open window should have been passed: %+v", set)
	}

	// Bob's faction has a reaction (Madness) he has not earned yet: it is
	// listed as such, so he can choose when to be asked before he has it.
	bobs, code := get(room, "Bob")
	if code != 200 || len(bobs.Abilities) != 1 || bobs.Abilities[0].Name != "Madness" || bobs.Abilities[0].Held {
		t.Fatalf("bob's settings %d %+v", code, bobs)
	}
	if _, code := get(room, "Nobody"); code != 403 {
		t.Fatalf("a stranger has no settings: %d", code)
	}
	// Bots are never asked about settings: they are asked about every reaction.
	if _, code := get(room, "Black%20Goat%20(Random)"); code != 403 {
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
		doc := cwPosition(t,
			[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
			map[string]int{"south-pacific": 1, "asia": 2},
			[5]int{0, 3, 3, 0, 0}, [5]uint8{0, 1 << 1, 0, 0, 0},
			func(d *cthulhuwars.Data) { d.Turn.Seat = 2 })
		var created createGameResponse
		body := map[string]any{
			"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat_bots": map[string]string{"3": "random", "4": "random"},
			"start_state": json.RawMessage(doc), "start_seat": 2,
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
	moveNotation(t, base, room, "Bob", "recruit@asia")
	if s := get(room); s.Pending != nil {
		t.Fatalf("passed for her: %+v", s)
	}

	// Pass for Black Goat only: asked after the others (Crawling Chaos and
	// Yellow Sign), and when targeted.
	room = newGame()
	legacy(room, `{"all":false,"seats":[3]}`)
	if s := get(room); !s.Targeted || s.AskAll || len(s.AskSeats) != 2 || s.AskSeats[0] != 2 || s.AskSeats[1] != 4 {
		t.Fatalf("asked after the others: %+v", s)
	}
	moveNotation(t, base, room, "Bob", "recruit@asia")
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

// A cult's reactions are listed from the first move, flagged as not earned:
// the playtester looked for Devolve's settings before earning the spellbook.
func TestReactionsAreListedBeforeTheSpellbookIsEarned(t *testing.T) {
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
	create := func(seat int, bots map[string]string) string {
		var created createGameResponse
		body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat": seat, "seat_bots": bots}
		if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
			t.Fatalf("create: %d", code)
		}
		return created.RoomCode
	}

	// Great Cthulhu at the very start: no spellbook, Devolve listed, not held.
	room := create(1, map[string]string{"2": "random", "3": "random", "4": "random"})
	var s settings
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/reactions?player_name=Alice", nil, &s); code != 200 {
		t.Fatalf("settings: %d", code)
	}
	if len(s.Abilities) != 1 || s.Abilities[0].ID != "devolve" || s.Abilities[0].Held {
		t.Fatalf("Great Cthulhu's reactions at the start = %+v", s.Abilities)
	}
	if len(s.Others) != 3 || !s.Targeted {
		t.Fatalf("others = %+v targeted = %v", s.Others, s.Targeted)
	}
	// The choice can be made already and is kept.
	var set settings
	if code := doJSON(t, "PUT", base+"/api/games/"+room+"/reactions", map[string]any{"player_name": "Alice", "ask_all": true}, &set); code != 200 || !set.AskAll || len(set.Abilities) != 1 {
		t.Fatalf("set before earning it: %d %+v", code, set)
	}
	doJSON(t, "GET", base+"/api/games/"+room+"/reactions?player_name=Alice", nil, &s)
	if !s.AskAll {
		t.Fatalf("the choice was lost: %+v", s)
	}

	// Yellow Sign has no reaction at all: nothing to set.
	room = create(4, map[string]string{"1": "random", "2": "random", "3": "random"})
	var ys settings
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/reactions?player_name=Alice", nil, &ys); code != 200 || len(ys.Abilities) != 0 {
		t.Fatalf("Yellow Sign's reactions %d %+v", code, ys.Abilities)
	}
}

// Each reaction has its own choice (Madness and the Unspeakable Oath can
// differ), and Madness asks on every occasion until the player says otherwise.
func TestEveryReactionHasItsOwnChoiceAndMadnessAsksAboutEveryBattle(t *testing.T) {
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
	newGame := func() string { return madnessGame(t, base) }
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
	if len(s.Abilities) != 2 || s.Abilities[0].ID != "madness" || !s.Abilities[0].AskAlways || s.Abilities[1].ID != "unspeakable-oath" || s.Abilities[1].AskAlways {
		t.Fatalf("the Crawling Chaos' reactions with the High Priests: %+v", s.Abilities)
	}
	// Madness: every box ticked, so Alice is asked whoever took part.
	m := s.Choices["madness"]
	if !m.Targeted || !m.AskAll || len(m.AskSeats) != 0 {
		t.Fatalf("Madness by default: %+v", m)
	}
	// The Oath: only when targeted, as before.
	o := s.Choices["unspeakable-oath"]
	if !o.Targeted || o.AskAll || len(o.AskSeats) != 0 {
		t.Fatalf("the Oath by default: %+v", o)
	}
	if s.Pending == nil || s.Pending.Ability != "Madness" {
		t.Fatalf("Alice is asked about Madness in a battle she is not in: %+v", s.Pending)
	}

	// Choosing for the Oath leaves Madness alone, and the other way round.
	set, code := put(room, map[string]any{"ability": "unspeakable-oath", "ask_all": true})
	if code != 200 || !set.Choices["unspeakable-oath"].AskAll || !set.Choices["madness"].AskAll {
		t.Fatalf("the Oath asks about everything: %d %+v", code, set.Choices)
	}
	if set.Pending == nil {
		t.Fatalf("the question about Madness is still open: %+v", set.Pending)
	}
	// Madness: never ask — the open window is passed for her at once.
	set, code = put(room, map[string]any{"ability": "madness", "ask_all": false, "targeted": false, "ask_seats": []int{}})
	if code != 200 {
		t.Fatalf("set Madness: %d", code)
	}
	if c := set.Choices["madness"]; c.Targeted || c.AskAll || len(c.AskSeats) != 0 {
		t.Fatalf("Madness is never asked: %+v", c)
	}
	if c := set.Choices["unspeakable-oath"]; !c.AskAll || !c.Targeted {
		t.Fatalf("the Oath kept its choice: %+v", c)
	}
	if set.Pending != nil {
		t.Fatalf("Madness is passed for her now: %+v", set.Pending)
	}
	// What was chosen is kept.
	if s = get(room); s.Choices["madness"].AskAll || !s.Choices["unspeakable-oath"].AskAll {
		t.Fatalf("saved: %+v", s.Choices)
	}

	// A choice for every reaction at once.
	room = newGame()
	set, code = put(room, map[string]any{"targeted": false, "ask_all": false})
	if code != 200 || set.Choices["madness"].AskAll || set.Choices["madness"].Targeted || set.Choices["unspeakable-oath"].Targeted {
		t.Fatalf("both reactions: %d %+v", code, set.Choices)
	}
	// Only a reaction of the faction can be chosen for.
	if _, code := put(room, map[string]any{"ability": "devolve", "ask_all": true}); code != 400 {
		t.Fatalf("Devolve is not the Crawling Chaos' reaction: %d", code)
	}
}

// madnessGame creates a game in which Alice plays the Crawling Chaos with the
// High Priests in the box and holds Madness. Yellow Sign has just fought the
// Black Goat: its pained Cultist retreats where Alice says, though she is not
// in the battle, so a question about Madness waits for her.
func madnessGame(t *testing.T, base string) string {
	t.Helper()
	doc := cwPosition(t,
		[]cwUnit{{4, "undead", "africa", 3}, {3, "cultist", "africa", 1}, {1, "cultist", "asia", 1}, {2, "cultist", "south-pacific", 1}},
		map[string]int{"south-pacific": 2},
		[5]int{0, 6, 6, 6, 6}, [5]uint8{0, 0, 1 << 4, 0, 0},
		func(d *cthulhuwars.Data) {
			d.Turn.Seat = 4
			d.Stage = "retreat"
			d.Bat = &cthulhuwars.Battle{Area: "africa", Att: 4, Def: 3, Step: "retreat", Retreater: 2, Pained: []cthulhuwars.PU{{Seat: 3, Type: "cultist"}}, Exempt: map[string]int{}, Called: map[string]int{}}
		})
	// The High Priests of the four cults wait in their pools.
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(doc)); err != nil {
		t.Fatal(err)
	}
	for _, color := range []string{"green", "blue", "red", "yellow"} {
		bs.SetReserve(color, "high-priest", 1)
	}
	raw, err := bs.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	doc = string(raw)
	var created createGameResponse
	body := map[string]any{
		"game_def_id": "cthulhu-wars~gc-cc-bg-ys-hp", "player_name": "Alice", "seat": 2,
		"seat_bots":   map[string]string{"1": "random", "3": "random", "4": "random"},
		"start_state": json.RawMessage(doc), "start_seat": 2,
	}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	return created.RoomCode
}
