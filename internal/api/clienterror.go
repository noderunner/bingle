package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/journal"
)

// POST /api/client-error — what went wrong in a browser.
//
// This is the one observability endpoint on the **public** listener, because a
// browser has to be able to reach it. Everything about it is shaped by that:
//
//   - it is **write-only**. Nothing here reads a fault back; the table is on
//     the private listener's Health page.
//   - **every field is validated against a closed set or a cap**, because
//     `page` becomes a metric label and a stranger is posting it. An unknown
//     page is "other" rather than a new series (ADR 0064).
//   - it is **rate limited per address** and capped per body, so it cannot be
//     used to fill the disk.
//   - **token-shaped strings are scrubbed** before the row is written, because
//     a stack trace from a page that holds a seat token may quote it.
//   - `BINGLE_CLIENT_ERRORS=off` removes the route altogether.

// Caps. A real stack from bingle's own modules is a few hundred bytes; these
// are generous enough for a minified frame and mean enough to be a cap.
const (
	maxClientErrorBody    = 8 << 10
	maxClientErrorMessage = 500
	maxClientErrorStack   = 4000
	maxClientErrorBrowser = 200
	maxClientErrorVersion = 40
)

// How often one address may report. A browser's own limit is a few per
// session; this is the server not taking that on trust.
const (
	clientErrorWindow = time.Hour
	clientErrorBurst  = 20
)

// clientErrorPages is the closed set of pages a report may name. The browser
// sends the pattern rather than its URL — a URL carries the room code — and
// anything not in this list becomes "other", so the label cannot grow.
var clientErrorPages = map[string]bool{
	"/":               true,
	"/game/{code}":    true,
	"/profile":        true,
	"/batches":        true,
	"/batches/{id}":   true,
	"/notify/confirm": true,
	"/notify/stop":    true,
	"other":           true,
}

// versionish is what a version may look like, so the label stays a closed set
// in practice: a release is "0.16.0".
var versionish = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)

// tokenish matches the shapes bingle's own secrets take: a long run of
// base64url or hex. A stack trace quoting a URL or a variable may carry one,
// and it must not reach the row.
var tokenish = regexp.MustCompile(`[A-Za-z0-9_-]{24,}`)

// roomPath matches a path that names a room or a batch. A room code is six
// characters of letters and digits — far too short to recognise on its own
// without mangling ordinary words — but in a path it is unmistakable, and a
// path is how it gets into a message or a stack frame in the first place.
//
// errors.js takes its own room code out before sending. This is the server
// not taking that on trust, because the endpoint is public and a report can
// come from anything.
var roomPath = regexp.MustCompile(`(/(?:game|batches)/)[A-Za-z0-9_-]+`)

// clientErrorLimiter counts reports per address, in memory. It is the same
// shape as the chat limiter: a map of recent times, pruned on use.
type clientErrorLimiter struct {
	mu   sync.Mutex
	seen map[string][]time.Time
}

func (l *clientErrorLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil {
		l.seen = map[string][]time.Time{}
	}
	recent := l.seen[key][:0]
	for _, t := range l.seen[key] {
		if now.Sub(t) < clientErrorWindow {
			recent = append(recent, t)
		}
	}
	// Addresses nobody has heard from in the window stop being remembered,
	// so a busy server does not grow a map of every visitor for ever.
	if len(recent) == 0 {
		delete(l.seen, key)
	} else {
		l.seen[key] = recent
	}
	if len(recent) >= clientErrorBurst {
		return false
	}
	l.seen[key] = append(recent, now)
	return true
}

// clientErrorReport is what a browser posts.
type clientErrorReport struct {
	Message string `json:"message"`
	Stack   string `json:"stack"`
	Page    string `json:"page"`
	Version string `json:"version"`
	Browser string `json:"browser"`
}

// handlePostClientError implements POST /api/client-error.
//
// It answers 204 whatever happens to the report — a browser can do nothing
// useful with a refusal, and telling a stranger which of their reports were
// kept is information this endpoint has no reason to give out. What happened
// goes in the log.
func (h *handlers) handlePostClientError(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)

	lg := h.logger()
	if !h.clientErrors.allow(clientIP(r), time.Now()) {
		lg.Debug("a client error was refused: too many from one address")
		h.metrics.ClientError("refused")
		return
	}

	var rep clientErrorReport
	if err := json.NewDecoder(io.LimitReader(r.Body, maxClientErrorBody)).Decode(&rep); err != nil {
		lg.Debug("a client error would not decode", "err", err)
		h.metrics.ClientError("refused")
		return
	}

	message := scrubClientText(rep.Message, maxClientErrorMessage)
	if message == "" {
		h.metrics.ClientError("refused")
		return
	}
	e := journal.ClientError{
		Message:  message,
		Stack:    scrubClientText(rep.Stack, maxClientErrorStack),
		Page:     clientErrorPage(rep.Page),
		Version:  clientErrorVersion(rep.Version),
		Browser:  scrubClientText(rep.Browser, maxClientErrorBrowser),
		LastSeen: time.Now(),
	}
	e.Hash = journal.ClientErrorHash(e.Message, e.Stack, e.Version)

	h.metrics.ClientError(e.Page)
	if h.journal == nil {
		// Counted but not kept: a server with no metrics database.
		return
	}
	count, err := h.journal.RecordClientError(r.Context(), e)
	if err != nil {
		lg.Error("a client error could not be recorded", "err", err)
		return
	}
	// The first sighting of a fault is worth a line; the four hundredth is
	// not, and a bad release makes four hundred.
	if count == 1 {
		lg.Warn("something went wrong in a browser",
			"page", e.Page, "version", e.Version, "message", e.Message, "hash", e.Hash)
	} else {
		lg.Debug("a browser hit a fault already known",
			"page", e.Page, "hash", e.Hash, "count", count)
	}
}

// clientErrorPage keeps the page label a closed set.
func clientErrorPage(page string) string {
	if clientErrorPages[page] {
		return page
	}
	return "other"
}

// clientErrorVersion keeps the version label a closed set in practice.
func clientErrorVersion(version string) string {
	version = strings.TrimSpace(version)
	if len(version) > maxClientErrorVersion || !versionish.MatchString(version) {
		return "unknown"
	}
	return version
}

// scrubClientText caps a field and takes out anything shaped like one of
// bingle's secrets.
//
// It is deliberately blunt: a long run of base64url or hex becomes
// "[redacted]", which occasionally eats a minified identifier or a hash in a
// filename, and a path naming a room or a batch loses that name. Losing a
// frame's name is a smaller harm than publishing a seat token (ADR 0063), and
// the message usually says enough on its own.
//
// What it cannot catch is a bare room code with nothing around it: six
// characters of letters and digits is any short word. A client that sends one
// anyway gets it stored, which is the residual risk, and the reason errors.js
// strips its own.
func scrubClientText(s string, max int) string {
	s = strings.TrimSpace(s)
	// Control characters would make a log line or the page lie about its own
	// shape.
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, s)
	s = tokenish.ReplaceAllString(s, "[redacted]")
	s = roomPath.ReplaceAllString(s, "${1}{code}")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// logger is the handlers' logger, or slog's default.
func (h *handlers) logger() *slog.Logger {
	if h.log == nil {
		return slog.Default()
	}
	return h.log
}
