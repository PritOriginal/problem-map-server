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

func (suite *MetricsSuite) TestMiddlewareAndHandler() {
	m := metrics.New()

	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/metrics", m.Handler())
	r.GET("/items/:id", func(c *gin.Context) { c.Status(http.StatusCreated) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/items/42", nil))
	suite.Equal(http.StatusCreated, w.Code)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/missing", nil))
	suite.Equal(http.StatusNotFound, w.Code)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	suite.Equal(http.StatusOK, w.Code)

	body := w.Body.String()
	suite.Contains(body, `http_requests_total{method="GET",route="/items/:id",status="201"} 1`)
	suite.Contains(body, `http_requests_total{method="GET",route="unknown",status="404"} 1`)
	suite.Contains(body, `http_request_duration_seconds_count{method="GET",route="/items/:id",status="201"} 1`)
}
