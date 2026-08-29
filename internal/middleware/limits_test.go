package middleware_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestMaxBodySize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// readAll reports 200 when the whole body could be read, 413 otherwise.
	readAll := func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusOK)
	}

	r := gin.New()
	r.Use(middleware.MaxBodySize(10))
	r.POST("/small", readAll)
	// The group limit replaces the router-wide one instead of nesting in it.
	r.POST("/large", middleware.MaxBodySize(100), readAll)

	tests := []struct {
		name     string
		path     string
		bodySize int
		want     int
	}{
		{name: "SmallWithinLimit", path: "/small", bodySize: 10, want: http.StatusOK},
		{name: "SmallOverLimit", path: "/small", bodySize: 11, want: http.StatusRequestEntityTooLarge},
		{name: "LargeOverGlobalWithinGroup", path: "/large", bodySize: 50, want: http.StatusOK},
		{name: "LargeOverGroup", path: "/large", bodySize: 101, want: http.StatusRequestEntityTooLarge},
		{name: "NoBody", path: "/small", bodySize: 0, want: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.bodySize > 0 {
				body = bytes.NewBufferString(strings.Repeat("x", tt.bodySize))
			}
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tt.path, body)
			r.ServeHTTP(w, req)
			assert.Equal(t, tt.want, w.Code)
		})
	}
}
