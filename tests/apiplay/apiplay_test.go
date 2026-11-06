// Package apiplay_test plays every game to a finish over HTTP, as a client with no browser and no
// knowledge of any game would: it learns the games from GET /game-defs, asks GET /selections what
// the seat to move may do, and plays one with POST /moves. A game that cannot be finished this way
// is a bug, because a native client and a private game both depend on it (the plugins plan, step 1).
package apiplay_test

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
)

// patience names the games random play is not expected to win, since it shuffles cards back and
// forth for ever. These play on for a while, and are then given up through the API, which ends them.
var patience = map[string]bool{
	"freecell":         true,
	"solitaire":        true,
	"solitaire-draw-3": true,
	"spider":           true,
	"spider-1":         true,
	"spider-2":         true,
}

const (
	maxMoves      = 5000 // well past the longest game seen, which ended at its limit of 2000
	patienceMoves = 500
)

// client speaks to the server the way an outside program does: HTTP and JSON, and nothing else.
type client struct {
	t    *testing.T
	base string
	key  string // the guest key every seat is taken with
}

func (c *client) call(method, path string, body, out any) (int, string) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+"/api"+path, r)
	if err != nil {
		c.t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("read %s %s: %v", method, path, err)
	}
	if out != nil && resp.StatusCode < 300 {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("decode %s %s: %v: %s", method, path, err, raw)
		}
	}
	return resp.StatusCode, string(raw)
}

func (c *client) must(method, path string, body, out any) {
	c.t.Helper()
	if code, raw := c.call(method, path, body, out); code >= 300 {
		c.t.Fatalf("%s %s = %d: %s", method, path, code, raw)
	}
}

type gameDef struct {
	ID     string `json:"id"`
	Family string `json:"family"`
	Seats  int    `json:"seats_required"`
}

type position struct {
	Status    string `json:"status"`
	Impulse   *int   `json:"impulse_seat"`
	Seq       int    `json:"move_seq"`
	EndReason string `json:"end_reason"`
}

func newServer(t *testing.T) string {
	t.Helper()
	// The limits a server normally sets on creating games would stop a test that makes one per game.
	caps, err := filepath.Abs("testdata/capabilities.yaml")
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.New(config.Config{DBPath: filepath.Join(t.TempDir(), "apiplay.db"), CapsFile: caps})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return srv.URL
}

func seatName(seat int) string { return "Seat " + strconv.Itoa(seat) }

// playOut seats a player in every seat and plays random legal moves, each way a client can name one
// in turn, until the game ends or limit moves have been played. It returns the room and the position.
func playOut(c *client, def gameDef, options map[string]any, rng *rand.Rand, limit int) (string, position) {
	c.t.Helper()
	var created struct {
		Code string `json:"room_code"`
	}
	c.must(http.MethodPost, "/games", map[string]any{"game_def_id": def.ID, "player_name": seatName(1), "seat": 1, "options": options}, &created)
	for seat := 2; seat <= def.Seats; seat++ {
		c.must(http.MethodPost, "/games/"+created.Code+"/join", map[string]any{"player_name": seatName(seat), "seat": seat}, nil)
	}

	game := "/games/" + created.Code
	var pos position
	for played := 0; ; played++ {
		c.must(http.MethodGet, game+"/state", nil, &pos)
		if pos.Status != "active" || played == limit {
			return created.Code, pos
		}
		if pos.Impulse == nil {
			c.t.Fatalf("move %d: the game is active but no seat is to move", played)
		}
		var legal struct {
			Seat  int              `json:"seat"`
			Moves []map[string]any `json:"options"`
		}
		c.must(http.MethodGet, game+"/selections", nil, &legal)
		if legal.Seat != *pos.Impulse {
			c.t.Fatalf("move %d: state says seat %d is to move, the legal moves are for seat %d", played, *pos.Impulse, legal.Seat)
		}
		if len(legal.Moves) == 0 {
			c.t.Fatalf("move %d: seat %d is to move and has no legal moves", played, legal.Seat)
		}
		mv := legal.Moves[rng.Intn(len(legal.Moves))]
		notation, _ := mv["notation"].(string)
		if notation == "" {
			c.t.Fatalf("move %d: a legal move has no notation: %v", played, mv)
		}
		body := map[string]any{"player_name": seatName(legal.Seat), "notation": notation}
		if played%2 == 1 { // the same move by its fields, as the list gives them
			body = map[string]any{"player_name": seatName(legal.Seat)}
			for k, v := range mv {
				if k != "notation" {
					body[k] = v
				}
			}
		}
		if code, raw := c.call(http.MethodPost, game+"/moves", body, nil); code != http.StatusOK {
			c.t.Fatalf("move %d: seat %d played a listed move %v: %d: %s", played, legal.Seat, body, code, raw)
		}
	}
}

