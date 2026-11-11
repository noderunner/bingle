package app_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type setupProfile struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Owner     string         `json:"owner"`
	Mine      bool           `json:"mine"`
	Shared    bool           `json:"shared"`
	GameDefID string         `json:"game_def_id"`
	Options   map[string]any `json:"options"`
}

func profileBody(name string, shared bool) map[string]any {
	return map[string]any{"name": name, "game_def_id": "test-council", "shared": shared,
		"options": map[string]any{"houses": []any{"owl", "stag", "wolf", "ram"}, "court": true}}
}

func listProfiles(t *testing.T, b *browser) []setupProfile {
	t.Helper()
	var out struct {
		Profiles []setupProfile `json:"profiles"`
	}
	if code := b.do("GET", "/api/setup-profiles?family=test-council", nil, &out); code != 200 {
		t.Fatalf("list: %d", code)
	}
	return out.Profiles
}

func TestSetupProfiles(t *testing.T) {
	a, base := membersServer(t)
	ana, bob := member(t, a, base, "ana"), member(t, a, base, "bob")
	guest := newBrowser(t, base)

	// A guest may look, and may not save or delete.
	if got := listProfiles(t, guest); len(got) != 0 {
		t.Fatalf("a new server lists %v", got)
	}
	if code := guest.do("POST", "/api/setup-profiles", profileBody("Mine", true), nil); code != 403 {
		t.Errorf("a guest saves: %d", code)
	}

	// Saving: what is private stays with its owner, what is shared is everyone's.
	var private, shared setupProfile
	if code := ana.do("POST", "/api/setup-profiles", profileBody("  Quiet   night ", false), &private); code != 200 {
		t.Fatalf("save: %d", code)
	}
	if private.Name != "Quiet night" {
		t.Errorf("name = %q", private.Name)
	}
	if code := ana.do("POST", "/api/setup-profiles", profileBody("Friday", true), &shared); code != 200 {
		t.Fatalf("save: %d", code)
	}
	if got := listProfiles(t, ana); len(got) != 2 || !got[0].Mine {
		t.Errorf("ana's list: %+v", got)
	}
	for who, b := range map[string]*browser{"bob": bob, "a guest": guest} {
		got := listProfiles(t, b)
		if len(got) != 1 || got[0].Name != "Friday" || got[0].Mine || got[0].Owner != "ana" {
			t.Errorf("%s's list: %+v", who, got)
		}
	}
	// What is listed can be sent as the options of a game.
	var game struct {
		RoomCode string `json:"room_code"`
	}
	got := listProfiles(t, guest)[0]
	if code := guest.do("POST", "/api/games", map[string]any{"game_def_id": got.GameDefID, "player_name": "Gus", "options": got.Options}, &game); code != 200 && code != 201 {
		t.Errorf("a game from the setup: %d", code)
	}

	// The same name, in any case, replaces; other accounts have their own names.
	var again setupProfile
	if code := ana.do("POST", "/api/setup-profiles", profileBody("friday", false), &again); code != 200 || again.ID != shared.ID {
		t.Errorf("replacing: %d, %+v", code, again)
	}
	if got := listProfiles(t, bob); len(got) != 0 {
		t.Errorf("unshared, it is still listed: %+v", got)
	}
	if code := bob.do("POST", "/api/setup-profiles", profileBody("Friday", true), nil); code != 200 {
		t.Errorf("bob's own Friday: %d", code)
	}

	// Nobody else can delete one; a private one does not exist for them.
	if code := bob.do("DELETE", fmt.Sprintf("/api/setup-profiles/%d", private.ID), nil, nil); code != 404 {
		t.Errorf("bob deletes ana's: %d", code)
	}
	if code := guest.do("DELETE", fmt.Sprintf("/api/setup-profiles/%d", private.ID), nil, nil); code != 403 {
		t.Errorf("a guest deletes ana's: %d", code)
	}
	if code := ana.do("DELETE", fmt.Sprintf("/api/setup-profiles/%d", private.ID), nil, nil); code != 204 {
		t.Errorf("ana deletes hers: %d", code)
	}
	if code := ana.do("DELETE", fmt.Sprintf("/api/setup-profiles/%d", private.ID), nil, nil); code != 404 {
		t.Errorf("deleted twice: %d", code)
	}
}

