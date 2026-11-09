// Tests of installed data packs at startup (ADR 0128): the server reads each one
// from its database again, plays it, keeps each version a game of its own, and
// keeps the games of one that no longer passes its checks without ending them.

package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/pack"
	"github.com/noderunner/bingle/internal/store"
)

func linesPack(t *testing.T, version string) []byte {
	t.Helper()
	def := fmt.Sprintf(`id: lines
name: Lines
description: Fill a small board.
version: %s
rules:
  - Place a stone. When the board is full, whoever has more stones wins.
board:
  generator: {type: grid, width: 3, height: 3, adjacency: orthogonal8, labels: algebraic}
seats:
  - {seat: 1, color: red, label: Red}
  - {seat: 2, color: blue, label: Blue}
pieces:
  stone: {label: Stone}
turn_order:
  type: round-robin
  seats: [1, 2]
actions:
  - {id: place, type: place, label: Place a stone, pieces: [stone], source: supply}
win_condition:
  type: rules
  rules:
    - {id: full, type: end, when: [no-legal-moves], decide: {type: most, piece: stone}, reason: full}
`, version)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(pack.DefinitionFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(def)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// install puts a pack in the database at path, as the install page will.
func install(t *testing.T, path string, zipped []byte) {
	t.Helper()
	p, err := pack.Read(zipped)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if err := st.SavePack(context.Background(), store.StoredPack{
		Name: p.Name, Version: p.Version, Title: p.Title, Games: p.Games, SHA256: p.SHA256, Zip: zipped,
	}); err != nil {
		t.Fatal(err)
	}
}

func startOn(t *testing.T, cfg config.Config) (base string, stop func()) {
	t.Helper()
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	srv := httptest.NewServer(a.Router())
	return srv.URL, func() { srv.Close(); _ = a.Close() }
}

type packGame struct {
	RoomCode  string `json:"room_code"`
	GameDefID string `json:"game_def_id"`
	Status    string `json:"status"`
	Retired   bool   `json:"retired"`
	GameName  string `json:"game_name"`
	MoveSeq   int    `json:"move_seq"`
	EndReason string `json:"end_reason"`
}

func gameOf(t *testing.T, base, room string) packGame {
	t.Helper()
	var g packGame
	if code := doJSON(t, "GET", base+"/api/games/"+room, nil, &g); code != 200 {
		t.Fatalf("GET %s: %d", room, code)
	}
	return g
}

func TestPacks_AnInstalledPackIsPlayedAndEachVersionKeepsItsGames(t *testing.T) {
	cfg := newTestConfig(t)
	install(t, cfg.DBPath, linesPack(t, "1.0.0"))

	base, stop := startOn(t, cfg)
	al, bo := newGuestKey(t, base), newGuestKey(t, base)
	var old packGame
	if code := doJSONAs(t, al, "POST", base+"/api/games", map[string]any{"game_def_id": "pack-lines", "player_name": "Al"}, &old); code != 200 {
		t.Fatalf("creating a game of the pack: %d", code)
	}
	if got := gameOf(t, base, old.RoomCode); got.GameDefID != "pack-lines-1.0.0" {
		t.Fatalf("the family should mean the version installed: %+v", got)
	}
	if code := doJSONAs(t, bo, "POST", base+"/api/games/"+old.RoomCode+"/join", map[string]any{"player_name": "Bo"}, nil); code != 200 {
		t.Fatalf("joining: %d", code)
	}
	if code := doJSONAs(t, al, "POST", base+"/api/games/"+old.RoomCode+"/moves", map[string]any{"player_name": "Al", "notation": "b2"}, nil); code != 200 {
		t.Fatalf("a move: %d", code)
	}
	stop()

	// A new version installs beside the old: new games use it, and the game
	// in progress goes on with the rules it started with.
	install(t, cfg.DBPath, linesPack(t, "1.1.0"))
	base, stop = startOn(t, cfg)
	var fresh packGame
	if code := doJSONAs(t, al, "POST", base+"/api/games", map[string]any{"game_def_id": "pack-lines", "player_name": "Al"}, &fresh); code != 200 {
		t.Fatalf("a new game after the upgrade: %d", code)
	}
	if got := gameOf(t, base, fresh.RoomCode); got.GameDefID != "pack-lines-1.1.0" {
		t.Fatalf("a new game after the upgrade should use it: %+v", got)
	}
	if got := gameOf(t, base, old.RoomCode); got.GameDefID != "pack-lines-1.0.0" {
		t.Fatalf("the game in progress should keep its version: %+v", got)
	}
	if code := doJSONAs(t, bo, "POST", base+"/api/games/"+old.RoomCode+"/moves", map[string]any{"player_name": "Bo", "notation": "a1"}, nil); code != 200 {
		t.Fatalf("the old game after the upgrade: %d", code)
	}
	var versions []store.GameVersion
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := st.GameVersions(context.Background())
	_ = st.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range recorded {
		if v.Family == "pack-lines" {
			versions = append(versions, v)
		}
	}
	if len(versions) != 2 || versions[0].Version != "1.0.0" || versions[1].Version != "1.1.0" {
		t.Errorf("game_versions should record both versions of the pack: %+v", versions)
	}
	stop()
}

func TestPacks_APackThatNoLongerPassesKeepsItsGames(t *testing.T) {
	cfg := newTestConfig(t)
	good := linesPack(t, "1.0.0")
	install(t, cfg.DBPath, good)

	base, stop := startOn(t, cfg)
	al, bo := newGuestKey(t, base), newGuestKey(t, base)
	var g packGame
	if code := doJSONAs(t, al, "POST", base+"/api/games", map[string]any{"game_def_id": "pack-lines", "player_name": "Al"}, &g); code != 200 {
		t.Fatalf("creating: %d", code)
	}
	doJSONAs(t, bo, "POST", base+"/api/games/"+g.RoomCode+"/join", map[string]any{"player_name": "Bo"}, nil)
	if code := doJSONAs(t, al, "POST", base+"/api/games/"+g.RoomCode+"/moves", map[string]any{"player_name": "Al", "notation": "b2"}, nil); code != 200 {
		t.Fatalf("a move: %d", code)
	}
	stop()

	// The stored zip stops reading as a pack, as one would if a later server
	// checked something this one did not.
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	packs, _ := st.Packs(context.Background())
	bad := packs[0]
	bad.Zip = append([]byte(nil), good[:len(good)/2]...)
	if err := st.DeletePack(context.Background(), bad.Name, bad.Version); err != nil {
		t.Fatal(err)
	}
	if err := st.SavePack(context.Background(), bad); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	// The server still starts, and the game is kept unfinished, as a record.
	base, stop = startOn(t, cfg)
	defer stop()
	got := gameOf(t, base, g.RoomCode)
	if !got.Retired || got.GameName != "Lines" || got.Status != "active" || got.EndReason != "" || got.MoveSeq != 1 {
		t.Fatalf("the game should be kept as it was, not ended: %+v", got)
	}
	if code := doJSONAs(t, bo, "POST", base+"/api/games/"+g.RoomCode+"/moves", map[string]any{"player_name": "Bo", "notation": "a1"}, nil); code != 410 {
		t.Fatalf("a move in it: %d, want 410", code)
	}
	if code := doJSONAs(t, al, "POST", base+"/api/games", map[string]any{"game_def_id": "pack-lines", "player_name": "Al"}, nil); code != 404 {
		t.Fatalf("creating a game of it: %d, want 404", code)
	}
}
