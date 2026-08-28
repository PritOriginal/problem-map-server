package checksrest

import (
	"mime/multipart"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type GetCheckByIdResponse struct {
	Check models.Check `json:"check"`
}

type GetChecksByMarkIdResponse struct {
	Checks []models.Check `json:"checks"`
}

type GetGroupedChecksByMarkStatusHistoryIdResponse struct {
	GroupedChecks []models.GroupedChecksByMarkStatusHistoryId `json:"groups"`
}

type GetChecksByUserIdResponse struct {
	Checks []models.Check `json:"checks"`
}

type AddCheckRequest struct {
	Photos  []*multipart.FileHeader `form:"photos" binding:"required"`
	MarkID  int                     `form:"mark_id" binding:"required"`
	Result  bool                    `form:"result"`
	Comment string                  `form:"comment"`
}

type AddCheckResponse struct {
	CheckId int `json:"check_id"`
}
