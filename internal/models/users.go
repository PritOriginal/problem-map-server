package models

import (
	"time"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
)

// Role is a user role that controls access to moderation endpoints.
type Role string

const (
	RoleUser      Role = "user"
	RoleModerator Role = "moderator"
	RoleAdmin     Role = "admin"
)

// IsValid reports whether the role is one of the known roles.
func (r Role) IsValid() bool {
	switch r {
	case RoleUser, RoleModerator, RoleAdmin:
		return true
	}
	return false
}

type User struct {
	Id           int    `json:"user_id" db:"user_id"`
	Name         string `json:"username" db:"name"`
	Login        string `json:"login" db:"login"`
	PasswordHash string `json:"-" db:"password_hash"`
	HomePoint    *Point `json:"home_point" db:"home_point"`
	Rating       int    `json:"rating" db:"rating"`
	Role         Role   `json:"role" db:"role"`
}

func (u *User) ToProtobufObject() *pb.User {
	return &pb.User{
		Id:        int64(u.Id),
		Name:      u.Name,
		Login:     u.Login,
		HomePoint: u.HomePoint.ToProtobufObject(),
		Rating:    int64(u.Rating),
	}
}

type Task struct {
	ID        int            `json:"task_id" db:"task_id"`
	Name      string         `json:"name" db:"name"`
	UserID    int            `json:"user_id" db:"user_id"`
	MarkID    int            `json:"mark_id" db:"mark_id"`
	StatusID  TaskStatusType `json:"status_id" db:"status_id"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
}

func (t *Task) ToProtobufObject() *pb.Task {
	return &pb.Task{
		Id:       int64(t.ID),
		Name:     t.Name,
		UserId:   int64(t.UserID),
		MarkId:   int64(t.MarkID),
		StatusId: int64(t.StatusID),
	}
}

type TaskStatusType int

const (
	UnfulfilledStatus TaskStatusType = iota + 1
	CompletedStatus
)

type GetTasksFilters struct {
	Statuses []int
}

type GetTasksByUserIdFilters struct {
	Statuses []int
}
