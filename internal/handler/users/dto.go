package usersrest

import "github.com/PritOriginal/problem-map-server/internal/models"

// PublicUser is the user representation exposed to other users:
// it omits private fields such as login and home point.
type PublicUser struct {
	Id     int         `json:"user_id"`
	Name   string      `json:"username"`
	Rating int         `json:"rating"`
	Role   models.Role `json:"role"`
}

func NewPublicUser(user models.User) PublicUser {
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

// GetUserStatsResponse contains the activity summary of a user.
type GetUserStatsResponse struct {
	Stats models.UserStats `json:"stats"`
}

// LeaderboardEntry is a leaderboard row: the public identity and rating.
type LeaderboardEntry struct {
	Id     int    `json:"user_id"`
	Name   string `json:"username"`
	Rating int    `json:"rating"`
}

func NewLeaderboardEntries(users []models.User) []LeaderboardEntry {
	result := make([]LeaderboardEntry, 0, len(users))
	for _, user := range users {
		result = append(result, LeaderboardEntry{Id: user.Id, Name: user.Name, Rating: user.Rating})
	}
	return result
}

type GetLeaderboardResponse struct {
	Leaderboard []LeaderboardEntry `json:"leaderboard"`
}

type GetRatingEventsResponse struct {
	Events []models.RatingEvent `json:"events"`
}
