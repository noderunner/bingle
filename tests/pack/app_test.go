package pack_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
	"github.com/noderunner/bingle/tests/testdata"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

// server is an app over one database, reached over HTTP as a guest.
type server struct {
	t   *testing.T
	app *app.App
	url string
	key string
}

func start(t *testing.T, db, packs string) *server {
	t.Helper()
	a, err := app.New(config.Config{DBPath: db, Packs: packs})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	srv := httptest.NewServer(a.Router())
	s := &server{t: t, app: a, url: srv.URL}
	t.Cleanup(func() { s.stop(); srv.Close() })
	return s
}

func (s *server) stop() {
	if s.app != nil {
		_ = s.app.Close()
		s.app = nil
	}
}

func (s *server) call(method, path string, body, out any) int {
	s.t.Helper()
	var r io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		r = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, s.url+"/api"+path, r)
	req.Header.Set("Content-Type", "application/json")
	if s.key != "" {
		req.Header.Set("Authorization", "Bearer "+s.key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 {
		if err := json.Unmarshal(raw, out); err != nil {
			s.t.Fatalf("%s %s: %v: %s", method, path, err, raw)
		}
	}
	return resp.StatusCode
}

func (s *server) must(method, path string, body, out any) {
	s.t.Helper()
	if code := s.call(method, path, body, out); code >= 300 {
		s.t.Fatalf("%s %s = %d", method, path, code)
	}
}

// game makes a game of id with both seats taken and returns its room.
func (s *server) game(id string) string {
	s.t.Helper()
	var created struct {
		Code string `json:"room_code"`
	}
	s.must(http.MethodPost, "/games", map[string]any{"game_def_id": id, "player_name": "Seat 1", "seat": 1}, &created)
	s.must(http.MethodPost, "/games/"+created.Code+"/join", map[string]any{"player_name": "Seat 2", "seat": 2}, nil)
	return created.Code
}

// play makes the first selection the seat to move is offered.
func (s *server) play(room string) int {
	s.t.Helper()
	var legal struct {
		Seat    int              `json:"seat"`
		Options []map[string]any `json:"options"`
	}
	s.must(http.MethodGet, "/games/"+room+"/selections", nil, &legal)
	if len(legal.Options) == 0 {
		s.t.Fatalf("room %s: no selections offered", room)
	}
	notation, _ := legal.Options[0]["notation"].(string)
	return s.call(http.MethodPost, "/games/"+room+"/moves", map[string]any{"player_name": "Seat " + string(rune('0'+legal.Seat)), "notation": notation}, nil)
}

func (s *server) guest() {
	s.t.Helper()
	var g struct {
		Key string `json:"key"`
	}
	s.must(http.MethodPost, "/guest", struct{}{}, &g)
	s.key = g.Key
}

type gameInfo struct {
	Status   string `json:"status"`
	Retired  bool   `json:"retired"`
	GameName string `json:"game_name"`
	MoveSeq  int    `json:"move_seq"`
}

// A server started without a pack it once had starts, and keeps that pack's
// games readable as a retired game's: finished ones with their result and
// unfinished ones as they stood, which play on when the pack is back.
func TestAGonePacksGamesStayReadable(t *testing.T) {
	packs, _ := testdata.DraughtsPack(t)
	db := filepath.Join(t.TempDir(), "packs.db")

	s := start(t, db, packs)
	s.guest()
	playing := s.game(testdata.DraughtsPackID)
	for range 2 {
		if code := s.play(playing); code != http.StatusOK {
			t.Fatalf("a move = %d", code)
		}
	}
	done := s.game(testdata.DraughtsPackSmallID)
	s.must(http.MethodPost, "/games/"+done+"/resign", map[string]any{"player_name": "Seat 1"}, nil)
	key := s.key
	s.stop()

	s = start(t, db, "")
	s.key = key
	var defs []struct {
		ID string `json:"id"`
	}
	s.must(http.MethodGet, "/game-defs", nil, &defs)
	for _, d := range defs {
		if strings.HasPrefix(d.ID, "pack-") {
			t.Errorf("a server without packs lists %s", d.ID)
		}
	}
	var info gameInfo
	read := func(room string) {
		t.Helper()
		info = gameInfo{}
		s.must(http.MethodGet, "/games/"+room, nil, &info)
	}
	read(playing)
	if !info.Retired || info.GameName != "Draughts 0.1.0" || info.Status != "active" || info.MoveSeq != 2 {
		t.Errorf("the unfinished game reads %+v, want retired, named Draughts 0.1.0, active after 2 moves", info)
	}
	var moves []json.RawMessage
	s.must(http.MethodGet, "/games/"+playing+"/moves", nil, &moves)
	if len(moves) != 2 {
		t.Errorf("GET /moves lists %d moves, want 2", len(moves))
	}
	if code := s.call(http.MethodPost, "/games/"+playing+"/moves", map[string]any{"player_name": "Seat 1", "notation": "x"}, nil); code != http.StatusGone {
		t.Errorf("a move in a gone pack's game = %d, want 410", code)
	}
	read(done)
	if !info.Retired || info.Status != "finished" || info.GameName != "Draughts (6×6) 0.1.0" {
		t.Errorf("the finished game reads %+v", info)
	}
	if code := s.call(http.MethodPost, "/games", map[string]any{"game_def_id": testdata.DraughtsPackID, "player_name": "Seat 1"}, nil); code < 400 {
		t.Errorf("a new game of a gone pack = %d", code)
	}
	s.stop()

	s = start(t, db, packs)
	s.key = key
	read(playing)
	if info.Retired || info.Status != "active" {
		t.Fatalf("with its pack back the game reads %+v", info)
	}
	if code := s.play(playing); code != http.StatusOK {
		t.Errorf("a move once the pack is back = %d", code)
	}
}

// noStart is the fixture's rules, which cannot start a game: a module's
// start can fail where linked-in rules cannot (a deadline under load).
type noStart struct{ guest.Rules }

func (noStart) Start(uint64) (*guest.Position, error) {
	return nil, errors.New("the rules could not start")
}

// A game whose rules cannot start it is not made, rather than made on an
// empty board.
func TestAGameIsNotMadeWhenItsRulesCannotStartIt(t *testing.T) {
	s := start(t, filepath.Join(t.TempDir(), "start.db"), "")
	rules := wasmrules.Native(func(g *guest.Game) (guest.Rules, error) {
		r, err := draughts.New(g)
		return noStart{r}, err
	})
	if err := s.app.Catalog().RegisterModule(testdata.DraughtsWith(rules)); err != nil {
		t.Fatal(err)
	}
	s.guest()
	if code := s.call(http.MethodPost, "/games", map[string]any{"game_def_id": draughts.ID, "player_name": "Seat 1", "seat": 1}, nil); code < 500 {
		t.Errorf("making a game whose start fails = %d, want a server error", code)
	}
	var own struct {
		Games []json.RawMessage `json:"games"`
	}
	s.must(http.MethodGet, "/me/games", nil, &own)
	if len(own.Games) != 0 {
		t.Errorf("%d games were made", len(own.Games))
	}
}
