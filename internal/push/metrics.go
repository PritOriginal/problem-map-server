package push

import (
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/prometheus/client_golang/prometheus"
)

// Result labels of the push_sent_total metric.
const (
	ResultOK           = "ok"
	ResultInvalidToken = "invalid_token"
	ResultError        = "error"
	ResultUnsupported  = "unsupported"
)

// Metrics counts push deliveries by platform and result.
type Metrics struct {
	sent *prometheus.CounterVec
}

// NewMetrics creates the push_sent_total counter and registers it on reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		sent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "push_sent_total",
			Help: "Push notifications sent, by platform and result (ok, invalid_token, error, unsupported).",
		}, []string{"platform", "result"}),
	}
	reg.MustRegister(m.sent)
	return m
}

// PushSent records one delivery attempt.
func (m *Metrics) PushSent(platform models.DevicePlatform, result string) {
	m.sent.WithLabelValues(string(platform), result).Inc()
}
