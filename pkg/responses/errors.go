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
	MsgTooManyReq   = "too many requests"
	MsgInternal     = "internal server error"
)

// FromError maps a usecase error to an HTTP response. Well-known usecase and
// handler errors become 4xx responses with a generic message and are logged
// at debug level; everything else is logged as an error with op and reported
// as 500 without details.
func FromError(c *gin.Context, log *slog.Logger, op string, err error) {
	var respond func(*gin.Context, string)
	var msg string

	kind := usecase.Kind(err)
	// Handler-level input errors are not usecase errors but are 400 as well.
	if errors.Is(err, handlers.ErrInvalidPhoto) || errors.Is(err, handlers.ErrBadRequest) {
		kind = usecase.KindInvalidArgument
	}

	switch kind {
	case usecase.KindNotFound:
		respond, msg = NotFound, MsgNotFound
	case usecase.KindConflict:
		respond, msg = Conflict, MsgConflict
	case usecase.KindUnauthorized:
		respond, msg = Unauthorized, MsgUnauthorized
	case usecase.KindForbidden:
		respond, msg = Forbidden, MsgForbidden
	case usecase.KindTooManyRequests:
		respond, msg = TooManyRequests, MsgTooManyReq
	case usecase.KindInvalidArgument:
		respond, msg = BadRequest, MsgBadRequest
	default:
		// KindInternal and KindUnavailable: logged with op, reported as 500
		// without details.
		log.Error(op, slogger.Err(err))
		Internal(c, MsgInternal)
		return
	}

	log.Debug(op, slogger.Err(err))
	respond(c, msg)
}
