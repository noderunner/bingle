package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/noderunner/bingle/internal/httpstat"
	"github.com/noderunner/bingle/internal/logging"
)

// Middleware records every request: how many, how long, how big, and how many
// were refused. A nil *Metrics returns the handler untouched, so a server with
// metrics switched off pays nothing at all.
//
// It records the **route pattern**, never the path — a path carries the room
// code, and a label is published (ADR 0063, ADR 0064). logging.Route is the
// one definition of that rule, so this uses it rather than keeping a second.
//
// It must be registered inside logging.Access, which wraps the writer this
// reads the status off, and outside logging.Recover, so that a request that
// panicked is still counted as the 500 it became.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.httpInFlight.Inc()
		start := time.Now()

		rec := httpstat.From(r.Context())
		if rec != nil {
			// The route is not known until chi has matched, which it has by
			// the time a handler accepts the upgrade.
			rec.AddOnUpgrade(func() { m.WSOpen(wsKind(r)) })
		}

		// Deferred, not written after the call: a panic that passes through
		// this middleware rather than being recovered under it —
		// http.ErrAbortHandler is re-panicked on purpose — must not leave a
		// request counted as in flight for ever, or a socket counted as open
		// after it has gone. A gauge that only ever climbs is worse than a
		// counter that misses one request.
		defer func() {
			m.httpInFlight.Dec()

			route := logging.Route(r)
			status := http.StatusOK
			var bytes int64
			if rec != nil {
				status = rec.Status()
				bytes = rec.Bytes()
			}

			// An accepted upgrade is a connection that has only just started,
			// so counting it as a request answered in however long the player
			// stayed on the page would make every duration meaningless. A
			// socket has its own series instead.
			if status == http.StatusSwitchingProtocols {
				m.WSClose(wsKind(r))
				return
			}

			m.httpRequests.WithLabelValues(route, r.Method, strconv.Itoa(status)).Inc()
			m.httpDuration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
			if bytes > 0 {
				m.httpBytes.WithLabelValues(route).Add(float64(bytes))
			}
			if kind := errorKind(status); kind != "" {
				m.httpErrors.WithLabelValues(route, kind).Inc()
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// errorKind names the family a refusal belongs to, or "" when the request was
// answered normally.
//
// The names are the families internal/api's statusForError already sorts
// errors into, read back off the status so that the counter needs no
// cooperation from the handlers — and so that a refusal written by anything
// else (chi's own 404, a body too large) is counted the same way. The set is
// closed, which is what keeps the label's cardinality a known number.
func errorKind(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad-request"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not-found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusTooManyRequests:
		return "rate-limited"
	}
	switch {
	case status >= 500:
		return "server"
	case status >= 400:
		return "client"
	}
	return ""
}

// WSKind is which of bingle's four sockets a connection is. The set is closed
// on purpose: it is a label.
type WSKind string

// The sockets: a game's page, a voice call, the global chat and the direct
// messages.
const (
	WSPage   WSKind = "page"
	WSVoice  WSKind = "voice"
	WSChat   WSKind = "chat"
	WSDirect WSKind = "dm"
)

// wsKind is which socket a request is for, by the route it matched. The voice
// call shares the room's route and is told apart by the query flag the handler
// itself reads ("only=voice"), because counting a voice socket as somebody
// looking at the game would make bingle_ws_connections{kind="page"} wrong.
func wsKind(r *http.Request) WSKind {
	switch logging.Route(r) {
	case "/ws-chat":
		return WSChat
	case "/ws-dm":
		return WSDirect
	default:
		if r.URL.Query().Get("only") == "voice" {
			return WSVoice
		}
		return WSPage
	}
}

// WSOpen records a socket that has just been accepted.
func (m *Metrics) WSOpen(kind WSKind) {
	if m == nil {
		return
	}
	m.wsConnections.WithLabelValues(string(kind)).Inc()
	m.wsTotal.WithLabelValues(string(kind)).Inc()
}

// WSClose records a socket that has closed.
//
// A room socket's handler is parked on the room's event channel rather than on
// the connection, so it notices a player who has gone only when the room next
// has something to say: the gauge falls a little later than the player did,
// for the same reason the access log's closing line arrives late. Stage 4,
// which instruments the handlers themselves, is where that gets tightened.
func (m *Metrics) WSClose(kind WSKind) {
	if m == nil {
		return
	}
	m.wsConnections.WithLabelValues(string(kind)).Dec()
}
