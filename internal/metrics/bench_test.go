// What the recording costs on the request path. The plan's budget is under 1%
// on a move; a move is an API request, so this is where to look. Stage 2 adds
// nothing to Legal, Apply or the batch runner, so there is nothing to measure
// there yet.

package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/httpstat"
	"github.com/noderunner/bingle/internal/metrics"
)

// A handler about as cheap as bingle has, so the middleware is as large a
// share of the measurement as it can be.
var benchHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(`{"status":"ok"}`))
})

func benchRouter(use func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	use(r)
	r.Get("/api/games/{code}/moves", benchHandler)
	return r
}

func wrapWriter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec, ctx := httpstat.Wrap(r.Context(), w)
		next.ServeHTTP(rec, r.WithContext(ctx))
	})
}

func benchServe(b *testing.B, h http.Handler) {
	b.ReportAllocs()
	req := httptest.NewRequest(http.MethodPost, "/api/games/AB12CD/moves", nil)
	w := httptest.NewRecorder()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, req)
	}
}

// The route and the handler, with nothing watching.
func BenchmarkRequest_Bare(b *testing.B) {
	benchServe(b, benchRouter(func(chi.Router) {}))
}

// The wrapped writer alone: what Stage 1's access log already costs.
func BenchmarkRequest_WrappedWriter(b *testing.B) {
	benchServe(b, benchRouter(func(r chi.Router) { r.Use(wrapWriter) }))
}

// With metrics switched off, which is a server with no BINGLE_METRICS_ADDR.
func BenchmarkRequest_MetricsOff(b *testing.B) {
	var m *metrics.Metrics
	benchServe(b, benchRouter(func(r chi.Router) {
		r.Use(wrapWriter)
		r.Use(m.Middleware)
	}))
}

// And with it recording, which is what Stage 2 adds.
func BenchmarkRequest_MetricsOn(b *testing.B) {
	m := metrics.New("1.2.3")
	benchServe(b, benchRouter(func(r chi.Router) {
		r.Use(wrapWriter)
		r.Use(m.Middleware)
	}))
}
