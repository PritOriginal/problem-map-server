package nats

import "github.com/prometheus/client_golang/prometheus"

// Results recorded in the "result" label of the event counters.
const (
	ResultOK        = "ok"
	ResultDuplicate = "duplicate"
	ResultError     = "error"
	ResultAck       = "ack"
	ResultNak       = "nak"
	ResultDLQ       = "dlq"
)

// Metrics are the Prometheus collectors of the event pipeline. A nil
// *Metrics is valid and records nothing, so the client works without a
// registry (tests, tools).
type Metrics struct {
	published    *prometheus.CounterVec
	consumed     *prometheus.CounterVec
	redeliveries prometheus.Counter
}

// NewMetrics creates the collectors and registers them on reg (a nil reg
// leaves them unregistered).
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		published: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "events_published_total",
			Help: "Domain events published to the broker by subject and result (ok, duplicate, error).",
		}, []string{"subject", "result"}),
		consumed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "events_consumed_total",
			Help: "Domain events handled by the consumer by subject and result (ack, nak, dlq, error).",
		}, []string{"subject", "result"}),
		redeliveries: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "events_redeliveries_total",
			Help: "Domain events delivered to the consumer more than once.",
		}),
	}
	if reg != nil {
		reg.MustRegister(m.published, m.consumed, m.redeliveries)
	}
	return m
}

func (m *Metrics) recordPublished(subject, result string) {
	if m != nil {
		m.published.WithLabelValues(subject, result).Inc()
	}
}

func (m *Metrics) recordConsumed(subject, result string) {
	if m != nil {
		m.consumed.WithLabelValues(subject, result).Inc()
	}
}

func (m *Metrics) recordRedelivery() {
	if m != nil {
		m.redeliveries.Inc()
	}
}

// Published returns the events_published_total counter (for tests and
// dashboards built in code).
func (m *Metrics) Published() *prometheus.CounterVec { return m.published }

// Consumed returns the events_consumed_total counter.
func (m *Metrics) Consumed() *prometheus.CounterVec { return m.consumed }

// Redeliveries returns the events_redeliveries_total counter.
func (m *Metrics) Redeliveries() prometheus.Counter { return m.redeliveries }
