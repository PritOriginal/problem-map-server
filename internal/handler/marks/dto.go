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

// AddMarkRequest describes the multipart form for creating a mark.
// Coordinates are WGS84 (SRID 4326): longitude is stored as X, latitude as Y
// (the same order as in GeoJSON/PostGIS). Do not swap them.
type AddMarkRequest struct {
	Photos []*multipart.FileHeader `form:"photos" binding:"required"`
	// Longitude in degrees (WGS84), stored as X. Example: 41.44 for Tambov.
	Longitude float64 `form:"longitude" binding:"required,longitude" example:"41.44"`
	// Latitude in degrees (WGS84), stored as Y. Example: 52.72 for Tambov.
	Latitude    float64 `form:"latitude" binding:"required,latitude" example:"52.72"`
	MarkTypeID  int     `form:"mark_type_id" binding:"required"`
	Description string  `form:"description" binding:"max=256"`
}

type AddMarkResponse struct {
	MarkId int `json:"mark_id"`
}

// AddMarkQuery is bound from the query string of POST /marks.
type AddMarkQuery struct {
	// Force skips duplicate detection.
	Force bool `form:"force"`
}

// SimilarMarksPayload is the payload of the 409 returned by POST /marks when
// active marks of the same type already exist nearby.
type SimilarMarksPayload struct {
	SimilarMarks []models.MarkWithDistance `json:"similar_marks"`
}

// GetSimilarMarksRequest is bound from the query string of GET /marks/similar.
type GetSimilarMarksRequest struct {
	Lon        *float64 `form:"lon" binding:"required,min=-180,max=180"`
	Lat        *float64 `form:"lat" binding:"required,min=-90,max=90"`
	MarkTypeID int      `form:"mark_type_id" binding:"required,min=1"`
	// Radius in meters; 0 means the server default (marks.dedup-radius-m).
	Radius float64 `form:"radius" binding:"omitempty,gt=0,max=50000"`
}

func (r GetSimilarMarksRequest) Filters() models.GetSimilarMarksFilters {
	return models.GetSimilarMarksFilters{
		Lon:        *r.Lon,
		Lat:        *r.Lat,
		MarkTypeID: r.MarkTypeID,
		RadiusM:    r.Radius,
	}
}

type GetSimilarMarksResponse struct {
	Marks []models.MarkWithDistance `json:"marks"`
}

// UpdateMarkRequest is the JSON body of PATCH /marks/{id}; omitted fields
// are left unchanged.
type UpdateMarkRequest struct {
	Description *string `json:"description" binding:"omitempty,max=256"`
	MarkTypeID  *int    `json:"mark_type_id" binding:"omitempty,min=1"`
}

func (r UpdateMarkRequest) Model() models.MarkUpdate {
	return models.MarkUpdate{Description: r.Description, MarkTypeID: r.MarkTypeID}
}

type UpdateMarkResponse struct {
	Mark models.Mark `json:"mark"`
}

type DeleteMarkResponse struct {
	MarkId int `json:"mark_id"`
}

// FollowResponse reports the subscription state after POST/DELETE /marks/{id}/follow.
type FollowResponse struct {
	MarkId    int  `json:"mark_id"`
	Following bool `json:"following"`
}

type GetFollowedMarksResponse struct {
	Marks []models.Mark `json:"marks"`
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
