package checksrest

import "github.com/PritOriginal/problem-map-server/internal/models"

type GetCheckByIdResponse struct {
	Check models.Check `json:"check"`
}

type GetChecksByMarkIdResponse struct {
	Checks []models.Check `json:"checks"`
}

type GetChecksByUserIdResponse struct {
	Checks []models.Check `json:"checks"`
}

type AddCheckRequest struct {
	MarkID  int    `json:"mark_id" validate:"required"`
	Result  bool   `json:"result" validate:"required"`
	Comment string `json:"comment"`
}

type AddCheckResponse struct {
	CheckId int `json:"check_id"`
}
