package wasmrules_test

import (
	"bytes"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/tests/testdata"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

// A game whose rules are a module is played over HTTP like any other, by a
// client that knows nothing of it (as tests/apiplay plays every built-in
// game): the session and the API are as they were. A chain of jumps is one
// move of several selections, built from what GET /selections offers next.
func TestAModulesGameIsPlayedThroughTheAPI(t *testing.T) {
	a, err := app.New(config.Config{DBPath: filepath.Join(t.TempDir(), "wasm.db")})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if err := a.Catalog().RegisterModule(testdata.DraughtsWith(draughtsModule(t).Rules())); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)

	key := ""
	call := func(method, path string, body, out any) {
		t.Helper()
		var r io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			r = bytes.NewReader(raw)
		}
		req, _ := http.NewRequest(method, srv.URL+"/api"+path, r)
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			t.Fatalf("%s %s = %d: %s", method, path, resp.StatusCode, raw)
		}
		if out != nil {
			if err := json.Unmarshal(raw, out); err != nil {
				t.Fatalf("%s %s: %v: %s", method, path, err, raw)
			}
		}
	}
	var guest struct {
		Key string `json:"key"`
	}
	call(http.MethodPost, "/guest", struct{}{}, &guest)
	key = guest.Key

	chains := 0
	for _, id := range []string{draughts.ID, draughts.SmallID, draughts.BallotID} {
		var created struct {
			Code string `json:"room_code"`
		}
		call(http.MethodPost, "/games", map[string]any{"game_def_id": id, "player_name": "Seat 1", "seat": 1}, &created)
		call(http.MethodPost, "/games/"+created.Code+"/join", map[string]any{"player_name": "Seat 2", "seat": 2}, nil)
		game := "/games/" + created.Code
		rng := rand.New(rand.NewSource(4))
		var pos struct {
			Status    string `json:"status"`
			Impulse   *int   `json:"impulse_seat"`
			EndReason string `json:"end_reason"`
		}
		for played := 0; ; played++ {
			call(http.MethodGet, game+"/state", nil, &pos)
			if pos.Status != "active" {
				break
			}
			if played == 2000 || pos.Impulse == nil {
				t.Fatalf("%s: after %d moves the game is %s and waits for %v", id, played, pos.Status, pos.Impulse)
			}
			// Selections are chosen until they are a move.
			var made []string
			for {
				var next struct {
					Complete bool `json:"complete"`
					Options  []struct {
						Notation string `json:"notation"`
					} `json:"options"`
				}
				query := url.Values{"made": {strings.Join(made, "; ")}}.Encode()
				call(http.MethodGet, game+"/selections?"+query, nil, &next)
				if next.Complete {
					break
				}
				if len(next.Options) == 0 || len(made) > 12 {
					t.Fatalf("%s: move %d has no selection after %v", id, played, made)
				}
				made = append(made, next.Options[rng.Intn(len(next.Options))].Notation)
			}
			if len(made) > 1 {
				chains++
			}
			call(http.MethodPost, game+"/moves", map[string]any{
				"player_name": "Seat " + strconv.Itoa(*pos.Impulse), "notation": strings.Join(made, "; "),
			}, nil)
		}
		if pos.Status != "finished" || (pos.EndReason != draughts.ReasonNoMoves && pos.EndReason != draughts.ReasonQuiet) {
			t.Fatalf("%s ended %s for the reason %q", id, pos.Status, pos.EndReason)
		}
	}
	if chains == 0 {
		t.Error("no chain of jumps was played in three games")
	}
}
