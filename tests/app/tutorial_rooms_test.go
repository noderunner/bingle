// Tests of tutorial rooms (ADR 0077): a game's tutorials are listed and
// served, a tutorial opens a room with the learner in every seat that plays
// through the ordinary move endpoint, and the room stays out of every list of
// games and out of turn notifications.

package app_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

type tutorialList struct {
	Family    string `json:"family"`
	Tutorials []struct {
		ID    string `json:"id"`
		Kind  string `json:"kind"`
		Title string `json:"title"`
		Game  string `json:"game"`
		Steps int    `json:"steps"`
	} `json:"tutorials"`
}

type tutorialDoc struct {
	ID    string `json:"id"`
	Game  string `json:"game"`
	Steps []struct {
		Text  string   `json:"text"`
		Play  []string `json:"play"`
		Reply []string `json:"reply"`
	} `json:"steps"`
}

func TestTutorials_ListedAndServed(t *testing.T) {
	base := newRESTTestServer(t)
	for _, id := range []string{"blooms", "blooms-4", "blooms~pie"} {
		var l tutorialList
		if code := doJSON(t, "GET", base+"/api/game-defs/"+id+"/tutorials", nil, &l); code != 200 {
			t.Fatalf("GET the tutorials of %s: %d", id, code)
		}
		if l.Family != "blooms" || len(l.Tutorials) == 0 || l.Tutorials[0].ID != "blooms-basics" ||
			l.Tutorials[0].Kind != "rules" || l.Tutorials[0].Game != "blooms-4" || l.Tutorials[0].Steps < 3 {
			t.Fatalf("the tutorials of %s: %+v", id, l)
		}
	}
	var none tutorialList
	if code := doJSON(t, "GET", base+"/api/game-defs/gomoku/tutorials", nil, &none); code != 200 || none.Family != "gomoku" || none.Tutorials == nil || len(none.Tutorials) != 0 {
		t.Fatalf("a game without tutorials: %d %+v (want an empty list)", code, none)
	}
	if code := doJSON(t, "GET", base+"/api/game-defs/no-such-game/tutorials", nil, nil); code != 404 {
		t.Fatalf("an unknown game: %d", code)
	}

	var doc tutorialDoc
	if code := doJSON(t, "GET", base+"/api/game-defs/blooms/tutorials/blooms-basics", nil, &doc); code != 200 {
		t.Fatalf("GET the tutorial: %d", code)
	}
	if doc.ID != "blooms-basics" || len(doc.Steps) < 3 || firstPlay(doc) != "red@d4" {
		t.Fatalf("the tutorial: %+v", doc)
	}
	if code := doJSON(t, "GET", base+"/api/game-defs/blooms/tutorials/nope", nil, nil); code != 404 {
		t.Fatalf("an unknown tutorial: %d", code)
	}
}

// startTutorial opens the Blooms tutorial as a guest of its own.
func startTutorial(t *testing.T, base, name string) (room, key string) {
	t.Helper()
	key = newGuestKey(t, base)
	var created createGameResponse
	if code := doJSONAs(t, key, "POST", base+"/api/tutorials", map[string]any{"game_def_id": "blooms", "tutorial": "blooms-basics", "player_name": name}, &created); code != 200 {
		t.Fatalf("starting the tutorial: %d", code)
	}
	return created.RoomCode, key
}

// The page plays a tutorial through the ordinary move endpoint: the
// learner's moves as the learner, the replies as the learner's other seat.
func TestTutorials_TheRoomPlaysTheScript(t *testing.T) {
	base := newRESTTestServer(t)
	room, key := startTutorial(t, base, "Lia")

	var g struct {
		GameDefID   string `json:"game_def_id"`
		Status      string `json:"status"`
		ImpulseSeat *int   `json:"impulse_seat"`
		Tutorial    string `json:"tutorial"`
		Seats       []struct {
			Seat  int    `json:"seat"`
			Name  string `json:"name"`
			Bot   string `json:"bot"`
			Yours bool   `json:"yours"`
		} `json:"seats"`
	}
	if code := doJSONAs(t, key, "GET", base+"/api/games/"+room, nil, &g); code != 200 {
		t.Fatalf("GET the room: %d", code)
	}
	if g.GameDefID != "blooms-4" || g.Status != "active" || g.Tutorial != "blooms/blooms-basics" || g.ImpulseSeat == nil || *g.ImpulseSeat != 1 {
		t.Fatalf("the room: %+v", g)
	}
	names := map[int]string{}
	for _, s := range g.Seats {
		if s.Bot != "" || !s.Yours || !strings.HasPrefix(s.Name, "Lia") {
			t.Fatalf("every seat is the learner's: %+v", g.Seats)
		}
		names[s.Seat] = s.Name
	}

	var doc tutorialDoc
	doJSON(t, "GET", base+"/api/game-defs/blooms/tutorials/blooms-basics", nil, &doc)
	play := func(seat int, mv string) {
		if code := doJSONAs(t, key, "POST", base+"/api/games/"+room+"/moves", map[string]any{"player_name": names[seat], "notation": mv}, nil); code != 200 {
			t.Fatalf("seat %d playing %s: %d", seat, mv, code)
		}
	}
	for _, s := range doc.Steps {
		for _, mv := range s.Play {
			play(1, mv)
		}
		for _, mv := range s.Reply {
			play(2, mv)
		}
	}
	var moves []struct{ Notation string }
	doJSON(t, "GET", base+"/api/games/"+room+"/moves", nil, &moves)
	if len(moves) != 9 || moves[8].Notation != "done" {
		t.Fatalf("the moves played: %+v", moves)
	}
}

