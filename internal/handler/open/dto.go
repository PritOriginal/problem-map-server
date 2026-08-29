package openrest

// GetStatsRequest is bound from the query string of GET /open/stats.
type GetStatsRequest struct {
	BoundaryID int `form:"boundary_id" binding:"omitempty,min=1"`
}
