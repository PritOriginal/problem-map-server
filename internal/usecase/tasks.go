package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
)

type TasksRepository interface {
	GetTasks(ctx context.Context, filters models.GetTasksFilters) (models.Page[models.Task], error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	GetTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) (models.Page[models.Task], error)
	GetTaskByUserIdAndMarkId(ctx context.Context, userId int, markId int, statusId models.TaskStatusType) (models.Task, error)
	AddTask(ctx context.Context, task models.Task) (int64, error)
	UpdateTaskStatus(ctx context.Context, taskId int, taskStatusId models.TaskStatusType) error
}

type Tasks struct {
	log    *slog.Logger
	repos  TasksRepositories
	events events.Publisher
}

// WithEvents sets the publisher of task.assigned events for tasks created
// manually via AddTask. Without it events are dropped.
func (uc *Tasks) WithEvents(p events.Publisher) *Tasks {
	if p != nil {
		uc.events = p
	}
	return uc
}

type TasksRepositories struct {
	Tasks TasksRepository
}

func NewTasks(log *slog.Logger, repos TasksRepositories) *Tasks {
	return &Tasks{log: log, repos: repos, events: events.NoopPublisher{}}
}

// ListTasks returns a page of tasks with the total count.
func (uc *Tasks) ListTasks(ctx context.Context, filters models.GetTasksFilters) (models.Page[models.Task], error) {
	const op = "usecase.Tasks.ListTasks"

	if err := filters.Pagination.Validate(); err != nil {
		return models.Page[models.Task]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Tasks.GetTasks(ctx, filters)
	if err != nil {
		return page, mapRepoErr(op, err)
	}

	return page, nil
}

// GetTasks returns all matching tasks without pagination (gRPC).
func (uc *Tasks) GetTasks(ctx context.Context, filters models.GetTasksFilters) ([]models.Task, error) {
	const op = "usecase.Tasks.GetTasks"

	filters.Pagination = models.Pagination{}
	page, err := uc.repos.Tasks.GetTasks(ctx, filters)
	if err != nil {
		return page.Items, mapRepoErr(op, err)
	}

	return page.Items, nil
}

func (uc *Tasks) GetTaskById(ctx context.Context, id int) (models.Task, error) {
	const op = "usecase.Tasks.GetTaskById"

	task, err := uc.repos.Tasks.GetTaskById(ctx, id)
	if err != nil {
		return task, mapRepoErr(op, err)
	}

	return task, nil
}

// ListTasksByUserId returns a page of the user's tasks with the total count.
func (uc *Tasks) ListTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) (models.Page[models.Task], error) {
	const op = "usecase.Tasks.ListTasksByUserId"

	if err := filters.Pagination.Validate(); err != nil {
		return models.Page[models.Task]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Tasks.GetTasksByUserId(ctx, userId, filters)
	if err != nil {
		return page, mapRepoErr(op, err)
	}

	return page, nil
}

// GetTasksByUserId returns all matching tasks of the user without pagination (gRPC).
func (uc *Tasks) GetTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) ([]models.Task, error) {
	const op = "usecase.Tasks.GetTasksByUserId"

	filters.Pagination = models.Pagination{}
	page, err := uc.repos.Tasks.GetTasksByUserId(ctx, userId, filters)
	if err != nil {
		return page.Items, mapRepoErr(op, err)
	}

	return page.Items, nil
}

func (uc *Tasks) AddTask(ctx context.Context, task models.Task) (int64, error) {
	const op = "usecase.Tasks.AddTask"

	id, err := uc.repos.Tasks.AddTask(ctx, task)
	if err != nil {
		return id, mapRepoErr(op, err)
	}

	events.PublishEvent(ctx, uc.log, uc.events, events.NewTaskAssigned(int(id), task.UserID, task.MarkID, task.DueAt.Ptr()))

	return id, nil
}
