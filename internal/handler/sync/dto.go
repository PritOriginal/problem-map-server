package syncrest

import (
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
)

// GetUserSyncRequest is bound from the query string of GET /users/me/sync.
type GetUserSyncRequest struct {
	listquery.Pagination
	Since string `form:"since" binding:"required"`
}

// Filters parses since (RFC3339). Returned errors are safe to show.
func (r GetUserSyncRequest) Filters() (models.UserSyncFilters, error) {
	since, err := time.Parse(time.RFC3339, r.Since)
	if err != nil {
		return models.UserSyncFilters{}, fmt.Errorf("since must be RFC3339")
	}
	filters := models.UserSyncFilters{Since: since, Pagination: r.Model()}
	if err := filters.Validate(); err != nil {
		return models.UserSyncFilters{}, err
	}
	return filters, nil
}

// GetUserSyncResponse is the payload of GET /users/me/sync.
type GetUserSyncResponse struct {
	Tasks         []models.Task         `json:"tasks"`
	Notifications []models.Notification `json:"notifications"`
	Checks        []models.Check        `json:"checks"`
	Totals        models.UserSyncTotals `json:"totals"`
	ServerTime    time.Time             `json:"server_time"`
}

func NewGetUserSyncResponse(s models.UserSync) GetUserSyncResponse {
	resp := GetUserSyncResponse{
		Tasks:         s.Tasks,
		Notifications: s.Notifications,
		Checks:        s.Checks,
		Totals:        s.Totals,
		ServerTime:    s.ServerTime,
	}
	// Arrays, never null, so clients can iterate without nil checks.
	if resp.Tasks == nil {
		resp.Tasks = []models.Task{}
	}
	if resp.Notifications == nil {
		resp.Notifications = []models.Notification{}
	}
	if resp.Checks == nil {
		resp.Checks = []models.Check{}
	}
	return resp
}
