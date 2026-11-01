package metrics

import "time"

// mailKinds and mailOutcomes are the whole of the mail counter's labels: a
// value outside them is counted as "other", so no caller can grow the series.
var (
	mailKinds    = map[string]bool{"confirm": true, "reset": true, "turn": true, "notice": true}
	mailOutcomes = map[string]bool{
		"queued": true, "sent": true, "failed": true, "limited": true, "suppressed": true,
		"dropped": true, "stopped": true, "auth_failed": true, "rate_limited": true,
	}
)

// Mail records what became of one mail of a kind: queued, sent, failed (the
// server refused this message), limited (held by the sending limits),
// suppressed (the address asked not to be mailed), dropped (the queue was
// full), stopped (not tried, because sending is paused or stopped),
// auth_failed (the server refused the login) or rate_limited (the server asked
// us to slow down).
func (m *Metrics) Mail(kind, outcome string) {
	if m == nil {
		return
	}
	if !mailKinds[kind] {
		kind = "other"
	}
	if !mailOutcomes[outcome] {
		outcome = "other"
	}
	m.mail.WithLabelValues(kind, outcome).Inc()
}

// MailSend records how long one attempt to hand a message to the mail server took.
func (m *Metrics) MailSend(d time.Duration) {
	if m == nil {
		return
	}
	m.mailSend.Observe(d.Seconds())
}
