// Package metrics provides a Prometheus registry with HTTP request
// collectors, a gin middleware recording them and handlers exposing them.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Path is the route exposing the metrics.
const Path = "/metrics"

const otherMethod = "other"

// knownMethods bounds the cardinality of the "method" label: anything else a
// client sends is recorded as "other".
var knownMethods = map[string]struct{}{
	http.MethodGet: {}, http.MethodHead: {}, http.MethodPost: {}, http.MethodPut: {},
	http.MethodPatch: {}, http.MethodDelete: {}, http.MethodConnect: {},
	http.MethodOptions: {}, http.MethodTrace: {},
}

// Metrics holds the HTTP collectors registered on a Prometheus registry.
type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// New creates HTTP collectors and registers them (together with the default Go
// and process collectors) on a fresh registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		registry: reg,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
	}
	reg.MustRegister(m.requests, m.duration)

	return m
}

// Registry returns the underlying Prometheus registry so that other
// collectors (e.g. gRPC server metrics) can be registered on it.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// Middleware records request count and latency labelled by method, matched
// route template and status code. Unmatched routes are labelled "unknown";
// requests to the routes listed in skip are not recorded.
func (m *Metrics) Middleware(skip ...string) gin.HandlerFunc {
	skipped := make(map[string]struct{}, len(skip))
	for _, p := range skip {
		skipped[p] = struct{}{}
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if _, ok := skipped[route]; ok {
			return
		}
		if route == "" {
			route = "unknown"
		}
		method := c.Request.Method
		if _, ok := knownMethods[method]; !ok {
			method = otherMethod
		}
		labels := prometheus.Labels{
			"method": method,
			"route":  route,
			"status": strconv.Itoa(c.Writer.Status()),
		}

		m.requests.With(labels).Inc()
		m.duration.With(labels).Observe(time.Since(start).Seconds())
	}
}

// HTTPHandler returns the net/http handler serving the registry.
func (m *Metrics) HTTPHandler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Handler returns the gin handler serving the registry.
func (m *Metrics) Handler() gin.HandlerFunc {
	return gin.WrapH(m.HTTPHandler())
}

// Server returns a standalone HTTP server exposing the registry at Path on
// addr, for binaries that have no HTTP router of their own.
func (m *Metrics) Server(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(Path, m.HTTPHandler())

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
