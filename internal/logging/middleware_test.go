// Tests of the three middlewares: the request id travels, a panic becomes one
// error line and a 500 instead of a dropped connection, and the access line
// carries the route pattern rather than the path it came to.

package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/logging"
)

// newRouter wires the three middlewares in the order internal/app does, over
// the routes a test needs, and returns it with the buffer its logger writes
// JSON lines to (JSON so a test can read a line's fields rather than grep it).
func newRouter(t *testing.T, level slog.Level, routes func(chi.Router)) (http.Handler, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	lg := logging.New(buf, level, logging.FormatJSON)
	r := chi.NewRouter()
	r.Use(logging.RequestID)
	r.Use(logging.Access(lg))
	r.Use(logging.Recover(lg))
	routes(r)
	return r, buf
}

// lines decodes every log line the buffer holds.
func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("a log line is not JSON: %v (%q)", err, raw)
		}
		out = append(out, line)
	}
	return out
}

// only returns the single line whose msg is msg, failing if there is not
// exactly one.
func only(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, l := range lines(t, buf) {
		if l["msg"] == msg {
			found = append(found, l)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one %q line, got %d: %q", msg, len(found), buf.String())
	}
	return found[0]
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestRequestID_IsMadeAndEchoedAndReachesTheLog(t *testing.T) {
	var inHandler string
	h, buf := newRouter(t, slog.LevelDebug, func(r chi.Router) {
		r.Get("/api/games/{code}", func(w http.ResponseWriter, r *http.Request) {
			inHandler = logging.RequestIDFrom(r.Context())
		})
	})

	w := get(h, "/api/games/AB12CD")
	id := w.Header().Get(logging.RequestIDHeader)
	if len(id) != 16 {
		t.Fatalf("%s = %q, want sixteen hex characters", logging.RequestIDHeader, id)
	}
	if inHandler != id {
		t.Errorf("the handler saw id %q, the response says %q", inHandler, id)
	}
	if got := only(t, buf, "request")["id"]; got != id {
		t.Errorf("the log line says id %v, the response says %q", got, id)
	}
}

func TestRequestID_KeepsASaneInboundOneAndReplacesTheRest(t *testing.T) {
	h, _ := newRouter(t, slog.LevelDebug, func(r chi.Router) {
		r.Get("/", func(http.ResponseWriter, *http.Request) {})
	})

	cases := map[string]struct {
		sent string
		keep bool
	}{
		"a proxy's own id":   {sent: "0b9c1d2e3f405162", keep: true},
		"too long":           {sent: strings.Repeat("a", 65), keep: false},
		"a space in it":      {sent: "id with spaces", keep: false},
		"a newline in it":    {sent: "id\nwith-a-newline", keep: false},
		"something unicodey": {sent: "идентификатор", keep: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			// Set the header directly: http.Header.Set would not take a
			// newline, and the point is what bingle does with one.
			req.Header[logging.RequestIDHeader] = []string{tc.sent}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			got := w.Header().Get(logging.RequestIDHeader)
			if tc.keep && got != tc.sent {
				t.Errorf("id = %q, want the inbound %q kept", got, tc.sent)
			}
			if !tc.keep && (got == tc.sent || len(got) != 16) {
				t.Errorf("id = %q, want it replaced by one of bingle's own", got)
			}
		})
	}
}

// A panic used to vanish: net/http dropped the connection and nothing was
// recorded. It must now be one error line with a stack, and a 500.
func TestRecover_LogsOnceWithAStackAndAnswers500(t *testing.T) {
	h, buf := newRouter(t, slog.LevelDebug, func(r chi.Router) {
		r.Get("/api/games/{code}", func(http.ResponseWriter, *http.Request) {
			panic("the ruleset fell over")
		})
	})

	w := get(h, "/api/games/AB12CD")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	line := only(t, buf, "a request panicked")
	if line["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", line["level"])
	}
	if line["route"] != "/api/games/{code}" {
		t.Errorf("route = %v, want the pattern", line["route"])
	}
	if !strings.Contains(line["panic"].(string), "the ruleset fell over") {
		t.Errorf("panic = %v", line["panic"])
	}
	stack, _ := line["stack"].(string)
	if !strings.Contains(stack, "middleware_test.go") {
		t.Errorf("the stack does not reach the handler that panicked: %q", stack)
	}
	if line["id"] == "" {
		t.Error("the panic line has no request id, so it cannot be matched to a request")
	}
	// And the request is still logged as a request, with the status it ended on.
	if got := only(t, buf, "request")["status"]; got != float64(http.StatusInternalServerError) {
		t.Errorf("the access line says status %v, want 500", got)
	}
}

