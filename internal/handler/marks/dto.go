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
	Point       Point  `json:"point" validate:"required"`
	MarkTypeID  int    `json:"mark_type_id" validate:"required"`
	Description string `json:"description" validate:"max=256"`
}

type AddMarkResponse struct {
	MarkId int `json:"mark_id"`
}

type Point struct {
	Longitude float64 `json:"longitude" validate:"required,longitude"`
	Latitude  float64 `json:"latitude" validate:"required,latitude"`
}
