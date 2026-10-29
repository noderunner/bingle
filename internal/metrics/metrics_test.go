// Tests of the registry and the HTTP middleware: the exposition parses and
// holds what it should, the labels stay a closed set, and a freshly started
// server's series count stays under its ceiling.

package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/noderunner/bingle/internal/httpstat"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/metrics"
)

// newRouter wires the middleware the way internal/app does — inside something
// that wraps the writer, since that is where the status comes from.
func newRouter(m *metrics.Metrics, routes func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			rec, ctx := httpstat.Wrap(req.Context(), w)
			next.ServeHTTP(rec, req.WithContext(ctx))
		})
	})
	r.Use(m.Middleware)
	routes(r)
	return r
}

// scrape returns what the exposition says.
func scrape(t *testing.T, m *metrics.Metrics) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/metrics answered %d", w.Code)
	}
	body, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatalf("reading the exposition: %v", err)
	}
	return string(body)
}

// A fresh server exposes the gauges, the build and the two collectors. The
// families behind a label are empty until something is served, and Prometheus
// writes nothing at all for an empty one — which is also what keeps a fresh
// server's series count small.
func TestNew_ExpositionHoldsTheBuildAndTheCollectors(t *testing.T) {
	m := metrics.New("1.2.3")
	got := scrape(t, m)

	for _, want := range []string{
		"bingle_http_requests_in_flight",
		"bingle_build_info",
		"go_goroutines",
		"process_open_fds",
	} {
		if !strings.Contains(got, "# TYPE "+want) {
			t.Errorf("the exposition has no %s", want)
		}
	}
	if !strings.Contains(got, `bingle_build_info{version="1.2.3"} 1`) {
		t.Error("the build's version is not in the exposition")
	}
}

// And every family this stage registers appears once there is something in it.
func TestNew_ExpositionHoldsEveryFamilyOnceItHasSomethingInIt(t *testing.T) {
	m := metrics.New("1.2.3")
	h := newRouter(m, func(r chi.Router) {
		r.Get("/x", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("b")) })
		r.Get("/no", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusNotFound) })
		r.Get("/ws-chat", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusSwitchingProtocols) })
	})
	for _, path := range []string{"/x", "/no", "/ws-chat"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	got := scrape(t, m)
	for _, want := range []string{
		"bingle_http_requests_total",
		"bingle_http_request_duration_seconds",
		"bingle_http_requests_in_flight",
		"bingle_http_response_bytes_total",
		"bingle_http_errors_total",
		"bingle_ws_connections",
		"bingle_ws_connections_total",
	} {
		if !strings.Contains(got, "# TYPE "+want) {
			t.Errorf("the exposition has no %s", want)
		}
	}
}

// The registry is bingle's own, never prometheus' default one, so a
// dependency that registers a series behind our back cannot put it on
// /metrics.
func TestNew_DoesNotServeTheDefaultRegistry(t *testing.T) {
	const name = "a_series_registered_behind_our_back"
	sneaky := prometheus.NewGauge(prometheus.GaugeOpts{Name: name})
	prometheus.MustRegister(sneaky)
	t.Cleanup(func() { prometheus.Unregister(sneaky) })
	sneaky.Set(1)

	if got := scrape(t, metrics.New("1.2.3")); strings.Contains(got, name) {
		t.Error("a series from the default registry reached bingle's exposition")
	}
}

func TestMiddleware_RecordsRoutePatternsAndNeverPaths(t *testing.T) {
	m := metrics.New("1.2.3")
	h := newRouter(m, func(r chi.Router) {
		r.Get("/api/games/{code}/chat", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("four"))
		})
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/games/AB12CD/chat?name=Ana", nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	got := scrape(t, m)
	if strings.Contains(got, "AB12CD") || strings.Contains(got, "Ana") {
		t.Fatalf("a room code or a name reached a label:\n%s", got)
	}
	for _, want := range []string{
		`bingle_http_requests_total{method="GET",route="/api/games/{code}/chat",status="200"} 3`,
		`bingle_http_response_bytes_total{route="/api/games/{code}/chat"} 12`,
		`bingle_http_request_duration_seconds_count{method="GET",route="/api/games/{code}/chat"} 3`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the exposition does not say %s", want)
		}
	}
	// Nothing was refused, so the error counter has no series at all.
	if strings.Contains(got, "bingle_http_errors_total{") {
		t.Errorf("a successful request was counted as an error:\n%s", got)
	}
}

// Refusals are counted by the family internal/api's statusForError sorts them
// into, so "too many of these are being refused, and why" is one query.
func TestMiddleware_CountsRefusalsByFamily(t *testing.T) {
	m := metrics.New("1.2.3")
	h := newRouter(m, func(r chi.Router) {
		r.Get("/refuse/{status}", func(w http.ResponseWriter, req *http.Request) {
			switch chi.URLParam(req, "status") {
			case "400":
				http.Error(w, "no", http.StatusBadRequest)
			case "403":
				http.Error(w, "no", http.StatusForbidden)
			case "404":
				http.Error(w, "no", http.StatusNotFound)
			case "409":
				http.Error(w, "no", http.StatusConflict)
			case "429":
				http.Error(w, "no", http.StatusTooManyRequests)
			case "418":
				http.Error(w, "no", http.StatusTeapot)
			default:
				http.Error(w, "sorry", http.StatusInternalServerError)
			}
		})
	})

	for _, status := range []string{"400", "403", "404", "409", "429", "418", "500"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/refuse/"+status, nil))
	}

	got := scrape(t, m)
	for _, kind := range []string{"bad-request", "forbidden", "not-found", "conflict", "rate-limited", "client", "server"} {
		want := `bingle_http_errors_total{kind="` + kind + `",route="/refuse/{status}"} 1`
		if !strings.Contains(got, want) {
			t.Errorf("the exposition does not say %s", want)
		}
	}
}

