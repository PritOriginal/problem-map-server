package models

import (
	"time"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/guregu/null/v6"
)

// Role is a user role that controls access to moderation endpoints.
type Role string

const (
	RoleUser      Role = "user"
	RoleModerator Role = "moderator"
	RoleAdmin     Role = "admin"
)

// ParseRole converts a raw role claim to a Role. Tokens without the role
// claim (empty string) are treated as plain users.
func ParseRole(raw string) Role {
	if raw == "" {
		return RoleUser
	}

	return Role(raw)
}

// Valid reports whether r is one of the known roles.
func (r Role) Valid() bool {
	switch r {
	case RoleUser, RoleModerator, RoleAdmin:
		return true
	default:
		return false
	}
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
	ID       int            `json:"task_id" db:"task_id"`
	Name     string         `json:"name" db:"name"`
	UserID   int            `json:"user_id" db:"user_id"`
	MarkID   int            `json:"mark_id" db:"mark_id"`
	StatusID TaskStatusType `json:"status_id" db:"status_id"`
	// DueAt is the deadline set by the tasker; tasks still issued after it
	// are moved to OverdueStatus by Tasker.ExpireOverdue.
	DueAt     null.Time `json:"due_at" db:"due_at" swaggertype:"string" format:"date-time"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
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

// Task statuses (task_statuses table, see migrations 000020 and 000029).
const (
	// UnfulfilledStatus — «Выдано»: the task is issued and awaits a check.
	UnfulfilledStatus TaskStatusType = iota + 1
	// CompletedStatus — «Выполнено»: the user submitted a check for the mark.
	CompletedStatus
	// OverdueStatus — «Просрочено»: the task was not completed before due_at.
	OverdueStatus
)

type GetTasksFilters struct {
	Statuses []int

	Pagination Pagination
}

type GetTasksByUserIdFilters struct {
	Statuses []int

	Pagination Pagination
}

// RatingReason explains a rating change (rating_events.reason).
type RatingReason string

const (
	// RatingReasonCheckCorrect — the checker's vote matched the stage outcome.
	RatingReasonCheckCorrect RatingReason = "check_correct"
	// RatingReasonCheckWrong — the checker's vote contradicted the outcome.
	RatingReasonCheckWrong RatingReason = "check_wrong"
	// RatingReasonMarkConfirmed — the author's mark was confirmed.
	RatingReasonMarkConfirmed RatingReason = "mark_confirmed"
	// RatingReasonMarkRefuted — the author's mark was refuted.
	RatingReasonMarkRefuted RatingReason = "mark_refuted"
	// RatingReasonTaskCompleted — a check closed an issued task.
	RatingReasonTaskCompleted RatingReason = "task_completed"
)

// RatingEvent is one change of a user's rating.
type RatingEvent struct {
	ID        int64        `json:"id" db:"id"`
	UserID    int          `json:"user_id" db:"user_id"`
	Delta     int          `json:"delta" db:"delta"`
	Reason    RatingReason `json:"reason" db:"reason"`
	MarkID    null.Int     `json:"mark_id" db:"mark_id" swaggertype:"integer"`
	CheckID   null.Int     `json:"check_id" db:"check_id" swaggertype:"integer"`
	CreatedAt time.Time    `json:"created_at" db:"created_at"`
}

// UserStats aggregates a user's activity for the profile page.
type UserStats struct {
	Rating         int `json:"rating" db:"rating"`
	MarksTotal     int `json:"marks_total" db:"marks_total"`
	MarksConfirmed int `json:"marks_confirmed" db:"marks_confirmed"`
	MarksRefuted   int `json:"marks_refuted" db:"marks_refuted"`
	ChecksTotal    int `json:"checks_total" db:"checks_total"`
	ChecksCorrect  int `json:"checks_correct" db:"checks_correct"`
	TasksCompleted int `json:"tasks_completed" db:"tasks_completed"`
}
