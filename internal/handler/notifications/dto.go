package notificationsrest

import (
	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
)

// GetNotificationsRequest is bound from the query string of GET /notifications.
type GetNotificationsRequest struct {
	listquery.Pagination
	// Unread keeps only unread notifications when true.
	Unread bool `form:"unread"`
}

// Filters converts the request to the domain filters.
func (r GetNotificationsRequest) Filters() models.GetNotificationsFilters {
	return models.GetNotificationsFilters{UnreadOnly: r.Unread, Pagination: r.Model()}
}

type GetNotificationsResponse struct {
	Notifications []models.Notification `json:"notifications"`
}

type UnreadCountResponse struct {
	Count int `json:"count"`
}

type MarkAllReadResponse struct {
	// Updated is the number of notifications marked as read.
	Updated int64 `json:"updated"`
}

// RegisterDeviceRequest registers a push token of the current user.
type RegisterDeviceRequest struct {
	Platform models.DevicePlatform `json:"platform" binding:"required,oneof=android ios web" enums:"android,ios,web"`
	Token    string                `json:"token" binding:"required,max=4096"`
}

type RegisterDeviceResponse struct {
	Device models.UserDevice `json:"device"`
}
