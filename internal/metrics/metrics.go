// Package metrics is what bingle says about itself in Prometheus' text format.
//
// Stage 2 of docs/history/observability-plan.md: the registry, the HTTP
// middleware and the exposition. The database, the games, the bots and the
// session gauges are later stages and are not here yet.
//
// Two rules shape the whole package, and ADR 0064 says why:
//
//   - **The registry is our own**, never prometheus' default one, so /metrics
//     carries exactly what bingle chose to register and nothing a dependency
//     registered behind our back.
//   - **Labels are a whitelist.** Call sites never name a label value
//     themselves: they call a method here, and the method decides. A room
//     code, a player's name, an address, a position or a move must never
//     become a label — a label is published, and a room code is the key to a
//     game (ADR 0063).
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Namespace prefixes every series bingle registers.
const Namespace = "bingle"

// durationBuckets are the buckets of every duration histogram here.
//
// They are chosen once, deliberately, and are not to be changed casually: a
// bucket change makes the quantiles of history recorded before it
// incomparable with those after (see ADR 0064). The range is a millisecond —
// a static file off the embedded filesystem — to thirty seconds, which is
// past the longest bot move, with +Inf above that.
var durationBuckets = []float64{
	0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30,
}

// Metrics owns bingle's registry and the series registered in it. A nil
// *Metrics is usable and records nothing, which is what the server does when
// BINGLE_METRICS_ADDR is empty and what most tests want.
type Metrics struct {
	reg *prometheus.Registry

	httpRequests  *prometheus.CounterVec
	httpDuration  *prometheus.HistogramVec
	httpInFlight  prometheus.Gauge
	httpBytes     *prometheus.CounterVec
	httpErrors    *prometheus.CounterVec
	wsConnections *prometheus.GaugeVec
	wsTotal       *prometheus.CounterVec
}

// New builds the registry, registers the Go runtime and process collectors
// explicitly, and records the build's version.
func New(version string) *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		reg: reg,

		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace, Subsystem: "http", Name: "requests_total",
			Help: "Requests answered, by route pattern, method and status.",
		}, []string{"route", "method", "status"}),

		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace, Subsystem: "http", Name: "request_duration_seconds",
			Help: "How long answering a request took, by route pattern and method.", Buckets: durationBuckets,
		}, []string{"route", "method"}),

		httpInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace, Subsystem: "http", Name: "requests_in_flight",
			Help: "Requests being answered right now.",
		}),

		httpBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace, Subsystem: "http", Name: "response_bytes_total",
			Help: "Response bodies written, in bytes, by route pattern.",
		}, []string{"route"}),

		httpErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace, Subsystem: "http", Name: "errors_total",
			Help: "Requests refused or failed, by route pattern and the family of the refusal.",
		}, []string{"route", "kind"}),

		wsConnections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace, Subsystem: "ws", Name: "connections",
			Help: "WebSocket connections open right now, by kind.",
		}, []string{"kind"}),

		wsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace, Subsystem: "ws", Name: "connections_total",
			Help: "WebSocket connections accepted, by kind.",
		}, []string{"kind"}),
	}

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace, Name: "build_info",
		Help: "Always 1; its label carries the version of the running engine.",
	}, []string{"version"})
	buildInfo.WithLabelValues(version).Set(1)

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		buildInfo,
		m.httpRequests, m.httpDuration, m.httpInFlight, m.httpBytes, m.httpErrors,
		m.wsConnections, m.wsTotal,
	)
	return m
}

// Registry is the registry, for a test that wants to gather it directly.
func (m *Metrics) Registry() *prometheus.Registry {
	if m == nil {
		return nil
	}
	return m.reg
}

// Handler serves the exposition in Prometheus' text format. It is mounted on
// the private listener only, never on the public site, which has no
// authentication of any kind (ADR 0064).
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{
		// A fault in gathering is worth a line in the log and a 500, not a
		// panic in the scraper's request.
		ErrorHandling: promhttp.HTTPErrorOnError,
		Registry:      m.reg,
	})
}
