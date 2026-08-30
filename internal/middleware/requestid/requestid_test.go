package requestid_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/middleware/requestid"
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
		{name: "RejectsTooLong", incomingID: strings.Repeat("a", 129), wantSameID: false},
		{name: "RejectsControlChars", incomingID: "abc\x1b[31m", wantSameID: false},
		{name: "RejectsSpaces", incomingID: "abc def", wantSameID: false},
		{name: "AcceptsPrintableASCII", incomingID: "req:1/2~x", wantSameID: true},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			var gotGin, gotHeader string
			r := gin.New()
			r.Use(requestid.New())
			r.GET("/", func(c *gin.Context) {
				gotGin = c.GetString(requestid.ContextKey)
				gotHeader = c.GetHeader(requestid.Header)
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
			suite.Equal(respID, gotHeader)
			if tt.wantSameID {
				suite.Equal(tt.incomingID, respID)
			} else {
				_, err := uuid.Parse(respID)
				suite.NoError(err)
			}
		})
	}
}
