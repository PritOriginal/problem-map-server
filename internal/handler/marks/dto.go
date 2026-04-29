package marksrest

import (
	"mime/multipart"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

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
