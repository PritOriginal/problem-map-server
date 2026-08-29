package usersrest

import (
	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
)

// PublicUser is the user representation exposed to other users:
// it omits private fields such as login and home point.
type PublicUser struct {
	Id     int         `json:"user_id"`
	Name   string      `json:"username"`
	Rating int         `json:"rating"`
	Role   models.Role `json:"role"`
}

func NewPublicUser(user models.User) PublicUser {
	user = user.Public()
	return PublicUser{
		Id:     user.Id,
		Name:   user.Name,
		Rating: user.Rating,
		Role:   user.Role,
	}
}

func NewPublicUsers(users []models.User) []PublicUser {
	result := make([]PublicUser, 0, len(users))
	for _, user := range users {
		result = append(result, NewPublicUser(user))
	}
	return result
}

type GetUsersResponse struct {
	Users []PublicUser `json:"users"`
}

type GetUserByIdResponse struct {
	User PublicUser `json:"user"`
}

// GetMeResponse contains the full profile of the authenticated user.
type GetMeResponse struct {
	User models.User `json:"user"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required,min=8,max=64"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=64"`
}

type SetRoleRequest struct {
	Role models.Role `json:"role" binding:"required,oneof=user moderator admin" enums:"user,moderator,admin"`
}

// GetUserStatsResponse contains the activity summary of a user.
type GetUserStatsResponse struct {
	Stats models.UserStats `json:"stats"`
}

// GetLeaderboardQuery are the query parameters of GET /leaderboard.
type GetLeaderboardQuery struct {
	listquery.Pagination
	BoundaryID int                      `form:"boundary_id" binding:"min=0"`
	Period     models.LeaderboardPeriod `form:"period" binding:"omitempty,oneof=all month week"`
}

func (q GetLeaderboardQuery) Filters() models.LeaderboardFilters {
	return models.LeaderboardFilters{BoundaryID: q.BoundaryID, Period: q.Period}
}

// LeaderboardEntry is a leaderboard row: the public identity, rating,
// level and the number of badges earned.
type LeaderboardEntry struct {
	Id          int          `json:"user_id"`
	Name        string       `json:"username"`
	Rating      int          `json:"rating"`
	Level       models.Level `json:"level"`
	BadgesCount int          `json:"badges_count"`
}

func NewLeaderboardEntries(entries []models.LeaderboardEntry, lang models.Lang) []LeaderboardEntry {
	result := make([]LeaderboardEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, LeaderboardEntry{
			Id:          e.UserID,
			Name:        e.Name,
			Rating:      e.Rating,
			Level:       models.LevelFor(e.Rating, lang),
			BadgesCount: e.BadgesCount,
		})
	}
	return result
}

type GetLeaderboardResponse struct {
	Leaderboard []LeaderboardEntry `json:"leaderboard"`
}

type GetRatingEventsResponse struct {
	Events []models.RatingEvent `json:"events"`
}
