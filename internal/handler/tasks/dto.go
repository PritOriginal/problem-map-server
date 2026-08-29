package tasksrest

import (
	"errors"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
)

// GetTasksRequest is bound from the query string of GET /tasks and
// GET /tasks/user/{id}.
type GetTasksRequest struct {
	listquery.Pagination
	// Statuses is a comma-separated list of status ids.
	Statuses string `form:"statuses"`
}

// Filters parses the status list. The returned error is safe to show to
// the client.
func (r GetTasksRequest) Filters() (models.GetTasksFilters, error) {
	statuses, err := handlers.ParseIntArray(r.Statuses)
	if err != nil {
		return models.GetTasksFilters{}, errors.New("failed parse statuses")
	}
	return models.GetTasksFilters{Statuses: statuses, Pagination: r.Model()}, nil
}

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
