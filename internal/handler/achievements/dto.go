package achievementsrest

import (
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

// GetBadgesResponse is the badge catalogue.
type GetBadgesResponse struct {
	Badges []models.Badge `json:"badges"`
}

// ProfileBadge is a badge the user earned.
type ProfileBadge struct {
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	EarnedAt    time.Time `json:"earned_at"`
}

// ProfileStats are the activity counters shown on the profile.
type ProfileStats struct {
	MarksTotal     int `json:"marks_total"`
	MarksConfirmed int `json:"marks_confirmed"`
	ChecksTotal    int `json:"checks_total"`
	ChecksCorrect  int `json:"checks_correct"`
	TasksCompleted int `json:"tasks_completed"`
}

// Profile is the public gamification profile of a user.
type Profile struct {
	Id          int            `json:"user_id"`
	Name        string         `json:"username"`
	Rating      int            `json:"rating"`
	Level       models.Level   `json:"level"`
	Badges      []ProfileBadge `json:"badges"`
	Stats       ProfileStats   `json:"stats"`
	MemberSince time.Time      `json:"member_since"`
}

func NewProfile(p models.UserProfile) Profile {
	badges := make([]ProfileBadge, 0, len(p.Badges))
	for _, b := range p.Badges {
		badges = append(badges, ProfileBadge{
			Code:        b.Code,
			Name:        b.Name,
			Description: b.Description,
			Icon:        b.Icon,
			EarnedAt:    b.EarnedAt,
		})
	}
	return Profile{
		Id:     p.User.Id,
		Name:   p.User.Name,
		Rating: p.User.Rating,
		Level:  p.Level,
		Badges: badges,
		Stats: ProfileStats{
			MarksTotal:     p.Stats.MarksTotal,
			MarksConfirmed: p.Stats.MarksConfirmed,
			ChecksTotal:    p.Stats.ChecksTotal,
			ChecksCorrect:  p.Stats.ChecksCorrect,
			TasksCompleted: p.Stats.TasksCompleted,
		},
		MemberSince: p.MemberSince,
	}
}

// GetProfileResponse wraps a profile.
type GetProfileResponse struct {
	Profile Profile `json:"profile"`
}
