package models

type User struct {
	Id     int    `json:"user_id" db:"user_id"`
	Name   string `json:"name" db:"name"`
	Rating int    `json:"rating" db:"rating"`
}

type Task struct {
	ID     int    `json:"task_id" db:"task_id"`
	Name   string `json:"name"`
	UserID int    `json:"user_id" db:"user_id"`
}
