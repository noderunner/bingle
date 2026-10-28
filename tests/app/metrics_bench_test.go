// What recording costs a real move, against the plan's budget of under 1%.
// The synthetic figure in internal/metrics measures the middleware alone; this
// measures what a player actually waits for.

package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/metrics"
)

// A move needs a game nobody can finish by accident, so this plays
// tic-tac-toe and resets by making a fresh game whenever the board fills.
func benchMoveServer(b *testing.B, mx *metrics.Metrics) *httptest.Server {
	b.Helper()
	cfg := config.Config{DBPath: b.TempDir() + "/bench.db"}
	a, err := app.New(cfg, app.WithMetrics(mx))
	if err != nil {
		b.Fatalf("app.New: %v", err)
	}
	b.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	b.Cleanup(srv.Close)
	return srv
}

// post sends JSON and throws the answer away, failing the benchmark on a
// status nobody expected.
func benchPost(b *testing.B, url string, body any, want int) []byte {
	b.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		b.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		b.Fatalf("POST %s: %v", url, err)
	}
	out, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != want {
		b.Fatalf("POST %s = %d (%s)", url, resp.StatusCode, out)
	}
	return out
}

// benchMoves plays one move after another through the real REST API, starting
// a new game each time a board runs out of squares.
func benchMoves(b *testing.B, mx *metrics.Metrics) {
	srv := benchMoveServer(b, mx)
	type legalMoves struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"moves"`
	}

	newGame := func() (string, []string) {
		var created createGameResponse
		raw := benchPost(b, srv.URL+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, http.StatusOK)
		if err := json.Unmarshal(raw, &created); err != nil {
			b.Fatalf("decode: %v", err)
		}
		benchPost(b, srv.URL+"/api/games/"+created.RoomCode+"/join", map[string]any{"player_name": "Bo", "role": "player"}, http.StatusOK)
		return created.RoomCode, []string{"Ana", "Bo"}
	}

	room, players := newGame()
	turn := 0

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Get(srv.URL + "/api/games/" + room + "/moves/legal")
		if err != nil {
			b.Fatalf("legal moves: %v", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var legal legalMoves
		if err := json.Unmarshal(raw, &legal); err != nil {
			b.Fatalf("decode legal moves: %v", err)
		}
		if len(legal.Moves) == 0 {
			b.StopTimer()
			room, players = newGame()
			turn = 0
			b.StartTimer()
			continue
		}
		benchPost(b, srv.URL+"/api/games/"+room+"/moves",
			map[string]any{"player_name": players[turn%2], "notation": legal.Moves[0].Notation}, http.StatusOK)
		turn++
	}
}

// With no registry: the server a BINGLE_METRICS_ADDR-less deployment runs.
func BenchmarkMove_MetricsOff(b *testing.B) { benchMoves(b, nil) }

// And recording, which is what Stage 2 adds.
func BenchmarkMove_MetricsOn(b *testing.B) { benchMoves(b, metrics.New("9.9.9")) }
