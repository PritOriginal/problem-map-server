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
	UserID  int    `json:"user_id"`
	MarkID  int    `json:"mark_id"`
	Result  bool   `json:"result"`
	Comment string `json:"comment"`
}
