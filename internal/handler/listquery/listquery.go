// Package listquery holds the query-string DTOs shared by list endpoints.
package listquery

import (
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
)

// Pagination is bound from ?limit=&offset=. Embed it in a request DTO and
// bind with ShouldBindQuery; limit must be in 1..models.MaxLimit and
// defaults to models.DefaultLimit when omitted. Limit is a pointer so that
// an explicit limit=0 is rejected instead of silently becoming the default.
type Pagination struct {
	Limit  *int `form:"limit" binding:"omitempty,min=1,max=500"`
	Offset int  `form:"offset" binding:"omitempty,min=0"`
}

// Model converts the DTO to the domain value with defaults applied.
func (p Pagination) Model() models.Pagination {
	limit := 0
	if p.Limit != nil {
		limit = *p.Limit
	}
	return models.Pagination{Limit: limit, Offset: p.Offset}.WithDefaults()
}

// Meta builds the response meta for a page fetched with p.
func Meta(p models.Pagination, total int) responses.ListMeta {
	return responses.ListMeta{Limit: p.Limit, Offset: p.Offset, Total: total}
}
