package models

import pb "github.com/PritOriginal/problem-map-protos/gen/go"

type User struct {
	Id     int    `json:"user_id" db:"user_id"`
	Name   string `json:"name" db:"name"`
	Rating int    `json:"rating" db:"rating"`
}

func (u *User) MarshalProtobuf() *pb.User {
	return &pb.User{
		Id:     int64(u.Id),
		Name:   u.Name,
		Rating: int64(u.Rating),
	}
}

type Task struct {
	ID     int    `json:"task_id" db:"task_id"`
	Name   string `json:"name"`
	UserID int    `json:"user_id" db:"user_id"`
}

func (t *Task) MarshalProtobuf() *pb.Task {
	return &pb.Task{
		Id:     int64(t.ID),
		Name:   t.Name,
		UserId: int64(t.UserID),
	}
}