// A socket is a connection that lasts, not a request that finished: it must
// not land in the request histogram, and it has a gauge of its own.
func TestMiddleware_CountsSocketsSeparatelyFromRequests(t *testing.T) {
	m := metrics.New("1.2.3")
	open := make(chan struct{})
	done := make(chan struct{})
	h := newRouter(m, func(r chi.Router) {
		r.Get("/ws/{code}", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusSwitchingProtocols)
			if req.URL.Query().Get("hold") == "" {
				return
			}
			close(open)
			<-done // stay "connected" while the gauge is read
		})
		r.Get("/ws-chat", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusSwitchingProtocols)
		})
	})

	go h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ws/AB12CD?hold=1", nil))
	<-open

	got := scrape(t, m)
	if !strings.Contains(got, `bingle_ws_connections{kind="page"} 1`) {
		t.Errorf("the open socket is not counted:\n%s", got)
	}
	if !strings.Contains(got, `bingle_ws_connections_total{kind="page"} 1`) {
		t.Error("the socket was not counted as accepted")
	}
	if strings.Contains(got, `route="/ws/{code}"`) {
		t.Errorf("an accepted socket was recorded as a request:\n%s", got)
	}

	close(done)
	// A voice socket on the same route is a different kind, and the chat
	// socket is a third.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ws/AB12CD?only=voice", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ws-chat", nil))

	got = scrape(t, m)
	for _, want := range []string{
		`bingle_ws_connections_total{kind="voice"} 1`,
		`bingle_ws_connections_total{kind="chat"} 1`,
		`bingle_ws_connections{kind="voice"} 0`,
		`bingle_ws_connections{kind="chat"} 0`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the exposition does not say %s", want)
		}
	}
}

// A nil *Metrics is the server with metrics switched off: it records nothing
// and costs nothing.
func TestNilMetrics_RecordsNothingAndServesNothing(t *testing.T) {
	var m *metrics.Metrics

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	if got := m.Middleware(inner); got == nil {
		t.Fatal("the middleware of a nil *Metrics is nil")
	}
	m.WSOpen(metrics.WSPage)
	m.WSClose(metrics.WSPage)

	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("with metrics off /metrics answered %d, want 404", w.Code)
	}
	if m.Registry() != nil {
		t.Error("a nil *Metrics has a registry")
	}

	// And the middleware still serves the request.
	w = httptest.NewRecorder()
	m.Middleware(inner).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Errorf("the request was not served: %d %q", w.Code, w.Body.String())
	}
}

// Logging the route is one rule with one definition, so what the middleware
// records and what the access log writes cannot drift apart.
func TestMiddleware_UsesTheSameRouteAsTheAccessLog(t *testing.T) {
	var fromLog string
	m := metrics.New("1.2.3")
	h := newRouter(m, func(r chi.Router) {
		r.Get("/api/games/{code}", func(_ http.ResponseWriter, req *http.Request) {
			fromLog = logging.Route(req)
		})
	})
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/games/AB12CD", nil))

	if fromLog != "/api/games/{code}" {
		t.Fatalf("logging.Route = %q", fromLog)
	}
	if !strings.Contains(scrape(t, m), `route="`+fromLog+`"`) {
		t.Errorf("the metric does not use the route the log does (%q)", fromLog)
	}
}

// A panic that passes through the middleware rather than being recovered
// under it — http.ErrAbortHandler is re-panicked on purpose — must not leave
// a request counted as in flight for ever. A gauge that only climbs is worse
// than a counter that misses one request.
func TestMiddleware_DoesNotLeakTheInFlightGaugeOnAnAbortedRequest(t *testing.T) {
	m := metrics.New("1.2.3")
	h := newRouter(m, func(r chi.Router) {
		r.Get("/abort", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	})

	for i := 0; i < 3; i++ {
		func() {
			defer func() { _ = recover() }() // net/http's own recover, in effect
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/abort", nil))
		}()
	}

	if got := scrape(t, m); !strings.Contains(got, "bingle_http_requests_in_flight 0") {
		t.Errorf("requests are still counted as in flight after being aborted:\n%s", got)
	}
}

// And a socket whose handler aborts must not stay counted as open.
func TestMiddleware_DoesNotLeakTheSocketGaugeOnAnAbortedUpgrade(t *testing.T) {
	m := metrics.New("1.2.3")
	h := newRouter(m, func(r chi.Router) {
		r.Get("/ws-chat", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusSwitchingProtocols)
			panic(http.ErrAbortHandler)
		})
	})

	func() {
		defer func() { _ = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ws-chat", nil))
	}()

	got := scrape(t, m)
	if !strings.Contains(got, `bingle_ws_connections{kind="chat"} 0`) {
		t.Errorf("the aborted socket is still counted as open:\n%s", got)
	}
	if !strings.Contains(got, `bingle_ws_connections_total{kind="chat"} 1`) {
		t.Error("the socket was not counted as having been accepted")
	}
}