func TestTutorials_StayOutOfTheLists(t *testing.T) {
	base := newRESTTestServer(t)
	room, key := startTutorial(t, base, "Lia")
	var own struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSONAs(t, key, "POST", base+"/api/games", map[string]any{"game_def_id": "gomoku", "player_name": "Lia"}, &own); code != 200 {
		t.Fatalf("an ordinary game: %d", code)
	}

	var mine struct {
		Games []struct {
			RoomCode string `json:"room_code"`
		} `json:"games"`
	}
	doJSONAs(t, key, "GET", base+"/api/me/games", nil, &mine)
	var open struct {
		Games []struct {
			RoomCode string `json:"room_code"`
		} `json:"games"`
	}
	doJSONAs(t, key, "GET", base+"/api/games", nil, &open)
	for what, list := range map[string][]string{"my games": roomsOf(mine.Games), "the open games": roomsOf(open.Games)} {
		if !contains(list, own.RoomCode) {
			t.Fatalf("%s lack the ordinary game %s: %v", what, own.RoomCode, list)
		}
		if contains(list, room) {
			t.Fatalf("%s show the tutorial room %s", what, room)
		}
	}
}

func roomsOf(games []struct {
	RoomCode string `json:"room_code"`
}) []string {
	var out []string
	for _, g := range games {
		out = append(out, g.RoomCode)
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestTutorials_StartRefusals(t *testing.T) {
	base := newRESTTestServer(t)
	key := newGuestKey(t, base)
	for _, c := range []struct {
		body map[string]any
		want int
	}{
		{map[string]any{"game_def_id": "blooms", "tutorial": "nope", "player_name": "Lia"}, 404},
		{map[string]any{"game_def_id": "nope", "tutorial": "blooms-basics", "player_name": "Lia"}, 404},
		{map[string]any{"game_def_id": "gomoku", "tutorial": "blooms-basics", "player_name": "Lia"}, 404},
		{map[string]any{"game_def_id": "blooms", "tutorial": "", "player_name": "Lia"}, 400},
		{map[string]any{"game_def_id": "blooms", "tutorial": "blooms-basics", "player_name": " "}, 400},
	} {
		if code := doJSONAs(t, key, "POST", base+"/api/tutorials", c.body, nil); code != c.want {
			t.Errorf("%v: %d, want %d", c.body, code, c.want)
		}
	}
}

// Every seat is the learner's: there is nobody to tell it is their turn.
func TestTutorials_NoTurnNotifications(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	room, key := startTutorial(t, e.URL, "Lia")
	c := e.verified(t, "lia@example.test")
	code, out := e.setNotify(t, seatGame{Room: room, Token: key}, "Lia", c, true, 0)
	if code != 409 || !strings.Contains(out["error"].(string), "tutorial") {
		t.Fatalf("turn notifications in a tutorial room: %d %v, want 409", code, out)
	}
}

// The list of games says which have tutorials, for every game of a family.
func TestTutorials_CountedInTheGameList(t *testing.T) {
	base := newRESTTestServer(t)
	var defs []struct {
		ID        string `json:"id"`
		Tutorials int    `json:"tutorials"`
	}
	if code := doJSON(t, "GET", base+"/api/game-defs", nil, &defs); code != 200 {
		t.Fatalf("GET /api/game-defs: %d", code)
	}
	got := map[string]int{}
	for _, d := range defs {
		got[d.ID] = d.Tutorials
	}
	if got["blooms"] < 1 || got["blooms-4"] != got["blooms"] || got["blooms-6"] != got["blooms"] {
		t.Fatalf("the Blooms family's tutorials: %v", got)
	}
	if got["gomoku"] != 0 {
		t.Fatalf("Gomoku has no tutorials: %d", got["gomoku"])
	}
}

// firstPlay is the first move a tutorial asks the learner for.
func firstPlay(doc tutorialDoc) string {
	for _, s := range doc.Steps {
		if len(s.Play) > 0 {
			return s.Play[0]
		}
	}
	return ""
}

// A served tutorial carries its guide: what to point the learner at, for any
// client to show in its own way, and the positions its steps show.
func TestTutorials_ServedWithTheirGuide(t *testing.T) {
	base := newRESTTestServer(t)
	var doc struct {
		Steps []struct {
			Play []string `json:"play"`
			Show []string `json:"show"`
		} `json:"steps"`
		Guide []struct {
			PlaySteps [][]struct {
				Kind  string `json:"kind"`
				Value string `json:"value"`
			} `json:"play_steps"`
			ShowState *struct {
				Cells map[string]any `json:"cells"`
			} `json:"show_state"`
		} `json:"guide"`
	}
	if code := doJSON(t, "GET", base+"/api/game-defs/blooms/tutorials/blooms-basics", nil, &doc); code != 200 {
		t.Fatalf("GET: %d", code)
	}
	if len(doc.Guide) != len(doc.Steps) {
		t.Fatalf("%d steps, %d guided", len(doc.Steps), len(doc.Guide))
	}
	sawShow, sawPlay := false, false
	for i, s := range doc.Steps {
		g := doc.Guide[i]
		if len(s.Show) > 0 {
			sawShow = true
			if g.ShowState == nil || len(g.ShowState.Cells) < 10 {
				t.Fatalf("step %d shows no position: %+v", i+1, g.ShowState)
			}
		}
		if len(s.Play) != len(g.PlaySteps) {
			t.Fatalf("step %d: %d moves, %d guided", i+1, len(s.Play), len(g.PlaySteps))
		}
		if len(s.Play) > 0 && !sawPlay {
			sawPlay = true
			sel := g.PlaySteps[0]
			if len(sel) != 2 || sel[0].Kind != "piece" || sel[0].Value != "red" || sel[1].Kind != "pos" || sel[1].Value != "d4" {
				t.Fatalf("the first move's selections: %+v", sel)
			}
		}
	}
	if !sawShow || !sawPlay {
		t.Fatalf("the Blooms tutorial shows a position (%v) and asks for moves (%v)", sawShow, sawPlay)
	}
}

// A tutorial with a written-out start opens its room from that position:
// the High Priests tutorial's round 2, not a new game.
func TestTutorials_RoomFromAWrittenStart(t *testing.T) {
	base := newRESTTestServer(t)
	key := newGuestKey(t, base)
	var created createGameResponse
	if code := doJSONAs(t, key, "POST", base+"/api/tutorials", map[string]any{"game_def_id": "cthulhu-wars", "tutorial": "high-priests", "player_name": "Lia"}, &created); code != 200 {
		t.Fatalf("starting it: %d", code)
	}
	var st struct {
		ImpulseSeat *int `json:"impulse_seat"`
		State       struct {
			Cells map[string][]struct {
				Piece string `json:"piece"`
				Color string `json:"color"`
			} `json:"cells"`
			Ext struct {
				Round int `json:"round"`
			} `json:"ext"`
		} `json:"state"`
	}
	if code := doJSONAs(t, key, "GET", base+"/api/games/"+created.RoomCode+"/state?player_name=Lia", nil, &st); code != 200 {
		t.Fatalf("GET its state: %d", code)
	}
	var g struct {
		GameDefID string `json:"game_def_id"`
	}
	doJSONAs(t, key, "GET", base+"/api/games/"+created.RoomCode, nil, &g)
	if g.GameDefID != "cthulhu-wars~gc-cc-bg-ys-hp" || st.State.Ext.Round != 2 || st.ImpulseSeat == nil || *st.ImpulseSeat != 1 {
		t.Fatalf("the room: %s, round %d, turn %v", g.GameDefID, st.State.Ext.Round, st.ImpulseSeat)
	}
	found := false
	for _, u := range st.State.Cells["indian-ocean"] {
		if u.Piece == "high-priest" && u.Color == "yellow" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the Yellow Sign's High Priest is not in the Indian Ocean: %+v", st.State.Cells["indian-ocean"])
	}
}

// A tutorial's script answers every reaction, so a tutorial room asks every
// one and its choice cannot be changed: one passed for the learner would leave
// the script behind.
func TestTutorials_ReactionChoiceIsFixed(t *testing.T) {
	base := newRESTTestServer(t)
	key := newGuestKey(t, base)
	var created createGameResponse
	if code := doJSONAs(t, key, "POST", base+"/api/tutorials", map[string]any{"game_def_id": "cthulhu-wars", "tutorial": "high-priests", "player_name": "Lia"}, &created); code != 200 {
		t.Fatalf("starting it: %d", code)
	}
	url := base + "/api/games/" + created.RoomCode + "/reactions"
	var got struct {
		Fixed  bool `json:"fixed"`
		AskAll bool `json:"ask_all"`
	}
	if code := doJSONAs(t, key, "GET", url+"?player_name=Lia", nil, &got); code != 200 || !got.Fixed || !got.AskAll {
		t.Fatalf("GET reactions: %d %+v, want fixed and asking every reaction", code, got)
	}
	for _, body := range []map[string]any{
		{"player_name": "Lia", "ask_all": false},
		{"player_name": "Lia", "targeted": false, "ability": "unspeakable-oath"},
		{"player_name": "Lia", "all": true},
	} {
		var out map[string]any
		if code := doJSONAs(t, key, "PUT", url, body, &out); code != 409 {
			t.Errorf("PUT reactions %v: %d %v, want 409", body, code, out)
		}
	}
	if code := doJSONAs(t, key, "GET", url+"?player_name=Lia", nil, &got); code != 200 || !got.AskAll {
		t.Fatalf("after the refusals: %d %+v", code, got)
	}
}
