package marksrest

import (
	"fmt"
	"mime/multipart"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
)

type GetMarkByIdResponse struct {
	Mark models.Mark `json:"mark"`
}

// GetMarksRequest is bound from the query string of GET /marks.
type GetMarksRequest struct {
	listquery.Pagination
	MarkTypeIds   string `form:"mark_type_ids"`
	MarkStatusIds string `form:"mark_status_ids"`
	UserID        int    `form:"user_id" binding:"omitempty,min=1"`
	// BBox is "minLon,minLat,maxLon,maxLat".
	BBox  string `form:"bbox"`
	Sort  string `form:"sort" binding:"omitempty,oneof=created_at updated_at"`
	Order string `form:"order" binding:"omitempty,oneof=asc desc"`
	// CreatedFrom / CreatedTo are RFC3339 timestamps.
	CreatedFrom string `form:"created_from"`
	CreatedTo   string `form:"created_to"`
}

// Filters converts the request to domain filters, parsing the list, bbox
// and time fields. Returned errors are safe to show to the client.
func (r GetMarksRequest) Filters() (models.GetMarksFilters, error) {
	markTypeIds, err := handlers.ParseIntArray(r.MarkTypeIds)
	if err != nil {
		return models.GetMarksFilters{}, fmt.Errorf("failed parse mark type ids")
	}
	markStatusIds, err := handlers.ParseIntArray(r.MarkStatusIds)
	if err != nil {
		return models.GetMarksFilters{}, fmt.Errorf("failed parse mark status ids")
	}

	filters := models.GetMarksFilters{
		MarkTypeIds:   markTypeIds,
		MarkStatusIds: markStatusIds,
		UserID:        r.UserID,
		Sort:          models.MarksSort(r.Sort),
		Order:         models.SortOrder(r.Order),
		Pagination:    r.Model(),
	}

	if r.BBox != "" {
		bbox, err := models.ParseBBox(r.BBox)
		if err != nil {
			return models.GetMarksFilters{}, err
		}
		filters.BBox = &bbox
	}
	if r.CreatedFrom != "" {
		if filters.CreatedFrom, err = time.Parse(time.RFC3339, r.CreatedFrom); err != nil {
			return models.GetMarksFilters{}, fmt.Errorf("created_from must be RFC3339")
		}
	}
	if r.CreatedTo != "" {
		if filters.CreatedTo, err = time.Parse(time.RFC3339, r.CreatedTo); err != nil {
			return models.GetMarksFilters{}, fmt.Errorf("created_to must be RFC3339")
		}
	}
	if err := filters.Validate(); err != nil {
		return models.GetMarksFilters{}, err
	}

	return filters, nil
}

type GetMarksResponse struct {
	Marks []models.Mark `json:"marks"`
}

// GetMarksNearbyRequest is bound from the query string of GET /marks/nearby.
// Coordinates are pointers so that 0 is a valid value while a missing
// parameter is rejected.
type GetMarksNearbyRequest struct {
	listquery.Pagination
	Lon *float64 `form:"lon" binding:"required,min=-180,max=180"`
	Lat *float64 `form:"lat" binding:"required,min=-90,max=90"`
	// Radius in meters, at most models.MaxNearbyRadiusM (50 km).
	Radius        float64 `form:"radius" binding:"required,gt=0,max=50000"`
	MarkTypeIds   string  `form:"mark_type_ids"`
	MarkStatusIds string  `form:"mark_status_ids"`
}

func (r GetMarksNearbyRequest) Filters() (models.GetMarksNearbyFilters, error) {
	markTypeIds, err := handlers.ParseIntArray(r.MarkTypeIds)
	if err != nil {
		return models.GetMarksNearbyFilters{}, fmt.Errorf("failed parse mark type ids")
	}
	markStatusIds, err := handlers.ParseIntArray(r.MarkStatusIds)
	if err != nil {
		return models.GetMarksNearbyFilters{}, fmt.Errorf("failed parse mark status ids")
	}

	return models.GetMarksNearbyFilters{
		Lon:           *r.Lon,
		Lat:           *r.Lat,
		RadiusM:       r.Radius,
		MarkTypeIds:   markTypeIds,
		MarkStatusIds: markStatusIds,
		Pagination:    r.Model(),
	}, nil
}

type GetMarksNearbyResponse struct {
	Marks []models.MarkWithDistance `json:"marks"`
}

type GetMarksByUserIdResponse struct {
	Marks []models.Mark `json:"marks"`
}

type GetMarkTypesResponse struct {
	MarkTypes []models.MarkType `json:"mark_types"`
}

type GetMarkStatusesResponse struct {
	MarkStatuses []models.MarkStatus `json:"mark_statuses"`
}

type AddMarkRequest struct {
	Photos      []*multipart.FileHeader `form:"photos" binding:"required"`
	Longitude   float64                 `form:"longitude" binding:"required,longitude"`
	Latitude    float64                 `form:"latitude" binding:"required,latitude"`
	MarkTypeID  int                     `form:"mark_type_id" binding:"required"`
	Description string                  `form:"description" binding:"max=256"`
}

type AddMarkResponse struct {
	MarkId int `json:"mark_id"`
}

type GetMarkStatusHistoryByMarkIdRequest struct {
	MarkId     int  `uri:"id" binding:"required"`
	WithChecks bool `form:"withChecks" default:"false"`
}

type GetMarkStatusHistoryByMarkIdResponse struct {
	HistoryItems []models.MarkStatusHistoryItem `json:"items"`
}

type ConfirmResponse struct {
	NewMarkStausId models.MarkStatusType `json:"new_mark_staus_id"`
}

type RejectResponse struct {
	NewMarkStausId models.MarkStatusType `json:"new_mark_staus_id"`
}
