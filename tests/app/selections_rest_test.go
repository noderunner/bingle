package app_test

import (
	"encoding/json"
	"net/url"
	"testing"
)

type selectionsAnswer struct {
	Seat    int      `json:"seat"`
	Ply     int      `json:"ply"`
	Made    []string `json:"made"`
	Options []struct {
		Notation string `json:"notation"`
	} `json:"options"`
	Complete bool            `json:"complete"`
	State    json.RawMessage `json:"state"`
}

func (a selectionsAnswer) has(notation string) bool {
	for _, o := range a.Options {
		if o.Notation == notation {
			return true
		}
	}
	return false
}

// The selections query and whole impulses over REST (ADR 0114), on Blooms: a
// turn is a stone, then another or done, and the opening is a single stone.
func TestREST_SelectionsAndWholeImpulses(t *testing.T) {
	base := newRESTTestServer(t)
	key := newGuestKey(t, base)
	var created createGameResponse
	if code := doJSONAs(t, key, "POST", base+"/api/games", map[string]any{"game_def_id": "blooms", "player_name": "Ana", "opponent": "random"}, &created); code != 200 || created.Seat != 1 {
		t.Fatalf("create: %d %+v", code, created)
	}
	game := base + "/api/games/" + created.RoomCode
	ask := func(made string) selectionsAnswer {
		t.Helper()
		var a selectionsAnswer
		if code := doJSONAs(t, key, "GET", game+"/selections?made="+url.QueryEscape(made), nil, &a); code != 200 {
			t.Fatalf("selections after %q: %d", made, code)
		}
		return a
	}
	move := func(body map[string]any) int {
		t.Helper()
		body["player_name"] = "Ana"
		var out map[string]any
		return doJSONAs(t, key, "POST", game+"/moves", body, &out)
	}

	first := ask("")
	if first.Seat != 1 || first.Ply != 0 || first.Complete || !first.has("red@e5") || len(first.State) == 0 {
		t.Fatalf("the first selections: seat %d ply %d complete %v, %d options, state %d bytes", first.Seat, first.Ply, first.Complete, len(first.Options), len(first.State))
	}
	// The opening stone is a whole move by itself; the answer then shows no position.
	if a := ask("red@e5"); !a.Complete || len(a.Options) != 0 || len(a.State) != 0 || len(a.Made) != 1 {
		t.Fatalf("after the opening stone: %+v", a)
	}
	if code := move(map[string]any{"notation": "red@e5", "ply": 1}); code != 409 {
		t.Fatalf("a move at a ply the game is not at: %d, want 409", code)
	}
	if code := move(map[string]any{"notation": "red@e5", "ply": 0}); code != 200 {
		t.Fatalf("the opening stone: %d", code)
	}

	// The bot has answered; Ana's turn is a stone and then done, sent as one.
	now := ask("")
	if now.Seat != 1 || now.Ply == 0 {
		t.Fatalf("after the bot's turn: seat %d ply %d", now.Seat, now.Ply)
	}
	stone := now.Options[0].Notation
	if a := ask(stone); a.Complete || !a.has("done") {
		t.Fatalf("after one stone the turn goes on, with done among the options: %+v", a)
	}
	if code := move(map[string]any{"notation": stone + "; done; " + stone, "ply": now.Ply}); code != 400 {
		t.Fatalf("one selection too many: %d, want 400", code)
	}
	if again := ask(""); again.Ply != now.Ply {
		t.Fatalf("a refused impulse recorded something: ply %d, was %d", again.Ply, now.Ply)
	}
	if code := move(map[string]any{"notation": stone + "; done", "ply": now.Ply}); code != 200 {
		t.Fatalf("a whole impulse: %d", code)
	}
	if after := ask(""); after.Ply < now.Ply+2 {
		t.Fatalf("the whole impulse: ply %d, want at least %d", after.Ply, now.Ply+2)
	}
}
