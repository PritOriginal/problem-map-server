package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/middleware/metrics"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

type MetricsSuite struct {
	suite.Suite
}

func TestMetrics(t *testing.T) {
	suite.Run(t, new(MetricsSuite))
}

func (suite *MetricsSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
}

func (suite *MetricsSuite) TestMiddleware() {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantSeries string // substring expected in the scrape; empty = must be absent
		absent     string
	}{
		{
			name: "MatchedRoute", method: http.MethodGet, path: "/items/42", wantStatus: http.StatusOK,
			wantSeries: `http_requests_total{method="GET",route="/items/:id",status="200"} 1`,
		},
		{
			name: "UnknownRoute", method: http.MethodGet, path: "/missing", wantStatus: http.StatusNotFound,
			wantSeries: `http_requests_total{method="GET",route="unknown",status="404"} 1`,
		},
		{
			name: "NonStandardMethod", method: "BREW", path: "/items/1", wantStatus: http.StatusNotFound,
			wantSeries: `http_requests_total{method="other",route="unknown",status="404"} 1`,
		},
		{
			name: "ScrapeNotRecorded", method: http.MethodGet, path: metrics.Path, wantStatus: http.StatusOK,
			absent: `route="` + metrics.Path + `"`,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			m := metrics.New()
			r := gin.New()
			r.Use(m.Middleware(metrics.Path))
			r.GET(metrics.Path, m.Handler())
			r.GET("/items/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			suite.Equal(tt.wantStatus, w.Code)

			scrape := httptest.NewRecorder()
			r.ServeHTTP(scrape, httptest.NewRequest(http.MethodGet, metrics.Path, nil))
			suite.Equal(http.StatusOK, scrape.Code)
			body := scrape.Body.String()
			suite.Contains(body, "go_goroutines")
			if tt.wantSeries != "" {
				suite.Contains(body, tt.wantSeries)
				suite.Contains(body, "http_request_duration_seconds_count")
			}
			if tt.absent != "" {
				suite.NotContains(body, tt.absent)
			}
		})
	}
}

func (suite *MetricsSuite) TestServer() {
	m := metrics.New()
	srv := m.Server(":0")

	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, metrics.Path, nil))

	suite.Equal(http.StatusOK, w.Code)
	suite.Contains(w.Body.String(), "go_goroutines")
}
