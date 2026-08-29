package responses_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

type ResponsesSuite struct {
	suite.Suite
}

func TestResponses(t *testing.T) {
	suite.Run(t, new(ResponsesSuite))
}

func (suite *ResponsesSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
}

func (suite *ResponsesSuite) TestFromError() {
	tests := []struct {
		name       string
		err        error
		statusCode int
		message    string
	}{
		{
			name:       "NotFound404",
			err:        fmt.Errorf("op: %w", usecase.ErrNotFound),
			statusCode: http.StatusNotFound,
			message:    responses.MsgNotFound,
		},
		{
			name:       "Conflict409",
			err:        usecase.ErrConflict,
			statusCode: http.StatusConflict,
			message:    responses.MsgConflict,
		},
		{
			name:       "Unauthorized401",
			err:        usecase.ErrUnauthorized,
			statusCode: http.StatusUnauthorized,
			message:    responses.MsgUnauthorized,
		},
		{
			name:       "Forbidden403",
			err:        usecase.ErrForbidden,
			statusCode: http.StatusForbidden,
			message:    responses.MsgForbidden,
		},
		{
			name:       "TooManyRequests429",
			err:        usecase.ErrTooManyRequests,
			statusCode: http.StatusTooManyRequests,
			message:    responses.MsgTooManyReq,
		},
		{
			name:       "InvalidPhoto400",
			err:        fmt.Errorf("%w: too big", handlers.ErrInvalidPhoto),
			statusCode: http.StatusBadRequest,
			message:    responses.MsgBadRequest,
		},
		{
			name:       "BadRequest400",
			err:        fmt.Errorf("%w: bad id", handlers.ErrBadRequest),
			statusCode: http.StatusBadRequest,
			message:    responses.MsgBadRequest,
		},
		{
			name:       "Unknown500",
			err:        errors.New("db is down"),
			statusCode: http.StatusInternalServerError,
			message:    responses.MsgInternal,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			responses.FromError(c, slogdiscard.NewDiscardLogger(), "op", tt.err)

			suite.Equal(tt.statusCode, w.Code)
			suite.JSONEq(fmt.Sprintf(`{"success":false,"error":{"message":%q}}`, tt.message), w.Body.String())
		})
	}
}
