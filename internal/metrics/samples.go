package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Sample is one series as it stood when the registry was gathered. It is the
// shape internal/journal stores, defined here so that only this package has
// to know what Prometheus' gathered form looks like.
type Sample struct {
	// Name is the series, with the suffix a histogram's parts carry
	// ("..._bucket", "..._sum", "..._count").
	Name string
	// Labels is the canonical form: pairs sorted by key, joined with commas,
	// "" for none. A histogram bucket carries its boundary as "le".
	Labels string
	Value  float64
}

// Samples gathers the registry and flattens it to one value per series.
//
// Counters come out as their monotonic total and gauges as their value, so a
// rate is worked out on read exactly as Prometheus does it and a restart shows
// as the total returning to zero rather than as a spike. A histogram comes out
// as its _count, its _sum and one row per bucket, which is enough to derive a
// quantile afterwards.
func (m *Metrics) Samples() ([]Sample, error) {
	if m == nil {
		return nil, nil
	}
	families, err := m.reg.Gather()
	if err != nil {
		return nil, fmt.Errorf("metrics: gathering the registry: %w", err)
	}

	var out []Sample
	for _, mf := range families {
		name := mf.GetName()
		for _, metric := range mf.GetMetric() {
			labels := canonicalLabels(metric.GetLabel(), "", "")

			// Which kind it is, asked by what the message actually carries
			// rather than by its declared type: that keeps this file free of
			// the generated protobuf package.
			switch {
			case metric.GetCounter() != nil:
				out = append(out, Sample{Name: name, Labels: labels, Value: metric.GetCounter().GetValue()})
			case metric.GetGauge() != nil:
				out = append(out, Sample{Name: name, Labels: labels, Value: metric.GetGauge().GetValue()})
			case metric.GetUntyped() != nil:
				out = append(out, Sample{Name: name, Labels: labels, Value: metric.GetUntyped().GetValue()})
			case metric.GetHistogram() != nil:
				h := metric.GetHistogram()
				out = append(out,
					Sample{Name: name + "_count", Labels: labels, Value: float64(h.GetSampleCount())},
					Sample{Name: name + "_sum", Labels: labels, Value: h.GetSampleSum()},
				)
				for _, b := range h.GetBucket() {
					le := strconv.FormatFloat(b.GetUpperBound(), 'g', -1, 64)
					out = append(out, Sample{
						Name:   name + "_bucket",
						Labels: canonicalLabels(metric.GetLabel(), "le", le),
						Value:  float64(b.GetCumulativeCount()),
					})
				}
			case metric.GetSummary() != nil:
				s := metric.GetSummary()
				out = append(out,
					Sample{Name: name + "_count", Labels: labels, Value: float64(s.GetSampleCount())},
					Sample{Name: name + "_sum", Labels: labels, Value: s.GetSampleSum()},
				)
			}
		}
	}
	return out, nil
}

// labelled is a gathered label pair. It is a type parameter rather than the
// generated protobuf type so that this file — and so the module — does not
// take prometheus/client_model as a dependency of its own for the sake of
// naming one struct; the type is inferred from what Gather returns.
type labelled interface {
	GetName() string
	GetValue() string
}

// canonicalLabels renders a gathered label set the one way the history stores
// it, optionally with one extra pair (a histogram bucket's "le").
//
// The pairs are the ones Prometheus gathered, so this never invents a label;
// what may be a label at all is ADR 0064's business.
func canonicalLabels[P labelled](pairs []P, extraKey, extraValue string) string {
	type kv struct{ k, v string }
	all := make([]kv, 0, len(pairs)+1)
	for _, p := range pairs {
		all = append(all, kv{p.GetName(), p.GetValue()})
	}
	if extraKey != "" {
		all = append(all, kv{extraKey, extraValue})
	}
	if len(all) == 0 {
		return ""
	}
	sort.Slice(all, func(i, j int) bool { return all[i].k < all[j].k })

	var b strings.Builder
	for i, p := range all {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(p.k)
		b.WriteByte('=')
		b.WriteString(p.v)
	}
	return b.String()
}
