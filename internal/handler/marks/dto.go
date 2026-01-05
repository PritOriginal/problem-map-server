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
