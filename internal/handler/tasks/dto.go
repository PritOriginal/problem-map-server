package tasksrest

import "github.com/PritOriginal/problem-map-server/internal/models"

type GetTasksResponse struct {
	Tasks []models.Task `json:"tasks"`
}

type GetTaskByIdResponse struct {
	Task models.Task `json:"task"`
}

type GetTasksByUserIdResponse struct {
	Tasks []models.Task `json:"tasks"`
}

type AddTaskRequest struct {
	Name   string `json:"name" binding:"required"`
	UserID int    `json:"user_id" binding:"required"`
	MarkID int    `json:"mark_id" binding:"required"`
}

type AddTaskResponse struct {
	TaskId int `json:"task_id"`
}
