package models

import pb "github.com/PritOriginal/problem-map-protos/gen/go"

type User struct {
	Id           int    `json:"user_id" db:"user_id"`
	Name         string `json:"username" db:"name"`
	Login        string `json:"login" db:"login"`
	PasswordHash string `json:"-" db:"password_hash"`
	HomePoint    *Point `json:"home_point" db:"home_point"`
	Rating       int    `json:"rating" db:"rating"`
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
	ID       int    `json:"task_id" db:"task_id"`
	Name     string `json:"name" db:"name"`
	UserID   int    `json:"user_id" db:"user_id"`
	MarkID   int    `json:"mark_id" db:"mark_id"`
	StatusID int    `json:"status_id" db:"status_id"`
}

func (t *Task) MarshalProtobuf() *pb.Task {
	return &pb.Task{
		Id:     int64(t.ID),
		Name:   t.Name,
		UserId: int64(t.UserID),
	}
}
