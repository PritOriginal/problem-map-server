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
// handler errors become 4xx responses with a generic message and are logged
// at debug level; everything else is logged as an error with op and reported
// as 500 without details.
func FromError(c *gin.Context, log *slog.Logger, op string, err error) {
	var respond func(*gin.Context, string)
	var msg string

	switch {
	case errors.Is(err, usecase.ErrNotFound):
		respond, msg = NotFound, MsgNotFound
	case errors.Is(err, usecase.ErrConflict):
		respond, msg = Conflict, MsgConflict
	case errors.Is(err, usecase.ErrUnauthorized):
		respond, msg = Unauthorized, MsgUnauthorized
	case errors.Is(err, usecase.ErrForbidden):
		respond, msg = Forbidden, MsgForbidden
	case errors.Is(err, handlers.ErrInvalidPhoto), errors.Is(err, handlers.ErrBadRequest):
		respond, msg = BadRequest, MsgBadRequest
	default:
		log.Error(op, slogger.Err(err))
		Internal(c, MsgInternal)
		return
	}

	log.Debug(op, slogger.Err(err))
	respond(c, msg)
}
