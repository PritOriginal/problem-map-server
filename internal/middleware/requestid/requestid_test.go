package requestid_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/middleware/requestid"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type RequestIDSuite struct {
	suite.Suite
}

func TestRequestID(t *testing.T) {
	suite.Run(t, new(RequestIDSuite))
}

func (suite *RequestIDSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
}

func (suite *RequestIDSuite) TestMiddleware() {
	tests := []struct {
		name       string
		incomingID string
		wantSameID bool
	}{
		{name: "GeneratesWhenMissing", incomingID: "", wantSameID: false},
		{name: "PropagatesIncoming", incomingID: "abc-123", wantSameID: true},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			var buf bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&buf, nil))

			var gotGin, gotCtx string
			r := gin.New()
			r.Use(requestid.New(log))
			r.GET("/", func(c *gin.Context) {
				gotGin = requestid.FromGin(c)
				gotCtx = requestid.FromContext(c.Request.Context())
				requestid.Logger(c.Request.Context(), slogdiscard.NewDiscardLogger()).Info("hello")
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.incomingID != "" {
				req.Header.Set(requestid.Header, tt.incomingID)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			respID := w.Header().Get(requestid.Header)
			suite.NotEmpty(respID)
			suite.Equal(respID, gotGin)
			suite.Equal(respID, gotCtx)
			if tt.wantSameID {
				suite.Equal(tt.incomingID, respID)
			} else {
				_, err := uuid.Parse(respID)
				suite.NoError(err)
			}
			suite.Contains(buf.String(), `"request_id":"`+respID+`"`)
		})
	}
}

func (suite *RequestIDSuite) TestHelpersWithoutMiddleware() {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	suite.Empty(requestid.FromContext(req.Context()))

	fallback := slogdiscard.NewDiscardLogger()
	suite.Same(fallback, requestid.Logger(req.Context(), fallback))
}
