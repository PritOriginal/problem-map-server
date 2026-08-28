package responses

import (
	"errors"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/gin-gonic/gin"
)

// Error messages returned to clients for the corresponding HTTP statuses.
const (
	MsgBadRequest   = "bad request"
	MsgUnauthorized = "unauthorized"
	MsgForbidden    = "forbidden"
	MsgNotFound     = "not found"
	MsgConflict     = "conflict"
	MsgInternal     = "internal server error"
)

// FromError maps a usecase error to an HTTP response. Well-known usecase and
// handler errors become 4xx responses with a generic message; everything else
// is logged with op and reported as 500 without details.
func FromError(c *gin.Context, log *slog.Logger, op string, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		NotFound(c, MsgNotFound)
	case errors.Is(err, usecase.ErrConflict):
		Conflict(c, MsgConflict)
	case errors.Is(err, usecase.ErrUnauthorized):
		Unauthorized(c, MsgUnauthorized)
	case errors.Is(err, usecase.ErrForbidden):
		Forbidden(c, MsgForbidden)
	case errors.Is(err, handlers.ErrInvalidPhoto), errors.Is(err, handlers.ErrBadRequest):
		BadRequest(c, MsgBadRequest)
	default:
		log.Error(op, slogger.Err(err))
		Internal(c, MsgInternal)
	}
}
