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

// AddTaskRequest describes a new task. The owner is taken from the JWT claims.
type AddTaskRequest struct {
	Name   string `json:"name" binding:"required"`
	MarkID int    `json:"mark_id" binding:"required"`
}

type AddTaskResponse struct {
	TaskId int `json:"task_id"`
}
