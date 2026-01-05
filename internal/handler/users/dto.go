package usersrest

import "github.com/PritOriginal/problem-map-server/internal/models"

type GetUsersResponse struct {
	Users []models.User `json:"users"`
}

type GetUserByIdResponse struct {
	User models.User `json:"user"`
}
