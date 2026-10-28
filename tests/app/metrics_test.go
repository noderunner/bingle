// Tests of the metrics on a real server: what the private listener says after
// real traffic, and that none of it is reachable from the public site.

package app_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/metrics"
)

// newMeasuredServer builds a server recording into its own registry, and
// returns the public base URL, a function that scrapes the private handler,
// and the registry.
func newMeasuredServer(t *testing.T) (string, func() string) {
	t.Helper()
	mx := metrics.New("9.9.9")
	a, err := app.New(newTestConfig(t), app.WithMetrics(mx))
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	registerFixtures(t, a)

	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)

	private := a.MetricsHandler()
	scrape := func() string {
		w := httptest.NewRecorder()
		private.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("the private /metrics answered %d", w.Code)
		}
		return w.Body.String()
	}
	return srv.URL, scrape
}

// The public site has no authentication of any kind, so nothing about what
// the server is doing may be readable from it.
func TestMetrics_AreNotOnThePublicListener(t *testing.T) {
	baseURL, _ := newMeasuredServer(t)

	for _, path := range []string{"/metrics", "/api/metrics", "/metrics/", "/debug/pprof/", "/debug/vars"} {
		resp, err := http.Get(baseURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body := make([]byte, 2048)
		n, _ := resp.Body.Read(body)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK && strings.Contains(string(body[:n]), "bingle_http_requests_total") {
			t.Errorf("GET %s on the public listener served the metrics", path)
		}
		if path == "/metrics" && resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET /metrics on the public listener = %d, want 404", resp.StatusCode)
		}
	}
}

// A game played over the real API turns up in the exposition, by route
// pattern, and nothing about the game itself does.
func TestMetrics_RecordRealTrafficWithoutLeakingIt(t *testing.T) {
	baseURL, scrape := newMeasuredServer(t)

	room := createRoom(t, baseURL, "Ana")
	doJSON(t, "POST", baseURL+"/api/games/"+room+"/join", map[string]any{"player_name": "Bo", "role": "player"}, nil)
	doJSON(t, "POST", baseURL+"/api/games/"+room+"/chat", map[string]any{"player_name": "Ana", "text": "a trap!"}, nil)
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, nil)
	doJSON(t, "GET", baseURL+"/api/games/ZZ9XYQ", nil, nil) // a room that does not exist

	conn, err := dialWS(t, baseURL, wsPath(room, "Bo", true, "player", true))
	if err != nil {
		t.Fatalf("Bo's socket: %v", err)
	}
	readFirstEvent(t, conn)

	got := scrape()

	for _, want := range []string{
		`bingle_http_requests_total{method="POST",route="/api/games",status="200"} 1`,
		`bingle_http_requests_total{method="POST",route="/api/games/{code}/chat",status="200"} 1`,
		`bingle_http_requests_total{method="GET",route="/api/games/{code}",status="200"} 1`,
		`bingle_http_errors_total{kind="not-found",route="/api/games/{code}"} 1`,
		`bingle_ws_connections{kind="page"} 1`,
		`bingle_build_info{version="9.9.9"} 1`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the exposition does not say %s", want)
		}
	}
	// A socket is not a request that finished.
	if strings.Contains(got, `route="/ws/{code}"`) {
		t.Error("an accepted socket was recorded as a finished request")
	}
	// And nothing about the game is in a label.
	for what, secret := range map[string]string{
		"the room code":   room,
		"a player's name": "Ana",
		"what was said":   "a trap!",
	} {
		if strings.Contains(got, secret) {
			t.Errorf("the exposition carries %s (%q)", what, secret)
		}
	}
	_ = conn.CloseNow()
}

// Many requests at once, counted exactly, with the race detector watching
// (`make test` and CI run this package under -race in the nightly).
func TestMetrics_CountExactlyUnderConcurrentRequests(t *testing.T) {
	baseURL, scrape := newMeasuredServer(t)

	const (
		workers = 8
		each    = 25
	)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				resp, err := http.Get(baseURL + "/api/game-defs")
				if err != nil {
					t.Errorf("GET /api/game-defs: %v", err)
					return
				}
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()

	got := scrape()
	want := `bingle_http_requests_total{method="GET",route="/api/game-defs",status="200"} ` + strconv.Itoa(workers*each)
	if !strings.Contains(got, want) {
		t.Errorf("the exposition does not say %s", want)
	}
	// Every request has finished, so nothing is in flight.
	if !strings.Contains(got, "bingle_http_requests_in_flight 0") {
		t.Errorf("requests are still counted as in flight:\n%s", firstLines(got, "bingle_http_requests_in_flight"))
	}
}

// firstLines pulls the lines mentioning name out of an exposition, for a
// readable failure.
func firstLines(exposition, name string) string {
	var out []string
	for _, line := range strings.Split(exposition, "\n") {
		if strings.Contains(line, name) {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
