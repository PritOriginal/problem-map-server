// Package listquery holds the query-string DTOs and the small helpers
// shared by list endpoints: binding, error mapping and response meta.
package listquery

import (
	"errors"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

// Pagination is bound from ?limit=&offset=. Embed it in a request DTO and
// bind with Bind; limit defaults to models.DefaultLimit when omitted and
// must otherwise be in 1..models.MaxLimit, so an explicit limit=0 is
// rejected instead of silently becoming the default.
type Pagination struct {
	Limit  int `form:"limit,default=100" binding:"min=1,max=500"`
	Offset int `form:"offset" binding:"min=0"`
}

// Model converts the DTO to the domain value.
func (p Pagination) Model() models.Pagination {
	return models.Pagination{Limit: p.Limit, Offset: p.Offset}
}

// Bind binds the query string into req. On failure it writes a 400 and
// returns false so the handler can simply return.
func Bind(c *gin.Context, log *slog.Logger, req any) bool {
	if err := c.ShouldBindQuery(req); err != nil {
		log.Debug("failed parse query params", logger.Err(err))
		responses.BadRequest(c, "invalid query params")
		return false
	}
	return true
}

// BindPagination binds a bare ?limit=&offset= query. On failure it writes
// a 400 and returns false.
func BindPagination(c *gin.Context, log *slog.Logger) (models.Pagination, bool) {
	var q Pagination
	if !Bind(c, log, &q) {
		return models.Pagination{}, false
	}
	return q.Model(), true
}

// Fail writes the response for a failed list usecase call: 400 for
// usecase.ErrInvalidArgument, otherwise 500 with msg.
func Fail(c *gin.Context, log *slog.Logger, err error, msg string, attrs ...any) {
	if errors.Is(err, usecase.ErrInvalidArgument) {
		log.Debug("invalid query params", append(attrs, logger.Err(err))...)
		responses.BadRequest(c, "invalid query params")
		return
	}
	log.Error(msg, append(attrs, logger.Err(err))...)
	responses.Internal(c, msg)
}

// OK writes a 200 with the page's items (wrapped by the caller into the
// endpoint's response type) and pagination meta.
func OK[T any](c *gin.Context, data T, p models.Pagination, total int) {
	responses.OKList(c, data, responses.ListMeta{Limit: p.Limit, Offset: p.Offset, Total: total})
}