func seedOf(id string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return int64(h.Sum64()) //nolint:gosec // a seed, so wrapping is harmless
}

// finish plays a game out, gives it up if it is a patience game still going, and checks how it ended.
func finish(t *testing.T, base, key string, def gameDef, options map[string]any, seed string) {
	t.Helper()
	c := &client{t: t, base: base, key: key}
	rng := rand.New(rand.NewSource(seedOf(seed))) //nolint:gosec // a repeatable choice of moves
	limit := maxMoves
	if patience[def.ID] {
		limit = patienceMoves
	}
	room, pos := playOut(c, def, options, rng, limit)
	if pos.Status == "active" && patience[def.ID] {
		c.must(http.MethodPost, "/games/"+room+"/resign", map[string]any{"player_name": seatName(*pos.Impulse)}, nil)
		c.must(http.MethodGet, "/games/"+room+"/state", nil, &pos)
	}
	if len(options) > 0 {
		var info struct {
			Game string `json:"game_def_id"`
		}
		c.must(http.MethodGet, "/games/"+room, nil, &info)
		if !strings.HasPrefix(info.Game, def.ID+"~") {
			t.Errorf("played as %q, not a game of %s with the options chosen", info.Game, def.ID)
		}
	}
	if pos.Status != "finished" {
		t.Fatalf("status %q after %d moves, want finished", pos.Status, pos.Seq)
	}
	if pos.EndReason == "" {
		t.Fatalf("finished after %d moves with no reason given", pos.Seq)
	}
	var history []json.RawMessage
	c.must(http.MethodGet, "/games/"+room+"/moves", nil, &history)
	if len(history) != pos.Seq {
		t.Fatalf("GET /moves lists %d moves, the state says %d", len(history), pos.Seq)
	}
}

// setUp returns a server, a guest to play as, and the games it offers.
func setUp(t *testing.T) (string, string, []gameDef) {
	t.Helper()
	base := newServer(t)
	root := &client{t: t, base: base}
	var guest struct {
		Key string `json:"key"`
	}
	root.must(http.MethodPost, "/guest", struct{}{}, &guest)
	var defs []gameDef
	root.must(http.MethodGet, "/game-defs", nil, &defs)
	if len(defs) == 0 {
		t.Fatal("GET /game-defs lists no games")
	}
	return base, guest.Key, defs
}

func TestEveryGamePlaysToAFinishThroughTheAPI(t *testing.T) {
	base, key, defs := setUp(t)
	seen := map[string]bool{}
	for _, def := range defs {
		// Short mode plays one game of each family, and leaves out the slow patience games.
		if testing.Short() && (seen[def.Family] || patience[def.ID]) {
			continue
		}
		seen[def.Family] = true
		t.Run(def.ID, func(t *testing.T) { finish(t, base, key, def, nil, def.ID) })
	}
}

// Every ready-made table a game's setup picker offers plays out too: their cults, neutral units and
// independent Great Old Ones are chosen with the options a client sends when it creates a game.
func TestEverySetupPresetPlaysToAFinishThroughTheAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("whole games of every preset")
	}
	base, key, defs := setUp(t)
	root := &client{t: t, base: base}
	byID := map[string]gameDef{}
	for _, def := range defs {
		byID[def.ID] = def
	}
	played := map[string]bool{}
	for _, def := range defs {
		var full struct {
			SetupCatalog bool `json:"setup_catalog"`
		}
		root.must(http.MethodGet, "/game-defs/"+def.ID, nil, &full)
		if !full.SetupCatalog {
			continue
		}
		var catalog struct {
			Presets []struct {
				ID      string         `json:"id"`
				Game    string         `json:"game_def_id"`
				Options map[string]any `json:"options"`
			} `json:"presets"`
		}
		root.must(http.MethodGet, "/game-defs/"+def.ID+"/setup-catalog", nil, &catalog)
		for _, preset := range catalog.Presets {
			name := preset.Game + "/" + preset.ID
			if played[name] {
				continue
			}
			played[name] = true
			game, ok := byID[preset.Game]
			if !ok {
				t.Errorf("preset %s names a game GET /game-defs does not list", name)
				continue
			}
			t.Run(name, func(t *testing.T) { finish(t, base, key, game, preset.Options, name) })
		}
	}
	if len(played) == 0 {
		t.Fatal("no game offers a preset, so this tested nothing")
	}
}
