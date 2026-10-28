// Package httpstat wraps an http.ResponseWriter so a middleware can learn
// what a handler answered — the status, and how many bytes went out — without
// the handler knowing anything about it.
//
// It lives in its own package because more than one middleware wants the same
// answer and they must not each wrap the writer again: the access log
// (internal/logging), the metrics (internal/metrics) and, from Stage 4 of
// docs/history/observability-plan.md, the slow-request journal. One Recorder is
// made per request, by the outermost of them, and shared through the request's
// context.
package httpstat

import (
	"context"
	"net/http"
)

type key struct{}

// Recorder is an http.ResponseWriter that remembers the status and the size
// of the response written through it.
//
// It deliberately implements nothing but Unwrap beyond that: net/http's
// ResponseController and the WebSocket upgrade's hijack both follow Unwrap to
// reach the real writer, so flushing and hijacking keep working without this
// type having to know they exist.
type Recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool

	// onUpgrade are called once each, from WriteHeader, when a WebSocket
	// upgrade is accepted (a 101) — see AddOnUpgrade.
	onUpgrade []func()
}

// AddOnUpgrade registers f to be called if this request turns out to be an
// accepted WebSocket upgrade (a 101). Every middleware that cares gets its
// own: a socket is a connection that is starting, not a request that has
// finished, and more than one of them wants to know.
func (rc *Recorder) AddOnUpgrade(f func()) {
	rc.onUpgrade = append(rc.onUpgrade, f)
}

// Wrap returns a Recorder around w, and a context carrying it so middlewares
// further in find the same one instead of wrapping again.
func Wrap(ctx context.Context, w http.ResponseWriter) (*Recorder, context.Context) {
	rec := &Recorder{ResponseWriter: w}
	return rec, context.WithValue(ctx, key{}, rec)
}

// From returns the Recorder ctx carries, or nil when nothing has wrapped the
// writer for this request.
func From(ctx context.Context) *Recorder {
	rec, _ := ctx.Value(key{}).(*Recorder)
	return rec
}

// Status is the status the client saw. A handler that writes nothing at all
// still ends in a 200 from net/http, so that is what this reports.
func (rc *Recorder) Status() int {
	if rc.status == 0 {
		return http.StatusOK
	}
	return rc.status
}

// Bytes is how much of the body was written.
func (rc *Recorder) Bytes() int64 { return rc.bytes }

// Wrote reports whether anything has been sent yet, so a middleware recovering
// from a panic knows not to write over an answer the handler had already given.
func (rc *Recorder) Wrote() bool { return rc.wrote }

func (rc *Recorder) WriteHeader(status int) {
	if rc.wrote {
		rc.ResponseWriter.WriteHeader(status) // let net/http complain, as it would without us
		return
	}
	rc.status = status
	rc.wrote = true
	rc.ResponseWriter.WriteHeader(status)
	if status == http.StatusSwitchingProtocols {
		for _, f := range rc.onUpgrade {
			f()
		}
	}
}

func (rc *Recorder) Write(b []byte) (int, error) {
	if !rc.wrote {
		rc.WriteHeader(http.StatusOK) // what net/http does on a first Write
	}
	n, err := rc.ResponseWriter.Write(b)
	rc.bytes += int64(n)
	return n, err
}

// Unwrap returns the wrapped writer, for http.ResponseController and for the
// WebSocket upgrade's hijack.
func (rc *Recorder) Unwrap() http.ResponseWriter { return rc.ResponseWriter }