// A handler that has already answered keeps its answer: the panic is logged,
// but a second status is not written over the first.
func TestRecover_LeavesAnAnswerThatWasAlreadySentAlone(t *testing.T) {
	h, buf := newRouter(t, slog.LevelDebug, func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("half an answer"))
			panic("and then it fell over")
		})
	})

	w := get(h, "/")
	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want the %d the handler had already sent", w.Code, http.StatusAccepted)
	}
	only(t, buf, "a request panicked")
}

// ErrAbortHandler is net/http's own way of saying "drop this connection
// quietly"; it must pass through rather than be reported as a fault.
func TestRecover_PassesErrAbortHandlerOn(t *testing.T) {
	h, buf := newRouter(t, slog.LevelDebug, func(r chi.Router) {
		r.Get("/", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	})

	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recover() = %v, want http.ErrAbortHandler to carry on", v)
		}
		for _, l := range lines(t, buf) {
			if l["msg"] == "a request panicked" {
				t.Error("an aborted connection was reported as a panic")
			}
		}
	}()
	get(h, "/")
}

// The line says which route was asked for, never the path: a path carries the
// room code, which is the key to the game.
func TestAccess_CarriesTheRoutePatternAndNotThePath(t *testing.T) {
	h, buf := newRouter(t, slog.LevelDebug, func(r chi.Router) {
		r.Route("/api", func(r chi.Router) {
			r.Get("/games/{code}/chat", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("four"))
			})
		})
	})

	get(h, "/api/games/AB12CD/chat?name=Ana")

	line := only(t, buf, "request")
	if line["route"] != "/api/games/{code}/chat" {
		t.Errorf("route = %v, want the chi pattern", line["route"])
	}
	if raw := buf.String(); strings.Contains(raw, "AB12CD") || strings.Contains(raw, "Ana") {
		t.Fatalf("the access line carries the path or the query: %q", raw)
	}
	if line["method"] != http.MethodGet || line["status"] != float64(200) || line["bytes"] != float64(4) {
		t.Errorf("the line = %v", line)
	}
	if _, ok := line["dur"]; !ok {
		t.Error("the line has no duration")
	}
}

// A normal server says nothing per request; only the server's own failures
// reach the default level.
func TestAccess_IsQuietAtInfoUntilTheServerItselfFails(t *testing.T) {
	h, buf := newRouter(t, slog.LevelInfo, func(r chi.Router) {
		r.Get("/ok", func(http.ResponseWriter, *http.Request) {})
		r.Get("/refused", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no", http.StatusBadRequest)
		})
		r.Get("/broken", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "sorry", http.StatusInternalServerError)
		})
	})

	get(h, "/ok")
	get(h, "/refused")
	if n := len(lines(t, buf)); n != 0 {
		t.Fatalf("at info a working server logged %d lines: %q", n, buf.String())
	}

	get(h, "/broken")
	line := only(t, buf, "request")
	if line["level"] != "INFO" || line["status"] != float64(500) || line["route"] != "/broken" {
		t.Errorf("the line = %v", line)
	}
}

// A WebSocket is a connection, not a request that finishes, so it is logged
// when it is accepted and again when it closes. (The upgrade itself is
// exercised against the real server in tests/app.)
func TestAccess_LogsAWebSocketWhenItOpensAndWhenItCloses(t *testing.T) {
	h, buf := newRouter(t, slog.LevelDebug, func(r chi.Router) {
		r.Get("/ws/{code}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusSwitchingProtocols)
		})
	})

	get(h, "/ws/AB12CD")

	for _, msg := range []string{"websocket open", "websocket closed"} {
		line := only(t, buf, msg)
		if line["route"] != "/ws/{code}" {
			t.Errorf("%s: route = %v", msg, line["route"])
		}
		if line["id"] == "" {
			t.Errorf("%s: no request id, so the two lines cannot be paired", msg)
		}
	}
	for _, l := range lines(t, buf) {
		if l["msg"] == "request" {
			t.Error("an accepted upgrade was also logged as a finished request")
		}
	}
	if _, ok := only(t, buf, "websocket closed")["dur"]; !ok {
		t.Error("the closing line does not say how long the connection lasted")
	}
}
