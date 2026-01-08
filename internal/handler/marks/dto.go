package marksrest

import "github.com/PritOriginal/problem-map-server/internal/models"

type GetMarkByIdResponse struct {
	Mark models.Mark `json:"mark"`
}

type GetMarksByUserIdResponse struct {
	Marks []models.Mark `json:"marks"`
}

type GetMarksResponse struct {
	Marks []models.Mark `json:"marks"`
}

type GetMarkTypesResponse struct {
	MarkTypes []models.MarkType `json:"mark_types"`
}

type GetMarkStatusesResponse struct {
	MarkStatuses []models.MarkStatus `json:"mark_statuses"`
}

type AddMarkRequest struct {
	Point        Point `json:"point" validate:"required"`
	TypeMarkID   int   `json:"type_mark_id" validate:"required"`
	MarkStatusID int   `json:"mark_status_id" validate:"required"`
	UserID       int   `json:"user_id" validate:"required"`
	DistrictID   int   `json:"district_id" validate:"required"`
}

type Point struct {
	Longitude float64 `json:"longitude" validate:"required,longitude"`
	Latitude  float64 `json:"latitude" validate:"required,latitude"`
}