// What cannot make a game is not saved.
func TestSetupProfiles_Refused(t *testing.T) {
	a, base := membersServer(t)
	ana := member(t, a, base, "ana")
	for name, body := range map[string]map[string]any{
		"no name":      profileBody("   ", false),
		"a long name":  profileBody(strings.Repeat("x", 41), false),
		"a tab":        profileBody("two\u0007words", false),
		"bad options":  {"name": "x", "game_def_id": "test-council", "options": map[string]any{"houses": []any{"crow"}}},
		"an unknown":   {"name": "x", "game_def_id": "nope", "options": map[string]any{}},
		"a derived id": {"name": "x", "game_def_id": "test-council~crow-stag-wolf-ram-court", "options": map[string]any{}},
		"no picker":    {"name": "x", "game_def_id": "hex-11", "options": map[string]any{}},
	} {
		if code := ana.do("POST", "/api/setup-profiles", body, nil); code < 400 || code >= 500 {
			t.Errorf("%s: %d", name, code)
		}
	}
	// As many as may be kept, and replacing still works at the limit.
	for i := 0; i < 30; i++ {
		if code := ana.do("POST", "/api/setup-profiles", profileBody(fmt.Sprintf("Setup %d", i), false), nil); code != 200 {
			t.Fatalf("setup %d: %d", i, code)
		}
	}
	if code := ana.do("POST", "/api/setup-profiles", profileBody("One more", false), nil); code != 409 {
		t.Errorf("past the limit: %d", code)
	}
	if code := ana.do("POST", "/api/setup-profiles", profileBody("Setup 3", true), nil); code != 200 {
		t.Errorf("replacing at the limit: %d", code)
	}
}

func TestSetupCatalog_Endpoint(t *testing.T) {
	base := newRESTTestServer(t)
	var c map[string]json.RawMessage
	if code := doJSON(t, "GET", base+"/api/game-defs/test-council/setup-catalog", nil, &c); code != 200 || len(c["tabs"]) == 0 {
		t.Errorf("catalog: %d", code)
	}
	if code := doJSON(t, "GET", base+"/api/game-defs/hex-11/setup-catalog", nil, nil); code != 404 {
		t.Errorf("a game without one: %d", code)
	}
	var info struct {
		SetupCatalog bool `json:"setup_catalog"`
	}
	if code := doJSON(t, "GET", base+"/api/game-defs/test-council", nil, &info); code != 200 || !info.SetupCatalog {
		t.Errorf("the description says %v (%d)", info.SetupCatalog, code)
	}
}

// At a table with a pool a player chooses their own seat's pick from it.
func TestFactionPool_PlayersChooseInTheLobby(t *testing.T) {
	base := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "test-council", "player_name": "Alice", "lobby": true,
		"options": map[string]any{"houses": []any{"crow", "stag", "wolf", "ram"}, "house-pool": []any{"crow", "stag", "wolf", "ram", "owl"}}}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 && code != 201 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	change := func(who string, seat int, house string) int {
		return doJSON(t, "POST", base+"/api/games/"+room+"/seat-faction", map[string]any{"player_name": who, "seat": seat, "faction": house}, nil)
	}
	id := func() string {
		var g struct {
			GameDefID string `json:"game_def_id"`
		}
		doJSON(t, "GET", base+"/api/games/"+room, nil, &g)
		return g.GameDefID
	}
	const pool = "-pool-crow-stag-wolf-ram-owl"
	if code := change("Alice", 1, "bear"); code == 200 {
		t.Error("a house that is not in the pool")
	}
	if code := change("Alice", 1, "owl"); code != 200 {
		t.Fatalf("her own seat: %d", code)
	}
	if got := id(); got != "test-council~owl-stag-wolf-ram"+pool {
		t.Errorf("after her choice: %q", got)
	}
	joinRoom(t, base, room, "Bob")
	// Bob sits in the first open seat, the Stag's. Alice's is not his to take, nor his hers.
	if code := change("Bob", 2, "owl"); code != 409 {
		t.Errorf("a house a player holds: %d", code)
	}
	if code := change("Alice", 2, "crow"); code != 409 {
		t.Errorf("another player's seat: %d", code)
	}
	if code := change("Bob", 2, "crow"); code != 200 {
		t.Errorf("Bob takes the free one: %d", code)
	}
	if got := id(); got != "test-council~owl-crow-wolf-ram"+pool {
		t.Errorf("after his choice: %q", got)
	}
}
