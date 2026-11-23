package metrics

import "time"

// translateOutcomes are the whole of the translation counter's labels: a value
// outside them is counted as "other".
var translateOutcomes = map[string]bool{
	"translated": true, "same": true, "skipped": true, "rejected": true,
	"failed": true, "dropped": true, "paused": true,
}

// EnableTranslation registers the translation series: the server has a
// translation service. Until then they are recorded into but not exposed.
func (m *Metrics) EnableTranslation() {
	if m == nil {
		return
	}
	m.translateOnce.Do(func() { m.reg.MustRegister(m.translations, m.translateReq) })
}

// Translation records what became of one chat line asked to be put into a
// reader's language (ADR 0143).
func (m *Metrics) Translation(outcome string) {
	if m == nil {
		return
	}
	if !translateOutcomes[outcome] {
		outcome = "other"
	}
	m.translations.WithLabelValues(outcome).Inc()
}

// TranslationRequest records how long one request to the translation service
// took.
func (m *Metrics) TranslationRequest(d time.Duration) {
	if m == nil {
		return
	}
	m.translateReq.Observe(d.Seconds())
}
